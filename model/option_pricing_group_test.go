package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func isolateLegacyPricingReferences(t *testing.T) {
	t.Helper()

	originalAutoGroups := setting.AutoGroups2JsonString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	originalSpecialUsable := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialUsable))
	})

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))
}

func TestUpdateOptionDoesNotPersistInvalidPricingGroups(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
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

func TestUpdateOptionPersistsLegacyGroupRatioAsPricingGroups(t *testing.T) {
	isolateLegacyPricingReferences(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
		{"id":7,"name":"sale","ratio":1,"selectable":true,"description":"Sale"}
	]`))
	require.NoError(t, DB.Create(&Option{
		Key:   "GroupRatio",
		Value: `{"default":1,"sale":1}`,
	}).Error)

	require.NoError(t, UpdateOption("GroupRatio", `{"default":0.9,"sale":0.3}`))

	var persisted Option
	require.NoError(t, DB.First(&persisted, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, `[
		{"id":1,"name":"default","ratio":0.9,"selectable":true,"description":"Default"},
		{"id":7,"name":"sale","ratio":0.3,"selectable":true,"description":"Sale"}
	]`, persisted.Value)

	var legacy Option
	require.NoError(t, DB.First(&legacy, "key = ?", "GroupRatio").Error)
	assert.JSONEq(t, `{"default":1,"sale":1}`, legacy.Value)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	sale, ok := ratio_setting.GetPricingGroupByID(7)
	require.True(t, ok)
	assert.InDelta(t, 0.3, sale.Ratio, 1e-9)
	assert.True(t, sale.Selectable)
	assert.Equal(t, "Sale", sale.Description)
}

func TestUpdateOptionsBulkPersistsLegacyGroupRatioAsPricingGroups(t *testing.T) {
	isolateLegacyPricingReferences(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
		{"id":7,"name":"sale","ratio":1,"selectable":true,"description":"Sale"}
	]`))

	require.NoError(t, UpdateOptionsBulk(map[string]string{
		"GroupRatio": `{"default":0.9,"sale":0.3}`,
	}))

	var persisted Option
	require.NoError(t, DB.First(&persisted, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, `[
		{"id":1,"name":"default","ratio":0.9,"selectable":true,"description":"Default"},
		{"id":7,"name":"sale","ratio":0.3,"selectable":true,"description":"Sale"}
	]`, persisted.Value)

	var legacyCount int64
	require.NoError(t, DB.Model(&Option{}).Where("key = ?", "GroupRatio").Count(&legacyCount).Error)
	assert.Zero(t, legacyCount)
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
}

func TestUpdateOptionRollsBackPricingGroupsWhenReferenceNormalizationFails(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}, &Channel{}, &Ability{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
		require.NoError(t, DB.Exec("DELETE FROM channels").Error)
		require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	})

	oldValue := `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"}
	]`
	require.NoError(t, UpdateOption("PricingGroups", oldValue))
	channel := Channel{
		Name:   "legacy-sale",
		Key:    "key",
		Models: "sale-model",
		Group:  "sale",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "sale",
		Model:     "sale-model",
		ChannelId: channel.Id,
		Enabled:   true,
	}).Error)

	callbackName := "test:fail_pricing_group_reference_update"
	runtimePublishedBeforeCommit := false
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			_, runtimePublishedBeforeCommit = ratio_setting.GetPricingGroupByID(7)
			tx.AddError(errors.New("channel normalization failed"))
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, DB.Callback().Update().Remove(callbackName))
	})

	err := UpdateOption("PricingGroups", `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
		{"id":7,"name":"sale","ratio":0.3,"selectable":true,"description":"Sale"}
	]`)
	require.ErrorContains(t, err, "channel normalization failed")
	assert.False(t, runtimePublishedBeforeCommit)

	var option Option
	require.NoError(t, DB.First(&option, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, oldValue, option.Value)
	_, saleExists := ratio_setting.GetPricingGroupByID(7)
	assert.False(t, saleExists)

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "sale", reloaded.Group)
}

