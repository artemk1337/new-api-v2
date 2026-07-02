package service

import (
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	normalizedGroups := make(map[string]string, len(groupsCopy))
	for groupName, desc := range groupsCopy {
		normalizedGroups[ratio_setting.PricingGroupKey(groupName)] = desc
	}
	groupsCopy = normalizedGroups
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					// 移除分组
					groupToRemove := ratio_setting.PricingGroupKey(strings.TrimPrefix(specialGroup, "-:"))
					delete(groupsCopy, groupToRemove)
				} else if strings.HasPrefix(specialGroup, "+:") {
					// 添加分组
					groupToAdd := ratio_setting.PricingGroupKey(strings.TrimPrefix(specialGroup, "+:"))
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[ratio_setting.PricingGroupKey(specialGroup)] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
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
