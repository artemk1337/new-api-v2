package model

import (
	"slices"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
)

// PublicGroupPrice contains retail prices only. Never embed a procurement DTO.
type PublicGroupPrice struct {
	billingexpr.PublicPricingTier
	GroupID     int                             `json:"group_id"`
	Group       string                          `json:"group"`
	GroupName   string                          `json:"group_name"`
	BillingMode string                          `json:"billing_mode"`
	PriceUnit   string                          `json:"price_unit"`
	PriceUSD    *float64                        `json:"price_usd,omitempty"`
	Tools       map[string]float64              `json:"tools,omitempty"`
	Tiers       []billingexpr.PublicPricingTier `json:"tiers,omitempty"`
	Unavailable bool                            `json:"unavailable,omitempty"`
}

type PublicPricingModel struct {
	ModelName              string                          `json:"model_name"`
	Description            string                          `json:"description,omitempty"`
	Icon                   string                          `json:"icon,omitempty"`
	Tags                   string                          `json:"tags,omitempty"`
	VendorID               int                             `json:"vendor_id,omitempty"`
	EnableGroup            []string                        `json:"enable_groups"`
	EnableGroupRefs        []ratio_setting.PricingGroupRef `json:"enable_group_refs,omitempty"`
	SupportedEndpointTypes []constant.EndpointType         `json:"supported_endpoint_types"`
	GroupPrices            []PublicGroupPrice              `json:"group_prices"`
	OfficialPrice          *OfficialAPIPrice               `json:"official_price,omitempty"`
}

func retailPrice(price *float64, multiplier float64) *float64 {
	if price == nil {
		return nil
	}
	value := *price * multiplier
	return &value
}

func publicGroupPrice(tariff types.SupplierModelTariff, multiplier float64) PublicGroupPrice {
	price := PublicGroupPrice{BillingMode: tariff.BillingMode, PriceUnit: tariff.PriceUnit}
	price.PublicPricingTier = billingexpr.PublicPricingTier{
		InputUSD: retailPrice(tariff.InputUSD, multiplier), OutputUSD: retailPrice(tariff.OutputUSD, multiplier),
		CacheReadUSD: retailPrice(tariff.CacheReadUSD, multiplier), CacheWriteUSD: retailPrice(tariff.CacheWriteUSD, multiplier),
		CacheWrite1hUSD: retailPrice(tariff.CacheWrite1hUSD, multiplier), ImageUSD: retailPrice(tariff.ImageUSD, multiplier),
		ImageOutputUSD: retailPrice(tariff.ImageOutputUSD, multiplier), AudioInputUSD: retailPrice(tariff.AudioInputUSD, multiplier),
		AudioOutputUSD: retailPrice(tariff.AudioOutputUSD, multiplier), ReasoningOutputUSD: retailPrice(tariff.ReasoningUSD, multiplier),
	}
	price.PriceUSD = retailPrice(tariff.PriceUSD, multiplier)
	if tariff.Expression != "" {
		tiers, ok := billingexpr.PublicPricingTiers(tariff.Expression, multiplier)
		if ok && len(tiers) > 0 {
			price.PublicPricingTier = tiers[0]
			price.Tiers = tiers
		} else {
			// Request-dependent expressions cannot be represented as fixed rates.
			price.PublicPricingTier = billingexpr.PublicPricingTier{}
		}
	}
	for key, value := range tariff.Tools {
		if price.Tools == nil {
			price.Tools = make(map[string]float64)
		}
		price.Tools[key] = value * multiplier
	}
	return price
}

func defaultModelTariff(modelName string) (types.SupplierModelTariff, bool) {
	tariff := types.SupplierModelTariff{ModelName: modelName, BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens"}
	if billing_setting.GetBillingMode(modelName) == billing_setting.BillingModeTieredExpr {
		expression, ok := billing_setting.GetBillingExpr(modelName)
		if !ok || strings.TrimSpace(expression) == "" {
			return tariff, false
		}
		tariff.BillingMode, tariff.Expression = types.SupplierBillingModeExpression, expression
		return tariff, true
	}
	if price, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		tariff.BillingMode, tariff.PriceUnit, tariff.PriceUSD = types.SupplierBillingModeFixed, "request", &price
		return tariff, true
	}
	ratio, ok, _ := ratio_setting.GetModelRatio(modelName)
	if !ok {
		return tariff, false
	}
	input := ratio * 1_000_000 / common.GetQuotaPerUnit()
	output := input * ratio_setting.GetCompletionRatio(modelName)
	tariff.InputUSD, tariff.OutputUSD = &input, &output
	set := func(target **float64, value float64, exists bool) {
		if exists {
			result := value * input
			*target = &result
		}
	}
	value, exists := ratio_setting.GetCacheRatio(modelName)
	set(&tariff.CacheReadUSD, value, exists)
	value, exists = ratio_setting.GetCreateCacheRatio(modelName)
	set(&tariff.CacheWriteUSD, value, exists)
	if tariff.CacheWriteUSD != nil {
		value := *tariff.CacheWriteUSD * 6 / 3.75
		tariff.CacheWrite1hUSD = &value
	}
	value, exists = ratio_setting.GetImageRatio(modelName)
	set(&tariff.ImageUSD, value, exists)
	set(&tariff.AudioInputUSD, ratio_setting.GetAudioRatio(modelName), ratio_setting.ContainsAudioRatio(modelName))
	set(&tariff.AudioOutputUSD, ratio_setting.GetAudioRatio(modelName)*ratio_setting.GetAudioCompletionRatio(modelName), ratio_setting.ContainsAudioCompletionRatio(modelName))
	return tariff, true
}

