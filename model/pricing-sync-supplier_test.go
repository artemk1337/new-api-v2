package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLegacySupplierPricingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &SupplierBindingState{}, &Option{}, &PricingSyncSource{}, &PricingSyncQuote{}, &PricingSyncModelState{}))
	DB = db
	t.Cleanup(func() { DB = originalDB; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	supplierID, profileID := 7, 8
	require.NoError(t, db.Create(&Channel{Id: 10, Name: "bound", SupplierID: &supplierID, SupplierProfileID: &profileID}).Error)
	require.NoError(t, db.Create(&Channel{Id: 11, Name: "legacy"}).Error)
	require.NoError(t, db.Create(&Option{Key: "ModelRatio", Value: `{"keep-model":2}`}).Error)
	require.NoError(t, db.Create(&PricingSyncModelState{ModelName: "keep-model", Mode: PricingSyncModelModeGeneral, Provenance: "[10]"}).Error)
	require.NoError(t, db.Create(&PricingSyncSource{ChannelID: 10, Enabled: true, Endpoint: "/api/pricing", IntervalSeconds: 60, LastAttemptAt: 42}).Error)
	return db
}

func requireLegacyGlobalPricePreserved(t *testing.T, db *gorm.DB) {
	t.Helper()
	var option Option
	require.NoError(t, db.First(&option, "key = ?", "ModelRatio").Error)
	require.Equal(t, `{"keep-model":2}`, option.Value)
}

func TestLegacyPricingSyncRejectsSupplierSourcesAtomically(t *testing.T) {
	db := setupLegacySupplierPricingTestDB(t)
	patches := map[string]JSONObjectPatch{"ModelRatio": {Set: map[string]any{"keep-model": 99}}}
	require.ErrorIs(t, SavePricingSyncSources([]PricingSyncSource{{ChannelID: 10, Enabled: true, Endpoint: "/api/pricing", IntervalSeconds: 60}}), ErrPricingSyncSupplierBound)
	require.ErrorIs(t, SavePricingSyncModelState(PricingSyncModelState{ModelName: "keep-model", Mode: PricingSyncModelModeChannel, ChannelID: 10}), ErrPricingSyncSupplierBound)
	_, err := ApplyPricingSyncUpdateWithPreferencesIfVersion(patches, []PricingSyncModelPreferenceInput{{ModelName: "keep-model", Mode: PricingSyncModelModeChannel, ChannelID: 10}}, -1)
	require.ErrorIs(t, err, ErrPricingSyncSupplierBound)
	require.ErrorIs(t, ApplyPricingSyncUpdateIfVersion(patches, []PricingSyncModelState{{ModelName: "keep-model", Mode: PricingSyncModelModeGeneral, Provenance: "[10]"}}, -1), ErrPricingSyncSupplierBound)
	// A missing model has no resulting provenance, but its source must still be
	// guarded at final apply if it was bound after the scheduler's initial read.
	require.ErrorIs(t, ApplyPricingSyncUpdateFromSourcesIfVersion(map[string]JSONObjectPatch{"ModelRatio": {Delete: []string{"keep-model"}}}, []PricingSyncModelState{{ModelName: "keep-model", Mode: PricingSyncModelModeGeneral, Status: PricingSyncModelStatusUnavailable}}, []int{10}, -1), ErrPricingSyncSupplierBound)
	requireLegacyGlobalPricePreserved(t, db)
	var state PricingSyncModelState
	require.NoError(t, db.First(&state, "model_name = ?", "keep-model").Error)
	require.Equal(t, "[10]", state.Provenance)
}

func TestLegacyConfigSaveRetainsBoundRowsAndPrices(t *testing.T) {
	db := setupLegacySupplierPricingTestDB(t)
	previous, err := GetPricingSyncSources()
	require.NoError(t, err)
	version, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)
	sources := []PricingSyncSource{{ChannelID: -100, Enabled: true, Endpoint: "/preset", IntervalSeconds: 60}, {ChannelID: 11, Enabled: true, Endpoint: "/api/pricing", IntervalSeconds: 60}}
	common.OptionMapRWMutex.Lock()
	previousStrategy := common.OptionMap["PricingSyncStrategy"]
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap["PricingSyncStrategy"] = previousStrategy
		common.OptionMapRWMutex.Unlock()
	})
	// Even an old caller's removal list cannot remove a supplier-bound source.
	require.NoError(t, SavePricingSyncConfigurationIfVersion(sources, PricingSyncStrategyHighest, []int{10}, previous, version))
	current, err := GetPricingSyncSources()
	require.NoError(t, err)
	require.Len(t, current, 3)
	var bound PricingSyncSource
	require.NoError(t, db.First(&bound, "channel_id = ?", 10).Error)
	require.Equal(t, previous[0], bound)
	requireLegacyGlobalPricePreserved(t, db)
	require.ErrorIs(t, SavePricingSyncConfiguration([]PricingSyncSource{{ChannelID: 10, Enabled: true, Endpoint: "/api/pricing", IntervalSeconds: 60}}, PricingSyncStrategyHighest, nil), ErrPricingSyncSupplierBound)
	// Detaching does not erase the retained source, making rollback possible.
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", 10).Updates(map[string]any{"supplier_id": nil, "supplier_profile_id": nil}).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return LockLegacyPricingSyncChannels(tx, []int{10, -100, -101}) }))
}

