package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

type supplierCatalogueItem struct {
	ModelName             string                `json:"model_name"`
	QuotaType             *int                  `json:"quota_type"`
	ModelRatio            *float64              `json:"model_ratio"`
	ModelPrice            *float64              `json:"model_price"`
	CompletionRatio       *float64              `json:"completion_ratio"`
	CacheRatio            *float64              `json:"cache_ratio"`
	CreateCacheRatio      *float64              `json:"create_cache_ratio"`
	CreateCache1hRatio    *float64              `json:"create_cache_ratio_1h"`
	CacheCreationRatio    *float64              `json:"cache_creation_ratio"`
	CacheCreation5mRatio  *float64              `json:"cache_creation_5m_ratio"`
	CacheCreationRatio5m  *float64              `json:"cache_creation_ratio_5m"`
	CacheCreation1hRatio  *float64              `json:"cache_creation_1h_ratio"`
	CacheCreationRatio1h  *float64              `json:"cache_creation_ratio_1h"`
	CacheCreation12hRatio *float64              `json:"cache_creation_12h_ratio"`
	ImageCacheRatio       *float64              `json:"image_cache_ratio"`
	ImageRatio            *float64              `json:"image_ratio"`
	ImageCompletionRatio  *float64              `json:"image_completion_ratio"`
	AudioRatio            *float64              `json:"audio_ratio"`
	AudioCompletionRatio  *float64              `json:"audio_completion_ratio"`
	ReasoningRatio        *float64              `json:"reasoning_ratio"`
	BillingMode           string                `json:"billing_mode"`
	BillingExpr           string                `json:"billing_expr"`
	BillingMultiplier     *float64              `json:"billing_multiplier"`
	TaskPriceUnit         string                `json:"task_price_unit"`
	EnableGroups          []string              `json:"enable_groups"`
	GroupRatio            map[string]float64    `json:"group_ratio"`
	StepRatios            []upstreamPricingStep `json:"step_ratios"`
	Tools                 map[string]float64    `json:"tool_prices"`
	ToolPriceUnit         string                `json:"tool_price_unit"`
	ModelGroupRatio       json.RawMessage       `json:"model_group_ratio"`
	PeakPricing           json.RawMessage       `json:"peak_pricing"`
	Tiers                 json.RawMessage       `json:"tiers"`
	ImageBilling          json.RawMessage       `json:"image_billing"`
	VideoResolutionRatios json.RawMessage       `json:"video_resolution_ratios"`
	ModelPriceMin         *float64              `json:"model_price_min"`
	ModelPriceMax         *float64              `json:"model_price_max"`
	PixverseRefAudio      json.RawMessage       `json:"pixverse_v6_ref_audio"`
	PixverseRefSilent     json.RawMessage       `json:"pixverse_v6_ref_silent"`
}

type supplierRoutingSource struct {
	Tier           string   `json:"tier"`
	Family         string   `json:"family"`
	Status         string   `json:"status"`
	Models         []string `json:"models"`
	ModelDiscounts []struct {
		Model           string   `json:"model"`
		PriceRatio      *float64 `json:"price_ratio"`
		PriceMultiplier *float64 `json:"price_multiplier"`
	} `json:"model_discounts"`
}

type supplierCatalogue struct {
	Success         *bool                      `json:"success"`
	Data            []supplierCatalogueItem    `json:"data"`
	GroupRatio      map[string]float64         `json:"group_ratio"`
	UsableGroup     map[string]json.RawMessage `json:"usable_group"`
	RoutingSources  []supplierRoutingSource    `json:"routing_sources"`
	ModelGroupRatio json.RawMessage            `json:"model_group_ratio"`
	PeakPricing     json.RawMessage            `json:"peak_pricing"`
	PeakActive      json.RawMessage            `json:"peak_active"`
}

func supplierModifierPresent(value json.RawMessage) bool {
	text := strings.TrimSpace(string(value))
	if text == "" {
		return false
	}
	var decoded any
	if common.Unmarshal(value, &decoded) != nil {
		return true
	}
	switch decoded := decoded.(type) {
	case nil:
		return false
	case map[string]any:
		return len(decoded) > 0
	case []any:
		return len(decoded) > 0
	default:
		return true
	}
}

