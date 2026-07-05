package ratio_setting

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

type PricingGroup struct {
	Id          int     `json:"id"`
	Name        string  `json:"name"`
	Ratio       float64 `json:"ratio"`
	Selectable  bool    `json:"selectable"`
	Description string  `json:"description,omitempty"`
}

type PricingGroupRef struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

var pricingGroupsMutex sync.RWMutex
var pricingGroups []*PricingGroup

var defaultPricingGroups = []*PricingGroup{
	{Id: 1, Name: "default", Ratio: 1, Selectable: true, Description: "默认分组"},
	{Id: 2, Name: "vip", Ratio: 1, Selectable: true, Description: "vip分组"},
	{Id: 3, Name: "svip", Ratio: 1, Selectable: false, Description: ""},
}

func clonePricingGroups(groups []*PricingGroup) []*PricingGroup {
	cloned := make([]*PricingGroup, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		item := *group
		item.Name = strings.TrimSpace(item.Name)
		item.Description = strings.TrimSpace(item.Description)
		cloned = append(cloned, &item)
	}
	return cloned
}

func preferredPricingGroupOrder(name string) int {
	switch strings.TrimSpace(name) {
	case "default":
		return 0
	case "vip":
		return 1
	case "svip":
		return 2
	default:
		return 3
	}
}

func normalizePricingGroups(groups []*PricingGroup) ([]*PricingGroup, error) {
	cleaned := make([]*PricingGroup, 0, len(groups))
	seenIDs := make(map[int]struct{})
	seenNames := make(map[string]struct{})
	nextID := 1

	for _, group := range groups {
		if group == nil {
			continue
		}
		name := strings.TrimSpace(group.Name)
		if name == "" {
			continue
		}
		if name == "auto" {
			return nil, errors.New("pricing group name is reserved: " + name)
		}
		if _, err := strconv.Atoi(name); err == nil {
			return nil, errors.New("pricing group name must not be numeric: " + name)
		}
		if _, exists := seenNames[name]; exists {
			return nil, errors.New("pricing group name must be unique: " + name)
		}
		seenNames[name] = struct{}{}
		item := &PricingGroup{
			Id:          group.Id,
			Name:        name,
			Ratio:       group.Ratio,
			Selectable:  group.Selectable,
			Description: strings.TrimSpace(group.Description),
		}
		if item.Id > 0 {
			if _, exists := seenIDs[item.Id]; exists {
				return nil, errors.New("pricing group id must be unique: " + strconv.Itoa(item.Id))
			} else {
				seenIDs[item.Id] = struct{}{}
				if item.Id >= nextID {
					nextID = item.Id + 1
				}
			}
		}
		cleaned = append(cleaned, item)
	}

	if len(cleaned) == 0 {
		return nil, errors.New("pricing groups cannot be empty")
	}

	sort.SliceStable(cleaned, func(i, j int) bool {
		ai := preferredPricingGroupOrder(cleaned[i].Name)
		aj := preferredPricingGroupOrder(cleaned[j].Name)
		if ai != aj {
			return ai < aj
		}
		if cleaned[i].Id > 0 && cleaned[j].Id > 0 {
			return cleaned[i].Id < cleaned[j].Id
		}
		return cleaned[i].Name < cleaned[j].Name
	})

	assignID := func() int {
		for {
			if _, exists := seenIDs[nextID]; !exists {
				id := nextID
				seenIDs[id] = struct{}{}
				nextID++
				return id
			}
			nextID++
		}
	}

	for _, group := range cleaned {
		if group.Name == "default" {
			if group.Id != 0 && group.Id != 1 {
				return nil, errors.New("default pricing group must keep id 1")
			}
			if _, exists := seenIDs[1]; exists && group.Id != 1 {
				return nil, errors.New("default pricing group id 1 is already used")
			}
			if group.Id == 0 {
				group.Id = 1
				seenIDs[1] = struct{}{}
				if nextID <= 1 {
					nextID = 2
				}
			}
		}
	}

	hasDefault := false
	for _, group := range cleaned {
		if group.Id == 1 {
			hasDefault = true
		}
		if group.Id == 0 {
			group.Id = assignID()
		}
	}
	if !hasDefault {
		return nil, errors.New("default pricing group cannot be deleted")
	}

	sort.SliceStable(cleaned, func(i, j int) bool {
		return cleaned[i].Id < cleaned[j].Id
	})

	return cleaned, nil
}

