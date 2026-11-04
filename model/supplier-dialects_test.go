package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This fixture has the legacy channel identity/routing columns, without either
// supplier column. The migration must preserve the row and its stable group ID.
type supplierDialectLegacyChannel struct {
	ID     int    `gorm:"primaryKey"`
	Type   int    `gorm:"default:0"`
	Key    string `gorm:"not null"`
	Name   string `gorm:"index"`
	Models string
	Group  string  `gorm:"type:varchar(64);default:'default'"`
	Tag    *string `gorm:"index"`
	Status int     `gorm:"default:1"`
}

func (supplierDialectLegacyChannel) TableName() string { return "channels" }

func openSupplierDialectTestDB(t *testing.T, envName string, expected common.DatabaseType) *gorm.DB {
	t.Helper()
	var db *gorm.DB
	var err error
	if envName == "" {
		db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "supplier.db")), &gorm.Config{})
	} else {
		if os.Getenv(envName) == "" {
			t.Skip(envName + " is not configured")
		}
		var actual common.DatabaseType
		db, actual, err = chooseDB(envName, false)
		require.NoError(t, err, "could not open the isolated supplier test database")
		require.Equal(t, expected, actual)
	}
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Silent)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	// A supplied DSN is never permission to reuse or truncate an existing DB.
	// Only a fresh dedicated database is accepted, before any schema writes.
	tables, err := db.Migrator().GetTables()
	require.NoError(t, err)
	require.Empty(t, tables, "supplier dialect tests require an empty isolated database")
	return db
}

