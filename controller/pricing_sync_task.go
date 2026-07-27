package controller

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
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
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
	return fetchPricingSyncURL(ctx, strings.TrimRight(channel.GetBaseURL(), "/")+defaultEndpoint, "pricing", hosts, "", client)
}

// fetchPricingSyncURL is shared by manual and scheduled pricing sync.  It
// deliberately accepts only an already resolved HTTPS URL and keeps redirects
// on the original source host.  This prevents a saved channel from becoming a
// server-side request proxy while still allowing providers which redirect their
// own public pricing endpoint.
func fetchPricingSyncURL(ctx context.Context, rawURL, mode string, hosts map[string]struct{}, apiKey string, client *http.Client) (fetchedUpstreamPricing, error) {
	requestURL, err := url.Parse(rawURL)
	if err != nil {
		return fetchedUpstreamPricing{}, err
	}
	// Legacy preview callers pass a nil allowlist. Scheduled sync always passes
	// a concrete source-host allowlist and therefore remains HTTPS-only.
	if hosts != nil && !isUpstreamPricingSyncURL(requestURL, hosts) {
		return fetchedUpstreamPricing{}, fmt.Errorf("pricing sync requires an allowlisted HTTPS URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return fetchedUpstreamPricing{}, err
	}
	if mode == "openrouter" {
		if strings.TrimSpace(apiKey) == "" {
			return fetchedUpstreamPricing{}, fmt.Errorf("OpenRouter requires a channel API key")
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
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
	bodyBytes, err := io.ReadAll(io.LimitReader(response.Body, maxRatioConfigBytes))
	if err != nil {
		return fetchedUpstreamPricing{}, err
	}
	switch mode {
	case "openrouter":
		data, err := convertOpenRouterToRatioData(bytes.NewReader(bodyBytes))
		return fetchedUpstreamPricing{Data: data}, err
	case "models_dev":
		data, err := convertModelsDevToRatioData(bytes.NewReader(bodyBytes))
		return fetchedUpstreamPricing{Data: data}, err
	}
	var body struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := common.DecodeJson(bytes.NewReader(bodyBytes), &body); err != nil {
		return fetchedUpstreamPricing{}, err
	}
	if !body.Success {
		return fetchedUpstreamPricing{}, fmt.Errorf("upstream returned an error: %s", body.Message)
	}
	var ratioData map[string]any
	if err := common.Unmarshal(body.Data, &ratioData); err == nil {
		for _, field := range pricingSyncFields {
			if _, ok := ratioData[field]; ok {
				return fetchedUpstreamPricing{Data: ratioData}, nil
			}
		}
	}
	var items []upstreamPricingItem
	if err := common.Unmarshal(body.Data, &items); err != nil {
		return fetchedUpstreamPricing{}, fmt.Errorf("unrecognized upstream pricing response: %w", err)
	}
	data, unsupportedModels := upstreamPricingItemsToSyncData(items)
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

func validPricingSyncContract(data map[string]any, modelName string) bool {
	switch pricingCategory(data, modelName) {
	case "price":
		value, ok := asFloat64(valueMap(data["model_price"])[modelName])
		return ok && value > 0 && nonNegativeFinite(value)
	case "ratio":
		value, ok := asFloat64(valueMap(data["model_ratio"])[modelName])
		return ok && value > 0 && nonNegativeFinite(value)
	case "tiered":
		expr, ok := valueMap(data[billing_setting.BillingExprField])[modelName].(string)
		return ok && strings.TrimSpace(expr) != "" && billing_setting.SmokeTestExpr(expr) == nil
	default:
		return false
	}
}

func pricingSyncSourceCurrentTx(tx *gorm.DB, source model.PricingSyncSource, expectedVersion int64) error {
	version := model.Option{}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&version, "key = ?", "PricingSyncConfigVersion").Error; err != nil {
		return err
	}
	if version.Value != strconv.FormatInt(expectedVersion, 10) {
		return fmt.Errorf("pricing sync configuration changed")
	}
	current := model.PricingSyncSource{}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&current, "channel_id = ?", source.ChannelID).Error; err != nil {
		return err
	}
	if current.Enabled != source.Enabled ||
		current.Endpoint != source.Endpoint ||
		current.IntervalSeconds != source.IntervalSeconds {
		return fmt.Errorf("pricing sync source configuration changed")
	}
	return nil
}

func updatePricingSyncSourceIfCurrent(source model.PricingSyncSource, expectedVersion int64, updates map[string]any) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := pricingSyncSourceCurrentTx(tx, source, expectedVersion); err != nil {
			return err
		}
		return tx.Model(&model.PricingSyncSource{}).
			Where("channel_id = ?", source.ChannelID).Updates(updates).Error
	})
}