func supplierCacheAlias(lane string, values ...*float64) (*float64, error) {
	var selected *float64
	for _, value := range values {
		if value == nil {
			continue
		}
		if !nonNegativeFinite(*value) {
			return nil, fmt.Errorf("invalid %s cache ratio", lane)
		}
		if selected != nil && *selected != *value {
			return nil, fmt.Errorf("conflicting %s cache ratio aliases", lane)
		}
		selected = value
	}
	return selected, nil
}

func validateSupplierCatalogueModifiers(catalogue supplierCatalogue, adapter string) error {
	for _, modifier := range []struct {
		name  string
		value json.RawMessage
	}{
		{"model_group_ratio", catalogue.ModelGroupRatio}, {"peak_pricing", catalogue.PeakPricing},
	} {
		if supplierModifierPresent(modifier.value) {
			return fmt.Errorf("unsupported supplier pricing modifier: %s", modifier.name)
		}
	}
	if supplierModifierPresent(catalogue.PeakActive) && strings.TrimSpace(string(catalogue.PeakActive)) != "false" {
		return fmt.Errorf("unsupported supplier pricing modifier: peak_active")
	}
	if adapter != "relay-fast" && len(catalogue.RoutingSources) > 0 {
		return fmt.Errorf("unsupported supplier pricing modifier: routing_sources")
	}
	return nil
}

func normalizeSupplierItemAliases(item *supplierCatalogueItem, adapter string) error {
	if item.CacheCreation12hRatio != nil {
		return fmt.Errorf("unsupported supplier price lane: cache_creation_12h_ratio")
	}
	if item.ImageCacheRatio != nil {
		return fmt.Errorf("unsupported supplier price lane: image_cache_ratio")
	}
	if item.ModelPriceMin != nil || item.ModelPriceMax != nil {
		return fmt.Errorf("unsupported supplier price modifier: model_price_min/model_price_max")
	}
	for _, modifier := range []struct {
		name  string
		value json.RawMessage
	}{
		{"model_group_ratio", item.ModelGroupRatio}, {"peak_pricing", item.PeakPricing}, {"tiers", item.Tiers}, {"image_billing", item.ImageBilling}, {"video_resolution_ratios", item.VideoResolutionRatios}, {"pixverse_v6_ref_audio", item.PixverseRefAudio}, {"pixverse_v6_ref_silent", item.PixverseRefSilent},
	} {
		if supplierModifierPresent(modifier.value) {
			return fmt.Errorf("unsupported supplier price modifier: %s", modifier.name)
		}
	}
	if adapter != "relay-fast" && len(item.GroupRatio) > 0 {
		return fmt.Errorf("unsupported supplier price modifier: per-model group_ratio")
	}
	var err error
	item.CreateCacheRatio, err = supplierCacheAlias("generic/5m", item.CreateCacheRatio, item.CacheCreationRatio, item.CacheCreation5mRatio, item.CacheCreationRatio5m)
	if err != nil {
		return err
	}
	item.CreateCache1hRatio, err = supplierCacheAlias("1h", item.CreateCache1hRatio, item.CacheCreation1hRatio, item.CacheCreationRatio1h)
	return err
}

func supplierSourceFactors(catalogue supplierCatalogue, adapter, group string) (map[string]float64, error) {
	if adapter != "relay-fast" || group == "default" {
		ratio, ok := catalogue.GroupRatio[group]
		if !ok || !nonNegativeFinite(ratio) {
			return nil, fmt.Errorf("selected upstream group has no valid price ratio")
		}
		if _, ok := catalogue.UsableGroup[group]; !ok {
			return nil, fmt.Errorf("selected upstream group is not usable")
		}
		factors := make(map[string]float64)
		for _, item := range catalogue.Data {
			if !slices.Contains(item.EnableGroups, group) {
				continue
			}
			factor := ratio
			if adapter == "relay-fast" {
				var ok bool
				factor, ok = item.GroupRatio[group]
				if !ok || !nonNegativeFinite(factor) {
					return nil, fmt.Errorf("%s: missing model route price ratio", item.ModelName)
				}
			}
			factors[item.ModelName] = factor
		}
		return factors, nil
	}
	var selected *supplierRoutingSource
	for i := range catalogue.RoutingSources {
		route := &catalogue.RoutingSources[i]
		if route.Tier != group && route.Family+":"+route.Tier != group {
			continue
		}
		if selected != nil {
			return nil, fmt.Errorf("ambiguous routing source; use family:tier")
		}
		selected = route
	}
	if selected == nil || selected.Status != "active" {
		return nil, fmt.Errorf("selected routing source is absent or inactive")
	}
	factors := make(map[string]float64, len(selected.Models))
	for _, discount := range selected.ModelDiscounts {
		if !slices.Contains(selected.Models, discount.Model) {
			continue
		}
		if discount.PriceRatio == nil || !nonNegativeFinite(*discount.PriceRatio) {
			return nil, fmt.Errorf("routing source has an invalid model price ratio")
		}
		if discount.PriceMultiplier != nil && (!nonNegativeFinite(*discount.PriceMultiplier) || *discount.PriceMultiplier != *discount.PriceRatio) {
			return nil, fmt.Errorf("routing source model has conflicting price_ratio/price_multiplier")
		}
		if _, duplicate := factors[discount.Model]; duplicate {
			return nil, fmt.Errorf("duplicate routing model price")
		}
		factors[discount.Model] = *discount.PriceRatio
	}
	for _, name := range selected.Models {
		if _, ok := factors[name]; !ok {
			return nil, fmt.Errorf("%s: routing source has no exact model price", name)
		}
	}
	return factors, nil
}