func defaultPricingGroupsCopy() []*PricingGroup {
	return clonePricingGroups(defaultPricingGroups)
}

func buildPricingGroupsFromLegacy() []*PricingGroup {
	return buildPricingGroupsFromLegacyWithIDs(existingPricingGroupIDsByName())
}

func buildPricingGroupsFromLegacyWithIDs(existingIDs map[string]int) []*PricingGroup {
	return buildPricingGroupsFromLegacyRatioWithIDs(GetLegacyGroupRatioCopy(), existingIDs)
}

func buildPricingGroupsFromLegacyRatioWithIDs(legacyRatios map[string]float64, existingIDs map[string]int) []*PricingGroup {
	legacyUsable := setting.GetUserUsableGroupsCopy()
	legacyAuto := setting.GetAutoGroups()
	legacyGroupRatios := groupGroupRatioMap.ReadAll()
	legacySpecial := GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()

	existingNameByKey := make(map[string]string, len(defaultPricingGroups)*2+len(existingIDs)*2)
	addExistingName := func(name string, id int) {
		name = strings.TrimSpace(name)
		if name == "" || id <= 0 {
			return
		}
		existingNameByKey[name] = name
		existingNameByKey[strconv.Itoa(id)] = name
	}
	for _, group := range defaultPricingGroups {
		if group == nil {
			continue
		}
		addExistingName(group.Name, group.Id)
	}
	for name, id := range existingIDs {
		addExistingName(name, id)
	}
	normalizeLegacyName := func(name string) string {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return ""
		}
		if existingName, ok := existingNameByKey[trimmed]; ok {
			return existingName
		}
		if trimmed == "auto" {
			return ""
		}
		if _, err := strconv.Atoi(trimmed); err == nil {
			return ""
		}
		return trimmed
	}

	normalizedLegacyRatios := make(map[string]float64, len(legacyRatios))
	for name, ratio := range legacyRatios {
		name = normalizeLegacyName(name)
		if name == "" {
			continue
		}
		normalizedLegacyRatios[name] = ratio
	}
	legacyRatios = normalizedLegacyRatios

	normalizedLegacyUsable := make(map[string]string, len(legacyUsable))
	for name, desc := range legacyUsable {
		name = normalizeLegacyName(name)
		if name == "" {
			continue
		}
		normalizedLegacyUsable[name] = desc
	}
	legacyUsable = normalizedLegacyUsable

	nameSet := make(map[string]struct{})
	addName := func(name string) {
		name = normalizeLegacyName(name)
		if name == "" {
			return
		}
		nameSet[name] = struct{}{}
	}

	for name := range legacyRatios {
		addName(name)
	}
	for name := range legacyUsable {
		addName(name)
	}
	for _, name := range legacyAuto {
		addName(name)
	}
	for _, overrides := range legacyGroupRatios {
		for name := range overrides {
			addName(name)
		}
	}
	for _, overrides := range legacySpecial {
		for rawName := range overrides {
			name := strings.TrimPrefix(strings.TrimPrefix(rawName, "+:"), "-:")
			addName(name)
		}
	}

	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		ai := preferredPricingGroupOrder(names[i])
		aj := preferredPricingGroupOrder(names[j])
		if ai != aj {
			return ai < aj
		}
		return names[i] < names[j]
	})

	groups := make([]*PricingGroup, 0, len(names))
	for _, name := range names {
		id := 0
		if existingID, ok := existingIDs[name]; ok {
			id = existingID
		}
		ratio, ok := legacyRatios[name]
		if !ok {
			ratio = 1
		}
		selectable := false
		if desc, ok := legacyUsable[name]; ok {
			selectable = true
			_ = desc
		}
		groups = append(groups, &PricingGroup{
			Id:          id,
			Name:        name,
			Ratio:       ratio,
			Selectable:  selectable,
			Description: legacyUsable[name],
		})
	}
	normalized, err := normalizePricingGroups(groups)
	if err != nil {
		return defaultPricingGroupsCopy()
	}
	return normalized
}

