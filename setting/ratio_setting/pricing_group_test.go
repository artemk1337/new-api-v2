package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withTestPricingGroups(t *testing.T, groups []*PricingGroup) func() {
	t.Helper()

	prev := PricingGroups2JSONString()

	if groups == nil {
		groups = defaultPricingGroupsCopy()
	}
	require.NoError(t, setPricingGroups(groups))

	return func() {
		require.NoError(t, UpdatePricingGroupsByJSONString(prev))
	}
}

func TestPricingGroupsStableIDResolution(t *testing.T) {
	restore := withTestPricingGroups(t, nil)
	defer restore()

	require.NoError(t, UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1.2,"selectable":true,"description":"vip"}
	]`))

	group, ok := ResolvePricingGroupKey("vip")
	require.True(t, ok)
	assert.Equal(t, 2, group.Id)
	assert.Equal(t, "vip", group.Name)
	assert.Equal(t, "2", PricingGroupKey("vip"))
	assert.Equal(t, "vip", PricingGroupNameByKey("2"))
	assert.True(t, ContainsPricingGroup("2"))
	assert.True(t, ContainsPricingGroup("vip"))
	assert.InDelta(t, 1.2, GetGroupRatio("2"), 1e-9)
	assert.InDelta(t, 1.2, GetGroupRatio("vip"), 1e-9)
}

func TestPricingGroupKeyOrDefault(t *testing.T) {
	restore := withTestPricingGroups(t, nil)
	defer restore()

	require.NoError(t, UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"Renamed VIP","ratio":1.2,"selectable":true,"description":"vip"}
	]`))

	assert.Equal(t, "1", PricingGroupKeyOrDefault(""))
	assert.Equal(t, "1", PricingGroupKeyOrDefault("paid-users"))
	assert.Equal(t, "2", PricingGroupKeyOrDefault("Renamed VIP"))
	assert.Equal(t, "2", PricingGroupKeyOrDefault("2"))
	assert.Equal(t, "auto", PricingGroupKeyOrDefault("auto"))
}

func TestPricingGroupsRejectDefaultDeletion(t *testing.T) {
	restore := withTestPricingGroups(t, nil)
	defer restore()

	err := UpdatePricingGroupsByJSONString(`[
		{"id":2,"name":"vip","ratio":1,"selectable":true,"description":"vip"}
	]`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "default pricing group cannot be deleted")
}

func TestPricingGroupsRenameKeepsStableID(t *testing.T) {
	restore := withTestPricingGroups(t, nil)
	defer restore()

	require.NoError(t, UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1.1,"selectable":true,"description":"vip"}
	]`))

	require.NoError(t, UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"legacy_vip","ratio":1.1,"selectable":true,"description":"legacy"}
	]`))

	group, ok := ResolvePricingGroupKey("2")
	require.True(t, ok)
	assert.Equal(t, "legacy_vip", group.Name)
	assert.Equal(t, 2, group.Id)
	assert.Equal(t, "2", PricingGroupKey("legacy_vip"))
}

func TestPricingGroupsRejectDuplicateNames(t *testing.T) {
	restore := withTestPricingGroups(t, nil)
	defer restore()

	err := UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1,"selectable":true,"description":"vip"},
		{"id":3,"name":" vip ","ratio":1,"selectable":true,"description":"duplicate"}
	]`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "pricing group name must be unique")
}

func TestPricingGroupsRejectReservedAutoName(t *testing.T) {
	restore := withTestPricingGroups(t, nil)
	defer restore()

	err := UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"auto","ratio":1,"selectable":true,"description":"reserved"}
	]`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "pricing group name is reserved")

	err = CheckGroupRatio(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"auto","ratio":1,"selectable":true}
	]`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "pricing group name is reserved")
}

func TestPricingGroupsRejectNumericNames(t *testing.T) {
	restore := withTestPricingGroups(t, nil)
	defer restore()

	err := UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"123","ratio":1,"selectable":true,"description":"numeric"}
	]`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "pricing group name must not be numeric")

	err = CheckGroupRatio(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"001","ratio":1,"selectable":true}
	]`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "pricing group name must not be numeric")
}

func TestGroupGroupRatioNormalizesOnlyPricingGroupKeys(t *testing.T) {
	restore := withTestPricingGroups(t, nil)
	defer restore()

	original := GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupGroupRatioByJSONString(original))
	})

	require.NoError(t, UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1.2,"selectable":true,"description":"vip"}
	]`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{
		"paid-user-group": {
			"vip": 0.75,
			"custom": 1.5
		}
	}`))

	ratio, ok := GetGroupGroupRatio("paid-user-group", "2")
	require.True(t, ok)
	assert.InDelta(t, 0.75, ratio, 1e-9)

	ratio, ok = GetGroupGroupRatio("paid-user-group", "vip")
	require.True(t, ok)
	assert.InDelta(t, 0.75, ratio, 1e-9)

	_, ok = GetGroupGroupRatio("2", "vip")
	assert.False(t, ok)
}

func TestNormalizeAutoGroupsUsesPricingGroupIDs(t *testing.T) {
	restore := withTestPricingGroups(t, nil)
	defer restore()

	originalAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
	})

	require.NoError(t, UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"renamed_vip","ratio":1.2,"selectable":true,"description":"vip"}
	]`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","renamed_vip","missing"]`))

	normalized, err := NormalizeAutoGroups()
	require.NoError(t, err)
	assert.JSONEq(t, `["1","2","missing"]`, normalized)
	assert.Equal(t, []string{"1", "2", "missing"}, setting.GetAutoGroups())
}