func TestUpdateOptionLeavesLegacyUsableGroupsUntouched(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "UserUsableGroups").Delete(&Option{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Where("key = ?", "UserUsableGroups").Delete(&Option{}).Error)
	})
	require.NoError(t, DB.Create(&Option{Key: "UserUsableGroups", Value: `{"default":"Old"}`}).Error)

	require.NoError(t, UpdateOption("UserUsableGroups", `{"default":"New"}`))

	var option Option
	require.NoError(t, DB.First(&option, "key = ?", "UserUsableGroups").Error)
	assert.JSONEq(t, `{"default":"Old"}`, option.Value)
}

func TestLoadOptionsMigratesLegacyUsableGroupsToPricingGroups(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	originalSpecialUsable := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialUsable))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))

	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `{"default":1,"vip":1.2,"premium":0.8}`},
		{Key: "UserUsableGroups", Value: `{"default":"Default","vip":"VIP","usable_only":"Usable"}`},
		{Key: "AutoGroups", Value: `["premium"]`},
	}).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	var pricingGroups Option
	require.NoError(t, DB.First(&pricingGroups, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
		{"id":2,"name":"vip","ratio":1.2,"selectable":true,"description":"VIP"},
		{"id":3,"name":"premium","ratio":0.8,"selectable":false},
		{"id":4,"name":"usable_only","ratio":1,"selectable":true,"description":"Usable"}
	]`, pricingGroups.Value)

	var legacy Option
	require.NoError(t, DB.First(&legacy, "key = ?", "UserUsableGroups").Error)
	assert.JSONEq(t, `{"default":"Default","vip":"VIP","usable_only":"Usable"}`, legacy.Value)

	common.OptionMapRWMutex.RLock()
	_, exposed := common.OptionMap["UserUsableGroups"]
	common.OptionMapRWMutex.RUnlock()
	assert.False(t, exposed)
}

func TestLoadOptionsMigratesLegacyDefaultUsableGroupsWhenOptionIsAbsent(t *testing.T) {
	isolateLegacyPricingReferences(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})

	require.NoError(t, DB.Create(&Option{
		Key:   "GroupRatio",
		Value: `{"default":1,"vip":1,"svip":1}`,
	}).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	var pricingGroups Option
	require.NoError(t, DB.First(&pricingGroups, "key = ?", "PricingGroups").Error)
	var groups []ratio_setting.PricingGroup
	require.NoError(t, common.Unmarshal([]byte(pricingGroups.Value), &groups))
	groupsByName := make(map[string]ratio_setting.PricingGroup, len(groups))
	for _, group := range groups {
		groupsByName[group.Name] = group
	}
	require.Contains(t, groupsByName, "default")
	require.Contains(t, groupsByName, "vip")
	require.Contains(t, groupsByName, "svip")
	assert.True(t, groupsByName["default"].Selectable)
	assert.Equal(t, "默认分组", groupsByName["default"].Description)
	assert.True(t, groupsByName["vip"].Selectable)
	assert.Equal(t, "vip分组", groupsByName["vip"].Description)
	assert.False(t, groupsByName["svip"].Selectable)
}

func TestLoadOptionsMigratesDefaultGroupsWhenGroupRatioIsAbsent(t *testing.T) {
	isolateLegacyPricingReferences(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	var pricingGroups Option
	require.NoError(t, DB.First(&pricingGroups, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"默认分组"},
		{"id":2,"name":"vip","ratio":1,"selectable":true,"description":"vip分组"},
		{"id":3,"name":"svip","ratio":1,"selectable":false}
	]`, pricingGroups.Value)
}

func TestLoadOptionsMigratesLegacyGroupsWithoutDefault(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	originalSpecialUsable := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialUsable))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))

	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `{"sale":0.3}`},
		{Key: "UserUsableGroups", Value: `{"sale":"Sale"}`},
	}).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	var pricingGroups Option
	require.NoError(t, DB.First(&pricingGroups, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, `[
		{"id":1,"name":"default","ratio":1,"selectable":false},
		{"id":2,"name":"sale","ratio":0.3,"selectable":true,"description":"Sale"}
	]`, pricingGroups.Value)
}

