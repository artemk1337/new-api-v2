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
			previousModelPrices := ratio_setting.ModelPrice2JSONString()
			common.OptionMapRWMutex.RLock()
			previousModelPriceOption, hadModelPriceOption := common.OptionMap["ModelPrice"]
			common.OptionMapRWMutex.RUnlock()
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
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousModelPrices))
				common.OptionMapRWMutex.Lock()
				if hadModelPriceOption {
					common.OptionMap["ModelPrice"] = previousModelPriceOption
				} else {
					delete(common.OptionMap, "ModelPrice")
				}
				common.OptionMapRWMutex.Unlock()
			})

			allTables := []any{&OfficialModelPrice{}, &Model{}, &SupplierModelPrice{}, &SupplierPricingRevision{}, &SupplierPricingState{}, &PricingGroupSupplierBinding{}, &SupplierPricingProfile{}, &Supplier{}, &SupplierBindingState{}, &PricingSyncModelState{}, &PricingSyncSource{}, &Midjourney{}, &Ability{}, &Channel{}, &PlatformTag{}, &Option{}}
			// The database was verified empty; these are exclusively this test's
			// tables. Cleanup makes repeated validation possible after failures.
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(allTables...)) })
			require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":2,"name":"coding","ratio":2,"selectable":true,"tag":"legacy-group"},{"id":3,"name":"other","ratio":3,"selectable":true}]`))
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
			require.NoError(t, (&Channel{Id: legacy.ID, Name: "legacy-updated", Group: stableGroup}).Update(), "unbound channel edits must work before supplier columns are migrated")
			_, bound, err := ResolveSupplierModelTariff("2", "model-a")
			require.NoError(t, err, "a legacy database still uses its default pricing scope")
			require.False(t, bound)

			for pass := 0; pass < 2; pass++ {
				require.NoError(t, DB.AutoMigrate(&Channel{}, &PlatformTag{}, &Supplier{}, &SupplierPricingProfile{}, &PricingGroupSupplierBinding{}, &SupplierBindingState{}, &SupplierPricingState{}, &SupplierPricingRevision{}, &SupplierModelPrice{}, &Midjourney{}, &PricingSyncSource{}, &PricingSyncModelState{}, &Model{}, &OfficialModelPrice{}), "supplier migrations must be additive and repeatable")
				require.NoError(t, migratePlatformTags())
			}
			tags, err := GetPlatformTags()
			require.NoError(t, err)
			require.ElementsMatch(t, []string{tag, "legacy-group"}, tags, "legacy channel and group tags remain independent of supplier identity")
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
			syncSources := []PricingSyncSource{{ChannelID: legacy.ID, Enabled: true, Endpoint: "/api/pricing"}, {ChannelID: -100, Enabled: true, Endpoint: "https://basellm.github.io/llm-metadata/api/newapi/ratio_config-v1-base.json"}}
			require.NoError(t, SavePricingSyncSources(syncSources))
			require.NoError(t, ApplyJSONOptionPatches(map[string]JSONObjectPatch{"ModelPrice": {Set: map[string]any{"dialect-owned-price": 0.25}}}))
			require.NoError(t, DB.Create(&PricingSyncModelState{ModelName: "dialect-owned-price", Mode: PricingSyncModelModeChannel, ChannelID: -100, Provenance: "[-100]", Status: PricingSyncModelStatusReady}).Error)
			versionBefore, err := GetPricingSyncConfigVersion()
			require.NoError(t, err)
			require.Error(t, ApplySupplierBindings([]SupplierGroupBindingPatch{groupPatch}, nil), "binding only the group must roll back against an unbound member channel")
			versionAfterFailure, err := GetPricingSyncConfigVersion()
			require.NoError(t, err)
			require.Equal(t, versionBefore, versionAfterFailure)
			binding, err := GetPricingGroupSupplierBinding("2")
			require.NoError(t, err)
			require.Zero(t, binding.SupplierID)
			require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{groupPatch}, []SupplierChannelBindingPatch{channelPatch}))
			versionAfterBinding, err := GetPricingSyncConfigVersion()
			require.NoError(t, err)
			require.Equal(t, versionBefore+1, versionAfterBinding)
			require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{groupPatch}, []SupplierChannelBindingPatch{channelPatch}), "idempotent same-value writes must work on MySQL")
			versionAfterRepeat, err := GetPricingSyncConfigVersion()
			require.NoError(t, err)
			require.Equal(t, versionAfterBinding, versionAfterRepeat)
			require.ErrorIs(t, SavePricingSyncConfigurationIfVersion(nil, PricingSyncStrategyHighest, []int{-100}, syncSources, versionBefore), ErrPricingSyncConfigurationChanged)
			preservedSources, err := GetPricingSyncSources()
			require.NoError(t, err)
			require.ElementsMatch(t, syncSources, preservedSources, "a stale save must preserve supplier-bound and global sources")
			var preservedPrice Option
			require.NoError(t, DB.First(&preservedPrice, map[string]any{"key": "ModelPrice"}).Error)
			require.JSONEq(t, `{"dialect-owned-price":0.25}`, preservedPrice.Value, "a rejected configuration save must roll back its JSON price removal")
			loaded, err = GetChannelById(legacy.ID, true)
			require.NoError(t, err)
			require.Equal(t, &supplier.ID, loaded.SupplierID)
			require.Equal(t, &first.ID, loaded.SupplierProfileID)
			require.Equal(t, &tag, loaded.Tag, "supplier binding must preserve the legacy channel tag")
			tags, err = GetPlatformTags()
			require.NoError(t, err)
			require.ElementsMatch(t, []string{tag, "legacy-group"}, tags)
			require.Contains(t, ratio_setting.PricingGroups2JSONString(), `"tag":"legacy-group"`)
			require.NoError(t, ValidateChannelPricingGroup(loaded, "2"))
			require.Error(t, ValidateChannelPricingGroup(loaded, "1"))
			resolved, bound, err := ResolveSupplierModelTariff("2", "model-a")
			require.NoError(t, err)
			require.True(t, bound)
			require.Equal(t, first.ID, resolved.ProfileID)
			require.EqualValues(t, 2, resolved.Revision)
			require.Equal(t, newInput, *resolved.InputUSD)
			require.Equal(t, cache, *resolved.CacheReadUSD)

			// The public loader uses a read-only coherent snapshot and batched
			// joins. Exercise that query shape on each real database dialect.
			mapping := `{"public-alias":"model-a"}`
			loaded.Type, loaded.Models, loaded.ModelMapping = 1, "public-alias", &mapping
			require.NoError(t, loaded.UpdateWithSupplierPermission(true))
			require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{{GroupID: 3, SupplierID: &supplier.ID, ProfileID: &first.ID}}, nil))
			peer := Channel{Name: "public-peer", Key: "second-private-key", Type: 1, Models: "public-alias", Group: "3", ModelMapping: &mapping, SupplierID: &supplier.ID, SupplierProfileID: &first.ID, Status: common.ChannelStatusEnabled}
			require.NoError(t, peer.Insert())
			require.NoError(t, DB.Create(&Model{ModelName: "public-alias", Status: 1}).Error)
			inputReference, outputReference := 2.0, 10.0
			require.NoError(t, SaveOfficialModelPrices([]OfficialModelPrice{{ModelName: "public-alias", InputUSD: &inputReference, OutputUSD: &outputReference, Source: "https://platform.openai.com/docs/pricing", UpdatedAt: 1_700_000_000, PriceUnit: "million_tokens"}}))
			publicCatalog := func() []PublicPricingModel {
				t.Helper()
				catalog, err := GetPublicPricingCatalog(map[string]string{"2": "coding", "3": "other"}, map[string]float64{"2": 2, "3": 3})
				require.NoError(t, err)
				return catalog
			}
			catalog := publicCatalog()
			require.Len(t, catalog, 1)
			require.Equal(t, "public-alias", catalog[0].ModelName)
			require.Len(t, catalog[0].GroupPrices, 2)
			for index, multiplier := range []float64{2, 3} {
				price := catalog[0].GroupPrices[index]
				require.False(t, price.Unavailable)
				require.Equal(t, index+2, price.GroupID)
				require.InDelta(t, newInput*multiplier, *price.InputUSD, 1e-12)
				require.InDelta(t, output*multiplier, *price.OutputUSD, 1e-12)
				require.InDelta(t, cache*multiplier, *price.CacheReadUSD, 1e-12)
			}
			require.NotNil(t, catalog[0].OfficialPrice)
			require.Equal(t, inputReference, catalog[0].OfficialPrice.InputUSD)
			publicJSON, err := common.Marshal(catalog)
			require.NoError(t, err)
			for _, private := range []string{"supplier_id", "profile_id", "model-a", "isolated-fixture-key", "second-private-key", tag, "legacy-group"} {
				require.NotContains(t, string(publicJSON), private)
			}

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
			catalog = publicCatalog()
			require.Len(t, catalog, 1)
			for _, price := range catalog[0].GroupPrices {
				require.True(t, price.Unavailable, "invalidated procurement revisions cannot borrow default prices")
			}
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
			require.Empty(t, publicCatalog(), "disabled suppliers must not appear as usable default-price routes")
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
