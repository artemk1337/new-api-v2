package service

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx          *gin.Context
	TokenGroup   string
	ModelName    string
	RequestPath  string
	Retry        *int
	resetNextTry bool
}

type channelFetcher func(group string, model string, retry int, requestPath string) (*model.Channel, error)

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

type autoGroupCandidate struct {
	group string
	ratio float64
}

// BuildAutoGroupSnapshot freezes the concrete groups available to an Auto
// request. An empty token candidate list means all groups available to the user.
func BuildAutoGroupSnapshot(param *RetryParam, userGroup string) ([]string, error) {
	if snapshot, ok := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupSnapshot); ok {
		if groups, ok := snapshot.([]string); ok {
			return slices.Clone(groups), nil
		}
	}

	requested := common.GetContextKeyStringSlice(param.Ctx, constant.ContextKeyTokenAutoGroupCandidates)
	fetch := channelFetcher(model.GetRandomSatisfiedChannel)
	if rawChannelID := strings.TrimSpace(common.GetContextKeyString(param.Ctx, constant.ContextKeyTokenSpecificChannelId)); rawChannelID != "" {
		channelID, parseErr := strconv.Atoi(rawChannelID)
		if parseErr != nil || channelID <= 0 {
			return nil, fmt.Errorf("invalid specific channel id %q", rawChannelID)
		}
		fetch = specificChannelAutoGroupFetcher(channelID, model.IsChannelEnabledForGroupModel)
	}
	groups, err := buildAutoGroupSnapshot(param, userGroup, requested, fetch)
	if err != nil {
		return nil, err
	}
	common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupSnapshot, slices.Clone(groups))
	return groups, nil
}

func specificChannelAutoGroupFetcher(
	channelID int,
	supports func(group string, modelName string, channelID int) bool,
) channelFetcher {
	return func(group string, modelName string, _ int, _ string) (*model.Channel, error) {
		if !supports(group, modelName, channelID) {
			return nil, nil
		}
		return &model.Channel{Id: channelID}, nil
	}
}

func buildAutoGroupSnapshot(param *RetryParam, userGroup string, requested []string, fetch channelFetcher) ([]string, error) {
	autoGroups := GetUserAutoGroup(userGroup)
	restrictToConfigured := len(requested) > 0
	allowed := make(map[string]struct{}, len(requested))
	for _, group := range requested {
		group = ratio_setting.PricingGroupKey(strings.TrimSpace(group))
		if group == "" || group == "auto" {
			continue
		}
		allowed[group] = struct{}{}
	}

	candidates := make([]autoGroupCandidate, 0, len(autoGroups))
	var lastFetchErr error
	for _, group := range autoGroups {
		group = ratio_setting.PricingGroupKey(group)
		if group == "" || group == "auto" {
			continue
		}
		if restrictToConfigured {
			if _, ok := allowed[group]; !ok {
				continue
			}
		}
		channel, err := fetch(group, param.ModelName, 0, param.RequestPath)
		if err != nil {
			lastFetchErr = err
			continue
		}
		if channel == nil {
			continue
		}
		candidates = append(candidates, autoGroupCandidate{
			group: group,
			ratio: GetUserGroupRatio(userGroup, group),
		})
	}
	slices.SortFunc(candidates, func(a, b autoGroupCandidate) int {
		if order := cmp.Compare(a.ratio, b.ratio); order != 0 {
			return order
		}
		return cmp.Compare(a.group, b.group)
	})

	groups := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		groups = append(groups, candidate.group)
	}
	if len(groups) == 0 && lastFetchErr != nil {
		return nil, lastFetchErr
	}
	return groups, nil
}

// CacheGetRandomSatisfiedChannel uses exactly one concrete group for each Auto
// upstream attempt. The outer relay retry count is therefore also the snapshot
// group index.
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	switch param.TokenGroup {
	case "":
		channel, selectGroup, err = SelectCheapestAvailableGroup(param, userGroup)
		if err != nil {
			return nil, selectGroup, err
		}
	case "auto":
		autoGroups, snapshotErr := BuildAutoGroupSnapshot(param, userGroup)
		if snapshotErr != nil {
			return nil, selectGroup, snapshotErr
		}
		return selectAutoSnapshotChannel(param, autoGroups, model.GetRandomSatisfiedChannel)
	default:
		channel, err = model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, param.GetRetry(), param.RequestPath)
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}

func selectAutoSnapshotChannel(param *RetryParam, groups []string, fetch channelFetcher) (*model.Channel, string, error) {
	index := param.GetRetry()
	if index < 0 || index >= len(groups) {
		return nil, "auto", nil
	}
	group := groups[index]
	logger.LogDebug(param.Ctx, "Auto selecting group: %s, attempt: %d", group, index)
	channel, err := fetch(group, param.ModelName, 0, param.RequestPath)
	common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, group)
	if err != nil {
		return nil, group, err
	}
	return channel, group, nil
}

func SelectCheapestAvailableGroup(param *RetryParam, userGroup string) (*model.Channel, string, error) {
	return selectCheapestAvailableGroup(param, userGroup, model.GetRandomSatisfiedChannel)
}

func selectCheapestAvailableGroup(param *RetryParam, userGroup string, fetch channelFetcher) (*model.Channel, string, error) {
	usableGroups := GetUserUsableGroups(userGroup)
	groupRatios := ratio_setting.GetGroupRatioCopy()
	groupNames := make([]string, 0, len(groupRatios))
	for group := range groupRatios {
		if _, ok := usableGroups[group]; ok {
			groupNames = append(groupNames, group)
		}
	}
	slices.Sort(groupNames)

	var selectedChannel *model.Channel
	selectedGroup := "auto"
	selectedRatio := 0.0

	for _, group := range groupNames {
		channel, err := fetch(group, param.ModelName, param.GetRetry(), param.RequestPath)
		if err != nil {
			return nil, group, err
		}
		if channel == nil {
			continue
		}
		ratio := GetUserGroupRatio(userGroup, group)
		if selectedChannel == nil || ratio < selectedRatio {
			selectedChannel = channel
			selectedGroup = group
			selectedRatio = ratio
		}
	}

	if selectedChannel != nil {
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, selectedGroup)
		logger.LogDebug(param.Ctx, "Auto selected cheapest group: %s", selectedGroup)
	}

	return selectedChannel, selectedGroup, nil
}