// confirmPricingSyncQuotes persists one complete contract per source/model.
// A value is eligible for scheduled adoption only after the same source has
// returned it twice.  This makes a transient malformed provider response
// harmless even when no other source participates in a sync run.
func confirmPricingSyncQuotes(channelID int, data map[string]any, now int64) (map[string]any, error) {
	return confirmPricingSyncQuotesWithTx(channelID, data, now, nil, nil)
}

func confirmPricingSyncQuotesIfCurrent(source model.PricingSyncSource, data map[string]any, now, expectedVersion int64) (map[string]any, error) {
	return confirmPricingSyncQuotesWithTx(
		source.ChannelID,
		data,
		now,
		func(tx *gorm.DB) error {
			return pricingSyncSourceCurrentTx(tx, source, expectedVersion)
		},
		func(tx *gorm.DB) error {
			return tx.Model(&model.PricingSyncSource{}).
				Where("channel_id = ?", source.ChannelID).
				Updates(map[string]any{"last_success_at": now, "last_error": ""}).Error
		},
	)
}

func confirmPricingSyncQuotesWithTx(channelID int, data map[string]any, now int64, before, after func(*gorm.DB) error) (map[string]any, error) {
	modelNames := make(map[string]struct{})
	for _, field := range pricingSyncFields {
		for modelName := range valueMap(data[field]) {
			if validPricingSyncContract(data, modelName) {
				modelNames[modelName] = struct{}{}
			}
		}
	}
	confirmed := make(map[string]bool, len(modelNames))
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if before != nil {
			if err := before(tx); err != nil {
				return err
			}
		}
		var existing []model.PricingSyncQuote
		if err := tx.Where("channel_id = ?", channelID).Find(&existing).Error; err != nil {
			return err
		}
		existingByModel := make(map[string]model.PricingSyncQuote, len(existing))
		for _, quote := range existing {
			existingByModel[quote.ModelName] = quote
		}
		for modelName := range modelNames {
			contract := make(map[string]any)
			for _, field := range pricingSyncFields {
				if value, ok := valueMap(data[field])[modelName]; ok {
					contract[field] = normalizeSyncValue(field, value)
				}
			}
			encoded, err := common.Marshal(contract)
			if err != nil {
				return err
			}
			hash := hex.EncodeToString(common.Sha256Raw(encoded))
			quote := existingByModel[modelName]
			if quote.CandidateHash == hash {
				quote.Confirmations++
			} else {
				quote.Confirmations = 1
			}
			quote.ChannelID = channelID
			quote.ModelName = modelName
			quote.Category = pricingCategory(data, modelName)
			quote.Data = string(encoded)
			quote.CandidateHash = hash
			quote.MissingCount = 0
			quote.ConfirmedAt = now
			if err := tx.Save(&quote).Error; err != nil {
				return err
			}
			confirmed[modelName] = quote.Confirmations >= 2
		}
		for _, quote := range existing {
			if _, found := modelNames[quote.ModelName]; found {
				continue
			}
			if err := tx.Model(&model.PricingSyncQuote{}).
				Where("channel_id = ? AND model_name = ?", channelID, quote.ModelName).
				Updates(map[string]any{"missing_count": quote.MissingCount + 1, "confirmations": 0}).Error; err != nil {
				return err
			}
		}
		if after != nil {
			return after(tx)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	filtered := make(map[string]any)
	for _, field := range pricingSyncFields {
		values := make(map[string]any)
		for modelName, value := range valueMap(data[field]) {
			if confirmed[modelName] {
				values[modelName] = value
			}
		}
		if len(values) > 0 {
			filtered[field] = values
		}
	}
	return filtered, nil
}

// confirmedPricingSyncQuoteData reconstructs a source snapshot from durable
// quotes. Different source intervals must still be compared together: a
// one-minute source may not silently override an hourly source merely because
// the latter was not due in the current scheduler tick.
func confirmedPricingSyncQuoteData(channelID int) (map[string]any, error) {
	quotes := make([]model.PricingSyncQuote, 0)
	if err := model.DB.Where("channel_id = ? AND confirmations >= ? AND missing_count < ?", channelID, 2, 2).Find(&quotes).Error; err != nil {
		return nil, err
	}
	data := make(map[string]any)
	for _, quote := range quotes {
		contract := make(map[string]any)
		if err := common.UnmarshalJsonStr(quote.Data, &contract); err != nil {
			return nil, fmt.Errorf("decode pricing quote for %s: %w", quote.ModelName, err)
		}
		for field, value := range contract {
			if !lo.Contains(pricingSyncFields, field) {
				continue
			}
			values := valueMap(data[field])
			if values == nil {
				values = make(map[string]any)
				data[field] = values
			}
			values[quote.ModelName] = value
		}
	}
	return data, nil
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

type pricingSyncFetchTarget struct {
	URL      string
	Mode     string
	Hosts    map[string]struct{}
	APIKey   string
	Identity string
}

// pricingSyncFetchTargetForSource resolves only the supported source modes.
// Custom endpoints are paths relative to a saved channel URL; an arbitrary URL
// is never accepted from the persisted configuration.
func pricingSyncFetchTargetForSource(source model.PricingSyncSource, channel *model.Channel) (pricingSyncFetchTarget, error) {
	baseURL := ""
	identity := strconv.Itoa(source.ChannelID)
	endpoint := strings.TrimSpace(source.Endpoint)
	mode := "pricing"
	apiKey := ""
	switch source.ChannelID {
	case officialRatioPresetID:
		if endpoint == "" {
			endpoint = "/llm-metadata/api/newapi/ratio_config-v1-base.json"
		}
		if absolute, parseErr := url.Parse(endpoint); parseErr == nil && absolute.IsAbs() {
			if absolute.Scheme != "https" || !strings.EqualFold(absolute.Hostname(), "basellm.github.io") {
				return pricingSyncFetchTarget{}, fmt.Errorf("invalid official pricing preset URL")
			}
			return pricingSyncFetchTarget{
				URL: absolute.String(), Mode: "ratio_config",
				Hosts: map[string]struct{}{"basellm.github.io": {}}, Identity: "basellm",
			}, nil
		}
		baseURL = officialRatioPresetBaseURL
		mode = "ratio_config"
	case modelsDevPresetID:
		return pricingSyncFetchTarget{
			URL: modelsDevPresetBaseURL + modelsDevPath, Mode: "models_dev",
			Hosts: map[string]struct{}{modelsDevHost: {}}, Identity: "models.dev",
		}, nil
	default:
		if channel == nil {
			return pricingSyncFetchTarget{}, fmt.Errorf("pricing sync channel %d was deleted", source.ChannelID)
		}
		baseURL = channel.GetBaseURL()
		identity = pricingSourceIdentity(channel)
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" {
		return pricingSyncFetchTarget{}, fmt.Errorf("pricing sync requires an HTTPS channel URL")
	}
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	if endpoint == "openrouter" {
		if channel == nil {
			return pricingSyncFetchTarget{}, fmt.Errorf("OpenRouter pricing sync requires a saved channel")
		}
		key, _, apiErr := channel.GetNextEnabledKey()
		if apiErr != nil {
			return pricingSyncFetchTarget{}, fmt.Errorf("OpenRouter channel key: %w", apiErr)
		}
		apiKey = key
		mode = "openrouter"
		endpoint = "/v1/models"
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	endpointURL, err := url.Parse(endpoint)
	if err != nil || endpointURL.IsAbs() || endpointURL.Host != "" || strings.HasPrefix(endpoint, "//") {
		return pricingSyncFetchTarget{}, fmt.Errorf("pricing sync endpoint must be a relative path")
	}
	resolved := base.ResolveReference(endpointURL)
	return pricingSyncFetchTarget{
		URL: resolved.String(), Mode: mode,
		Hosts:  map[string]struct{}{strings.ToLower(base.Hostname()): {}},
		APIKey: apiKey, Identity: identity,
	}, nil
}

// resolveComparableNumericPricing applies the configured conflict policy to a
// complete fixed-price or ratio contract. Ratio lanes are converted to actual
// prices before applying the strategy and then converted back to ratios.
func resolveComparableNumericPricing(upstreams []map[string]any, modelName, category string) (map[string]any, bool) {
	fields := []string{"model_price"}
	if category == "ratio" {
		fields = []string{"model_ratio", "completion_ratio", "cache_ratio", "create_cache_ratio", "image_ratio", "audio_ratio", "audio_completion_ratio"}
	}
	contracts := make([]map[string]float64, 0, len(upstreams))
	for _, upstream := range upstreams {
		if pricingCategory(upstream, modelName) != category {
			continue
		}
		contract := make(map[string]float64)
		for _, field := range fields {
			value, present := valueMap(upstream[field])[modelName]
			if !present {
				continue
			}
			parsed, ok := asFloat64(value)
			if !ok || !nonNegativeFinite(parsed) {
				return nil, false
			}
			contract[field] = parsed
		}
		contracts = append(contracts, contract)
	}
	if len(contracts) == 0 {
		return nil, false
	}
	// Optional lanes are comparable only when every source declares exactly the
	// same lanes. model_ratio/completion_ratio are required for a ratio contract.
	for _, field := range fields {
		present := 0
		for _, contract := range contracts {
			if _, ok := contract[field]; ok {
				present++
			}
		}
		if present != 0 && present != len(contracts) {
			return nil, false
		}
		if category == "ratio" && (field == "model_ratio" || field == "completion_ratio") && present != len(contracts) {
			return nil, false
		}
	}
	strategy := model.GetPricingSyncStrategy()
	if category == "price" {
		values := make([]float64, 0, len(contracts))
		for _, contract := range contracts {
			values = append(values, contract["model_price"])
		}
		value := values[0]
		switch strategy {
		case model.PricingSyncStrategyLowest:
			for _, candidate := range values[1:] {
				if candidate < value {
					value = candidate
				}
			}
		case model.PricingSyncStrategyAverage:
			value = 0
			for _, candidate := range values {
				value += candidate
			}
			value /= float64(len(values))
		default:
			for _, candidate := range values[1:] {
				if candidate > value {
					value = candidate
				}
			}
		}
		return map[string]any{"model_price": value}, true
	}
	aggregate := func(values []float64) float64 {
		result := values[0]
		switch strategy {
		case model.PricingSyncStrategyLowest:
			for _, value := range values[1:] {
				if value < result {
					result = value
				}
			}
		case model.PricingSyncStrategyAverage:
			result = 0
			for _, value := range values {
				result += value
			}
			result /= float64(len(values))
		default:
			for _, value := range values[1:] {
				if value > result {
					result = value
				}
			}
		}
		return result
	}
	baseValues := lo.Map(contracts, func(contract map[string]float64, _ int) float64 {
		return contract["model_ratio"]
	})
	base := aggregate(baseValues)
	if base == 0 {
		return nil, false
	}
	resolved := map[string]any{"model_ratio": base}
	for _, field := range fields[1:] {
		if _, present := contracts[0][field]; !present {
			continue
		}
		if field == "audio_completion_ratio" {
			continue
		}
		lanePrices := lo.Map(contracts, func(contract map[string]float64, _ int) float64 {
			return contract["model_ratio"] * contract[field]
		})
		resolved[field] = aggregate(lanePrices) / base
	}
	if _, present := contracts[0]["audio_completion_ratio"]; present {
		audioRatio, ok := resolved["audio_ratio"].(float64)
		if !ok || audioRatio == 0 {
			return nil, false
		}
		audioOutputPrices := lo.Map(contracts, func(contract map[string]float64, _ int) float64 {
			return contract["model_ratio"] * contract["audio_ratio"] * contract["audio_completion_ratio"]
		})
		resolved["audio_completion_ratio"] = aggregate(audioOutputPrices) / base / audioRatio
	}
	return resolved, true
}

// buildUpstreamPricingSyncPatches accepts a value only when every upstream
// exposing that model agrees on its billing category and field values. A
// category transition is intentionally left for the manual conflict dialog.
func buildUpstreamPricingSyncPatches(local map[string]any, upstreams []map[string]any, blockedModels map[string]struct{}) (map[string]model.JSONObjectPatch, []string, int) {
	return buildUpstreamPricingSyncPatchesWithPreferences(local, upstreams, nil, blockedModels, nil)
}

func buildUpstreamPricingSyncPatchesWithPreferences(local map[string]any, upstreams []map[string]any, sourceIDs []int, blockedModels map[string]struct{}, preferences map[string]model.PricingSyncModelState) (map[string]model.JSONObjectPatch, []string, int) {
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
		modelUpstreams := upstreams
		explicitChannel := false
		if preference, ok := preferences[modelName]; ok {
			switch preference.Mode {
			case model.PricingSyncModelModeManual:
				skipped = append(skipped, modelName)
				continue
			case model.PricingSyncModelModeChannel:
				explicitChannel = true
				modelUpstreams = make([]map[string]any, 0, 1)
				for index, sourceID := range sourceIDs {
					if sourceID == preference.ChannelID && index < len(upstreams) {
						modelUpstreams = append(modelUpstreams, upstreams[index])
					}
				}
				if len(modelUpstreams) == 0 {
					skipped = append(skipped, modelName)
					continue
				}
			}
		}
		category := ""
		conflict := false
		exposingUpstreams := make([]map[string]any, 0, len(modelUpstreams))
		for _, upstream := range modelUpstreams {
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
		if conflict || category == "" ||
			(!explicitChannel && pricingCategory(local, modelName) != "" && pricingCategory(local, modelName) != category) {
			skipped = append(skipped, modelName)
			continue
		}
		resolvedNumeric := map[string]any(nil)
		if category == "price" || category == "ratio" {
			var comparable bool
			resolvedNumeric, comparable = resolveComparableNumericPricing(exposingUpstreams, modelName, category)
			if !comparable {
				skipped = append(skipped, modelName)
				continue
			}
		}

		modelChanged := false
		modelPatches := make(map[string]model.JSONObjectPatch)
		for _, field := range pricingSyncFields {
			var expected any
			present := 0
			if resolvedNumeric != nil {
				value, exists := resolvedNumeric[field]
				if exists {
					expected = value
					expected = normalizeSyncValue(field, expected)
					present = 1
				}
			} else {
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
			}
			if conflict {
				break
			}
			if resolvedNumeric == nil && present > 0 && present != len(exposingUpstreams) {
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

func pricingSyncPatchModels(patches map[string]model.JSONObjectPatch) []string {
	models := make(map[string]struct{})
	for _, patch := range patches {
		for modelName := range patch.Set {
			models[modelName] = struct{}{}
		}
		for _, modelName := range patch.Delete {
			models[modelName] = struct{}{}
		}
	}
	return lo.Keys(models)
}

func pricingSyncMissingModels(activeSourceIDs []int, preferences map[string]model.PricingSyncModelState) (map[string]struct{}, error) {
	if len(activeSourceIDs) == 0 {
		return nil, nil
	}
	quotes := make([]model.PricingSyncQuote, 0)
	if err := model.DB.Where("channel_id IN ? AND missing_count >= ?", activeSourceIDs, 2).Find(&quotes).Error; err != nil {
		return nil, err
	}
	missingByModel := make(map[string]map[int]struct{})
	for _, quote := range quotes {
		if missingByModel[quote.ModelName] == nil {
			missingByModel[quote.ModelName] = make(map[int]struct{})
		}
		missingByModel[quote.ModelName][quote.ChannelID] = struct{}{}
	}
	result := make(map[string]struct{})
	for modelName, missingSources := range missingByModel {
		state, ok := preferences[modelName]
		if !ok || state.Mode == model.PricingSyncModelModeManual {
			continue
		}
		if state.Mode == model.PricingSyncModelModeChannel {
			if _, missing := missingSources[state.ChannelID]; missing {
				result[modelName] = struct{}{}
			}
			continue
		}
		var provenance []int
		if common.UnmarshalJsonStr(state.Provenance, &provenance) != nil ||
			!lo.SomeBy(provenance, func(channelID int) bool {
				_, missing := missingSources[channelID]
				return missing
			}) {
			continue
		}
		result[modelName] = struct{}{}
	}
	return result, nil
}

func removeUnavailablePricingSyncModels(patches map[string]model.JSONObjectPatch, unavailable map[string]struct{}) {
	for modelName := range unavailable {
		for _, field := range pricingSyncFields {
			key := optionKeyForPricingField(field)
			patch := patches[key]
			delete(patch.Set, modelName)
			patch.Delete = append(patch.Delete, modelName)
			patches[key] = patch
		}
	}
}

func pricingSyncAppliedStates(patches map[string]model.JSONObjectPatch, unavailable map[string]struct{}, staleSources map[int]struct{}, upstreams []map[string]any, sourceIDs []int, preferences map[string]model.PricingSyncModelState, now int64) []model.PricingSyncModelState {
	states := make([]model.PricingSyncModelState, 0)
	modelNames := lo.SliceToMap(pricingSyncPatchModels(patches), func(modelName string) (string, struct{}) {
		return modelName, struct{}{}
	})
	for modelName, state := range preferences {
		if state.Mode == model.PricingSyncModelModeManual {
			continue
		}
		if lo.SomeBy(upstreams, func(upstream map[string]any) bool {
			return pricingCategory(upstream, modelName) != ""
		}) {
			modelNames[modelName] = struct{}{}
		}
	}
	for modelName := range modelNames {
		previous := preferences[modelName]
		mode := previous.Mode
		if mode != model.PricingSyncModelModeChannel {
			mode = model.PricingSyncModelModeGeneral
		}
		provenance := make([]int, 0)
		signatures := make(map[string]struct{})
		for index, upstream := range upstreams {
			if index >= len(sourceIDs) || pricingCategory(upstream, modelName) == "" {
				continue
			}
			if mode == model.PricingSyncModelModeChannel && sourceIDs[index] != previous.ChannelID {
				continue
			}
			provenance = append(provenance, sourceIDs[index])
			contract := make(map[string]any)
			for _, field := range pricingSyncFields {
				if value, ok := valueMap(upstream[field])[modelName]; ok {
					contract[field] = normalizeSyncValue(field, value)
				}
			}
			encoded, _ := common.Marshal(contract)
			signatures[string(encoded)] = struct{}{}
		}
		provenanceJSON, _ := common.Marshal(lo.Uniq(provenance))
		status := model.PricingSyncModelStatusReady
		conflictDetails := ""
		if _, missing := unavailable[modelName]; missing {
			status = model.PricingSyncModelStatusUnavailable
		} else if lo.SomeBy(provenance, func(channelID int) bool {
			_, stale := staleSources[channelID]
			return stale
		}) {
			status = model.PricingSyncModelStatusStale
		} else if len(signatures) > 1 {
			status = model.PricingSyncModelStatusConflict
			conflictJSON, _ := common.Marshal(lo.Uniq(provenance))
			conflictDetails = string(conflictJSON)
		}
		states = append(states, model.PricingSyncModelState{
			ModelName: modelName, Mode: mode, ChannelID: previous.ChannelID,
			Provenance: string(provenanceJSON), Status: status,
			ConflictDetails: conflictDetails, LastAppliedAt: now,
		})
	}
	return states
}

func pricingSyncIncompatibleStates(local map[string]any, upstreams []map[string]any, sourceIDs []int, staleSources map[int]struct{}, preferences map[string]model.PricingSyncModelState, now int64) []model.PricingSyncModelState {
	modelNames := make(map[string]struct{})
	for _, upstream := range upstreams {
		for _, field := range pricingSyncFields {
			for modelName := range valueMap(upstream[field]) {
				modelNames[modelName] = struct{}{}
			}
		}
	}
	states := make([]model.PricingSyncModelState, 0)
	for modelName := range modelNames {
		previous := preferences[modelName]
		if previous.Mode == model.PricingSyncModelModeManual {
			continue
		}
		mode := previous.Mode
		if mode != model.PricingSyncModelModeChannel {
			mode = model.PricingSyncModelModeGeneral
		}
		categories := make(map[string]struct{})
		exposing := make([]map[string]any, 0)
		provenance := make([]int, 0)
		details := make(map[int]map[string]any)
		for index, upstream := range upstreams {
			if index >= len(sourceIDs) || pricingCategory(upstream, modelName) == "" {
				continue
			}
			if mode == model.PricingSyncModelModeChannel && sourceIDs[index] != previous.ChannelID {
				continue
			}
			category := pricingCategory(upstream, modelName)
			categories[category] = struct{}{}
			exposing = append(exposing, upstream)
			provenance = append(provenance, sourceIDs[index])
			contract := make(map[string]any)
			for _, field := range pricingSyncFields {
				if value, ok := valueMap(upstream[field])[modelName]; ok {
					contract[field] = normalizeSyncValue(field, value)
				}
			}
			details[sourceIDs[index]] = contract
		}
		if len(exposing) == 0 {
			continue
		}
		incompatible := len(categories) != 1
		category := lo.Keys(categories)[0]
		if !incompatible && (category == "price" || category == "ratio") {
			_, comparable := resolveComparableNumericPricing(exposing, modelName, category)
			incompatible = !comparable
		}
		if !incompatible && category == "tiered" {
			signatures := make(map[string]struct{})
			for _, contract := range details {
				encoded, _ := common.Marshal(contract)
				signatures[string(encoded)] = struct{}{}
			}
			incompatible = len(signatures) > 1
		}
		localCategory := pricingCategory(local, modelName)
		if mode != model.PricingSyncModelModeChannel && localCategory != "" && localCategory != category {
			incompatible = true
		}
		if !incompatible {
			continue
		}
		provenanceJSON, _ := common.Marshal(lo.Uniq(provenance))
		detailsJSON, _ := common.Marshal(details)
		status := model.PricingSyncModelStatusConflict
		if lo.SomeBy(provenance, func(channelID int) bool {
			_, stale := staleSources[channelID]
			return stale
		}) {
			status = model.PricingSyncModelStatusStale
		}
		states = append(states, model.PricingSyncModelState{
			ModelName: modelName, Mode: mode, ChannelID: previous.ChannelID,
			Provenance: string(provenanceJSON), Status: status,
			ConflictDetails: string(detailsJSON), LastAppliedAt: now,
		})
	}
	return states
}

func runUpstreamPricingSyncTaskOnce(ctx context.Context, previousCandidateHash string, report func(processed, total int)) (upstreamPricingSyncSummary, error) {
	summary := upstreamPricingSyncSummary{}
	_ = previousCandidateHash
	configVersion, err := model.GetPricingSyncConfigVersion()
	if err != nil {
		return summary, err
	}
	syncSources, err := model.GetPricingSyncSources()
	if err != nil {
		common.SysLog(fmt.Sprintf("upstream pricing sync query failed: %v", err))
		return summary, err
	}
	channelIDs := make([]int, 0, len(syncSources))
	for _, source := range syncSources {
		if source.Enabled && source.IntervalSeconds > 0 && source.ChannelID > 0 {
			channelIDs = append(channelIDs, source.ChannelID)
		}
	}
	channels, err := model.GetChannelsByIds(channelIDs)
	if err != nil {
		return summary, err
	}
	channelsByID := make(map[int]*model.Channel, len(channels))
	for _, channel := range channels {
		channelsByID[channel.Id] = channel
	}
	now := common.GetTimestamp()

	upstreams := make([]map[string]any, 0)
	sources := make([]string, 0)
	blockedModels := make(map[string]struct{})
	staleSources := make(map[int]struct{})
	activeSourceIDs := make(map[int]struct{})
	for index, source := range syncSources {
		if report != nil {
			report(index+1, len(syncSources))
		}
		if ctx.Err() != nil || !source.Enabled || source.IntervalSeconds == 0 {
			continue
		}
		channel := channelsByID[source.ChannelID]
		if source.ChannelID > 0 && (channel == nil || channel.Status != common.ChannelStatusEnabled) {
			staleSources[source.ChannelID] = struct{}{}
			continue
		}
		activeSourceIDs[source.ChannelID] = struct{}{}
		if source.LastError != "" {
			staleSources[source.ChannelID] = struct{}{}
		}
		if source.LastAttemptAt > 0 && now-source.LastAttemptAt < int64(source.IntervalSeconds) {
			continue
		}
		if err := updatePricingSyncSourceIfCurrent(source, configVersion, map[string]any{"last_attempt_at": now}); err != nil {
			return summary, err
		}
		target, targetErr := pricingSyncFetchTargetForSource(source, channel)
		if targetErr != nil {
			summary.FailedChannels++
			staleSources[source.ChannelID] = struct{}{}
			if err := updatePricingSyncSourceIfCurrent(source, configVersion, map[string]any{"last_error": targetErr.Error()}); err != nil {
				return summary, err
			}
			continue
		}
		summary.CheckedChannels++
		channelCtx, cancel := context.WithTimeout(ctx, time.Duration(defaultTimeoutSeconds)*time.Second)
		fetched, err := fetchPricingSyncURL(channelCtx, target.URL, target.Mode, target.Hosts, target.APIKey, getHTTPClient())
		cancel()
		if err != nil {
			summary.FailedChannels++
			staleSources[source.ChannelID] = struct{}{}
			if updateErr := updatePricingSyncSourceIfCurrent(source, configVersion, map[string]any{"last_error": err.Error()}); updateErr != nil {
				return summary, updateErr
			}
			common.SysLog(fmt.Sprintf("upstream pricing sync failed: channel_id=%d err=%v", source.ChannelID, err))
			continue
		}
		_, err = confirmPricingSyncQuotesIfCurrent(source, fetched.Data, now, configVersion)
		if err != nil {
			summary.FailedChannels++
			staleSources[source.ChannelID] = struct{}{}
			return summary, err
		}
		delete(staleSources, source.ChannelID)
		summary.SkippedModels = append(summary.SkippedModels, fetched.UnsupportedModels...)
		for _, modelName := range fetched.UnsupportedModels {
			blockedModels[modelName] = struct{}{}
		}
	}
	if ctx.Err() != nil {
		return summary, ctx.Err()
	}
	if err := model.MarkPricingSyncSourcesStale(lo.Keys(staleSources)); err != nil {
		return summary, err
	}
	if summary.FailedChannels > 0 {
		return summary, fmt.Errorf("upstream pricing sync failed for %d channel(s)", summary.FailedChannels)
	}
	for _, source := range syncSources {
		if !source.Enabled || source.IntervalSeconds == 0 {
			continue
		}
		if _, active := activeSourceIDs[source.ChannelID]; !active {
			continue
		}
		data, err := confirmedPricingSyncQuoteData(source.ChannelID)
		if err != nil {
			return summary, err
		}
		if len(data) == 0 {
			continue
		}
		upstreams = append(upstreams, data)
		sources = append(sources, strconv.Itoa(source.ChannelID))
	}
	preferences, err := model.GetPricingSyncModelStates()
	if err != nil {
		return summary, err
	}
	localPricing := getLocalPricingSyncData()
	patches, skipped, applied := buildUpstreamPricingSyncPatchesWithPreferences(localPricing, upstreams, lo.Map(sources, func(source string, _ int) int {
		channelID, _ := strconv.Atoi(source)
		return channelID
	}), blockedModels, preferences)
	sourceIDs := lo.Map(sources, func(source string, _ int) int {
		channelID, _ := strconv.Atoi(source)
		return channelID
	})
	activeIDs := lo.Keys(activeSourceIDs)
	unavailable, err := pricingSyncMissingModels(activeIDs, preferences)
	if err != nil {
		return summary, err
	}
	removeUnavailablePricingSyncModels(patches, unavailable)
	summary.SkippedModels = lo.Uniq(append(summary.SkippedModels, skipped...))
	states := pricingSyncAppliedStates(patches, unavailable, staleSources, upstreams, sourceIDs, preferences, now)
	for _, state := range pricingSyncIncompatibleStates(localPricing, upstreams, sourceIDs, staleSources, preferences, now) {
		replaced := false
		for index := range states {
			if states[index].ModelName == state.ModelName {
				states[index] = state
				replaced = true
				break
			}
		}
		if !replaced {
			states = append(states, state)
		}
	}
	if len(patches) == 0 && len(states) == 0 {
		return summary, nil
	}
	if len(patches) > 0 {
		candidateHash, err := pricingCandidateHash(sources, patches)
		if err != nil {
			return summary, err
		}
		summary.CandidateHash = candidateHash
	}
	if err := model.ApplyPricingSyncUpdateIfVersion(patches, states, configVersion); err != nil {
		return summary, fmt.Errorf("upstream pricing sync apply failed: %w", err)
	}
	summary.AppliedModels = applied
	return summary, nil
}
