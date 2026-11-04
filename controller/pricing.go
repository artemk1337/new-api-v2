package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		groups := make([]string, 0, len(item.EnableGroup))
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				groups = append(groups, group)
			}
		}
		if common.StringsContains(item.EnableGroup, "all") {
			for group := range usableGroup {
				groups = append(groups, group)
			}
		}
		if len(groups) == 0 {
			continue
		}
		item.EnableGroup = groups
		item.EnableGroupRefs = ratio_setting.PricingGroupRefsByKeys(groups)
		filtered = append(filtered, item)
	}
	return filtered
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	// The legacy API can represent only the default pricing scope.
	for key := range usableGroup {
		binding, err := model.GetPricingGroupSupplierBinding(key)
		if err != nil || binding.SupplierID != 0 {
			delete(usableGroup, key)
		}
	}
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"pricing_groups":     publicPricingGroups(usableGroup, true),
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

type publicPricingGroup struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Selectable  bool     `json:"selectable"`
	Description string   `json:"description,omitempty"`
	Ratio       *float64 `json:"ratio,omitempty"`
}

func publicPricingGroups(usable map[string]string, includeRatios bool) []publicPricingGroup {
	groups := make([]publicPricingGroup, 0)
	for _, group := range ratio_setting.GetPricingGroupsCopy() {
		if _, ok := usable[ratio_setting.PricingGroupKey(group.Name)]; !ok {
			continue
		}
		item := publicPricingGroup{ID: group.Id, Name: group.Name, Selectable: group.Selectable, Description: group.Description}
		if includeRatios {
			ratio := group.Ratio
			item.Ratio = &ratio
		}
		groups = append(groups, item)
	}
	return groups
}

// GetPricingV2 returns complete retail contracts with an explicit public DTO.
func GetPricingV2(c *gin.Context) {
	userGroup := ""
	if userID, exists := c.Get("id"); exists {
		user, err := model.GetUserCache(userID.(int))
		if err != nil {
			common.ApiError(c, err)
			return
		}
		userGroup = user.Group
	}
	usable := service.GetUserUsableGroups(userGroup)
	multipliers := make(map[string]float64, len(usable))
	neutral := make(map[string]float64, len(usable))
	for group := range usable {
		multiplier := ratio_setting.GetGroupRatio(group)
		if override, ok := ratio_setting.GetGroupGroupRatio(userGroup, group); ok {
			multiplier = override
		}
		multipliers[group], neutral[group] = multiplier, 1
	}
	// Keep shared developer metadata and custom endpoint definitions initialized.
	model.GetPricing()
	catalog, err := model.GetPublicPricingCatalog(usable, multipliers)
	if err != nil {
		c.JSON(503, gin.H{"success": false, "message": "Pricing is temporarily unavailable"})
		return
	}
	c.JSON(200, gin.H{
		"success": true, "data": catalog, "vendors": model.GetVendors(),
		"group_ratio": neutral, "usable_group": usable, "pricing_groups": publicPricingGroups(usable, false),
		"supported_endpoint": model.GetSupportedEndpointMap(), "auto_groups": service.GetUserAutoGroup(userGroup),
		"pricing_version": "2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}