func scaleSupplierExpression(expression string, factor float64) string {
	expression = strings.TrimSpace(expression)
	prefix := ""
	if strings.HasPrefix(expression, "v1:") {
		prefix = "v1:"
		expression = strings.TrimPrefix(expression, "v1:")
	}
	return prefix + "(" + expression + ") * " + strconv.FormatFloat(factor, 'g', -1, 64)
}

// normalizeSupplierCatalogue converts provider credits once, before the retail
// group multiplier is applied. No source model is merged with a global tariff.
func normalizeSupplierCatalogue(body []byte, adapter string, profile model.SupplierPricingProfile) ([]types.SupplierModelTariff, error) {
	if profile.USDPerCredit <= 0 || math.IsNaN(profile.USDPerCredit) || math.IsInf(profile.USDPerCredit, 0) {
		return nil, fmt.Errorf("an explicit positive real USD per provider credit is required")
	}
	catalogue := supplierCatalogue{}
	if err := common.DecodeJson(bytes.NewReader(body), &catalogue); err != nil {
		return nil, fmt.Errorf("invalid supplier pricing JSON: %w", err)
	}
	if catalogue.Success == nil || !*catalogue.Success || catalogue.Data == nil || catalogue.GroupRatio == nil || catalogue.UsableGroup == nil {
		return nil, fmt.Errorf("supplier pricing response is incomplete or unsuccessful")
	}
	if err := validateSupplierCatalogueModifiers(catalogue, adapter); err != nil {
		return nil, err
	}
	group := profile.UpstreamGroup
	if strings.TrimSpace(group) == "" {
		return nil, fmt.Errorf("select an exact upstream group or routing source")
	}
	factors, err := supplierSourceFactors(catalogue, adapter, group)
	if err != nil {
		return nil, err
	}
	tariffs := make([]types.SupplierModelTariff, 0, len(factors))
	seen := make(map[string]bool)
	for _, item := range catalogue.Data {
		factor, selected := factors[item.ModelName]
		if !selected {
			continue
		}
		if seen[item.ModelName] {
			return nil, fmt.Errorf("duplicate supplier model %s", item.ModelName)
		}
		seen[item.ModelName] = true
		if err := normalizeSupplierItemAliases(&item, adapter); err != nil {
			return nil, fmt.Errorf("%s: %w", item.ModelName, err)
		}
		for _, value := range []*float64{item.ModelRatio, item.ModelPrice, item.CompletionRatio, item.CacheRatio, item.CreateCacheRatio, item.CreateCache1hRatio, item.ImageRatio, item.ImageCompletionRatio, item.AudioRatio, item.AudioCompletionRatio, item.ReasoningRatio, item.BillingMultiplier} {
			if value != nil && !nonNegativeFinite(*value) {
				return nil, fmt.Errorf("%s: invalid source price component", item.ModelName)
			}
		}
		conversion := decimal.NewFromFloat(factor).Mul(decimal.NewFromFloat(profile.USDPerCredit))
		factor, _ = conversion.Float64()
		if item.BillingMultiplier != nil && *item.BillingMultiplier != 1 {
			return nil, fmt.Errorf("%s: unsupported additional billing multiplier", item.ModelName)
		}
		if item.QuotaType == nil || (*item.QuotaType != 0 && *item.QuotaType != 1) {
			return nil, fmt.Errorf("%s: unsupported or missing quota type", item.ModelName)
		}
		tariff := types.SupplierModelTariff{ModelName: item.ModelName, PriceUnit: "million_tokens", BillingMode: types.SupplierBillingModeToken}
		price := func(values ...float64) *float64 {
			result := conversion
			for _, value := range values {
				result = result.Mul(decimal.NewFromFloat(value))
			}
			value, _ := result.Float64()
			return &value
		}
		if item.ModelRatio != nil {
			tariff.InputUSD = price(2, *item.ModelRatio)
			if item.CompletionRatio != nil {
				tariff.OutputUSD = price(2, *item.ModelRatio, *item.CompletionRatio)
			}
			lane := func(ratio *float64) *float64 {
				if ratio == nil {
					return nil
				}
				return price(2, *item.ModelRatio, *ratio)
			}
			tariff.CacheReadUSD = lane(item.CacheRatio)
			tariff.CacheWriteUSD = lane(item.CreateCacheRatio)
			tariff.CacheWrite1hUSD = lane(item.CreateCache1hRatio)
			tariff.ImageUSD = lane(item.ImageRatio)
			tariff.ImageOutputUSD = lane(item.ImageCompletionRatio)
			tariff.AudioInputUSD = lane(item.AudioRatio)
			if item.AudioCompletionRatio != nil && item.AudioRatio != nil {
				tariff.AudioOutputUSD = price(2, *item.ModelRatio, *item.AudioRatio, *item.AudioCompletionRatio)
			}
			tariff.ReasoningUSD = lane(item.ReasoningRatio)
			// New API completion tokens include reasoning unless the source
			// explicitly declares a separate reasoning multiplier.
			if item.ReasoningRatio == nil {
				tariff.ReasoningUSD = tariff.OutputUSD
			}
		}
		if len(item.Tools) > 0 {
			if item.ToolPriceUnit != "request" && item.ToolPriceUnit != "per_1000_calls" {
				return nil, fmt.Errorf("%s: tool price unit is missing or unsupported", item.ModelName)
			}
			tariff.Tools = make(map[string]float64, len(item.Tools))
			for key, value := range item.Tools {
				if !nonNegativeFinite(value) {
					return nil, fmt.Errorf("%s: invalid tool price", item.ModelName)
				}
				if item.ToolPriceUnit == "per_1000_calls" {
					tariff.Tools[key] = *price(value, 0.001)
				} else {
					tariff.Tools[key] = *price(value)
				}
			}
		}
		switch {
		case item.BillingMode == "tiered_expr":
			if strings.TrimSpace(item.BillingExpr) == "" {
				return nil, fmt.Errorf("%s: missing billing expression", item.ModelName)
			}
			tariff.BillingMode = types.SupplierBillingModeExpression
			tariff.Expression = scaleSupplierExpression(item.BillingExpr, factor)
		case len(item.StepRatios) > 0:
			if item.ModelRatio == nil || item.CompletionRatio == nil {
				return nil, fmt.Errorf("%s: incomplete step pricing", item.ModelName)
			}
			expr, err := supplierStepPricingExpression(item, tariff)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", item.ModelName, err)
			}
			tariff.BillingMode = types.SupplierBillingModeExpression
			tariff.Expression = expr
		case *item.QuotaType == 1:
			if item.ModelPrice == nil {
				return nil, fmt.Errorf("%s: fixed price is missing", item.ModelName)
			}
			tariff.BillingMode = types.SupplierBillingModeFixed
			tariff.PriceUSD = price(*item.ModelPrice)
			tariff.PriceUnit = "request"
			if item.TaskPriceUnit != "" {
				if item.TaskPriceUnit == "per_call" {
					tariff.PriceUnit = "request"
				} else {
					tariff.PriceUnit = item.TaskPriceUnit
				}
			}
		default:
			if item.BillingMode != "" && item.BillingMode != "ratio" {
				return nil, fmt.Errorf("%s: unsupported billing mode", item.ModelName)
			}
		}
		if err := model.ValidateSupplierModelTariff(tariff); err != nil {
			return nil, fmt.Errorf("%s: %w", item.ModelName, err)
		}
		tariffs = append(tariffs, tariff)
	}
	// A source listing an unknown model is a schema inconsistency, not deletion.
	if adapter == "relay-fast" && group != "default" && len(seen) != len(factors) {
		return nil, fmt.Errorf("routing source references absent catalogue models")
	}
	return tariffs, nil
}
