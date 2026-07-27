package controller

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/samber/lo"
)

const upstreamPricingSyncDefaultHost = "yunwu.ai"

const upstreamCacheCreation1hMultiplier = 6.0 / 3.75

type upstreamPricingSyncSummary struct {
	CheckedChannels     int      `json:"checked_channels"`
	FailedChannels      int      `json:"failed_channels"`
	AppliedModels       int      `json:"applied_models"`
	SkippedModels       []string `json:"skipped_models,omitempty"`
	CandidateHash       string   `json:"candidate_hash,omitempty"`
	PendingConfirmation bool     `json:"pending_confirmation,omitempty"`
}

type fetchedUpstreamPricing struct {
	Data              map[string]any
	UnsupportedModels []string
}

type upstreamPricingItem struct {
	ModelName            string                `json:"model_name"`
	QuotaType            int                   `json:"quota_type"`
	ModelRatio           float64               `json:"model_ratio"`
	ModelPrice           float64               `json:"model_price"`
	CompletionRatio      float64               `json:"completion_ratio"`
	CacheRatio           *float64              `json:"cache_ratio"`
	CreateCacheRatio     *float64              `json:"create_cache_ratio"`
	ImageRatio           *float64              `json:"image_ratio"`
	AudioRatio           *float64              `json:"audio_ratio"`
	AudioCompletionRatio *float64              `json:"audio_completion_ratio"`
	BillingMode          string                `json:"billing_mode"`
	BillingExpr          string                `json:"billing_expr"`
	StepRatios           []upstreamPricingStep `json:"step_ratios"`
}

type upstreamPricingStep struct {
	StepSize                    int     `json:"step_size"`
	CompletionStepSize          int     `json:"completion_step_size"`
	PromptStepRatio             float64 `json:"prompt_step_ratio"`
	CompletionStepRatio         float64 `json:"completion_step_ratio"`
	CacheStepRatio              float64 `json:"cache_step_ratio"`
	PromptThinkingStepRatio     float64 `json:"prompt_thinking_step_ratio"`
	CompletionThinkingStepRatio float64 `json:"completion_thinking_step_ratio"`
}

func upstreamPricingSyncHosts() map[string]struct{} {
	raw := common.GetEnvOrDefaultString("UPSTREAM_PRICING_SYNC_HOSTS", upstreamPricingSyncDefaultHost)
	hosts := make(map[string]struct{})
	for _, host := range strings.Split(raw, ",") {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			hosts[host] = struct{}{}
		}
	}
	return hosts
}

func isUpstreamPricingSyncChannel(channel *model.Channel, hosts map[string]struct{}) bool {
	baseURL, err := url.Parse(channel.GetBaseURL())
	return err == nil && isUpstreamPricingSyncURL(baseURL, hosts)
}

func isUpstreamPricingSyncURL(baseURL *url.URL, hosts map[string]struct{}) bool {
	if baseURL.Scheme != "https" {
		return false
	}
	_, ok := hosts[strings.ToLower(baseURL.Hostname())]
	return ok
}

func fetchChannelPricing(ctx context.Context, channel *model.Channel, hosts map[string]struct{}) (fetchedUpstreamPricing, error) {
	return fetchChannelPricingWithClient(ctx, channel, hosts, getHTTPClient())
}

func fetchChannelPricingWithClient(ctx context.Context, channel *model.Channel, hosts map[string]struct{}, client *http.Client) (fetchedUpstreamPricing, error) {
	endpoint := strings.TrimRight(channel.GetBaseURL(), "/") + "/api/pricing"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fetchedUpstreamPricing{}, err
	}
	pricingClient := *client
	pricingClient.CheckRedirect = func(request *http.Request, _ []*http.Request) error {
		if !isUpstreamPricingSyncURL(request.URL, hosts) {
			return fmt.Errorf("pricing sync redirect target is not allowlisted: %s", request.URL)
		}
		return nil
	}
	response, err := pricingClient.Do(req)
	if err != nil {
		return fetchedUpstreamPricing{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fetchedUpstreamPricing{}, fmt.Errorf("unexpected status: %s", response.Status)
	}

	var body struct {
		Success bool                  `json:"success"`
		Message string                `json:"message"`
		Data    []upstreamPricingItem `json:"data"`
	}
	if err := common.DecodeJson(io.LimitReader(response.Body, maxRatioConfigBytes), &body); err != nil {
		return fetchedUpstreamPricing{}, err
	}
	if !body.Success {
		return fetchedUpstreamPricing{}, fmt.Errorf("upstream returned an error: %s", body.Message)
	}
	data, unsupportedModels := upstreamPricingItemsToSyncData(body.Data)
	return fetchedUpstreamPricing{Data: data, UnsupportedModels: unsupportedModels}, nil
}