func TestSupplierPersistenceDialects(t *testing.T) {
	for _, dialect := range []struct {
		name string
		env  string
		kind common.DatabaseType
	}{
		{"sqlite", "", common.DatabaseTypeSQLite},
		{"postgres", "SUPPLIER_TEST_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
		{"mysql", "SUPPLIER_TEST_MYSQL_DSN", common.DatabaseTypeMySQL},
	} {
		t.Run(dialect.name, func(t *testing.T) {
			db := openSupplierDialectTestDB(t, dialect.env, dialect.kind)
			previousDB := DB
			previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
			previousGroups := ratio_setting.PricingGroups2JSONString()
			previousMemoryCache := common.MemoryCacheEnabled
			DB = db
			common.SetMainDatabaseType(dialect.kind)
			common.MemoryCacheEnabled = false
			initCol()
			t.Cleanup(func() {
				DB = previousDB
				common.SetDatabaseTypes(previousMain, previousLog)
				common.MemoryCacheEnabled = previousMemoryCache
				initCol()
				require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups))
			})

			allTables := []any{&SupplierModelPrice{}, &SupplierPricingRevision{}, &SupplierPricingState{}, &PricingGroupSupplierBinding{}, &SupplierPricingProfile{}, &Supplier{}, &SupplierBindingState{}, &PricingSyncSource{}, &Midjourney{}, &Ability{}, &Channel{}, &Option{}}
			// The database was verified empty; these are exclusively this test's
			// tables. Cleanup makes repeated validation possible after failures.
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(allTables...)) })
			require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":2,"name":"coding","ratio":2,"selectable":true},{"id":3,"name":"other","ratio":2,"selectable":true}]`))
			require.NoError(t, DB.AutoMigrate(&supplierDialectLegacyChannel{}, &Ability{}, &Option{}))
			require.NoError(t, DB.Create(&Option{Key: "PricingGroups", Value: ratio_setting.PricingGroups2JSONString()}).Error)
			tag := "legacy-provider"
			legacy := supplierDialectLegacyChannel{ID: 41, Name: "legacy-channel", Key: "isolated-fixture-key", Models: "model-a", Group: "coding", Tag: &tag, Status: common.ChannelStatusEnabled}
			require.NoError(t, DB.Create(&legacy).Error)
			require.NoError(t, DB.Create(&Ability{Group: "coding", Model: "model-a", ChannelId: legacy.ID, Enabled: true}).Error)
			require.False(t, DB.Migrator().HasColumn(&Channel{}, "supplier_id"))
			require.NoError(t, ReplaceChannelGroupNamesWithIDs())
			var stableGroup string
			require.NoError(t, DB.Model(&supplierDialectLegacyChannel{}).Where("id = ?", legacy.ID).Pluck("group", &stableGroup).Error)
			require.Equal(t, "2", stableGroup)
			_, bound, err := ResolveSupplierModelTariff("2", "model-a")
			require.NoError(t, err, "a legacy database still uses its default pricing scope")
			require.False(t, bound)

			for pass := 0; pass < 2; pass++ {
				require.NoError(t, DB.AutoMigrate(&Channel{}, &Supplier{}, &SupplierPricingProfile{}, &PricingGroupSupplierBinding{}, &SupplierBindingState{}, &SupplierPricingState{}, &SupplierPricingRevision{}, &SupplierModelPrice{}, &Midjourney{}, &PricingSyncSource{}), "supplier migrations must be additive and repeatable")
			}
			loaded, err := GetChannelById(legacy.ID, true)
			require.NoError(t, err)
			require.Equal(t, legacy.ID, loaded.Id)
			require.Equal(t, "2", loaded.Group)
			require.Equal(t, legacy.Key, loaded.Key)
			require.Equal(t, legacy.Tag, loaded.Tag)
			require.Nil(t, loaded.SupplierID)
			require.Nil(t, loaded.SupplierProfileID)
			var ability Ability
			require.NoError(t, DB.Where("channel_id = ?", legacy.ID).First(&ability).Error)
			require.Equal(t, "2", ability.Group)

			supplier := Supplier{Code: "dialect-supplier", Name: "Dialect supplier", Adapter: "manual", Enabled: true}
			require.NoError(t, CreateSupplier(&supplier))
			first := SupplierPricingProfile{SupplierID: supplier.ID, Code: "first", Name: "First", Enabled: true, USDPerCredit: 0.006}
			second := SupplierPricingProfile{SupplierID: supplier.ID, Code: "second", Name: "Second", Enabled: true, USDPerCredit: 0.01}
			require.NoError(t, CreateSupplierPricingProfile(&first))
			require.NoError(t, CreateSupplierPricingProfile(&second))
			input, output, cache := 0.012, 0.06, 0.0012
			tariff := types.SupplierModelTariff{ModelName: "model-a", BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens", InputUSD: &input, OutputUSD: &output, CacheReadUSD: &cache}
			revision, err := PublishSupplierProfileTariffs(first.ID, first.ConfigRevision, 0, []types.SupplierModelTariff{tariff})
			require.NoError(t, err)
			require.EqualValues(t, 1, revision)
			_, err = PublishSupplierProfileTariffs(first.ID, first.ConfigRevision, 0, []types.SupplierModelTariff{tariff})
			require.ErrorIs(t, err, ErrSupplierPricingConflict)
			_, err = PublishSupplierProfileTariffs(first.ID, first.ConfigRevision+1, revision, []types.SupplierModelTariff{tariff})
			require.ErrorIs(t, err, ErrSupplierPricingConflict)
			tariffs, current, err := ListSupplierProfileTariffs(first.ID)
			require.NoError(t, err)
			require.EqualValues(t, 1, current)
			require.Len(t, tariffs, 1)
			require.Equal(t, input, *tariffs[0].InputUSD)

			newInput := 0.02
			updated := tariff.Clone()
			updated.InputUSD = &newInput
			revision, pending, err := ConfirmSupplierProfileTariffs(first.ID, first.ConfigRevision, 1, []types.SupplierModelTariff{updated})
			require.NoError(t, err)
			require.True(t, pending)
			require.EqualValues(t, 1, revision)
			revision, pending, err = ConfirmSupplierProfileTariffs(first.ID, first.ConfigRevision, 1, []types.SupplierModelTariff{updated})
			require.NoError(t, err)
			require.False(t, pending)
			require.EqualValues(t, 2, revision)

			groupPatch := SupplierGroupBindingPatch{GroupID: 2, SupplierID: &supplier.ID, ProfileID: &first.ID}
			channelPatch := SupplierChannelBindingPatch{ChannelID: legacy.ID, SupplierID: &supplier.ID, ProfileID: &first.ID}
			require.Error(t, ApplySupplierBindings([]SupplierGroupBindingPatch{groupPatch}, nil), "binding only the group must roll back against an unbound member channel")
			binding, err := GetPricingGroupSupplierBinding("2")
			require.NoError(t, err)
			require.Zero(t, binding.SupplierID)
			require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{groupPatch}, []SupplierChannelBindingPatch{channelPatch}))
			require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{groupPatch}, []SupplierChannelBindingPatch{channelPatch}), "idempotent same-value writes must work on MySQL")
			loaded, err = GetChannelById(legacy.ID, true)
			require.NoError(t, err)
			require.Equal(t, &supplier.ID, loaded.SupplierID)
			require.Equal(t, &first.ID, loaded.SupplierProfileID)
			require.NoError(t, ValidateChannelPricingGroup(loaded, "2"))
			require.Error(t, ValidateChannelPricingGroup(loaded, "1"))
			resolved, bound, err := ResolveSupplierModelTariff("2", "model-a")
			require.NoError(t, err)
			require.True(t, bound)
			require.Equal(t, first.ID, resolved.ProfileID)
			require.EqualValues(t, 2, resolved.Revision)
			require.Equal(t, newInput, *resolved.InputUSD)
			require.Equal(t, cache, *resolved.CacheReadUSD)

			wrongGroup := SupplierGroupBindingPatch{GroupID: 2, SupplierID: &supplier.ID, ProfileID: &second.ID}
			require.Error(t, ApplySupplierBindings([]SupplierGroupBindingPatch{wrongGroup}, []SupplierChannelBindingPatch{channelPatch}), "profiles of the same supplier must stay isolated")
			binding, err = GetPricingGroupSupplierBinding("coding")
			require.NoError(t, err)
			require.Equal(t, first.ID, binding.ProfileID, "failed rebinding must preserve the old group")
			loaded, err = GetChannelById(legacy.ID, true)
			require.NoError(t, err)
			require.Equal(t, &first.ID, loaded.SupplierProfileID, "failed rebinding must preserve the channel")
			invalid := Channel{Name: "wrong-profile", Key: "isolated-fixture-key", Models: "model-a", Group: "2", SupplierID: &supplier.ID, SupplierProfileID: &second.ID}
			require.Error(t, invalid.Insert())
			var invalidCount int64
			require.NoError(t, DB.Model(&Channel{}).Where("name = ?", invalid.Name).Count(&invalidCount).Error)
			require.Zero(t, invalidCount)
			_, bound, err = ResolveSupplierModelTariff("2", "missing-model")
			require.True(t, bound)
			require.ErrorIs(t, err, ErrSupplierTariffUnavailable)
			require.Error(t, DeleteSupplierPricingProfile(first.ID))
			require.Error(t, DeleteSupplier(supplier.ID))

			first.Enabled = false
			require.NoError(t, UpdateSupplierPricingProfile(&first))
			_, bound, err = ResolveSupplierModelTariff("2", "model-a")
			require.True(t, bound)
			require.ErrorIs(t, err, ErrSupplierTariffUnavailable, "disabled profiles cannot fall back to default pricing")
			require.Error(t, ValidateChannelPricingGroup(loaded, "2"))
			first.Enabled = true
			require.NoError(t, UpdateSupplierPricingProfile(&first))
			_, bound, err = ResolveSupplierModelTariff("2", "model-a")
			require.True(t, bound)
			require.ErrorIs(t, err, ErrSupplierTariffUnavailable, "reenabling a changed profile requires a current price revision")
			_, err = PublishSupplierProfileTariffs(first.ID, first.ConfigRevision, 2, []types.SupplierModelTariff{updated})
			require.NoError(t, err)
			_, bound, err = ResolveSupplierModelTariff("2", "model-a")
			require.NoError(t, err)
			require.True(t, bound)
			supplier.Enabled = false
			require.NoError(t, UpdateSupplier(&supplier))
			_, bound, err = ResolveSupplierModelTariff("2", "model-a")
			require.True(t, bound)
			require.ErrorIs(t, err, ErrSupplierTariffUnavailable, "disabled suppliers cannot fall back to default pricing")
			require.Error(t, DeleteSupplierPricingProfile(first.ID))
			_, bound, err = ResolveSupplierModelTariff("1", "model-a")
			require.NoError(t, err)
			require.False(t, bound, "an unbound group keeps its own default pricing scope")
		})
	}
}
