package model

import (
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// PublicBaseReference is the public global tariff's standard-context reference.
// It never provides billing fallback for a supplier-bound group.
type PublicBaseReference struct {
	InputUSD  float64 `json:"input_usd"`
	OutputUSD float64 `json:"output_usd"`
	PriceUnit string  `json:"price_unit"`
}

func publicBaseReference(modelName string) *PublicBaseReference {
	if modelName == "" || !hasAvailableModelPricing(modelName) {
		return nil
	}
	tariff, ok := defaultModelTariff(modelName)
	if !ok {
		return nil
	}
	price := publicGroupPrice(tariff, 1)
	if price.PriceUnit != "million_tokens" {
		return nil
	}
	if tariff.Expression != "" {
		// Preserve an explicitly configured legacy display pair. Without one,
		// publicGroupPrice already exposes the first/default global tier.
		name := ratio_setting.FormatMatchingModelName(modelName)
		ratio, exists := ratio_setting.GetModelRatioCopy()[name]
		completion := ratio_setting.GetCompletionRatioInfo(name)
		_, configuredCompletion := ratio_setting.GetCompletionRatioCopy()[name]
		if exists && (configuredCompletion || completion.Locked) {
			input := ratio * 1_000_000 / common.GetQuotaPerUnit()
			output := input * completion.Ratio
			if reference := publicTokenReference(&input, &output); reference != nil {
				return reference
			}
		}
	}
	return publicTokenReference(price.InputUSD, price.OutputUSD)
}

func publicTokenReference(input, output *float64) *PublicBaseReference {
	if input == nil || output == nil {
		return nil
	}
	for _, rate := range []float64{*input, *output} {
		if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return nil
		}
	}
	if *input == 0 && *output == 0 {
		return nil
	}
	return &PublicBaseReference{InputUSD: *input, OutputUSD: *output, PriceUnit: "million_tokens"}
}

func publicPricingVendorMaps(vendors []Vendor) (map[int]PricingVendor, map[string]PricingVendor) {
	raw := make(map[int]*Vendor, len(vendors))
	for index := range vendors {
		raw[vendors[index].Id] = &vendors[index]
	}
	byName := make(map[string]PricingVendor, len(vendors))
	for _, vendor := range buildPricingVendorsList(raw) {
		byName[vendor.Name] = vendor
	}
	byID := make(map[int]PricingVendor, len(vendors))
	for _, vendor := range vendors {
		byID[vendor.Id] = byName[getDefaultVendorDisplayName(vendor.Name)]
	}
	return byID, byName
}

func publicPricingVendor(modelName string, id int, byID map[int]PricingVendor, byName map[string]PricingVendor) PricingVendor {
	if vendor, ok := byID[id]; ok {
		return vendor
	}
	for pattern, name := range defaultVendorRules {
		if !strings.Contains(strings.ToLower(modelName), pattern) {
			continue
		}
		if vendor, ok := byName[name]; ok {
			return vendor
		}
		return PricingVendor{Name: name, Icon: getDefaultVendorIcon(name)}
	}
	return PricingVendor{}
}

// GetPublicPricingVendors derives the provider facet from the same catalogue,
// including name/icon inference when the legacy metadata cache is stale.
func GetPublicPricingVendors(catalog []PublicPricingModel) []PricingVendor {
	byName := make(map[string]PricingVendor)
	for _, item := range catalog {
		if item.VendorName == "" {
			continue
		}
		vendor := PricingVendor{ID: item.VendorID, Name: item.VendorName, Icon: item.VendorIcon, Description: item.VendorDescription}
		if previous, ok := byName[vendor.Name]; !ok || previous.ID == 0 {
			byName[vendor.Name] = vendor
		}
	}
	result := make([]PricingVendor, 0, len(byName))
	for _, vendor := range byName {
		result = append(result, vendor)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