// GetPublicPricingCatalog resolves mappings independently for each group. It
// does not require a global tariff for models served through a supplier profile.
func GetPublicPricingCatalog(usable map[string]string, multipliers map[string]float64) ([]PublicPricingModel, error) {
	data, err := loadPublicPricingData(usable)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(data.abilities))
	for _, ability := range data.abilities {
		names = append(names, ability.Model)
	}
	metadata := buildPricingModelMap(data.metadata, names)
	type groupTarget struct {
		target   string
		conflict bool
	}
	targets := make(map[string]map[string]groupTarget)
	endpoints := make(map[string][]constant.EndpointType)
	for index, ability := range data.abilities {
		group := ratio_setting.PricingGroupKey(ability.Group)
		if _, ok := usable[group]; !ok {
			continue
		}
		channel, exists := data.channels[ability.ChannelId]
		if !exists {
			continue
		}
		ref, ok := ratio_setting.PricingGroupRefByKey(group)
		if !ok || !data.channelMatches(channel, data.bindings[ref.Id]) {
			continue
		}
		target := data.targets[index]
		if targets[ability.Model] == nil {
			targets[ability.Model] = make(map[string]groupTarget)
		}
		previous, exists := targets[ability.Model][group]
		if exists && previous.target != target {
			previous.conflict = true
		} else {
			previous.target = target
		}
		if target == "" {
			previous.conflict = true
		}
		targets[ability.Model][group] = previous
		for _, endpoint := range common.GetEndpointTypesByChannelType(ability.ChannelType, ability.Model) {
			if !slices.Contains(endpoints[ability.Model], endpoint) {
				endpoints[ability.Model] = append(endpoints[ability.Model], endpoint)
			}
		}
	}
	result := make([]PublicPricingModel, 0, len(targets))
	for name, groups := range targets {
		item := PublicPricingModel{ModelName: name, GroupPrices: make([]PublicGroupPrice, 0), SupportedEndpointTypes: endpoints[name]}
		if customEndpoints := GetModelSupportEndpointTypes(name); len(customEndpoints) > 0 {
			item.SupportedEndpointTypes = customEndpoints
		}
		if price, ok := data.official[name]; ok {
			item.OfficialPrice = &price
		}
		if meta, ok := metadata[name]; ok {
			if meta.Status != 1 {
				continue
			}
			item.Description, item.Icon, item.Tags, item.VendorID = meta.Description, meta.Icon, meta.Tags, meta.VendorID
		}
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, group := range keys {
			ref, ok := ratio_setting.PricingGroupRefByKey(group)
			if !ok {
				continue
			}
			price := PublicGroupPrice{Unavailable: true, PriceUnit: "million_tokens"}
			target := groups[group]
			if !target.conflict {
				binding := data.bindings[ref.Id]
				bound := binding.SupplierID != 0 || binding.ProfileID != 0
				tariff, available := data.profiles[binding.ProfileID].tariffs[target.target]
				if !bound {
					tariff, available = defaultModelTariff(target.target)
				}
				if available {
					price = publicGroupPrice(tariff, multipliers[group])
					if bound && tariff.BillingMode == types.SupplierBillingModeFixed && slices.Contains(item.SupportedEndpointTypes, constant.EndpointTypeImageGeneration) {
						price = PublicGroupPrice{Unavailable: true, PriceUnit: tariff.PriceUnit}
					}
				}
			}
			price.Group, price.GroupID, price.GroupName = group, ref.Id, ref.Name
			item.GroupPrices = append(item.GroupPrices, price)
			item.EnableGroup = append(item.EnableGroup, group)
			item.EnableGroupRefs = append(item.EnableGroupRefs, ref)
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ModelName < result[j].ModelName })
	return result, nil
}
