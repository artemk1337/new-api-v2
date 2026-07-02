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
		{"id":3,"name":"vip","ratio":1,"selectable":true,"description":"duplicate"}
	]`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "pricing group name must be unique")
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

func TestLegacyUsableOnlyGroupDefaultsRatioToOne(t *testing.T) {
	originalGroups := PricingGroups2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
		require.NoError(t, UpdatePricingGroupsByJSONString(originalGroups))
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"default": "default",
		"usable_only": "usable only"
	}`))
	ResetPricingGroupsForTest()

	assert.InDelta(t, 1, GetGroupRatio("usable_only"), 1e-9)
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
