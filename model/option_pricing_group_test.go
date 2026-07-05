package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionDoesNotPersistInvalidPricingGroups(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = map[string]string{}
		common.OptionMapRWMutex.Unlock()
	})

	validValue := `[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1,"selectable":true}
	]`
	require.NoError(t, UpdateOption("PricingGroups", validValue))

	err := UpdateOption("PricingGroups", `[
		{"id":2,"name":"vip","ratio":1,"selectable":true}
	]`)
	require.Error(t, err)

	var option Option
	require.NoError(t, DB.First(&option, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, validValue, option.Value)
}

func TestUpdateOptionsBulkDoesNotPersistInvalidPricingGroups(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)

	err := UpdateOptionsBulk(map[string]string{
		"PricingGroups": `[
			{"id":2,"name":"vip","ratio":1,"selectable":true}
		]`,
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, DB.Model(&Option{}).Where("key = ?", "PricingGroups").Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateOptionRejectsPricingGroupIDChange(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1,"selectable":true}
	]`))

	err := UpdateOption("PricingGroups", `[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":7,"name":"vip","ratio":1,"selectable":true}
	]`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "pricing group id cannot be changed for name: vip")
	assert.Equal(t, "2", ratio_setting.PricingGroupKey("vip"))

	err = UpdateOption("PricingGroups", `[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":7,"name":"renamed-vip","ratio":1,"selectable":true}
	]`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "pricing group id cannot be changed for name: vip")
	assert.Equal(t, "2", ratio_setting.PricingGroupKey("vip"))

	require.NoError(t, UpdateOption("PricingGroups", `[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1,"selectable":true},
		{"id":7,"name":"premium","ratio":1,"selectable":true}
	]`))
	assert.Equal(t, "7", ratio_setting.PricingGroupKey("premium"))
}

func TestLoadOptionsBuildsPricingGroupsAfterLegacyDependencies(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	optionKeys := []string{"GroupRatio", "UserUsableGroups", "GroupGroupRatio", "AutoGroups"}
	require.NoError(t, DB.Where("key IN ?", optionKeys).Delete(&Option{}).Error)
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Where("key IN ?", optionKeys).Delete(&Option{}).Error)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = map[string]string{}
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, DB.Create(&Option{
		Key:   "GroupRatio",
		Value: `{"default":1,"vip":1.2,"premium":0.8}`,
	}).Error)
	require.NoError(t, DB.Create(&Option{
		Key:   "UserUsableGroups",
		Value: `{"default":"Default","vip":"VIP","usable_only":"Usable"}`,
	}).Error)
	require.NoError(t, DB.Create(&Option{
		Key:   "GroupGroupRatio",
		Value: `{"paid-user-group":{"premium":0.5}}`,
	}).Error)
	require.NoError(t, DB.Create(&Option{
		Key:   "AutoGroups",
		Value: `["premium"]`,
	}).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	assert.NotEqual(t, "usable_only", ratio_setting.PricingGroupKey("usable_only"))
	premiumKey := ratio_setting.PricingGroupKey("premium")
	assert.NotEqual(t, "premium", premiumKey)
	assert.InDelta(t, 1, ratio_setting.GetGroupRatio("usable_only"), 1e-9)
	assert.InDelta(t, 1.2, ratio_setting.GetGroupRatio("vip"), 1e-9)
	assert.InDelta(t, 0.8, ratio_setting.GetGroupRatio(premiumKey), 1e-9)
	ratio, ok := ratio_setting.GetGroupGroupRatio("paid-user-group", premiumKey)
	require.True(t, ok)
	assert.InDelta(t, 0.5, ratio, 1e-9)
	assert.Equal(t, []string{premiumKey}, setting.GetAutoGroups())
}

func TestLoadOptionsNormalizesDBRefsAfterCanonicalPricingGroups(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM tokens").Error)
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = map[string]string{}
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, DB.Create(&Option{
		Key:   "GroupRatio",
		Value: `{"default":1,"vip":1,"premium":0.8}`,
	}).Error)
	require.NoError(t, DB.Create(&Option{
		Key: "PricingGroups",
		Value: `[
			{"id":1,"name":"default","ratio":1,"selectable":true},
			{"id":2,"name":"vip","ratio":1,"selectable":true},
			{"id":7,"name":"premium","ratio":0.8,"selectable":true}
		]`,
	}).Error)
	channel := &Channel{
		Name:   "legacy-premium",
		Key:    "test-key",
		Models: "gpt-premium",
		Group:  "premium",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "premium",
		Model:     "gpt-premium",
		ChannelId: channel.Id,
		Enabled:   true,
	}).Error)
	token := &Token{
		UserId: 1,
		Key:    "legacy-premium-token",
		Status: common.TokenStatusEnabled,
		Name:   "legacy-premium",
		Group:  "premium",
	}
	require.NoError(t, DB.Create(token).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	assert.Equal(t, "7", ratio_setting.PricingGroupKey("premium"))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "7", reloaded.Group)

	var abilityGroups []string
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Pluck("group", &abilityGroups).Error)
	assert.ElementsMatch(t, []string{"7"}, abilityGroups)

	var reloadedToken Token
	require.NoError(t, DB.First(&reloadedToken, token.Id).Error)
	assert.Equal(t, "7", reloadedToken.Group)
}

func TestUpdatePricingGroupsPersistsUsableGroupsByIDBeforeRename(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key IN ?", []string{"PricingGroups", "UserUsableGroups"}).Delete(&Option{}).Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, DB.Where("key IN ?", []string{"PricingGroups", "UserUsableGroups"}).Delete(&Option{}).Error)
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":4,"name":"claude code x3","ratio":2.8,"selectable":true}
	]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","claude code x3":"Stable Claude"}`))
	require.NoError(t, DB.Create(&Option{
		Key:   "UserUsableGroups",
		Value: `{"default":"Default","claude code x3":"Stable Claude"}`,
	}).Error)

	require.NoError(t, UpdateOption("PricingGroups", `[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":4,"name":"claude code x2.8","ratio":2.8,"selectable":true}
	]`))

	var option Option
	require.NoError(t, DB.First(&option, "key = ?", "UserUsableGroups").Error)
	assert.JSONEq(t, `{"1":"Default","4":"Stable Claude"}`, option.Value)
	assert.Equal(t, "Stable Claude", setting.GetUserUsableGroupsCopy()["4"])
	assert.Equal(t, "4", ratio_setting.PricingGroupKey("claude code x2.8"))
}