func TestLoadOptionsPreservesExplicitlyEmptyLegacyUsableGroups(t *testing.T) {
	isolateLegacyPricingReferences(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})

	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `{"default":1,"vip":1}`},
		{Key: "UserUsableGroups", Value: `{}`},
	}).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	var pricingGroups Option
	require.NoError(t, DB.First(&pricingGroups, "key = ?", "PricingGroups").Error)
	var groups []ratio_setting.PricingGroup
	require.NoError(t, common.Unmarshal([]byte(pricingGroups.Value), &groups))
	groupsByName := make(map[string]ratio_setting.PricingGroup, len(groups))
	for _, group := range groups {
		groupsByName[group.Name] = group
	}
	require.Contains(t, groupsByName, "default")
	require.Contains(t, groupsByName, "vip")
	assert.False(t, groupsByName["default"].Selectable)
	assert.False(t, groupsByName["vip"].Selectable)
}

func TestLoadOptionsUsesLegacyAvailabilityForCanonicalGroupRatio(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	originalSpecialUsable := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialUsable))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))

	require.NoError(t, DB.Create(&[]Option{
		{
			Key: "GroupRatio",
			Value: `[
				{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Stale default"},
				{"id":7,"name":"sale","ratio":0.3,"selectable":false,"description":"Stale sale"}
			]`,
		},
		{Key: "UserUsableGroups", Value: `{"7":"Current sale"}`},
	}).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	var pricingGroups Option
	require.NoError(t, DB.First(&pricingGroups, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, `[
		{"id":1,"name":"default","ratio":1,"selectable":false},
		{"id":7,"name":"sale","ratio":0.3,"selectable":true,"description":"Current sale"}
	]`, pricingGroups.Value)
}

func TestLoadOptionsRestoresLegacyNumericUsableGroupID(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	originalSpecialUsable := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialUsable))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))

	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `{"default":1,"vip":1,"sale":0.3}`},
		{Key: "UserUsableGroups", Value: `{"3":"Sale"}`},
	}).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	var pricingGroups Option
	require.NoError(t, DB.First(&pricingGroups, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, `[
		{"id":1,"name":"default","ratio":1,"selectable":false},
		{"id":2,"name":"vip","ratio":1,"selectable":false},
		{"id":3,"name":"sale","ratio":0.3,"selectable":true,"description":"Sale"}
	]`, pricingGroups.Value)
}

func TestLoadOptionsPreservesUnknownNumericReferenceDuringMigration(t *testing.T) {
	isolateLegacyPricingReferences(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})

	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `{"default":1,"vip":1,"sale":0.3}`},
		{Key: "UserUsableGroups", Value: `{"sale":"Sale"}`},
		{Key: "GroupGroupRatio", Value: `{"paid-users":{"5":0.5}}`},
	}).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	var pricingGroups Option
	require.NoError(t, DB.First(&pricingGroups, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, `[
		{"id":1,"name":"default","ratio":1,"selectable":false},
		{"id":2,"name":"vip","ratio":1,"selectable":false},
		{"id":3,"name":"sale","ratio":0.3,"selectable":true,"description":"Sale"}
	]`, pricingGroups.Value)

	ratio, ok := ratio_setting.GetGroupGroupRatio("paid-users", "5")
	require.True(t, ok)
	assert.InDelta(t, 0.5, ratio, 1e-9)
}

func TestLoadOptionsRejectsLegacyGroupTrimCollision(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})

	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `{"default":1,"sale":0.3," sale ":0.8}`},
		{Key: "UserUsableGroups", Value: `{"sale":"Sale"}`},
	}).Error)

	loadOptionsFromDatabase()

	var count int64
	require.NoError(t, DB.Model(&Option{}).Where("key = ?", "PricingGroups").Count(&count).Error)
	assert.Zero(t, count)
}

