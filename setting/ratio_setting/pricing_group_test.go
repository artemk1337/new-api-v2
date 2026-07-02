package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withTestPricingGroups(t *testing.T, groups []*PricingGroup) func() {
	t.Helper()

	pricingGroupsMutex.Lock()
	prev := pricingGroups
	pricingGroupsMutex.Unlock()

	setPricingGroups(groups)

	return func() {
		pricingGroupsMutex.Lock()
		pricingGroups = prev
		pricingGroupsMutex.Unlock()
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