func TestLegacyUsableOnlyGroupDefaultsRatioToOne(t *testing.T) {
	originalGroups := PricingGroups2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	originalSpecial := GroupSpecialUsableGroup2JSONString()
	originalGroupGroupRatio := GroupGroupRatio2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
		require.NoError(t, UpdateGroupSpecialUsableGroupByJSONString(originalSpecial))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, UpdatePricingGroupsByJSONString(originalGroups))
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"default": "default",
		"usable_only": "usable only"
	}`))
	require.NoError(t, UpdateGroupSpecialUsableGroupByJSONString(`{}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	ResetPricingGroupsForTest()

	assert.InDelta(t, 1, GetGroupRatio("usable_only"), 1e-9)
}

func TestLegacyGroupRatioKeepsUsableOnlyGroups(t *testing.T) {
	originalGroups := PricingGroups2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	originalSpecial := GroupSpecialUsableGroup2JSONString()
	originalGroupGroupRatio := GroupGroupRatio2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
		require.NoError(t, UpdateGroupSpecialUsableGroupByJSONString(originalSpecial))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, UpdatePricingGroupsByJSONString(originalGroups))
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"default": "default",
		"vip": "vip",
		"usable_only": "usable only"
	}`))
	require.NoError(t, UpdateGroupSpecialUsableGroupByJSONString(`{}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	ResetPricingGroupsForTest()
	require.NoError(t, UpdateGroupRatioByJSONString(`{"default":1,"vip":1.2}`))

	assert.NotEqual(t, "usable_only", PricingGroupKey("usable_only"))
	assert.InDelta(t, 1, GetGroupRatio("usable_only"), 1e-9)
	assert.InDelta(t, 1.2, GetGroupRatio("vip"), 1e-9)
}

func TestLegacyGroupRatioPreservesExistingPricingGroupIDs(t *testing.T) {
	restore := withTestPricingGroups(t, nil)
	defer restore()

	originalGroupGroupRatio := GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
	})

	require.NoError(t, UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":7,"name":"premium","ratio":1,"selectable":true,"description":"premium"}
	]`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{"vip":{"5":0.9}}`))

	require.NoError(t, UpdateGroupRatioByJSONString(`{"default":1,"premium":0.8}`))

	group, ok := ResolvePricingGroupKey("premium")
	require.True(t, ok)
	assert.Equal(t, 7, group.Id)
	assert.Equal(t, "7", PricingGroupKey("premium"))
	assert.InDelta(t, 0.8, GetGroupRatio("7"), 1e-9)
}

func TestLegacyGroupRatioMapsIDBasedUsableGroupsThroughDefaultCatalog(t *testing.T) {
	originalGroups := PricingGroups2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	originalSpecial := GroupSpecialUsableGroup2JSONString()
	originalGroupGroupRatio := GroupGroupRatio2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
		require.NoError(t, UpdateGroupSpecialUsableGroupByJSONString(originalSpecial))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, UpdatePricingGroupsByJSONString(originalGroups))
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"1":"Default","2":"VIP"}`))
	require.NoError(t, UpdateGroupSpecialUsableGroupByJSONString(`{}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))

	ResetPricingGroupsForTest()
	require.NoError(t, UpdateGroupRatioByJSONString(`{"default":1,"vip":1.2}`))

	defaultGroup, ok := GetPricingGroupByID(1)
	require.True(t, ok)
	assert.Equal(t, "default", defaultGroup.Name)
	assert.Equal(t, "Default", defaultGroup.Description)
	assert.True(t, defaultGroup.Selectable)

	vip, ok := GetPricingGroupByID(2)
	require.True(t, ok)
	assert.Equal(t, "vip", vip.Name)
	assert.Equal(t, "VIP", vip.Description)
	assert.True(t, vip.Selectable)

	_, ok = GetPricingGroupByName("1")
	assert.False(t, ok)
	_, ok = GetPricingGroupByName("2")
	assert.False(t, ok)
	assert.InDelta(t, 1.2, GetGroupRatio("2"), 1e-9)
}

func TestCheckGroupRatioAcceptsPricingGroupsArray(t *testing.T) {
	require.NoError(t, CheckGroupRatio(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1.2,"selectable":true}
	]`))

	err := CheckGroupRatio(`[
		{"id":1,"name":"default","ratio":-1,"selectable":true}
	]`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "group ratio must be not less than 0")
}

func TestCheckGroupRatioRejectsDefaultDeletion(t *testing.T) {
	err := CheckGroupRatio(`[
		{"id":2,"name":"vip","ratio":1,"selectable":true}
	]`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "default pricing group cannot be deleted")

	err = CheckGroupRatio(`{"vip":1}`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "default pricing group cannot be deleted")
}