func TestLoadOptionsDoesNotNormalizeReferencesForInvalidLegacyUsableGroups(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}, &Task{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	require.NoError(t, DB.Exec("DELETE FROM tasks").Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
		require.NoError(t, DB.Exec("DELETE FROM tasks").Error)
	})

	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `{"default":1,"sale":0.3}`},
		{Key: "UserUsableGroups", Value: `{"sale":`},
	}).Error)
	task := Task{TaskID: "invalid-legacy-usable", Group: "sale"}
	require.NoError(t, DB.Create(&task).Error)

	loadOptionsFromDatabase()

	var count int64
	require.NoError(t, DB.Model(&Option{}).Where("key = ?", "PricingGroups").Count(&count).Error)
	assert.Zero(t, count)

	var reloadedTask Task
	require.NoError(t, DB.First(&reloadedTask, task.ID).Error)
	assert.Equal(t, "sale", reloadedTask.Group)
}

func TestLoadOptionsDoesNotNormalizeReferencesForInvalidPricingGroups(t *testing.T) {
	isolateLegacyPricingReferences(t)
	require.NoError(t, DB.AutoMigrate(&Option{}, &Task{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	require.NoError(t, DB.Exec("DELETE FROM tasks").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
		require.NoError(t, DB.Exec("DELETE FROM tasks").Error)
	})

	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `{"default":1,"sale":0.3}`},
		{Key: "UserUsableGroups", Value: `{"sale":"Sale"}`},
		{Key: "PricingGroups", Value: `[]`},
	}).Error)
	task := Task{TaskID: "invalid-pricing-groups", Group: "sale"}
	require.NoError(t, DB.Create(&task).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	var reloadedTask Task
	require.NoError(t, DB.First(&reloadedTask, task.ID).Error)
	assert.Equal(t, "sale", reloadedTask.Group)

	var pricingGroups Option
	require.NoError(t, DB.First(&pricingGroups, "key = ?", "PricingGroups").Error)
	assert.JSONEq(t, `[]`, pricingGroups.Value)
}

func TestLoadOptionsDoesNotCompleteMigrationForNumericLegacyGroupName(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}, &Task{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	require.NoError(t, DB.Exec("DELETE FROM tasks").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
		require.NoError(t, DB.Exec("DELETE FROM tasks").Error)
	})

	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `{"default":1,"123":0.3}`},
		{Key: "UserUsableGroups", Value: `{"default":"Default","123":"Sale"}`},
	}).Error)
	task := Task{TaskID: "numeric-legacy-group", Group: "123"}
	require.NoError(t, DB.Create(&task).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	var count int64
	require.NoError(t, DB.Model(&Option{}).Where("key = ?", "PricingGroups").Count(&count).Error)
	assert.Zero(t, count)

	var legacy Option
	require.NoError(t, DB.First(&legacy, "key = ?", "UserUsableGroups").Error)
	assert.JSONEq(t, `{"default":"Default","123":"Sale"}`, legacy.Value)

	var reloadedTask Task
	require.NoError(t, DB.First(&reloadedTask, task.ID).Error)
	assert.Equal(t, "123", reloadedTask.Group)
}

func TestLoadOptionsReadFailureDoesNotCreatePricingGroups(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})
	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `{"default":1}`},
		{Key: "UserUsableGroups", Value: `{"default":"Default"}`},
	}).Error)

	callbackName := "test:fail_options_read"
	failed := false
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if !failed && tx.Statement.Table == "options" {
			failed = true
			tx.AddError(errors.New("options read failed"))
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, DB.Callback().Query().Remove(callbackName))
	})

	loadOptionsFromDatabase()

	var count int64
	require.NoError(t, DB.Model(&Option{}).Where("key = ?", "PricingGroups").Count(&count).Error)
	assert.Zero(t, count)
}

