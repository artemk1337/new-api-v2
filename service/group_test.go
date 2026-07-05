package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserUsableGroupsDoesNotTreatNumericUserGroupAsPricingID(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1,"selectable":true}
	]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))

	groups := GetUserUsableGroups("2")

	assert.Contains(t, groups, "1")
	assert.NotContains(t, groups, "2")
}

func TestGetUserUsableGroupsAddsOverlappingPricingGroupName(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1,"selectable":true}
	]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))

	groups := GetUserUsableGroups("vip")

	assert.Contains(t, groups, "1")
	assert.Contains(t, groups, "2")
}
