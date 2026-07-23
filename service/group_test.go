package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserUsableGroupsUsesSelectablePricingGroups(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
		{"id":7,"name":"sale","ratio":0.3,"selectable":true,"description":"Sale"},
		{"id":8,"name":"internal","ratio":1,"selectable":false,"description":"Internal"}
	]`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))

	groups := GetUserUsableGroups("paid-users")

	assert.Equal(t, map[string]string{"1": "Default", "7": "Sale"}, groups)
}

func TestGetUserUsableGroupsAppliesSpecialOverrides(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
		{"id":2,"name":"public","ratio":1,"selectable":true,"description":"Public"},
		{"id":7,"name":"sale","ratio":0.3,"selectable":false,"description":"Sale"}
	]`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{
		"vip": {"+:sale": "VIP sale", "-:public": ""}
	}`))

	groups := GetUserUsableGroups("vip")

	assert.Equal(t, map[string]string{"1": "Default", "7": "VIP sale"}, groups)
}

func TestGetUserUsableGroupsExplicitRemovalWinsOverOverlappingUserGroup(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
		{"id":2,"name":"vip","ratio":1,"selectable":true,"description":"VIP"}
	]`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{
		"vip": {"-:vip": ""}
	}`))

	groups := GetUserUsableGroups("vip")

	assert.Equal(t, map[string]string{"1": "Default"}, groups)
}

func TestGetUserUsableGroupsExplicitRemovalWinsOverConflictingAddition(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
		{"id":2,"name":"vip","ratio":1,"selectable":false,"description":"VIP"}
	]`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{
		"paid-users": {"+:vip": "VIP", "-:vip": ""}
	}`))

	groups := GetUserUsableGroups("paid-users")

	assert.Equal(t, map[string]string{"1": "Default"}, groups)
}
