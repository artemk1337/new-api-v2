package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestApplyJSONOptionPatchesMergesLatestDatabaseValue(t *testing.T) {
	const optionKey = "ModelRatio"
	setPricingPatchOption(t, optionKey, `{"preserved":1,"removed":2}`)
	common.OptionMapRWMutex.Lock()
	common.OptionMap[optionKey] = `{"stale":true}`
	common.OptionMapRWMutex.Unlock()

	require.NoError(t, ApplyJSONOptionPatches(map[string]JSONObjectPatch{
		optionKey: {
			Set:    map[string]any{"updated": 3},
			Delete: []string{"removed"},
		},
	}))

	var persisted Option
	require.NoError(t, DB.First(&persisted, "key = ?", optionKey).Error)
	assert.JSONEq(t, `{"preserved":1,"updated":3}`, persisted.Value)
	common.OptionMapRWMutex.RLock()
	assert.JSONEq(t, `{"preserved":1,"updated":3}`, common.OptionMap[optionKey])
	common.OptionMapRWMutex.RUnlock()
}

func TestApplyJSONOptionPatchesCreatesMissingOptionFromEmptyObject(t *testing.T) {
	const optionKey = "ImageRatio"
	setPricingPatchOption(t, optionKey, "{}")

	require.NoError(t, ApplyJSONOptionPatches(map[string]JSONObjectPatch{
		optionKey: {Set: map[string]any{"model": 1.25}},
	}))

	var persisted Option
	require.NoError(t, DB.First(&persisted, "key = ?", optionKey).Error)
	assert.JSONEq(t, `{"model":1.25}`, persisted.Value)
}

func TestApplyJSONOptionPatchesDoesNotPersistWhenCurrentOptionIsInvalid(t *testing.T) {
	const (
		validKey   = "ModelPrice"
		invalidKey = "AudioRatio"
	)
	setPricingPatchOption(t, validKey, `{"value":1}`)
	setPricingPatchOption(t, invalidKey, `null`)

	err := ApplyJSONOptionPatches(map[string]JSONObjectPatch{
		validKey:   {Set: map[string]any{"value": 2}},
		invalidKey: {Set: map[string]any{"value": 2}},
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "must contain a JSON object")

	var persisted Option
	require.NoError(t, DB.First(&persisted, "key = ?", validKey).Error)
	assert.JSONEq(t, `{"value":1}`, persisted.Value)
}

func TestApplyJSONOptionPatchesRejectsNonPricingOption(t *testing.T) {
	err := ApplyJSONOptionPatches(map[string]JSONObjectPatch{
		"AutoGroups": {Set: map[string]any{"model": 1}},
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "only supported for model pricing options")
}

func setPricingPatchOption(t *testing.T, key, value string) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Option{}))
	var previous Option
	err := DB.First(&previous, "key = ?", key).Error
	require.True(t, err == nil || errors.Is(err, gorm.ErrRecordNotFound))
	require.NoError(t, DB.Where("key = ?", key).Delete(&Option{}).Error)
	require.NoError(t, DB.Create(&Option{Key: key, Value: value}).Error)
	t.Cleanup(func() {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			require.NoError(t, DB.Where("key = ?", key).Delete(&Option{}).Error)
		} else {
			require.NoError(t, DB.Save(&previous).Error)
		}
		loadOptionsFromDatabase()
	})
}
