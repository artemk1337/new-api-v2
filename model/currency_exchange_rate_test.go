package model

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCurrencyExchangeRateSupportsUSDT(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&CurrencyExchangeRate{}))

	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	require.NoError(t, CreateCurrencyExchangeRate(&CurrencyExchangeRate{
		BaseCurrency:  "USDT",
		QuoteCurrency: "RUB",
		Provider:      "bybit_p2p",
		Rate:          92.5,
		RecordedAt:    time.Date(2026, time.August, 1, 12, 0, 0, 0, time.FixedZone("UTC+3", 3*60*60)),
	}))

	var stored CurrencyExchangeRate
	require.NoError(t, db.First(&stored).Error)
	assert.Equal(t, "USDT", stored.BaseCurrency)
	assert.Equal(t, "RUB", stored.QuoteCurrency)
	assert.Equal(t, time.UTC, stored.RecordedAt.Location())

	var columns []struct {
		Name string
		Type string
	}
	require.NoError(t, db.Raw("PRAGMA table_info(currency_exchange_rates)").Scan(&columns).Error)
	currencyColumnCount := 0
	for _, column := range columns {
		if column.Name == "base_currency" || column.Name == "quote_currency" {
			currencyColumnCount++
			assert.Equal(t, "varchar(4)", strings.ToLower(column.Type))
		}
	}
	assert.Equal(t, 2, currencyColumnCount)
}