func existingPricingGroupIDsByName() map[string]int {
	pricingGroupsMutex.RLock()
	defer pricingGroupsMutex.RUnlock()
	ids := make(map[string]int, len(pricingGroups))
	for _, group := range pricingGroups {
		if group == nil {
			continue
		}
		ids[group.Name] = group.Id
	}
	return ids
}

func existingPricingGroupIDByName(name string) (int, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return 0, false
	}
	pricingGroupsMutex.RLock()
	defer pricingGroupsMutex.RUnlock()
	for _, group := range pricingGroups {
		if group == nil {
			continue
		}
		if group.Name == trimmed {
			return group.Id, true
		}
	}
	return 0, false
}

func ensurePricingGroupsInitialized() {
	pricingGroupsMutex.RLock()
	initialized := len(pricingGroups) > 0
	pricingGroupsMutex.RUnlock()
	if initialized {
		return
	}

	pricingGroupsMutex.Lock()
	defer pricingGroupsMutex.Unlock()
	if len(pricingGroups) > 0 {
		return
	}
	legacyGroups := buildPricingGroupsFromLegacyWithIDs(nil)
	if len(legacyGroups) == 0 {
		pricingGroups = defaultPricingGroupsCopy()
		return
	}
	pricingGroups = legacyGroups
	syncGroupRatioMapLocked(pricingGroups)
}

func GetPricingGroupsCopy() []*PricingGroup {
	ensurePricingGroupsInitialized()
	pricingGroupsMutex.RLock()
	defer pricingGroupsMutex.RUnlock()
	if len(pricingGroups) == 0 {
		return defaultPricingGroupsCopy()
	}
	return clonePricingGroups(pricingGroups)
}

func setPricingGroups(groups []*PricingGroup) error {
	normalized, err := normalizePricingGroups(groups)
	if err != nil {
		return err
	}
	pricingGroupsMutex.Lock()
	defer pricingGroupsMutex.Unlock()
	pricingGroups = normalized
	syncGroupRatioMapLocked(pricingGroups)
	return nil
}

func syncGroupRatioMapLocked(groups []*PricingGroup) {
	groupRatioMap.Clear()
	for _, group := range groups {
		if group == nil {
			continue
		}
		groupRatioMap.Set(strconv.Itoa(group.Id), group.Ratio)
	}
}

func PricingGroups2JSONString() string {
	value, err := pricingGroupsToJSONString(GetPricingGroupsCopy())
	if err != nil {
		return "[]"
	}
	return value
}

func pricingGroupsToJSONString(groups []*PricingGroup) (string, error) {
	bytes, err := common.Marshal(groups)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func parsePricingGroupsJSONString(jsonStr string) ([]*PricingGroup, error) {
	var groups []*PricingGroup
	if err := common.Unmarshal([]byte(jsonStr), &groups); err != nil {
		var legacy map[string]float64
		if legacyErr := common.Unmarshal([]byte(jsonStr), &legacy); legacyErr != nil {
			return nil, err
		}
		names := make([]string, 0, len(legacy))
		for name := range legacy {
			names = append(names, name)
		}
		sort.Strings(names)
		groups = make([]*PricingGroup, 0, len(legacy))
		for _, name := range names {
			ratio := legacy[name]
			id := 0
			if existingID, ok := existingPricingGroupIDByName(name); ok {
				id = existingID
			}
			groups = append(groups, &PricingGroup{
				Id:    id,
				Name:  strings.TrimSpace(name),
				Ratio: ratio,
			})
		}
	} else {
		for _, group := range groups {
			if group == nil || group.Id != 0 {
				continue
			}
			if existingID, ok := existingPricingGroupIDByName(group.Name); ok {
				group.Id = existingID
			}
		}
	}
	return normalizePricingGroups(groups)
}

func ValidatePricingGroupsJSONString(jsonStr string) error {
	groups, err := parsePricingGroupsJSONString(jsonStr)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group.Ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + group.Name)
		}
	}
	return nil
}

func NormalizePricingGroupsJSONString(jsonStr string) (string, error) {
	groups, err := parsePricingGroupsJSONString(jsonStr)
	if err != nil {
		return "", err
	}
	for _, group := range groups {
		if group.Ratio < 0 {
			return "", errors.New("group ratio must be not less than 0: " + group.Name)
		}
	}
	return pricingGroupsToJSONString(groups)
}

