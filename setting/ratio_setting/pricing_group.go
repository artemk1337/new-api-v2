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

func normalizePricingGroups(groups []*PricingGroup) []*PricingGroup {
	cleaned := make([]*PricingGroup, 0, len(groups))
	seenIDs := make(map[int]struct{})
	nextID := 1

	for _, group := range groups {
		if group == nil {
			continue
		}
		name := strings.TrimSpace(group.Name)
		if name == "" {
			continue
		}
		item := &PricingGroup{
			Id:          group.Id,
			Name:        name,
			Ratio:       group.Ratio,
			Selectable:  group.Selectable,
			Description: strings.TrimSpace(group.Description),
		}
		if item.Id > 0 {
			if _, exists := seenIDs[item.Id]; exists {
				item.Id = 0
			} else {
				seenIDs[item.Id] = struct{}{}
				if item.Id >= nextID {
					nextID = item.Id + 1
				}
			}
		}
		cleaned = append(cleaned, item)
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
		if group.Id > 0 {
			continue
		}
		if group.Name == "default" && nextID <= 1 {
			group.Id = 1
			seenIDs[1] = struct{}{}
			if nextID <= 1 {
				nextID = 2
			}
			continue
		}
		group.Id = assignID()
	}

	sort.SliceStable(cleaned, func(i, j int) bool {
		return cleaned[i].Id < cleaned[j].Id
	})

	return cleaned
}

func defaultPricingGroupsCopy() []*PricingGroup {
	return clonePricingGroups(defaultPricingGroups)
}

func buildPricingGroupsFromLegacy() []*PricingGroup {
	legacyRatios := GetLegacyGroupRatioCopy()
	legacyUsable := setting.GetUserUsableGroupsCopy()
	legacyAuto := setting.GetAutoGroups()
	legacyGroupRatios := groupGroupRatioMap.ReadAll()
	legacySpecial := GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()

	nameSet := make(map[string]struct{})
	addName := func(name string) {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return
		}
		nameSet[trimmed] = struct{}{}
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
	nextID := 1
	for _, name := range names {
		id := nextID
		nextID++
		ratio := legacyRatios[name]
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
	return normalizePricingGroups(groups)
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
	legacyGroups := buildPricingGroupsFromLegacy()
	if len(legacyGroups) == 0 {
		pricingGroups = defaultPricingGroupsCopy()
		return
	}
	pricingGroups = legacyGroups
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

func setPricingGroups(groups []*PricingGroup) {
	pricingGroupsMutex.Lock()
	defer pricingGroupsMutex.Unlock()
	pricingGroups = normalizePricingGroups(groups)
}

func PricingGroups2JSONString() string {
	bytes, err := common.Marshal(GetPricingGroupsCopy())
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

func UpdatePricingGroupsByJSONString(jsonStr string) error {
	var groups []*PricingGroup
	if err := common.Unmarshal([]byte(jsonStr), &groups); err != nil {
		var legacy map[string]float64
		if legacyErr := common.Unmarshal([]byte(jsonStr), &legacy); legacyErr != nil {
			return err
		}
		names := make([]string, 0, len(legacy))
		for name := range legacy {
			names = append(names, name)
		}
		sort.Strings(names)
		groups = make([]*PricingGroup, 0, len(legacy))
		for _, name := range names {
			ratio := legacy[name]
			groups = append(groups, &PricingGroup{
				Name:  strings.TrimSpace(name),
				Ratio: ratio,
			})
		}
	}
	normalized := normalizePricingGroups(groups)
	hasDefault := false
	for _, group := range normalized {
		if group == nil {
			continue
		}
		if group.Id == 1 {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		return errors.New("default pricing group cannot be deleted")
	}
	setPricingGroups(normalized)
	return nil
}

func ResolvePricingGroupKey(key string) (PricingGroup, bool) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return PricingGroup{}, false
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

func NormalizePricingGroupKeys(groups []string) []string {
	normalized := make([]string, 0, len(groups))
	for _, group := range groups {
		normalized = append(normalized, PricingGroupKey(group))
	}
	return normalized
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
