package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceChannelGroupNamesWithIDs(t *testing.T) {
	truncateTables(t)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1,"selectable":true,"description":"vip"},
		{"id":3,"name":"svip","ratio":1,"selectable":false,"description":""}
	]`))

	channel := &Channel{
		Name:   "group-migration",
		Key:    "test-key",
		Models: "gpt-test",
		Group:  "default,vip,custom",
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	require.NoError(t, ReplaceChannelGroupNamesWithIDs())

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "1,2,custom", reloaded.Group)

	var abilityGroups []string
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Order(commonGroupCol+" asc").Pluck("group", &abilityGroups).Error)
	assert.ElementsMatch(t, []string{"1", "2", "custom"}, abilityGroups)
}

func TestChannelInsertStoresPricingGroupIDsAndSurvivesRename(t *testing.T) {
	truncateTables(t)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1,"selectable":true,"description":"vip"}
	]`))

	channel := &Channel{
		Name:   "group-insert",
		Key:    "test-key",
		Models: "gpt-test",
		Group:  "vip",
	}
	require.NoError(t, channel.Insert())

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "2", reloaded.Group)

	assert.ElementsMatch(t, []string{"gpt-test"}, GetGroupEnabledModels("2"))

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"renamed-vip","ratio":1,"selectable":true,"description":"vip"}
	]`))

	assert.ElementsMatch(t, []string{"gpt-test"}, GetGroupEnabledModels("2"))
}
