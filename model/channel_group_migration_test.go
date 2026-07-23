package model

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestChannelInsertRollsBackWhenAbilityBatchFails(t *testing.T) {
	truncateTables(t)

	models := make([]string, 51)
	for i := range models {
		models[i] = "model-" + strconv.Itoa(i)
	}
	channel := &Channel{
		Name:   "ability-rollback",
		Key:    "test-key",
		Models: strings.Join(models, ","),
		Group:  "default",
	}

	callbackName := "test:fail_second_ability_batch"
	abilityBatch := 0
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "abilities" {
			return
		}
		abilityBatch++
		if abilityBatch == 2 {
			tx.AddError(errors.New("second ability batch failed"))
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, DB.Callback().Create().Remove(callbackName))
	})

	err := channel.Insert()
	require.ErrorContains(t, err, "second ability batch failed")

	var channelCount int64
	require.NoError(t, DB.Model(&Channel{}).Where("name = ?", channel.Name).Count(&channelCount).Error)
	assert.Zero(t, channelCount)

	var abilityCount int64
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&abilityCount).Error)
	assert.Zero(t, abilityCount)
}

func TestChannelUpdateRollsBackWhenAbilityBatchFails(t *testing.T) {
	truncateTables(t)

	channel := &Channel{
		Name:   "ability-update-rollback",
		Key:    "test-key",
		Models: "original-model",
		Group:  "default",
	}
	require.NoError(t, channel.Insert())

	models := make([]string, 51)
	for i := range models {
		models[i] = "updated-model-" + strconv.Itoa(i)
	}
	channel.Models = strings.Join(models, ",")

	callbackName := "test:fail_second_ability_update_batch"
	abilityBatch := 0
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "abilities" {
			return
		}
		abilityBatch++
		if abilityBatch == 2 {
			tx.AddError(errors.New("second ability update batch failed"))
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, DB.Callback().Create().Remove(callbackName))
	})

	err := channel.Update()
	require.ErrorContains(t, err, "second ability update batch failed")

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "original-model", reloaded.Models)

	var abilityModels []string
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Pluck("model", &abilityModels).Error)
	assert.Equal(t, []string{"original-model"}, abilityModels)
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

func TestEditChannelByTagRollsBackWhenAbilityBatchFails(t *testing.T) {
	truncateTables(t)

	tag := "ability-tag-rollback"
	channel := &Channel{
		Name:   "ability-tag-rollback",
		Key:    "test-key",
		Models: "original-model",
		Group:  "default",
		Tag:    &tag,
	}
	require.NoError(t, channel.Insert())

	models := make([]string, 51)
	for i := range models {
		models[i] = "updated-model-" + strconv.Itoa(i)
	}
	updatedModels := strings.Join(models, ",")

	callbackName := "test:fail_second_tag_ability_batch"
	abilityBatch := 0
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "abilities" {
			return
		}
		abilityBatch++
		if abilityBatch == 2 {
			tx.AddError(errors.New("second tag ability batch failed"))
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, DB.Callback().Create().Remove(callbackName))
	})

	err := EditChannelByTag(tag, nil, nil, &updatedModels, nil, nil, nil, nil, nil)
	require.ErrorContains(t, err, "second tag ability batch failed")

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "original-model", reloaded.Models)

	var abilityModels []string
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Pluck("model", &abilityModels).Error)
	assert.Equal(t, []string{"original-model"}, abilityModels)
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

