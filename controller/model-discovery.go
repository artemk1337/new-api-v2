package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

type modelListPricing struct {
	supplierModels  map[string]bool
	defaultModels   map[string]bool
	hasDefaultGroup bool
}

// Discovery follows the same group/profile boundary as dispatch. Legacy token
// allowlists may list a globally priced model before a channel is configured.
func (pricing modelListPricing) available(name string, legacyAllowlist bool) bool {
	if pricing.supplierModels[name] {
		return true
	}
	return (pricing.defaultModels[name] || legacyAllowlist && pricing.hasDefaultGroup) && helper.HasModelBillingConfig(name)
}

func getModelListPricing(groups []string) (modelListPricing, error) {
	pricing := modelListPricing{supplierModels: make(map[string]bool), defaultModels: make(map[string]bool)}
	supplierGroups := make(map[string]string)
	multipliers := make(map[string]float64)
	for _, group := range groups {
		binding, err := model.GetPricingGroupSupplierBinding(group)
		if err != nil {
			return pricing, err
		}
		if binding.SupplierID == 0 {
			pricing.hasDefaultGroup = true
			for _, name := range model.GetGroupEnabledModels(group) {
				pricing.defaultModels[name] = true
			}
			continue
		}
		key := ratio_setting.PricingGroupKey(group)
		supplierGroups[key], multipliers[key] = "", 1
	}
	if len(supplierGroups) == 0 {
		return pricing, nil
	}
	catalogue, err := model.GetPublicPricingCatalog(supplierGroups, multipliers)
	if err != nil {
		return pricing, err
	}
	for _, item := range catalogue {
		for _, price := range item.GroupPrices {
			if !price.Unavailable {
				pricing.supplierModels[item.ModelName] = true
				break
			}
		}
	}
	return pricing, nil
}