func NormalizePricingGroupsJSONStringIfInitialized(jsonStr string) (string, bool, error) {
	if !pricingGroupsInitialized() {
		return "", false, nil
	}
	value, err := NormalizePricingGroupsJSONString(jsonStr)
	return value, true, err
}

func ValidatePricingGroupIDStabilityJSONString(jsonStr string) error {
	var groups []*PricingGroup
	if err := common.Unmarshal([]byte(jsonStr), &groups); err != nil {
		return nil
	}
	normalized, err := parsePricingGroupsJSONString(jsonStr)
	if err != nil {
		return err
	}
	current := GetPricingGroupsCopy()
	idsByName := make(map[string]int, len(current))
	ids := make(map[int]struct{}, len(current))
	for _, group := range current {
		if group == nil {
			continue
		}
		idsByName[group.Name] = group.Id
		ids[group.Id] = struct{}{}
	}
	seenExistingIDs := make(map[int]struct{}, len(normalized))
	newIDCount := 0
	for _, group := range normalized {
		if group == nil {
			continue
		}
		if id, ok := idsByName[group.Name]; ok && group.Id != id {
			return errors.New("pricing group id cannot be changed for name: " + group.Name)
		}
		if _, ok := ids[group.Id]; ok {
			seenExistingIDs[group.Id] = struct{}{}
		} else if group.Id > 0 {
			newIDCount++
		}
	}
	if newIDCount == 0 {
		return nil
	}
	for _, group := range current {
		if group == nil {
			continue
		}
		if _, ok := seenExistingIDs[group.Id]; !ok {
			return errors.New("pricing group id cannot be changed for name: " + group.Name)
		}
	}
	return nil
}

func updatePricingGroupsFromLegacyRatioJSON(jsonStr string) error {
	legacy := make(map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &legacy); err != nil {
		return err
	}
	for name, ratio := range legacy {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	groupRatioMap.Clear()
	groupRatioMap.AddAll(legacy)
	return setPricingGroups(buildPricingGroupsFromLegacy())
}

func UpdatePricingGroupsByJSONString(jsonStr string) error {
	normalized, err := parsePricingGroupsJSONString(jsonStr)
	if err != nil {
		return err
	}
	return setPricingGroups(normalized)
}

func ResolvePricingGroupKey(key string) (PricingGroup, bool) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return PricingGroup{}, false
	}
	if trimmed == "default" {
		return GetPricingGroupByID(1)
	}
	if id, err := strconv.Atoi(trimmed); err == nil {
		return GetPricingGroupByID(id)
	}
	return GetPricingGroupByName(trimmed)
}

func GetPricingGroupByID(id int) (PricingGroup, bool) {
	for _, group := range GetPricingGroupsCopy() {
		if group.Id == id {
			return *group, true
		}
	}
	return PricingGroup{}, false
}

func GetPricingGroupByName(name string) (PricingGroup, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return PricingGroup{}, false
	}
	for _, group := range GetPricingGroupsCopy() {
		if group.Name == trimmed {
			return *group, true
		}
	}
	return PricingGroup{}, false
}

func PricingGroupKey(name string) string {
	group, ok := ResolvePricingGroupKey(name)
	if !ok {
		return strings.TrimSpace(name)
	}
	return strconv.Itoa(group.Id)
}

func PricingGroupKeyOrDefault(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "auto" {
		return trimmed
	}
	if group, ok := ResolvePricingGroupKey(trimmed); ok {
		return strconv.Itoa(group.Id)
	}
	if group, ok := GetPricingGroupByID(1); ok {
		return strconv.Itoa(group.Id)
	}
	return "1"
}

func PricingGroupKeyByNameOrDefault(name string) string {
	if group, ok := GetPricingGroupByName(name); ok {
		return strconv.Itoa(group.Id)
	}
	if group, ok := GetPricingGroupByID(1); ok {
		return strconv.Itoa(group.Id)
	}
	return "1"
}

func PricingGroupKeysCSV(value string) string {
	parts := strings.Split(strings.Trim(value, ","), ",")
	normalized := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		key := PricingGroupKey(part)
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	return strings.Join(normalized, ",")
}