func TestPricingGroupsMigrationWriteFailureLeavesNoCompletionMarker(t *testing.T) {
	isolateLegacyPricingReferences(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":false,"description":"Runtime"}
	]`))

	callbackName := "test:fail_pricing_groups_migration"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		option, ok := tx.Statement.Dest.(*Option)
		if ok && option.Key == "PricingGroups" {
			tx.AddError(errors.New("migration write failed"))
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, DB.Callback().Create().Remove(callbackName))
	})

	completed, err := migratePricingGroupsFromLegacy(`{"default":1}`, map[string]string{"default": "Default"})
	assert.False(t, completed)
	require.ErrorContains(t, err, "migration write failed")

	var count int64
	require.NoError(t, DB.Model(&Option{}).Where("key = ?", "PricingGroups").Count(&count).Error)
	assert.Zero(t, count)

	group, ok := ratio_setting.GetPricingGroupByID(1)
	require.True(t, ok)
	assert.False(t, group.Selectable)
	assert.Equal(t, "Runtime", group.Description)
}

func TestPricingGroupsMigrationReadFailureDoesNotChangeRuntime(t *testing.T) {
	isolateLegacyPricingReferences(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":false,"description":"Runtime"}
	]`))

	callbackName := "test:fail_pricing_groups_migration_read"
	failed := false
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if !failed && tx.Statement.Table == "options" {
			failed = true
			tx.AddError(errors.New("migration read failed"))
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, DB.Callback().Query().Remove(callbackName))
	})

	completed, err := migratePricingGroupsFromLegacy(`{"default":1}`, map[string]string{"default": "Migrated"})
	assert.False(t, completed)
	require.ErrorContains(t, err, "migration read failed")

	group, ok := ratio_setting.GetPricingGroupByID(1)
	require.True(t, ok)
	assert.False(t, group.Selectable)
	assert.Equal(t, "Runtime", group.Description)
}

func TestPricingGroupsMigrationConflictAppliesPersistedValue(t *testing.T) {
	isolateLegacyPricingReferences(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	})

	persistedValue := `[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Persisted"}
	]`
	require.NoError(t, DB.Create(&Option{Key: "PricingGroups", Value: persistedValue}).Error)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":false}
	]`))

	completed, err := migratePricingGroupsFromLegacy(`{"default":1}`, map[string]string{})
	require.NoError(t, err)
	assert.True(t, completed)

	group, ok := ratio_setting.GetPricingGroupByID(1)
	require.True(t, ok)
	assert.True(t, group.Selectable)
	assert.Equal(t, "Persisted", group.Description)
}

func TestLoadOptionsNormalizesDBRefsAfterCanonicalPricingGroups(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}, &Channel{}, &Ability{}, &Token{}))
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM tokens").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})

	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `{"default":1,"vip":1,"premium":0.8}`},
		{Key: "PricingGroups", Value: `[
			{"id":1,"name":"default","ratio":1,"selectable":true},
			{"id":2,"name":"vip","ratio":1,"selectable":true},
			{"id":7,"name":"premium","ratio":0.8,"selectable":true}
		]`},
	}).Error)
	channel := &Channel{Name: "legacy-premium", Key: "test-key", Models: "gpt-premium", Group: "premium", Status: common.ChannelStatusEnabled}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{Group: "premium", Model: "gpt-premium", ChannelId: channel.Id, Enabled: true}).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "7", reloaded.Group)
}

func TestLoadOptionsPrefersPricingGroupsOverLegacyGroupRatio(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
	})

	require.NoError(t, DB.Create(&[]Option{
		{Key: "GroupRatio", Value: `[
			{"id":1,"name":"default","ratio":1,"selectable":true},
			{"id":7,"name":"sale","ratio":0.3,"selectable":false}
		]`},
		{Key: "PricingGroups", Value: `[
			{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
			{"id":7,"name":"sale","ratio":0.3,"selectable":true,"description":"Sale"}
		]`},
	}).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	sale, ok := ratio_setting.GetPricingGroupByID(7)
	require.True(t, ok)
	assert.True(t, sale.Selectable)
	assert.Equal(t, "Sale", sale.Description)
}
