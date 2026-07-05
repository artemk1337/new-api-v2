package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
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

	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})

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
	assert.True(t, IsChannelEnabledForGroupModel("2", "gpt-test", channel.Id))
	assert.True(t, IsChannelEnabledForGroupModel("renamed-vip", "gpt-test", channel.Id))
}

func TestEditChannelByTagStoresPricingGroupIDs(t *testing.T) {
	truncateTables(t)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1,"selectable":true,"description":"vip"}
	]`))

	tag := "pricing-tag"
	channel := &Channel{
		Name:   "group-tag-edit",
		Key:    "test-key",
		Models: "gpt-test",
		Group:  "default",
		Tag:    &tag,
	}
	require.NoError(t, channel.Insert())

	group := "vip"
	require.NoError(t, EditChannelByTag(tag, nil, nil, nil, &group, nil, nil, nil, nil))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "2", reloaded.Group)

	var abilityGroups []string
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Pluck("group", &abilityGroups).Error)
	assert.ElementsMatch(t, []string{"2"}, abilityGroups)
}

func TestUpdatePricingGroupsNormalizesLegacyChannelGroupsBeforeRename(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1,"selectable":true,"description":"vip"}
	]`))

	channel := &Channel{
		Name:   "legacy-group-rename",
		Key:    "test-key",
		Models: "gpt-test",
		Group:  "vip",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "vip",
		Model:     "gpt-test",
		ChannelId: channel.Id,
		Enabled:   true,
	}).Error)

	require.NoError(t, UpdateOption("PricingGroups", `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"renamed-vip","ratio":1,"selectable":true,"description":"vip"}
	]`))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "2", reloaded.Group)

	var abilityGroups []string
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Pluck("group", &abilityGroups).Error)
	assert.ElementsMatch(t, []string{"2"}, abilityGroups)
	assert.ElementsMatch(t, []string{"gpt-test"}, GetGroupEnabledModels("2"))
	assert.True(t, IsChannelEnabledForGroupModel("renamed-vip", "gpt-test", channel.Id))
}

func TestUpdatePricingGroupsInvalidatesPricingCacheRefs(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	InvalidatePricingCache()

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
		InvalidatePricingCache()
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1,"selectable":true,"description":"vip"}
	]`))

	channel := &Channel{
		Name:   "pricing-cache-refs",
		Key:    "test-key",
		Models: "gpt-cache-ref",
		Group:  "vip",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, channel.Insert())

	groupRefName := func() string {
		for _, pricing := range GetPricing() {
			if pricing.ModelName != "gpt-cache-ref" {
				continue
			}
			for _, ref := range pricing.EnableGroupRefs {
				if ref.Id == 2 {
					return ref.Name
				}
			}
			require.FailNow(t, "pricing group ref id=2 not found")
		}
		require.FailNow(t, "pricing model not found")
		return ""
	}
	assert.Equal(t, "vip", groupRefName())

	require.NoError(t, UpdateOption("PricingGroups", `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"renamed-vip","ratio":1,"selectable":true,"description":"vip"}
	]`))

	assert.Equal(t, "renamed-vip", groupRefName())
}

func TestUpdatePricingGroupsNormalizesLegacyTokenGroupsBeforeRename(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1,"selectable":true,"description":"vip"}
	]`))

	token := &Token{
		UserId: 1,
		Key:    "legacy-token-key",
		Status: common.TokenStatusEnabled,
		Name:   "legacy",
		Group:  "vip",
	}
	autoToken := &Token{
		UserId: 1,
		Key:    "auto-token-key",
		Status: common.TokenStatusEnabled,
		Name:   "auto",
		Group:  "auto",
	}
	require.NoError(t, DB.Create(token).Error)
	require.NoError(t, DB.Create(autoToken).Error)

	require.NoError(t, UpdateOption("PricingGroups", `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"renamed-vip","ratio":1,"selectable":true,"description":"vip"}
	]`))

	var reloaded Token
	require.NoError(t, DB.First(&reloaded, token.Id).Error)
	assert.Equal(t, "2", reloaded.Group)

	var reloadedAuto Token
	require.NoError(t, DB.First(&reloadedAuto, autoToken.Id).Error)
	assert.Equal(t, "auto", reloadedAuto.Group)
}

func TestUpdatePricingGroupsNormalizesLegacyTaskGroupsBeforeRename(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1.2,"selectable":true,"description":"vip"}
	]`))

	task := &Task{
		TaskID:    "legacy-task-group",
		UserId:    1,
		Group:     "vip",
		Status:    TaskStatusInProgress,
		CreatedAt: common.GetTimestamp(),
		UpdatedAt: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(task).Error)

	require.NoError(t, UpdateOption("PricingGroups", `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"renamed-vip","ratio":1.2,"selectable":true,"description":"vip"}
	]`))

	var reloaded Task
	require.NoError(t, DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, "2", reloaded.Group)
}