func NormalizePricingGroupKeys(groups []string) []string {
	normalized := make([]string, 0, len(groups))
	for _, group := range groups {
		key := PricingGroupKey(group)
		if strings.TrimSpace(key) == "" {
			continue
		}
		normalized = append(normalized, key)
	}
	return normalized
}

func NormalizeUserUsableGroupsJSONString(jsonStr string) (string, error) {
	parsed := make(map[string]string)
	if err := common.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return "", err
	}
	normalized := make(map[string]string, len(parsed))
	for group, desc := range parsed {
		key := PricingGroupKey(group)
		if strings.TrimSpace(key) == "" {
			continue
		}
		normalized[key] = desc
	}
	bytes, err := common.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func NormalizeUserUsableGroups() (string, error) {
	value, err := NormalizeUserUsableGroupsJSONString(setting.UserUsableGroups2JSONString())
	if err != nil {
		return "", err
	}
	if err := setting.UpdateUserUsableGroupsByJSONString(value); err != nil {
		return "", err
	}
	return value, nil
}

func NormalizeUserUsableGroupsJSONStringIfInitialized(jsonStr string) (string, bool, error) {
	if !pricingGroupsInitialized() {
		return "", false, nil
	}
	value, err := NormalizeUserUsableGroupsJSONString(jsonStr)
	return value, true, err
}

func pricingGroupsInitialized() bool {
	pricingGroupsMutex.RLock()
	defer pricingGroupsMutex.RUnlock()
	return len(pricingGroups) > 0
}

func normalizeAutoGroupValues(groups []string) (string, error) {
	normalized := NormalizePricingGroupKeys(groups)
	bytes, err := common.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func NormalizeAutoGroupsJSONString(jsonStr string) (string, error) {
	var groups []string
	if err := common.Unmarshal([]byte(jsonStr), &groups); err != nil {
		return "", err
	}
	return normalizeAutoGroupValues(groups)
}

func NormalizeAutoGroupsJSONStringIfInitialized(jsonStr string) (string, bool, error) {
	if !pricingGroupsInitialized() {
		return "", false, nil
	}
	value, err := NormalizeAutoGroupsJSONString(jsonStr)
	return value, true, err
}

func NormalizeAutoGroups() (string, error) {
	value, err := normalizeAutoGroupValues(setting.GetAutoGroups())
	if err != nil {
		return "", err
	}
	if err := setting.UpdateAutoGroupsByJsonString(value); err != nil {
		return "", err
	}
	return value, nil
}

func NormalizeAutoGroupsIfInitialized() (string, bool, error) {
	if !pricingGroupsInitialized() {
		return "", false, nil
	}
	value, err := NormalizeAutoGroups()
	return value, true, err
}

func ContainsPricingGroup(name string) bool {
	_, ok := ResolvePricingGroupKey(name)
	return ok
}

func PricingGroupNameByKey(key string) string {
	group, ok := ResolvePricingGroupKey(key)
	if !ok {
		return strings.TrimSpace(key)
	}
	return group.Name
}

func PricingGroupIDByName(name string) (int, bool) {
	group, ok := GetPricingGroupByName(name)
	if !ok {
		return 0, false
	}
	return group.Id, true
}

func PricingGroupRefByKey(key string) (PricingGroupRef, bool) {
	group, ok := ResolvePricingGroupKey(key)
	if !ok {
		return PricingGroupRef{}, false
	}
	return PricingGroupRef{Id: group.Id, Name: group.Name}, true
}

func PricingGroupRefsByKeys(keys []string) []PricingGroupRef {
	refs := make([]PricingGroupRef, 0, len(keys))
	seen := make(map[int]struct{}, len(keys))
	for _, key := range keys {
		ref, ok := PricingGroupRefByKey(key)
		if !ok {
			continue
		}
		if _, exists := seen[ref.Id]; exists {
			continue
		}
		seen[ref.Id] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}

func PricingGroupNameMap() map[string]string {
	groups := GetPricingGroupsCopy()
	names := make(map[string]string, len(groups))
	for _, group := range groups {
		names[strconv.Itoa(group.Id)] = group.Name
	}
	return names
}

func ResetPricingGroupsForTest() {
	pricingGroupsMutex.Lock()
	defer pricingGroupsMutex.Unlock()
	pricingGroups = nil
	groupRatioMap.Clear()
	groupRatioMap.AddAll(defaultGroupRatio)
}