func upstreamPricingItemsToSyncData(items []upstreamPricingItem) (map[string]any, []string) {
	data := make(map[string]any)
	fields := make(map[string]map[string]any)
	unsupportedModels := make([]string, 0)
	seenModels := make(map[string]struct{})
	put := func(field string, modelName string, value any) {
		if fields[field] == nil {
			fields[field] = make(map[string]any)
		}
		fields[field][modelName] = value
	}
	block := func(modelName string) {
		unsupportedModels = append(unsupportedModels, modelName)
		for _, values := range fields {
			delete(values, modelName)
		}
	}
	for _, item := range items {
		modelName := strings.TrimSpace(item.ModelName)
		if modelName == "" {
			continue
		}
		if _, seen := seenModels[modelName]; seen {
			block(modelName)
			continue
		}
		seenModels[modelName] = struct{}{}
		if !validOptionalPricingRatios(item) {
			block(modelName)
			continue
		}
		explicitExpr := strings.TrimSpace(item.BillingExpr)
		tiered := false
		if item.BillingMode == billing_setting.BillingModeTieredExpr && explicitExpr != "" {
			if billing_setting.SmokeTestExpr(explicitExpr) != nil {
				block(modelName)
				continue
			}
			put(billing_setting.BillingModeField, modelName, billing_setting.BillingModeTieredExpr)
			put(billing_setting.BillingExprField, modelName, explicitExpr)
			tiered = true
		} else if len(item.StepRatios) > 0 {
			expr, ok := pricingStepRatiosExpr(item)
			if !ok || billing_setting.SmokeTestExpr(expr) != nil {
				block(modelName)
				continue
			}
			put(billing_setting.BillingModeField, modelName, billing_setting.BillingModeTieredExpr)
			put(billing_setting.BillingExprField, modelName, expr)
			tiered = true
		} else if item.BillingMode == billing_setting.BillingModeTieredExpr {
			block(modelName)
			continue
		} else if item.QuotaType == 1 {
			if !nonNegativeFinite(item.ModelPrice) {
				block(modelName)
				continue
			}
			put("model_price", modelName, item.ModelPrice)
		} else {
			if !nonNegativeFinite(item.ModelRatio) || !nonNegativeFinite(item.CompletionRatio) {
				block(modelName)
				continue
			}
			put("model_ratio", modelName, item.ModelRatio)
			put("completion_ratio", modelName, item.CompletionRatio)
		}
		if tiered {
			continue
		}
		if item.CacheRatio != nil {
			put("cache_ratio", modelName, *item.CacheRatio)
		}
		if item.CreateCacheRatio != nil {
			put("create_cache_ratio", modelName, *item.CreateCacheRatio)
		}
		if item.ImageRatio != nil {
			put("image_ratio", modelName, *item.ImageRatio)
		}
		if item.AudioRatio != nil {
			put("audio_ratio", modelName, *item.AudioRatio)
		}
		if item.AudioCompletionRatio != nil {
			put("audio_completion_ratio", modelName, *item.AudioCompletionRatio)
		}
	}
	for field, values := range fields {
		data[field] = values
	}
	return data, lo.Uniq(unsupportedModels)
}

