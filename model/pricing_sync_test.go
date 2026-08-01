package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidatePricingSyncSourceEndpoint(t *testing.T) {
	for _, endpoint := range []string{"/api/pricing", "ratio_config", "openrouter"} {
		t.Run(endpoint, func(t *testing.T) {
			require.NoError(t, ValidatePricingSyncSource(PricingSyncSource{ChannelID: 1, Endpoint: endpoint}))
		})
	}

	for _, endpoint := range []string{"https://untrusted.example/prices", "//untrusted.example/prices"} {
		t.Run(endpoint, func(t *testing.T) {
			require.Error(t, ValidatePricingSyncSource(PricingSyncSource{ChannelID: 1, Endpoint: endpoint}))
		})
	}
}

func TestGetPricingSyncConfigVersionCreatesMissingOption(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "PricingSyncConfigVersion").Delete(&Option{}).Error)
	t.Cleanup(func() {
		DB.Where("key = ?", "PricingSyncConfigVersion").Delete(&Option{})
	})

	version, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)
	require.Zero(t, version)
}

func TestDisablePricingSyncSourcesClearsOwnedModelPricing(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	version, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)
	const modelName = "owned-model"
	keys := []string{
		"ModelPrice", "ModelRatio", "CompletionRatio", "CacheRatio", "CreateCacheRatio",
		"ImageRatio", "AudioRatio", "AudioCompletionRatio",
		"billing_setting.billing_mode", "billing_setting.billing_expr",
	}
	patches := make(map[string]JSONObjectPatch, len(keys))
	for _, key := range keys {
		patches[key] = JSONObjectPatch{Set: map[string]any{modelName: 1}}
	}
	require.NoError(t, ApplyJSONOptionPatches(patches))
	t.Cleanup(func() {
		cleanup := make(map[string]JSONObjectPatch, len(keys))
		for _, key := range keys {
			cleanup[key] = JSONObjectPatch{Delete: []string{modelName}}
		}
		require.NoError(t, ApplyJSONOptionPatches(cleanup))
	})

	require.NoError(t, DB.Create(&PricingSyncSource{ChannelID: 11, Enabled: true, Endpoint: "/api/pricing"}).Error)
	require.NoError(t, DB.Create(&PricingSyncQuote{ChannelID: 11, ModelName: modelName}).Error)
	require.NoError(t, SavePricingSyncModelState(PricingSyncModelState{
		ModelName: modelName,
		Mode:      PricingSyncModelModeChannel,
		ChannelID: 11,
	}))

	require.NoError(t, DisablePricingSyncSources([]int{11}))
	for _, key := range keys {
		var option Option
		require.NoError(t, DB.First(&option, "key = ?", key).Error)
		values := map[string]any{}
		require.NoError(t, common.UnmarshalJsonStr(option.Value, &values))
		assert.NotContains(t, values, modelName)
	}
	state, err := GetPricingSyncModelState(modelName)
	require.NoError(t, err)
	assert.Equal(t, PricingSyncModelModeManual, state.Mode)
	assert.Equal(t, PricingSyncModelStatusUnavailable, state.Status)
	var sourceCount, quoteCount int64
	require.NoError(t, DB.Model(&PricingSyncSource{}).Where("channel_id = ?", 11).Count(&sourceCount).Error)
	require.NoError(t, DB.Model(&PricingSyncQuote{}).Where("channel_id = ?", 11).Count(&quoteCount).Error)
	assert.Zero(t, sourceCount)
	assert.Zero(t, quoteCount)
	updatedVersion, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)
	assert.Equal(t, version+1, updatedVersion)
}

func TestDisablePricingSyncSourcesClearsGeneralProvenance(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	state := PricingSyncModelState{
		ModelName:  "general-owned-model",
		Mode:       PricingSyncModelModeGeneral,
		Provenance: "[5,9]",
		Status:     PricingSyncModelStatusReady,
	}
	require.NoError(t, SavePricingSyncModelState(state))
	require.NoError(t, DisablePricingSyncSources([]int{9}))
	updated, err := GetPricingSyncModelState(state.ModelName)
	require.NoError(t, err)
	assert.Equal(t, PricingSyncModelModeManual, updated.Mode)
	assert.Equal(t, PricingSyncModelStatusUnavailable, updated.Status)
	assert.Empty(t, updated.Provenance)
}

func TestApplyPricingSyncUpdatePersistsPriceAndProvenance(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}, &PricingSyncModelState{}))
	const modelName = "synced-with-provenance"
	version, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)

	require.NoError(t, ApplyPricingSyncUpdate(
		map[string]JSONObjectPatch{
			"ModelPrice": {Set: map[string]any{modelName: 0.25}},
		},
		[]PricingSyncModelState{{
			ModelName: modelName, Mode: PricingSyncModelModeGeneral,
			Provenance: "[7,9]", Status: PricingSyncModelStatusConflict,
		}},
	))

	state, err := GetPricingSyncModelState(modelName)
	require.NoError(t, err)
	assert.Equal(t, "[7,9]", state.Provenance)
	assert.Equal(t, PricingSyncModelStatusConflict, state.Status)
	var option Option
	require.NoError(t, DB.First(&option, "key = ?", "ModelPrice").Error)
	assert.Contains(t, option.Value, modelName)
	updatedVersion, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)
	assert.Equal(t, version+1, updatedVersion)
}

func TestPricingOptionChangedModels(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Save(&Option{Key: "ModelPrice", Value: `{"a":1,"b":2}`}).Error)

	changed, err := PricingOptionChangedModels("ModelPrice", `{"a":1,"b":3,"c":4}`)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"b", "c"}, changed)
}

