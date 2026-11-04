package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func supplierPricingTestDB(t *testing.T) SupplierPricingProfile {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Supplier{}, &SupplierPricingProfile{}, &SupplierPricingState{}, &SupplierPricingRevision{}, &SupplierModelPrice{}, &PricingGroupSupplierBinding{}))
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous; _ = sqlDB.Close() })
	supplier := Supplier{Code: "test", Name: "Test", Adapter: "new-api", Enabled: true}
	require.NoError(t, DB.Create(&supplier).Error)
	profile := SupplierPricingProfile{SupplierID: supplier.ID, Code: "test", Name: "Test", Enabled: true, USDPerCredit: 1, ConfigRevision: 1}
	require.NoError(t, DB.Create(&profile).Error)
	return profile
}

func supplierTestTariff(name string, input float64) types.SupplierModelTariff {
	output := input * 5
	return types.SupplierModelTariff{ModelName: name, BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens", InputUSD: &input, OutputUSD: &output}
}

func TestSupplierPricingPublishesWholeImmutableRevision(t *testing.T) {
	profile := supplierPricingTestDB(t)
	revision, err := PublishSupplierProfileTariffs(profile.ID, 1, 0, []types.SupplierModelTariff{supplierTestTariff("model-b", 2), supplierTestTariff("model-a", 1)})
	require.NoError(t, err)
	require.EqualValues(t, 1, revision)

	_, err = PublishSupplierProfileTariffs(profile.ID, 1, 0, []types.SupplierModelTariff{supplierTestTariff("model-a", 3)})
	require.ErrorIs(t, err, ErrSupplierPricingConflict)
	_, err = PublishSupplierProfileTariffs(profile.ID, 0, 1, []types.SupplierModelTariff{supplierTestTariff("model-a", 3)})
	require.ErrorIs(t, err, ErrSupplierPricingConflict)
	_, err = PublishSupplierProfileTariffs(profile.ID, 1, 1, []types.SupplierModelTariff{supplierTestTariff("model-a", 3), {ModelName: "incomplete", BillingMode: types.SupplierBillingModeToken}})
	require.Error(t, err)

	tariffs, current, err := ListSupplierProfileTariffs(profile.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, current)
	require.Len(t, tariffs, 2)
	require.Equal(t, "model-a", tariffs[0].ModelName)
	require.Equal(t, 1.0, *tariffs[0].InputUSD)

	revision, err = PublishSupplierProfileTariffs(profile.ID, 1, 1, []types.SupplierModelTariff{supplierTestTariff("model-a", 3)})
	require.NoError(t, err)
	require.EqualValues(t, 2, revision)
	tariffs, current, err = ListSupplierProfileTariffs(profile.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, current)
	require.Len(t, tariffs, 1)
	require.Equal(t, 3.0, *tariffs[0].InputUSD)
	var historical int64
	require.NoError(t, DB.Model(&SupplierModelPrice{}).Where("profile_id = ? AND revision = ?", profile.ID, 1).Count(&historical).Error)
	require.EqualValues(t, 2, historical)
}

func TestSupplierPricingConfirmsReplacementAndRemovalTwice(t *testing.T) {
	profile := supplierPricingTestDB(t)
	first := []types.SupplierModelTariff{supplierTestTariff("model-a", 1)}
	revision, pending, err := ConfirmSupplierProfileTariffs(profile.ID, 1, 0, first)
	require.NoError(t, err)
	require.True(t, pending)
	require.Zero(t, revision)
	revision, pending, err = ConfirmSupplierProfileTariffs(profile.ID, 1, 0, first)
	require.NoError(t, err)
	require.False(t, pending)
	require.EqualValues(t, 1, revision)

	_, pending, err = ConfirmSupplierProfileTariffs(profile.ID, 1, 1, []types.SupplierModelTariff{supplierTestTariff("model-a", 5)})
	require.NoError(t, err)
	require.True(t, pending)
	_, pending, err = ConfirmSupplierProfileTariffs(profile.ID, 1, 1, []types.SupplierModelTariff{})
	require.NoError(t, err)
	require.True(t, pending)
	tariffs, _, err := ListSupplierProfileTariffs(profile.ID)
	require.NoError(t, err)
	require.Len(t, tariffs, 1)
	require.Equal(t, 1.0, *tariffs[0].InputUSD)

	revision, pending, err = ConfirmSupplierProfileTariffs(profile.ID, 1, 1, []types.SupplierModelTariff{})
	require.NoError(t, err)
	require.False(t, pending)
	require.EqualValues(t, 2, revision)
	tariffs, _, err = ListSupplierProfileTariffs(profile.ID)
	require.NoError(t, err)
	require.Empty(t, tariffs)
}

func TestSupplierPricingConfigRevisionInvalidatesCandidateAndTariff(t *testing.T) {
	profile := supplierPricingTestDB(t)
	group, ok := ratio_setting.ResolvePricingGroupKey("default")
	require.True(t, ok)
	require.NoError(t, DB.Create(&PricingGroupSupplierBinding{GroupID: group.Id, SupplierID: profile.SupplierID, ProfileID: profile.ID}).Error)
	contracts := []types.SupplierModelTariff{supplierTestTariff("model-a", 1)}
	_, err := PublishSupplierProfileTariffs(profile.ID, 1, 0, contracts)
	require.NoError(t, err)
	resolved, bound, err := ResolveSupplierModelTariff("default", "model-a")
	require.NoError(t, err)
	require.True(t, bound)
	require.Equal(t, profile.SupplierID, resolved.SupplierID)
	require.EqualValues(t, 1, resolved.Revision)

	require.NoError(t, DB.Model(&SupplierPricingProfile{}).Where("id = ?", profile.ID).Update("config_revision", 2).Error)
	_, bound, err = ResolveSupplierModelTariff("default", "model-a")
	require.True(t, bound)
	require.ErrorIs(t, err, ErrSupplierTariffUnavailable)
	_, _, err = ConfirmSupplierProfileTariffs(profile.ID, 1, 1, contracts)
	require.ErrorIs(t, err, ErrSupplierPricingConflict)
	_, pending, err := ConfirmSupplierProfileTariffs(profile.ID, 2, 1, contracts)
	require.NoError(t, err)
	require.True(t, pending)
	revision, pending, err := ConfirmSupplierProfileTariffs(profile.ID, 2, 1, contracts)
	require.NoError(t, err)
	require.False(t, pending)
	require.EqualValues(t, 2, revision)
	_, bound, err = ResolveSupplierModelTariff("default", "missing-model")
	require.True(t, bound)
	require.ErrorIs(t, err, ErrSupplierTariffUnavailable)

	require.NoError(t, DB.Where("group_id = ?", group.Id).Delete(&PricingGroupSupplierBinding{}).Error)
	_, bound, err = ResolveSupplierModelTariff("default", "missing-model")
	require.NoError(t, err)
	require.False(t, bound)
}

func TestSupplierPricingPreservesExplicitZeroAndRejectsIncompleteContracts(t *testing.T) {
	profile := supplierPricingTestDB(t)
	tariff := supplierTestTariff("free-input", 0)
	output := 2.0
	tariff.OutputUSD = &output
	_, err := PublishSupplierProfileTariffs(profile.ID, 1, 0, []types.SupplierModelTariff{tariff})
	require.NoError(t, err)
	tariffs, _, err := ListSupplierProfileTariffs(profile.ID)
	require.NoError(t, err)
	require.NotNil(t, tariffs[0].InputUSD)
	require.Zero(t, *tariffs[0].InputUSD)
	require.Nil(t, tariffs[0].CacheReadUSD)
	clone := tariffs[0].Clone()
	*clone.OutputUSD = 9
	require.Equal(t, 2.0, *tariffs[0].OutputUSD)
	tariff.OutputUSD = nil
	require.Error(t, ValidateSupplierModelTariff(tariff))
	tariff = supplierTestTariff("bad-tools", 1)
	tariff.Tools = map[string]float64{"unknown_operation": 1}
	require.Error(t, ValidateSupplierModelTariff(tariff))
}
