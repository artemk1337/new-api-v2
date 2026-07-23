package service

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	pricingGroups := ratio_setting.GetPricingGroupsCopy()
	groupsCopy := make(map[string]string, len(pricingGroups))
	for _, group := range pricingGroups {
		if group.Selectable {
			groupsCopy[strconv.Itoa(group.Id)] = group.Description
		}
	}
	if userGroup != "" {
		// userGroup is a user-domain group. Add it only when it also exists as
		// a pricing group for legacy installations that intentionally overlap.
		// Explicit special rules below must have the final say.
		if pricingGroup, ok := ratio_setting.GetPricingGroupByName(userGroup); ok {
			userPricingGroup := strconv.Itoa(pricingGroup.Id)
			if _, ok := groupsCopy[userPricingGroup]; !ok {
				groupsCopy[userPricingGroup] = "用户分组"
			}
		}
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// Apply additions first because map iteration order is undefined.
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					continue
				}
				if strings.HasPrefix(specialGroup, "+:") {
					groupToAdd := ratio_setting.PricingGroupKey(strings.TrimPrefix(specialGroup, "+:"))
					groupsCopy[groupToAdd] = desc
				} else {
					groupsCopy[ratio_setting.PricingGroupKey(specialGroup)] = desc
				}
			}
			// Explicit removals always have the final say.
			for specialGroup := range specialSettings {
				if !strings.HasPrefix(specialGroup, "-:") {
					continue
				}
				groupToRemove := ratio_setting.PricingGroupKey(strings.TrimPrefix(specialGroup, "-:"))
				delete(groupsCopy, groupToRemove)
			}
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[ratio_setting.PricingGroupKey(groupName)]
	return ok
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	groups := GetUserUsableGroups(userGroup)
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		normalizedGroup := ratio_setting.PricingGroupKey(group)
		if _, ok := groups[normalizedGroup]; ok {
			autoGroups = append(autoGroups, normalizedGroup)
		}
	}
	return autoGroups
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}
