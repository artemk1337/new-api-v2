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
	assert.Equal(t, validValue, option.Value)
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

func TestLoadOptionsBuildsPricingGroupsAfterLegacyDependencies(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key IN ?", []string{"GroupRatio", "UserUsableGroups"}).Delete(&Option{}).Error)
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, DB.Where("key IN ?", []string{"GroupRatio", "UserUsableGroups"}).Delete(&Option{}).Error)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = map[string]string{}
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, DB.Create(&Option{
		Key:   "GroupRatio",
		Value: `{"default":1,"vip":1.2}`,
	}).Error)
	require.NoError(t, DB.Create(&Option{
		Key:   "UserUsableGroups",
		Value: `{"default":"Default","vip":"VIP","usable_only":"Usable"}`,
	}).Error)

	ratio_setting.ResetPricingGroupsForTest()
	loadOptionsFromDatabase()

	assert.NotEqual(t, "usable_only", ratio_setting.PricingGroupKey("usable_only"))
	assert.InDelta(t, 1, ratio_setting.GetGroupRatio("usable_only"), 1e-9)
	assert.InDelta(t, 1.2, ratio_setting.GetGroupRatio("vip"), 1e-9)
}