func TestUpdateOptionPersistsCanonicalPricingGroupsForLegacyGroupRatio(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "GroupRatio").Delete(&Option{}).Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	originalSpecialUsable := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialUsable))
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Where("key = ?", "GroupRatio").Delete(&Option{}).Error)
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
		{"id":2,"name":"renamed_vip","ratio":1.2,"selectable":true,"description":"VIP"}
	]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"1": "Default",
		"2": "VIP"
	}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))

	require.NoError(t, UpdateOption("GroupRatio", `{"default":1,"renamed_vip":1.2}`))

	var option Option
	require.NoError(t, DB.First(&option, "key = ?", "GroupRatio").Error)
	assert.JSONEq(t, `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
		{"id":2,"name":"renamed_vip","ratio":1.2,"selectable":true,"description":"VIP"}
	]`, option.Value)
}

func TestUpdateOptionPersistsCanonicalPricingGroupsForLegacyPricingGroups(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":7,"name":"premium","ratio":0.8,"selectable":true}
	]`))

	require.NoError(t, UpdateOption("PricingGroups", `{"default":1,"premium":0.8}`))

	var option Option
	require.NoError(t, DB.First(&option, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, `[
		{"id":1,"name":"default","ratio":1,"selectable":false},
		{"id":7,"name":"premium","ratio":0.8,"selectable":false}
	]`, option.Value)
}

func TestUpdateOptionPersistsNormalizedPricingGroupOptions(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	optionKeys := []string{
		"AutoGroups",
		"UserUsableGroups",
		"GroupGroupRatio",
		"group_ratio_setting.group_special_usable_group",
	}
	require.NoError(t, DB.Where("key IN ?", optionKeys).Delete(&Option{}).Error)
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	originalSpecialUsable := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialUsable))
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Where("key IN ?", optionKeys).Delete(&Option{}).Error)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = map[string]string{}
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"renamed_vip","ratio":1.2,"selectable":true,"description":"vip"}
	]`))

	require.NoError(t, UpdateOption("AutoGroups", `["default","renamed_vip","missing"]`))
	require.NoError(t, UpdateOption("UserUsableGroups", `{
		"default": "Default",
		"renamed_vip": "VIP",
		"missing": "Missing"
	}`))
	require.NoError(t, UpdateOption("GroupGroupRatio", `{
		"paid-user-group": {
			"renamed_vip": 0.75,
			"missing": 1.5
		}
	}`))
	require.NoError(t, UpdateOption("group_ratio_setting.group_special_usable_group", `{
		"paid-user-group": {
			"+:renamed_vip": "VIP",
			"-:default": "remove",
			"missing": "Missing"
		}
	}`))

	var autoGroupsOption Option
	require.NoError(t, DB.First(&autoGroupsOption, "key = ?", "AutoGroups").Error)
	assert.JSONEq(t, `["1","2","missing"]`, autoGroupsOption.Value)

	var usableGroupsOption Option
	require.NoError(t, DB.First(&usableGroupsOption, "key = ?", "UserUsableGroups").Error)
	assert.JSONEq(t, `{"1":"Default","2":"VIP","missing":"Missing"}`, usableGroupsOption.Value)

	var groupGroupRatioOption Option
	require.NoError(t, DB.First(&groupGroupRatioOption, "key = ?", "GroupGroupRatio").Error)
	assert.JSONEq(t, `{"paid-user-group":{"2":0.75,"missing":1.5}}`, groupGroupRatioOption.Value)

	var specialUsableOption Option
	require.NoError(t, DB.First(&specialUsableOption, "key = ?", "group_ratio_setting.group_special_usable_group").Error)
	assert.JSONEq(t, `{"paid-user-group":{"+:2":"VIP","-:1":"remove","missing":"Missing"}}`, specialUsableOption.Value)

	common.OptionMapRWMutex.RLock()
	optionMapValue := common.OptionMap["AutoGroups"]
	usableGroupsValue := common.OptionMap["UserUsableGroups"]
	groupGroupRatioValue := common.OptionMap["GroupGroupRatio"]
	specialUsableValue := common.OptionMap["group_ratio_setting.group_special_usable_group"]
	common.OptionMapRWMutex.RUnlock()
	assert.JSONEq(t, `["1","2","missing"]`, optionMapValue)
	assert.JSONEq(t, `{"1":"Default","2":"VIP","missing":"Missing"}`, usableGroupsValue)
	assert.JSONEq(t, `{"paid-user-group":{"2":0.75,"missing":1.5}}`, groupGroupRatioValue)
	assert.JSONEq(t, `{"paid-user-group":{"+:2":"VIP","-:1":"remove","missing":"Missing"}}`, specialUsableValue)
	assert.Equal(t, []string{"1", "2", "missing"}, setting.GetAutoGroups())
}

