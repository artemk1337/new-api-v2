package controller

import (
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// Rebuild the public envelope so upstream metadata and provider-specific codes
// cannot escape through an error body. Internal accounting keeps the original.
func publicSupplierRelayError(err *types.NewAPIError) *types.NewAPIError {
	message := "Request could not be completed"
	code := types.ErrorCodeBadResponse
	switch err.GetErrorCode() {
	case types.ErrorCodeInsufficientUserQuota:
		message, code = "Insufficient account balance", err.GetErrorCode()
	case types.ErrorCodePreConsumeTokenQuotaFailed:
		message, code = "Insufficient API key quota", err.GetErrorCode()
	case types.ErrorCodeInsufficientAutoReserve:
		message, code = "Insufficient balance to reserve this request", err.GetErrorCode()
	case types.ErrorCodeModelPriceError:
		message, code = "Model pricing is unavailable", err.GetErrorCode()
	}
	status := err.StatusCode
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	return types.NewErrorWithStatusCode(errors.New(message), code, status)
}

// A group owns its billing target. Different suppliers may expose the same
// public alias through different upstream model names.
func resolveGroupBillingModels(c *gin.Context, info *relaycommon.RelayInfo, groups []string) ([]string, error) {
	info.GroupBillingModelNames = make(map[string]string, len(groups))
	available := make([]string, 0, len(groups))
	var forced *model.Channel
	if rawID := common.GetContextKeyString(c, constant.ContextKeyTokenSpecificChannelId); rawID != "" {
		id, err := strconv.Atoi(rawID)
		if err != nil || id <= 0 {
			return nil, errors.New("selected route is unavailable")
		}
		forced, err = model.GetChannelById(id, true)
		if err != nil {
			return nil, errors.New("selected route is unavailable")
		}
	}
	for _, rawGroup := range groups {
		group := ratio_setting.PricingGroupKey(rawGroup)
		var mappings []string
		if forced != nil {
			if !channelSupportsCandidateModels(forced.Id, group, info) || model.ValidateChannelPricingGroup(forced, group) != nil {
				continue
			}
			mappings = []string{forced.GetModelMapping()}
		} else {
			var err error
			mappings, err = model.GetEnabledChannelModelMappingsForModels([]string{group}, candidateAbilityModelNames(info), c.Request.URL.Path)
			if err != nil {
				return nil, err
			}
		}
		if len(mappings) == 0 {
			continue
		}
		if err := helper.ValidateModelMappingCandidates(
			common.GetContextKeyString(c, constant.ContextKeyTokenModelMapping),
			info.RequestedModelName, info.RelayMode, mappings,
		); err != nil {
			return nil, err
		}
		target, _, err := helper.ResolveCandidateModelMapping(mappings, info.OriginModelName, info.RelayMode)
		if err != nil {
			return nil, err
		}
		info.GroupBillingModelNames[group] = target
		available = append(available, group)
	}
	return available, nil
}

func channelSupportsCandidateModels(channelID int, group string, info *relaycommon.RelayInfo) bool {
	for _, name := range candidateAbilityModelNames(info) {
		if model.IsChannelEnabledForGroupModel(group, name, channelID) {
			return true
		}
	}
	return false
}

func previewTaskRoutes(c *gin.Context, info *relaycommon.RelayInfo, groups []string) error {
	info.AutoRoute = relaycommon.AutoRouteState{}
	for _, group := range groups {
		candidate := *info
		candidate.UsingGroup = group
		candidate.BillingModelName = info.GroupBillingModelNames[group]
		common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
		price, err := helper.ModelPriceHelperPerCall(c, &candidate)
		if err != nil {
			return err
		}
		info.AutoRoute.Candidates = append(info.AutoRoute.Candidates, relaycommon.AutoRouteCandidate{
			Group: group, BillingModelName: candidate.BillingModelName,
			EstimatedQuota: price.Quota, PriceData: price,
		})
	}
	return nil
}

// Estimates have a common quota/USD scale; group multipliers alone cannot
// compare tariffs from different procurement profiles.
func orderQuotedAutoRoute(c *gin.Context, info *relaycommon.RelayInfo, cheapestOnly bool) error {
	slices.SortStableFunc(info.AutoRoute.Candidates, func(a, b relaycommon.AutoRouteCandidate) int {
		if a.EstimatedQuota < b.EstimatedQuota {
			return -1
		}
		if a.EstimatedQuota > b.EstimatedQuota {
			return 1
		}
		if a.Group < b.Group {
			return -1
		}
		if a.Group > b.Group {
			return 1
		}
		return 0
	})
	if len(info.AutoRoute.Candidates) == 0 {
		return errors.New("no available priced route for this model")
	}
	first := info.AutoRoute.Candidates[0]
	if cheapestOnly {
		info.AutoRoute.Candidates = info.AutoRoute.Candidates[:1]
		info.AutoRoute.ReservedQuota = first.EstimatedQuota
		info.AutoRoute.ReserveGroup = first.Group
	}
	groups := make([]string, 0, len(info.AutoRoute.Candidates))
	for _, candidate := range info.AutoRoute.Candidates {
		groups = append(groups, candidate.Group)
	}
	info.AutoRoute.InitialGroup = first.Group
	info.AutoRoute.UsedGroup = first.Group
	info.ApplyAutoRouteCandidate(first.Group)
	common.SetContextKey(c, constant.ContextKeyAutoGroupSnapshot, groups)
	common.SetContextKey(c, constant.ContextKeyAutoGroup, first.Group)
	return nil
}

func setupQuotedInitialChannel(c *gin.Context, info *relaycommon.RelayInfo) error {
	return setupInitialRequestChannel(c, info, true)
}

func setupUnquotedTaskChannel(c *gin.Context, info *relaycommon.RelayInfo) error {
	return setupInitialRequestChannel(c, info, false)
}

func setupInitialRequestChannel(c *gin.Context, info *relaycommon.RelayInfo, frozen bool) error {
	var channel *model.Channel
	var err error
	previousID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	if rawID := common.GetContextKeyString(c, constant.ContextKeyTokenSpecificChannelId); rawID != "" {
		id, parseErr := strconv.Atoi(rawID)
		if parseErr != nil {
			return errors.New("selected route is unavailable")
		}
		channel, err = model.GetChannelById(id, true)
	} else {
		// Keep a valid affinity/provisional selection when it can serve the
		// quoted group. Repricing does not consume another channel key.
		if previousID > 0 {
			selected, selectErr := model.GetChannelById(previousID, true)
			if selectErr == nil && channelSupportsCandidateModels(selected.Id, info.UsingGroup, info) &&
				model.ValidateChannelPricingGroup(selected, info.UsingGroup) == nil {
				channel = selected
			}
		}
		if channel == nil {
			channel, err = model.GetRandomSatisfiedChannel(info.UsingGroup, candidateAbilityModelName(info), 0, c.Request.URL.Path)
		}
	}
	if err != nil || channel == nil {
		return errors.New("selected route is unavailable")
	}
	if frozen {
		err = validateFrozenRoute(channel, info)
	} else {
		err = validateSelectedRequestRoute(channel, info)
	}
	if err != nil {
		return err
	}
	if previousID == channel.Id && common.GetContextKeyString(c, constant.ContextKeyChannelKey) != "" {
		if setupErr := middleware.SetupContextForLockedChannel(c, channel, info.OriginModelName); setupErr != nil {
			return setupErr
		}
		return nil
	}
	if setupErr := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName); setupErr != nil {
		return setupErr
	}
	return nil
}

