package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func publicPricingCatalogFixture(t *testing.T, count int) []SupplierPricingProfile {
	t.Helper()
	first := supplierPricingTestDB(t)
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}, &Model{}, &OfficialModelPrice{}))
	originalGroups := ratio_setting.PricingGroups2JSONString()
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":1001,"name":"First","ratio":2,"selectable":true},{"id":1002,"name":"Second","ratio":3,"selectable":true}]`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups)) })
	second := SupplierPricingProfile{SupplierID: first.SupplierID, Code: "second", Name: "Private Profile", Enabled: true, ConfigRevision: 1}
	require.NoError(t, DB.Create(&second).Error)
	profiles := []SupplierPricingProfile{first, second}
	mapping := make(map[string]string, count)
	names := make([]string, 0, count)
	for index := range count {
		alias := fmt.Sprintf("alias-%04d", index)
		mapping[alias] = fmt.Sprintf("billing-%04d", index)
		names = append(names, alias)
	}
	encoded, err := common.Marshal(mapping)
	require.NoError(t, err)
	mappingJSON := string(encoded)
	for index, profile := range profiles {
		group := fmt.Sprint(1001 + index)
		channel := Channel{Id: index + 1, Name: "Private channel", Key: "private-secret", Type: 1, SupplierID: &profile.SupplierID, SupplierProfileID: &profile.ID, Models: strings.Join(names, ","), Group: group, ModelMapping: &mappingJSON, Status: common.ChannelStatusEnabled}
		require.NoError(t, DB.Create(&channel).Error)
		abilities := make([]Ability, 0, count)
		tariffs := make([]types.SupplierModelTariff, 0, count)
		for _, alias := range names {
			abilities = append(abilities, Ability{Model: alias, Group: group, ChannelId: channel.Id, Enabled: true})
			tariff := supplierTestTariff(mapping[alias], float64(index+1))
			cache := float64(index) / 10
			tariff.CacheReadUSD = &cache
			tariff.Tools = map[string]float64{"web_search": float64(index) / 100, "file_search": float64(index+1) / 100}
			tariffs = append(tariffs, tariff)
		}
		require.NoError(t, DB.CreateInBatches(abilities, 100).Error)
		_, err := PublishSupplierProfileTariffs(profile.ID, 1, 0, tariffs)
		require.NoError(t, err)
		require.NoError(t, DB.Create(&PricingGroupSupplierBinding{GroupID: 1001 + index, SupplierID: profile.SupplierID, ProfileID: profile.ID}).Error)
	}
	return profiles
}

func TestPublicPricingCatalogBatchesQueriesAndKeepsGroupContracts(t *testing.T) {
	readCounts := make(map[int]int)
	for _, count := range []int{1, 100, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			publicPricingCatalogFixture(t, count)
			reads := 0
			countRead := func(*gorm.DB) { reads++ }
			require.NoError(t, DB.Callback().Query().Before("gorm:query").Register("public-pricing-query-count", countRead))
			require.NoError(t, DB.Callback().Row().Before("gorm:row").Register("public-pricing-row-count", countRead))
			catalog, err := GetPublicPricingCatalog(map[string]string{"1001": "First", "1002": "Second"}, map[string]float64{"1001": 2, "1002": 3})
			require.NoError(t, err)
			require.Len(t, catalog, count)
			readCounts[count] = reads
			for _, item := range catalog {
				require.Len(t, item.GroupPrices, 2)
				first, second := item.GroupPrices[0], item.GroupPrices[1]
				require.False(t, first.Unavailable)
				require.False(t, second.Unavailable)
				require.Equal(t, 2.0, *first.InputUSD)
				require.Equal(t, 6.0, *second.InputUSD)
				require.Zero(t, *first.CacheReadUSD)
				require.InDelta(t, 0.3, *second.CacheReadUSD, 1e-12)
				require.Zero(t, first.Tools["web_search"])
				require.Equal(t, 0.03, second.Tools["web_search"])
			}
			data, err := common.Marshal(catalog)
			require.NoError(t, err)
			for _, secret := range []string{"supplier_id", "profile_id", "billing-", "private-secret", "Private Profile"} {
				require.NotContains(t, string(data), secret)
			}
		})
	}
	require.Equal(t, readCounts[1], readCounts[100], "query count must not grow per model")
	require.LessOrEqual(t, readCounts[1000], readCounts[1]+4, "only bounded IN-clause batches may add queries")
}

func TestPublicPricingCatalogUsesCurrentRevisionWithoutDefaultFallback(t *testing.T) {
	profiles := publicPricingCatalogFixture(t, 1)
	usable, multipliers := map[string]string{"1001": "First", "1002": "Second"}, map[string]float64{"1001": 2, "1002": 3}
	load := func() []PublicPricingModel {
		t.Helper()
		catalog, err := GetPublicPricingCatalog(usable, multipliers)
		require.NoError(t, err)
		return catalog
	}
	updated := supplierTestTariff("billing-0000", 0)
	_, err := PublishSupplierProfileTariffs(profiles[0].ID, 1, 1, []types.SupplierModelTariff{updated})
	require.NoError(t, err)
	catalog := load()
	require.Zero(t, *catalog[0].GroupPrices[0].InputUSD)
	require.Nil(t, catalog[0].GroupPrices[0].CacheReadUSD)
	// A configuration edit invalidates the previously published revision.
	require.NoError(t, DB.Model(&SupplierPricingProfile{}).Where("id = ?", profiles[0].ID).Update("config_revision", 2).Error)
	catalog = load()
	require.True(t, catalog[0].GroupPrices[0].Unavailable)
	require.Equal(t, 6.0, *catalog[0].GroupPrices[1].InputUSD)
	// Missing model prices stay unavailable, even when another group serves the alias.
	require.NoError(t, DB.Where("profile_id = ?", profiles[1].ID).Delete(&SupplierModelPrice{}).Error)
	catalog = load()
	require.True(t, catalog[0].GroupPrices[1].Unavailable)
	// Missing expanded price tables must never turn bound groups into defaults.
	require.NoError(t, DB.Migrator().DropTable(&SupplierPricingRevision{}))
	require.Empty(t, load())
}

func TestPublicPricingCatalogSupportsLegacyUnboundSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous; _ = sqlDB.Close() })
	require.NoError(t, db.Exec("CREATE TABLE channels (id INTEGER PRIMARY KEY, type INTEGER, status INTEGER, model_mapping TEXT)").Error)
	require.NoError(t, db.AutoMigrate(&Ability{}, &Model{}))
	require.NoError(t, db.Exec("INSERT INTO channels (id, type, status) VALUES (1, 1, 1)").Error)
	originalGroups, originalRatios := ratio_setting.PricingGroups2JSONString(), ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalRatios))
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":1001,"name":"Legacy","ratio":2,"selectable":true}]`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"legacy-model":1}`))
	require.NoError(t, db.Create(&Ability{Model: "legacy-model", Group: "1001", ChannelId: 1, Enabled: true}).Error)
	catalog, err := GetPublicPricingCatalog(map[string]string{"1001": "Legacy"}, map[string]float64{"1001": 2})
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.False(t, catalog[0].GroupPrices[0].Unavailable)
	require.NotNil(t, catalog[0].GroupPrices[0].InputUSD)
}