func nonNegativeFinite(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validOptionalPricingRatios(item upstreamPricingItem) bool {
	for _, value := range []*float64{
		item.CacheRatio,
		item.CreateCacheRatio,
		item.ImageRatio,
		item.AudioRatio,
		item.AudioCompletionRatio,
	} {
		if value != nil && !nonNegativeFinite(*value) {
			return false
		}
	}
	return true
}

// pricingStepRatiosExpr maps Yunwu's context pricing steps to the existing
// billing expression contract. A step is an upper bound; the final step is the
// fallback so usage beyond the published maximum is still charged predictably.
// Thinking steps are deliberately not converted: the upstream response does
// not expose a separately billable thinking-token count to this gateway.
func pricingStepRatiosExpr(item upstreamPricingItem) (string, bool) {
	if item.QuotaType == 1 || len(item.StepRatios) == 0 ||
		!nonNegativeFinite(item.ModelRatio) || !nonNegativeFinite(item.CompletionRatio) ||
		!validOptionalPricingRatios(item) {
		return "", false
	}
	cacheRatio := 1.0
	if item.CacheRatio != nil {
		cacheRatio = *item.CacheRatio
	}
	createCacheRatio := 1.25
	if item.CreateCacheRatio != nil {
		createCacheRatio = *item.CreateCacheRatio
	}
	audioRatio := 1.0
	if item.AudioRatio != nil {
		audioRatio = *item.AudioRatio
	}
	audioCompletionRatio := 1.0
	if item.AudioCompletionRatio != nil {
		audioCompletionRatio = *item.AudioCompletionRatio
	}
	steps := item.StepRatios
	parts := make([]string, 0, len(steps))
	for index, step := range steps {
		if step.StepSize <= 0 || !nonNegativeFinite(step.PromptStepRatio) || !nonNegativeFinite(step.CompletionStepRatio) ||
			!nonNegativeFinite(step.CacheStepRatio) || step.CompletionStepSize == 0 || step.CompletionStepSize < -1 ||
			!nonNegativeFinite(step.PromptThinkingStepRatio) || !nonNegativeFinite(step.CompletionThinkingStepRatio) ||
			step.PromptThinkingStepRatio != 0 || step.CompletionThinkingStepRatio != 0 {
			return "", false
		}
		if index > 0 {
			previous := steps[index-1]
			if step.StepSize < previous.StepSize ||
				(step.StepSize == previous.StepSize && !completionStepAfter(previous.CompletionStepSize, step.CompletionStepSize)) {
				return "", false
			}
		}
		inputPrice := item.ModelRatio * 2 * step.PromptStepRatio
		outputPrice := item.ModelRatio * item.CompletionRatio * 2 * step.CompletionStepRatio
		cost := "p * " + strconv.FormatFloat(inputPrice, 'f', -1, 64) + " + c * " + strconv.FormatFloat(outputPrice, 'f', -1, 64)
		cachePrice := item.ModelRatio * cacheRatio * 2 * step.CacheStepRatio
		cost += " + cr * " + strconv.FormatFloat(cachePrice, 'f', -1, 64)
		cacheCreationPrice := item.ModelRatio * createCacheRatio * 2 * step.CacheStepRatio
		cost += " + cc * " + strconv.FormatFloat(cacheCreationPrice, 'f', -1, 64)
		cost += " + cc1h * " + strconv.FormatFloat(cacheCreationPrice*upstreamCacheCreation1hMultiplier, 'f', -1, 64)
		if item.ImageRatio != nil {
			imagePrice := item.ModelRatio * *item.ImageRatio * 2 * step.PromptStepRatio
			cost += " + img * " + strconv.FormatFloat(imagePrice, 'f', -1, 64)
		}
		audioInputPrice := item.ModelRatio * audioRatio * 2 * step.PromptStepRatio
		cost += " + ai * " + strconv.FormatFloat(audioInputPrice, 'f', -1, 64)
		audioOutputPrice := item.ModelRatio * audioRatio * audioCompletionRatio * 2 * step.CompletionStepRatio
		cost += " + ao * " + strconv.FormatFloat(audioOutputPrice, 'f', -1, 64)
		tier := "tier(\"step_" + strconv.Itoa(index+1) + "\", " + cost + ")"
		if index == len(steps)-1 {
			parts = append(parts, tier)
			continue
		}
		condition := "len <= " + strconv.Itoa(step.StepSize)
		if step.CompletionStepSize > 0 {
			completionLength := "c + ao"
			condition += " && " + completionLength + " <= " + strconv.Itoa(step.CompletionStepSize)
		}
		parts = append(parts, condition+" ? "+tier+" : ")
	}
	return strings.Join(parts, ""), true
}

func completionStepAfter(previous, current int) bool {
	if previous == -1 {
		return false
	}
	return current == -1 || current > previous
}

func pricingCategory(data map[string]any, modelName string) string {
	if mode, ok := valueMap(data[billing_setting.BillingModeField])[modelName]; ok && mode == billing_setting.BillingModeTieredExpr {
		return "tiered"
	}
	if _, ok := valueMap(data["model_price"])[modelName]; ok {
		return "price"
	}
	for _, field := range []string{"model_ratio", "completion_ratio", "cache_ratio", "create_cache_ratio", "image_ratio", "audio_ratio", "audio_completion_ratio"} {
		if _, ok := valueMap(data[field])[modelName]; ok {
			return "ratio"
		}
	}
	return ""
}

func optionKeyForPricingField(field string) string {
	switch field {
	case billing_setting.BillingModeField:
		return "billing_setting.billing_mode"
	case billing_setting.BillingExprField:
		return "billing_setting.billing_expr"
	default:
		return strings.Join(lo.Map(strings.Split(field, "_"), func(part string, _ int) string {
			return strings.ToUpper(part[:1]) + part[1:]
		}), "")
	}
}

func pricingCandidateHash(sources []string, patches map[string]model.JSONObjectPatch) (string, error) {
	sortedSources := append([]string(nil), sources...)
	sort.Strings(sortedSources)
	encodedOptions, err := common.Marshal(struct {
		Sources []string                         `json:"sources"`
		Patches map[string]model.JSONObjectPatch `json:"patches"`
	}{Sources: sortedSources, Patches: patches})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(common.Sha256Raw(encodedOptions)), nil
}

func pricingCandidateConfirmed(previousHash string, candidateHash string) bool {
	return candidateHash != "" && candidateHash == previousHash
}

func pricingSourceIdentity(channel *model.Channel) string {
	baseURL, err := url.Parse(channel.GetBaseURL())
	if err != nil {
		return strconv.Itoa(channel.Id) + ":" + channel.GetBaseURL()
	}
	baseURL.Scheme = strings.ToLower(baseURL.Scheme)
	baseURL.Host = strings.ToLower(baseURL.Host)
	baseURL.Path = strings.TrimRight(baseURL.Path, "/")
	return strconv.Itoa(channel.Id) + ":" + baseURL.String()
}

// buildUpstreamPricingSyncPatches accepts a value only when every upstream
// exposing that model agrees on its billing category and field values. A
// category transition is intentionally left for the manual conflict dialog.
func buildUpstreamPricingSyncPatches(local map[string]any, upstreams []map[string]any, blockedModels map[string]struct{}) (map[string]model.JSONObjectPatch, []string, int) {
	allModels := make(map[string]struct{})
	for _, upstream := range upstreams {
		for _, field := range pricingSyncFields {
			for modelName := range valueMap(upstream[field]) {
				allModels[modelName] = struct{}{}
			}
		}
	}
	models := lo.Keys(allModels)
	sort.Strings(models)
	patches := make(map[string]model.JSONObjectPatch)
	skipped := make([]string, 0)
	applied := 0

	for _, modelName := range models {
		if _, blocked := blockedModels[modelName]; blocked {
			skipped = append(skipped, modelName)
			continue
		}
		category := ""
		conflict := false
		exposingUpstreams := make([]map[string]any, 0, len(upstreams))
		for _, upstream := range upstreams {
			upstreamCategory := pricingCategory(upstream, modelName)
			if upstreamCategory == "" {
				continue
			}
			exposingUpstreams = append(exposingUpstreams, upstream)
			if category != "" && category != upstreamCategory {
				conflict = true
				break
			}
			category = upstreamCategory
		}
		if conflict || category == "" || (pricingCategory(local, modelName) != "" && pricingCategory(local, modelName) != category) {
			skipped = append(skipped, modelName)
			continue
		}

		modelChanged := false
		modelPatches := make(map[string]model.JSONObjectPatch)
		for _, field := range pricingSyncFields {
			var expected any
			present := 0
			for _, upstream := range exposingUpstreams {
				value, ok := valueMap(upstream[field])[modelName]
				if !ok {
					continue
				}
				value = normalizeSyncValue(field, value)
				if present > 0 && !valuesEqual(expected, value) {
					conflict = true
					break
				}
				expected = value
				present++
			}
			if conflict {
				break
			}
			if present > 0 && present != len(exposingUpstreams) {
				conflict = true
				break
			}
			current, exists := valueMap(local[field])[modelName]
			optionKey := optionKeyForPricingField(field)
			patch := modelPatches[optionKey]
			if present == 0 {
				if exists {
					patch.Delete = append(patch.Delete, modelName)
					modelPatches[optionKey] = patch
					modelChanged = true
				}
				continue
			}
			if exists && valuesEqual(normalizeSyncValue(field, current), expected) {
				continue
			}
			if patch.Set == nil {
				patch.Set = make(map[string]any)
			}
			patch.Set[modelName] = expected
			modelPatches[optionKey] = patch
			modelChanged = true
		}
		if conflict {
			skipped = append(skipped, modelName)
			continue
		}
		if modelChanged {
			for optionKey, modelPatch := range modelPatches {
				patch := patches[optionKey]
				if len(modelPatch.Set) > 0 {
					if patch.Set == nil {
						patch.Set = make(map[string]any)
					}
					patch.Set[modelName] = modelPatch.Set[modelName]
				}
				patch.Delete = append(patch.Delete, modelPatch.Delete...)
				patches[optionKey] = patch
			}
			applied++
		}
	}
	return patches, lo.Uniq(skipped), applied
}

func runUpstreamPricingSyncTaskOnce(ctx context.Context, previousCandidateHash string, report func(processed, total int)) (upstreamPricingSyncSummary, error) {
	summary := upstreamPricingSyncSummary{}
	hosts := upstreamPricingSyncHosts()
	var channels []*model.Channel
	if err := model.DB.Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		common.SysLog(fmt.Sprintf("upstream pricing sync query failed: %v", err))
		return summary, err
	}

	upstreams := make([]map[string]any, 0)
	sources := make([]string, 0)
	blockedModels := make(map[string]struct{})
	for index, channel := range channels {
		if report != nil {
			report(index+1, len(channels))
		}
		if ctx.Err() != nil || !isUpstreamPricingSyncChannel(channel, hosts) {
			continue
		}
		summary.CheckedChannels++
		timeoutSeconds := common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", defaultTimeoutSeconds)
		if timeoutSeconds < 1 {
			timeoutSeconds = defaultTimeoutSeconds
		}
		channelCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		fetched, err := fetchChannelPricing(channelCtx, channel, hosts)
		cancel()
		if err != nil {
			summary.FailedChannels++
			common.SysLog(fmt.Sprintf("upstream pricing sync failed: channel_id=%d err=%v", channel.Id, err))
			continue
		}
		upstreams = append(upstreams, fetched.Data)
		sources = append(sources, pricingSourceIdentity(channel))
		summary.SkippedModels = append(summary.SkippedModels, fetched.UnsupportedModels...)
		for _, modelName := range fetched.UnsupportedModels {
			blockedModels[modelName] = struct{}{}
		}
	}
	if ctx.Err() != nil {
		return summary, ctx.Err()
	}
	if summary.FailedChannels > 0 {
		return summary, fmt.Errorf("upstream pricing sync failed for %d channel(s)", summary.FailedChannels)
	}
	if len(upstreams) == 0 {
		return summary, nil
	}
	patches, skipped, applied := buildUpstreamPricingSyncPatches(getLocalPricingSyncData(), upstreams, blockedModels)
	summary.SkippedModels = lo.Uniq(append(summary.SkippedModels, skipped...))
	if len(patches) == 0 {
		return summary, nil
	}
	candidateHash, err := pricingCandidateHash(sources, patches)
	if err != nil {
		return summary, err
	}
	summary.CandidateHash = candidateHash
	if !pricingCandidateConfirmed(previousCandidateHash, summary.CandidateHash) {
		summary.PendingConfirmation = true
		return summary, nil
	}
	if err := model.ApplyJSONOptionPatches(patches); err != nil {
		return summary, fmt.Errorf("upstream pricing sync apply failed: %w", err)
	}
	summary.AppliedModels = applied
	return summary, nil
}