func validateFrozenRoute(channel *model.Channel, info *relaycommon.RelayInfo) error {
	if channel.Status != common.ChannelStatusEnabled || model.ValidateChannelPricingGroup(channel, info.UsingGroup) != nil {
		return errors.New("selected route is unavailable")
	}
	supplierID, profileID := 0, 0
	if channel.SupplierID != nil {
		supplierID = *channel.SupplierID
	}
	if channel.SupplierProfileID != nil {
		profileID = *channel.SupplierProfileID
	}
	if tariff := info.PriceData.SupplierTariff; tariff != nil {
		if supplierID != tariff.SupplierID || profileID != tariff.ProfileID {
			return errors.New("selected route no longer matches the reserved tariff")
		}
	} else if supplierID != 0 || profileID != 0 {
		return errors.New("selected route no longer matches the reserved tariff")
	}
	return validateFrozenRouteMapping(channel.GetModelMapping(), info)
}

func validateSelectedRequestRoute(channel *model.Channel, info *relaycommon.RelayInfo) error {
	if channel.Status != common.ChannelStatusEnabled || model.ValidateChannelPricingGroup(channel, info.UsingGroup) != nil {
		return errors.New("selected route is unavailable")
	}
	if info.ForcePreConsume || len(info.AutoRoute.Candidates) > 0 || info.PriceData.SupplierTariff != nil {
		return validateFrozenRoute(channel, info)
	}
	// Async adapters construct their final quote only after parsing the task
	// request; the group/model boundary is still checked before initialization.
	return validateFrozenRouteMapping(channel.GetModelMapping(), info)
}

func validateFrozenRouteMapping(mapping string, info *relaycommon.RelayInfo) error {
	target, mapped, err := helper.ResolveRelayModelMapping(mapping, info.OriginModelName, info.RelayMode)
	if err != nil {
		return err
	}
	if !mapped {
		target = info.OriginModelName
	}
	if target != helper.GetBillingModelName(info) {
		return errors.New("selected route no longer matches the reserved model")
	}
	return nil
}