func TestUpdatePricingOptionManualPersistsPriceAndStateTogether(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}, &PricingSyncModelState{}))
	const modelName = "atomic-manual-model"
	version, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)

	require.NoError(t, UpdatePricingOptionManual("ModelPrice", `{"atomic-manual-model":0.5}`))

	state, err := GetPricingSyncModelState(modelName)
	require.NoError(t, err)
	assert.Equal(t, PricingSyncModelModeManual, state.Mode)
	updatedVersion, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)
	assert.Equal(t, version+1, updatedVersion)
	var option Option
	require.NoError(t, DB.First(&option, "key = ?", "ModelPrice").Error)
	assert.Contains(t, option.Value, modelName)
}

func TestApplyPricingSyncPreferencesInitialVersionCAS(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}, &PricingSyncModelState{}))
	require.NoError(t, DB.Where("key = ?", "PricingSyncConfigVersion").Delete(&Option{}).Error)
	t.Cleanup(func() {
		DB.Where("key = ?", "PricingSyncConfigVersion").Delete(&Option{})
	})

	version, err := ApplyPricingSyncUpdateWithPreferencesIfVersion(nil, []PricingSyncModelPreferenceInput{{
		ModelName: "first-model",
		Mode:      PricingSyncModelModeGeneral,
	}}, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), version)

	_, err = ApplyPricingSyncUpdateWithPreferencesIfVersion(nil, []PricingSyncModelPreferenceInput{{
		ModelName: "stale-model",
		Mode:      PricingSyncModelModeManual,
	}}, 0)
	require.ErrorIs(t, err, ErrPricingSyncConfigurationChanged)
	var count int64
	require.NoError(t, DB.Model(&PricingSyncModelState{}).Where("model_name = ?", "stale-model").Count(&count).Error)
	require.Zero(t, count)
}

func TestUpdatePricingOptionManualRejectsInvalidExpressionWithoutMutation(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}, &PricingSyncModelState{}))
	const key = "billing_setting.billing_expr"
	const previous = `{"safe-model":"tier(\"safe\", p)"}`
	require.NoError(t, DB.Save(&Option{Key: key, Value: previous}).Error)

	err := UpdatePricingOptionManual(key, `{"safe-model":"tier(\"broken\", p *)"}`)

	require.Error(t, err)
	var option Option
	require.NoError(t, DB.First(&option, "key = ?", key).Error)
	assert.Equal(t, previous, option.Value)
	var stateCount int64
	require.NoError(t, DB.Model(&PricingSyncModelState{}).Count(&stateCount).Error)
	assert.Zero(t, stateCount)
}

func TestApplyPricingSyncUpdateRejectsChangedConfigVersion(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}, &PricingSyncModelState{}))
	require.NoError(t, DB.Save(&Option{Key: "PricingSyncConfigVersion", Value: "2"}).Error)

	err := ApplyPricingSyncUpdateIfVersion(
		map[string]JSONObjectPatch{"ModelPrice": {Set: map[string]any{"stale-task-model": 1.0}}},
		[]PricingSyncModelState{{ModelName: "stale-task-model", Mode: PricingSyncModelModeGeneral}},
		1,
	)

	require.ErrorContains(t, err, "configuration changed")
	var stateCount int64
	require.NoError(t, DB.Model(&PricingSyncModelState{}).Where("model_name = ?", "stale-task-model").Count(&stateCount).Error)
	assert.Zero(t, stateCount)
	var option Option
	err = DB.First(&option, "key = ?", "ModelPrice").Error
	if err == nil {
		assert.NotContains(t, option.Value, "stale-task-model")
	} else {
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}
}

func TestSavePricingSyncConfigurationRejectsStaleSourceSnapshot(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}, &PricingSyncSource{}, &PricingSyncModelState{}, &Channel{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	require.NoError(t, DB.Exec("DELETE FROM pricing_sync_sources").Error)
	require.NoError(t, DB.Exec("DELETE FROM pricing_sync_model_states").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
	previous := []PricingSyncSource{{ChannelID: 1, Enabled: true, Endpoint: "/api/pricing"}}
	require.NoError(t, DB.Create(&previous[0]).Error)
	require.NoError(t, DB.Create(&Channel{Id: 1, Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, DB.Create(&PricingSyncSource{ChannelID: 2, Enabled: true, Endpoint: "/api/pricing"}).Error)
	version, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)

	err = SavePricingSyncConfigurationIfVersion(
		[]PricingSyncSource{{ChannelID: 1, Enabled: true, Endpoint: "/api/pricing"}},
		PricingSyncStrategyHighest,
		[]int{},
		previous,
		version,
	)

	require.ErrorContains(t, err, "configuration changed")
	var count int64
	require.NoError(t, DB.Model(&PricingSyncSource{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestSavePricingSyncConfigurationAcceptsCurrentVersion(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}, &PricingSyncSource{}, &PricingSyncModelState{}, &Channel{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	require.NoError(t, DB.Exec("DELETE FROM pricing_sync_sources").Error)
	require.NoError(t, DB.Exec("DELETE FROM pricing_sync_model_states").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
	previous := []PricingSyncSource{{ChannelID: 1, Enabled: true, Endpoint: "/api/pricing"}}
	require.NoError(t, DB.Create(&previous[0]).Error)
	require.NoError(t, DB.Create(&Channel{Id: 1, Status: common.ChannelStatusEnabled}).Error)
	version, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)

	err = SavePricingSyncConfigurationIfVersion(previous, PricingSyncStrategyHighest, nil, previous, version)

	require.NoError(t, err)
	updatedVersion, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)
	assert.Equal(t, version+1, updatedVersion)
}
