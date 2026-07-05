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
	var groups []*PricingGroup
	if err := common.Unmarshal([]byte(jsonStr), &groups); err == nil {
		return UpdatePricingGroupsByJSONString(jsonStr)
	}
	return updatePricingGroupsFromLegacyRatioJSON(jsonStr)
}

func NormalizeGroupRatioJSONStringForSave(jsonStr string) (string, error) {
	var groups []*PricingGroup
	if err := common.Unmarshal([]byte(jsonStr), &groups); err == nil {
		return NormalizePricingGroupsJSONString(jsonStr)
	}

	legacy := make(map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &legacy); err != nil {
		return "", err
	}
	normalizedRatios := make(map[string]float64, len(legacy))
	for name, ratio := range legacy {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := normalizedRatios[name]; exists {
			return "", errors.New("pricing group name must be unique: " + name)
		}
		if ratio < 0 {
			return "", errors.New("group ratio must be not less than 0: " + name)
		}
		normalizedRatios[name] = ratio
	}

	groups = buildPricingGroupsFromLegacyRatioWithIDs(normalizedRatios, existingPricingGroupIDsByName())
	return pricingGroupsToJSONString(groups)
}

func NormalizeGroupRatioJSONStringForSaveIfInitialized(jsonStr string) (string, bool, error) {
	if !pricingGroupsInitialized() {
		return "", false, nil
	}
	value, err := NormalizeGroupRatioJSONStringForSave(jsonStr)
	return value, true, err
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

func normalizeGroupGroupRatioMap(parsed map[string]map[string]float64) map[string]map[string]float64 {
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
	return normalized
}

func trimGroupGroupRatioMap(parsed map[string]map[string]float64) map[string]map[string]float64 {
	trimmed := make(map[string]map[string]float64, len(parsed))
	for userGroup, ratios := range parsed {
		userGroup = strings.TrimSpace(userGroup)
		if userGroup == "" {
			continue
		}
		trimmedRatios := make(map[string]float64, len(ratios))
		for pricingGroup, ratio := range ratios {
			key := strings.TrimSpace(pricingGroup)
			if key == "" {
				continue
			}
			trimmedRatios[key] = ratio
		}
		trimmed[userGroup] = trimmedRatios
	}
	return trimmed
}

func NormalizeGroupGroupRatioJSONString(jsonStr string) (string, error) {
	parsed := make(map[string]map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return "", err
	}
	bytes, err := common.Marshal(normalizeGroupGroupRatioMap(parsed))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	parsed := make(map[string]map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return err
	}
	normalized := trimGroupGroupRatioMap(parsed)
	if pricingGroupsInitialized() {
		normalized = normalizeGroupGroupRatioMap(parsed)
	}
	groupGroupRatioMap.Clear()
	groupGroupRatioMap.AddAll(normalized)
	return nil
}

func NormalizeGroupGroupRatio() (string, error) {
	value, err := NormalizeGroupGroupRatioJSONString(GroupGroupRatio2JSONString())
	if err != nil {
		return "", err
	}
	if err := UpdateGroupGroupRatioByJSONString(value); err != nil {
		return "", err
	}
	return value, nil
}

func NormalizeGroupGroupRatioJSONStringIfInitialized(jsonStr string) (string, bool, error) {
	if !pricingGroupsInitialized() {
		return "", false, nil
	}
	value, err := NormalizeGroupGroupRatioJSONString(jsonStr)
	return value, true, err
}

func normalizeSpecialUsablePricingGroupKey(rawKey string) string {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return ""
	}
	if strings.HasPrefix(rawKey, "-:") {
		key := normalizePricingGroupKey(strings.TrimPrefix(rawKey, "-:"))
		if key == "" {
			return ""
		}
		return "-:" + key
	}
	if strings.HasPrefix(rawKey, "+:") {
		key := normalizePricingGroupKey(strings.TrimPrefix(rawKey, "+:"))
		if key == "" {
			return ""
		}
		return "+:" + key
	}
	return normalizePricingGroupKey(rawKey)
}

func NormalizeGroupSpecialUsableGroupJSONString(jsonStr string) (string, error) {
	parsed := make(map[string]map[string]string)
	if err := common.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return "", err
	}
	normalized := make(map[string]map[string]string, len(parsed))
	for userGroup, rules := range parsed {
		userGroup = strings.TrimSpace(userGroup)
		if userGroup == "" {
			continue
		}
		normalizedRules := make(map[string]string, len(rules))
		for pricingGroup, desc := range rules {
			key := normalizeSpecialUsablePricingGroupKey(pricingGroup)
			if key == "" {
				continue
			}
			normalizedRules[key] = desc
		}
		normalized[userGroup] = normalizedRules
	}
	bytes, err := common.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func UpdateGroupSpecialUsableGroupByJSONString(jsonStr string) error {
	parsed := make(map[string]map[string]string)
	if err := common.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return err
	}
	normalized := parsed
	if pricingGroupsInitialized() {
		normalizedJSON, err := NormalizeGroupSpecialUsableGroupJSONString(jsonStr)
		if err != nil {
			return err
		}
		normalized = make(map[string]map[string]string)
		if err := common.Unmarshal([]byte(normalizedJSON), &normalized); err != nil {
			return err
		}
	}
	setting := GetGroupRatioSetting()
	setting.GroupSpecialUsableGroup.Clear()
	setting.GroupSpecialUsableGroup.AddAll(normalized)
	return nil
}

func GroupSpecialUsableGroup2JSONString() string {
	bytes, err := common.Marshal(GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll())
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func NormalizeGroupSpecialUsableGroup() (string, error) {
	value, err := NormalizeGroupSpecialUsableGroupJSONString(GroupSpecialUsableGroup2JSONString())
	if err != nil {
		return "", err
	}
	if err := UpdateGroupSpecialUsableGroupByJSONString(value); err != nil {
		return "", err
	}
	return value, nil
}

func NormalizeGroupSpecialUsableGroupJSONStringIfInitialized(jsonStr string) (string, bool, error) {
	if !pricingGroupsInitialized() {
		return "", false, nil
	}
	value, err := NormalizeGroupSpecialUsableGroupJSONString(jsonStr)
	return value, true, err
}

func CheckGroupRatio(jsonStr string) error {
	return ValidatePricingGroupsJSONString(jsonStr)
}

func normalizePricingGroupKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if group, ok := ResolvePricingGroupKey(trimmed); ok {
		return strconv.Itoa(group.Id)
	}
	return trimmed
}