func TestNormalizeChannelPricingGroupsRebuildsStaleAbilityNamesAfterRename(t *testing.T) {
	truncateTables(t)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"renamed-vip","ratio":1,"selectable":true,"description":"vip"}
	]`))

	channel := &Channel{
		Name:   "stale-ability-group",
		Key:    "test-key",
		Models: "gpt-test",
		Group:  "2",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "vip",
		Model:     "gpt-test",
		ChannelId: channel.Id,
		Enabled:   true,
	}).Error)

	require.NoError(t, NormalizeChannelPricingGroups())

	var abilityGroups []string
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Pluck("group", &abilityGroups).Error)
	assert.ElementsMatch(t, []string{"2"}, abilityGroups)
	assert.ElementsMatch(t, []string{"gpt-test"}, GetGroupEnabledModels("renamed-vip"))
	assert.True(t, IsChannelEnabledForGroupModel("renamed-vip", "gpt-test", channel.Id))
}

func TestNormalizeChannelPricingGroupsRebuildsIncompleteAbilities(t *testing.T) {
	truncateTables(t)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":7,"name":"sale","ratio":0.3,"selectable":true,"description":"sale"}
	]`))

	channel := &Channel{
		Name:   "incomplete-abilities",
		Key:    "test-key",
		Models: "codex-model,,codex-model",
		Group:  "1,7",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create([]Ability{
		{
			Group:     "1",
			Model:     "codex-model",
			ChannelId: channel.Id,
			Enabled:   true,
		},
		{
			Group:     "1",
			Model:     "",
			ChannelId: channel.Id,
			Enabled:   true,
		},
	}).Error)

	require.NoError(t, NormalizeChannelPricingGroups())

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	assert.ElementsMatch(t, []string{
		"1\x00codex-model",
		"1\x00",
		"7\x00codex-model",
		"7\x00",
	}, lo.Map(abilities, func(ability Ability, _ int) string {
		assert.True(t, ability.Enabled)
		return ability.Group + "\x00" + ability.Model
	}))
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

func TestChannelUpdateInvalidatesPricingCache(t *testing.T) {
	truncateTables(t)
	InvalidatePricingCache()
	t.Cleanup(InvalidatePricingCache)

	channel := &Channel{
		Name:   "pricing-cache-channel-update",
		Key:    "test-key",
		Models: "cached-model",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, channel.Insert())
	require.Contains(t, lo.Map(GetPricing(), func(item Pricing, _ int) string {
		return item.ModelName
	}), "cached-model")

	channel.Models = "cached-model,new-model"
	require.NoError(t, channel.Update())

	modelNames := lo.Map(GetPricing(), func(item Pricing, _ int) string {
		return item.ModelName
	})
	assert.Contains(t, modelNames, "cached-model")
	assert.Contains(t, modelNames, "new-model")
}

func TestChannelStatusChangesInvalidatePricingCache(t *testing.T) {
	truncateTables(t)
	InvalidatePricingCache()
	t.Cleanup(InvalidatePricingCache)

	tag := "pricing-cache-status"
	channel := &Channel{
		Name:   "pricing-cache-status",
		Key:    "test-key",
		Models: "status-model",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
		Tag:    &tag,
	}
	require.NoError(t, channel.Insert())

	modelNames := func() []string {
		return lo.Map(GetPricing(), func(item Pricing, _ int) string {
			return item.ModelName
		})
	}
	require.Contains(t, modelNames(), "status-model")

	require.NoError(t, DisableChannelByTag(tag))
	assert.NotContains(t, modelNames(), "status-model")

	require.NoError(t, EnableChannelByTag(tag))
	assert.Contains(t, modelNames(), "status-model")

	assert.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "test"))
	assert.NotContains(t, modelNames(), "status-model")
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

func TestUpdatePricingGroupsAssignsNewGroupToLegacyTaskReference(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	})

	require.NoError(t, UpdateOption("PricingGroups", `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"}
	]`))
	task := Task{
		TaskID:    "new-pricing-group-task",
		UserId:    1,
		Group:     "sale",
		Status:    TaskStatusInProgress,
		CreatedAt: common.GetTimestamp(),
		UpdatedAt: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(&task).Error)

	require.NoError(t, UpdateOption("PricingGroups", `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":7,"name":"sale","ratio":0.3,"selectable":true,"description":"Sale"}
	]`))

	var reloaded Task
	require.NoError(t, DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, "7", reloaded.Group)
}
