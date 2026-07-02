package ratio_setting

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{
	"vip": {
		"append_1":   "vip_special_group_1",
		"-:remove_1": "vip_removed_group_1",
	},
}

type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]            `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]  `json:"group_special_usable_group"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	return &groupRatioSetting
}

func GetGroupRatioCopy() map[string]float64 {
	groups := GetPricingGroupsCopy()
	groupRatios := make(map[string]float64, len(groups))
	for _, group := range groups {
		groupRatios[normalizePricingGroupKey(group.Name)] = group.Ratio
	}
	return groupRatios
}

func GetLegacyGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func ContainsGroupRatio(name string) bool {
	return ContainsPricingGroup(name)
}

func GroupRatio2JSONString() string {
	return PricingGroups2JSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return UpdatePricingGroupsByJSONString(jsonStr)
}

func GetGroupRatio(name string) float64 {
	key := normalizePricingGroupKey(name)
	ratio, ok := groupRatioMap.Get(key)
	if !ok {
		if group, groupOk := ResolvePricingGroupKey(name); groupOk {
			return group.Ratio
		}
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	return ratio
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	usingGroup = normalizePricingGroupKey(usingGroup)
	if groupName := PricingGroupNameByKey(usingGroup); groupName != "" {
		if ratio, ok := gp[groupName]; ok {
			return ratio, true
		}
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	parsed := make(map[string]map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return err
	}
	normalized := make(map[string]map[string]float64, len(parsed))
	for userGroup, ratios := range parsed {
		userGroup = strings.TrimSpace(userGroup)
		if userGroup == "" {
			continue
		}
		normalizedRatios := make(map[string]float64, len(ratios))
		for pricingGroup, ratio := range ratios {
			key := normalizePricingGroupKey(pricingGroup)
			if key == "" {
				continue
			}
			normalizedRatios[key] = ratio
		}
		normalized[userGroup] = normalizedRatios
	}
	groupGroupRatioMap.Clear()
	groupGroupRatioMap.AddAll(normalized)
	return nil
}

func CheckGroupRatio(jsonStr string) error {
	var groups []*PricingGroup
	if err := common.Unmarshal([]byte(jsonStr), &groups); err == nil {
		for _, group := range groups {
			if group == nil {
				continue
			}
			if group.Ratio < 0 {
				return errors.New("group ratio must be not less than 0: " + group.Name)
			}
		}
		return nil
	}

	checkGroupRatio := make(map[string]float64)
	err := common.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}

func normalizePricingGroupKey(key string) string {
	trimmed := key
	if group, ok := ResolvePricingGroupKey(trimmed); ok {
		return strconv.Itoa(group.Id)
	}
	return trimmed
}