func TestUpdateOptionsBulkNormalizesPricingGroupOptionsWithoutMutatingInput(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	optionKeys := []string{
		"AutoGroups",
		"UserUsableGroups",
		"GroupGroupRatio",
		"group_ratio_setting.group_special_usable_group",
	}
	require.NoError(t, DB.Where("key IN ?", optionKeys).Delete(&Option{}).Error)
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	originalSpecialUsable := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialUsable))
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Where("key IN ?", optionKeys).Delete(&Option{}).Error)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = map[string]string{}
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"renamed_vip","ratio":1.2,"selectable":true,"description":"vip"}
	]`))

	values := map[string]string{
		"AutoGroups": `["renamed_vip"]`,
		"UserUsableGroups": `{
			"default": "Default",
			"renamed_vip": "VIP"
		}`,
		"GroupGroupRatio": `{
			"paid-user-group": {
				"renamed_vip": 0.75
			}
		}`,
		"group_ratio_setting.group_special_usable_group": `{
			"paid-user-group": {
				"+:renamed_vip": "VIP"
			}
		}`,
	}
	require.NoError(t, UpdateOptionsBulk(values))

	assert.JSONEq(t, `["renamed_vip"]`, values["AutoGroups"])
	assert.JSONEq(t, `{"default":"Default","renamed_vip":"VIP"}`, values["UserUsableGroups"])
	assert.JSONEq(t, `{"paid-user-group":{"renamed_vip":0.75}}`, values["GroupGroupRatio"])
	assert.JSONEq(t, `{"paid-user-group":{"+:renamed_vip":"VIP"}}`, values["group_ratio_setting.group_special_usable_group"])

	var autoGroupsOption Option
	require.NoError(t, DB.First(&autoGroupsOption, "key = ?", "AutoGroups").Error)
	assert.JSONEq(t, `["2"]`, autoGroupsOption.Value)

	var usableGroupsOption Option
	require.NoError(t, DB.First(&usableGroupsOption, "key = ?", "UserUsableGroups").Error)
	assert.JSONEq(t, `{"1":"Default","2":"VIP"}`, usableGroupsOption.Value)

	var groupGroupRatioOption Option
	require.NoError(t, DB.First(&groupGroupRatioOption, "key = ?", "GroupGroupRatio").Error)
	assert.JSONEq(t, `{"paid-user-group":{"2":0.75}}`, groupGroupRatioOption.Value)

	var specialUsableOption Option
	require.NoError(t, DB.First(&specialUsableOption, "key = ?", "group_ratio_setting.group_special_usable_group").Error)
	assert.JSONEq(t, `{"paid-user-group":{"+:2":"VIP"}}`, specialUsableOption.Value)
}
