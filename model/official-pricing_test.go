package model

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func officialReferencePrice() OfficialModelPrice {
	input, output := 2.0, 10.0
	return OfficialModelPrice{
		ModelName: "developer-model", InputUSD: &input, OutputUSD: &output,
		Source: "https://platform.openai.com/docs/pricing#tokens", UpdatedAt: time.Now().Add(-time.Hour).Unix(), PriceUnit: "million_tokens",
	}
}

func setupOfficialPricingDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "official-pricing.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&OfficialModelPrice{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func TestOfficialModelPriceRequiresCompleteDatedPublicReference(t *testing.T) {
	zero := 0.0
	price := officialReferencePrice()
	price.InputUSD, price.OutputUSD = &zero, &zero
	require.NoError(t, ValidateOfficialModelPrice(price))
	for _, test := range []struct {
		name   string
		change func(*OfficialModelPrice)
	}{
		{"missing input", func(p *OfficialModelPrice) { p.InputUSD = nil }},
		{"missing output", func(p *OfficialModelPrice) { p.OutputUSD = nil }},
		{"negative price", func(p *OfficialModelPrice) { value := -1.0; p.InputUSD = &value }},
		{"not a number", func(p *OfficialModelPrice) { value := math.NaN(); p.OutputUSD = &value }},
		{"infinite price", func(p *OfficialModelPrice) { value := math.Inf(1); p.InputUSD = &value }},
		{"missing timestamp", func(p *OfficialModelPrice) { p.UpdatedAt = 0 }},
		{"future timestamp", func(p *OfficialModelPrice) { p.UpdatedAt = time.Now().Add(time.Hour).Unix() }},
		{"incomparable unit", func(p *OfficialModelPrice) { p.PriceUnit = "request" }},
		{"HTTP source", func(p *OfficialModelPrice) { p.Source = "http://platform.openai.com/pricing" }},
		{"credentialed source", func(p *OfficialModelPrice) { p.Source = "https://secret@platform.openai.com/pricing" }},
		{"loopback source", func(p *OfficialModelPrice) { p.Source = "https://127.0.0.1/pricing" }},
		{"private source", func(p *OfficialModelPrice) { p.Source = "https://192.168.1.1/pricing" }},
		{"abbreviated loopback", func(p *OfficialModelPrice) { p.Source = "https://127.1/pricing" }},
		{"encoded loopback", func(p *OfficialModelPrice) { p.Source = "https://0x7f.0.0.1/pricing" }},
		{"local source", func(p *OfficialModelPrice) { p.Source = "https://pricing.internal/pricing" }},
		{"missing model", func(p *OfficialModelPrice) { p.ModelName = "" }},
		{"ambiguous model", func(p *OfficialModelPrice) { p.ModelName = "model\nother" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			price := officialReferencePrice()
			test.change(&price)
			require.Error(t, ValidateOfficialModelPrice(price))
		})
	}
}

func TestOfficialModelPriceBatchIsAtomicAndPreservesExplicitZero(t *testing.T) {
	setupOfficialPricingDB(t)
	price := officialReferencePrice()
	require.NoError(t, SaveOfficialModelPrices([]OfficialModelPrice{price}))

	updated := officialReferencePrice()
	zero := 0.0
	updated.InputUSD, updated.OutputUSD = &zero, &zero
	invalid := officialReferencePrice()
	invalid.ModelName, invalid.OutputUSD = "invalid-model", nil
	require.Error(t, SaveOfficialModelPrices([]OfficialModelPrice{updated, invalid}))
	stored, err := ListOfficialModelPrices()
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, *price.InputUSD, *stored[0].InputUSD)

	require.NoError(t, SaveOfficialModelPrices([]OfficialModelPrice{updated}))
	stored, err = ListOfficialModelPrices()
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Zero(t, *stored[0].InputUSD)
	require.Zero(t, *stored[0].OutputUSD)
	require.Equal(t, updated.UpdatedAt, stored[0].UpdatedAt)

	duplicate := officialReferencePrice()
	duplicate.ModelName = " " + updated.ModelName + " "
	require.Error(t, SaveOfficialModelPrices([]OfficialModelPrice{updated, duplicate}))
}

func TestOfficialAPIPriceLookupIsExactAndSkipsInvalidStoredReferences(t *testing.T) {
	db := setupOfficialPricingDB(t)
	price := officialReferencePrice()
	require.NoError(t, SaveOfficialModelPrices([]OfficialModelPrice{price}))
	corrupt := officialReferencePrice()
	corrupt.ModelName, corrupt.Source = "corrupt-model", "https://localhost/pricing"
	require.NoError(t, db.Create(&corrupt).Error)

	prices, err := GetOfficialAPIPrices([]string{price.ModelName, "developer-model-alias", corrupt.ModelName})
	require.NoError(t, err)
	require.Len(t, prices, 1)
	require.Equal(t, *price.InputUSD, prices[price.ModelName].InputUSD)
	require.Equal(t, price.Source, prices[price.ModelName].Source)
	require.Equal(t, price.UpdatedAt, prices[price.ModelName].UpdatedAt)
	prices, err = GetOfficialAPIPrices(nil)
	require.NoError(t, err)
	require.Empty(t, prices)

	require.NoError(t, DeleteOfficialModelPrice(price.ModelName))
	prices, err = GetOfficialAPIPrices([]string{price.ModelName})
	require.NoError(t, err)
	require.Empty(t, prices)
	require.NoError(t, DeleteOfficialModelPrice(price.ModelName))
	require.Error(t, DeleteOfficialModelPrice(""))
}