func TestLegacySourceCleanupPreservesBoundGlobalPrices(t *testing.T) {
	db := setupLegacySupplierPricingTestDB(t)
	require.NoError(t, DisablePricingSyncSources([]int{10}))
	requireLegacyGlobalPricePreserved(t, db)
	var source PricingSyncSource
	require.NoError(t, db.First(&source, "channel_id = ?", 10).Error)
	require.Equal(t, int64(42), source.LastAttemptAt)
}

func prepareLegacySourceUnbind(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&Ability{}, &PricingGroupSupplierBinding{}, &Supplier{}, &SupplierPricingProfile{}))
}

func requireLegacySourcePreserved(t *testing.T, db *gorm.DB, source PricingSyncSource) {
	t.Helper()
	var current PricingSyncSource
	require.NoError(t, db.First(&current, "channel_id = ?", source.ChannelID).Error)
	require.Equal(t, source, current)
	requireLegacyGlobalPricePreserved(t, db)
	var state PricingSyncModelState
	require.NoError(t, db.First(&state, "model_name = ?", "keep-model").Error)
	require.Equal(t, "[10]", state.Provenance)
}

func TestLegacyConfigReadBeforeUnbindCannotRemoveReturnedSource(t *testing.T) {
	db := setupLegacySupplierPricingTestDB(t)
	prepareLegacySourceUnbind(t, db)
	previous, err := GetPricingSyncSources()
	require.NoError(t, err)
	version, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)

	// The old configuration UI omitted the supplier-bound source. Returning
	// its channel to default pricing must invalidate that UI's version.
	require.NoError(t, ApplySupplierBindings(nil, []SupplierChannelBindingPatch{{ChannelID: 10}}))
	currentVersion, err := GetPricingSyncConfigVersion()
	require.NoError(t, err)
	require.Greater(t, currentVersion, version)
	err = SavePricingSyncConfigurationIfVersion(nil, PricingSyncStrategyHighest, []int{10}, previous, version)
	require.ErrorIs(t, err, ErrPricingSyncConfigurationChanged)
	requireLegacySourcePreserved(t, db, previous[0])
}

func TestLegacyConfigConcurrentUnbindRetainsSourceAndDefaultTariff(t *testing.T) {
	for _, versioned := range []bool{false, true} {
		name := "internal"
		if versioned {
			name = "versioned"
		}
		t.Run(name, func(t *testing.T) {
			db := setupLegacySupplierPricingTestDB(t)
			prepareLegacySourceUnbind(t, db)
			previous, err := GetPricingSyncSources()
			require.NoError(t, err)
			version, err := GetPricingSyncConfigVersion()
			require.NoError(t, err)
			changed := false
			var unbindErr error
			const callback = "test:unbind-before-legacy-config-transaction"
			require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(query *gorm.DB) {
				if changed || query.Statement.Table != "pricing_sync_model_states" {
					return
				}
				changed = true
				unbindErr = ApplySupplierBindings(nil, []SupplierChannelBindingPatch{{ChannelID: 10}})
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })
			if versioned {
				err = SavePricingSyncConfigurationIfVersion(nil, PricingSyncStrategyHighest, nil, previous, version)
			} else {
				err = SavePricingSyncConfiguration(nil, PricingSyncStrategyHighest, nil)
			}
			require.True(t, changed)
			require.NoError(t, unbindErr)
			require.ErrorIs(t, err, ErrPricingSyncConfigurationChanged)
			requireLegacySourcePreserved(t, db, previous[0])
		})
	}
}
