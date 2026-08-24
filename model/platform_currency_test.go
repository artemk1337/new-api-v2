package model

import (
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPlatformCurrencyDefaultsAndLatestRate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PlatformCurrency{}, &CurrencyExchangeRate{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	require.NoError(t, SeedDefaultPlatformCurrencies())
	got, err := GetPlatformCurrency("rub")
	require.NoError(t, err)
	assert.Equal(t, "RUB", got.Code)
	assert.Equal(t, 90.0, got.ManualRateToUSD)
	require.NoError(t, DB.Model(&PlatformCurrency{}).Where("code = ?", "RUB").Updates(map[string]interface{}{
		"sync_enabled":  true,
		"sync_provider": "bybit_p2p",
	}).Error)
	require.NoError(t, SeedDefaultPlatformCurrencies())
	got, err = GetPlatformCurrency("RUB")
	require.NoError(t, err)
	assert.True(t, got.SyncEnabled)
	assert.Equal(t, "cbr", got.SyncProvider)
	now := time.Now().UTC()
	require.NoError(t, CreateCurrencyExchangeRate(&CurrencyExchangeRate{BaseCurrency: "USD", QuoteCurrency: "RUB", Provider: "cbr", Rate: 91, RecordedAt: now}))
	require.NoError(t, CreateCurrencyExchangeRate(&CurrencyExchangeRate{BaseCurrency: "USD", QuoteCurrency: "RUB", Provider: "cbr", Rate: 92, RecordedAt: now.Add(time.Second)}))
	latest, err := GetLatestCurrencyExchangeRateByProvider("usd", "rub", "cbr")
	require.NoError(t, err)
	assert.Equal(t, 92.0, latest.Rate)
	assert.Error(t, DeletePlatformCurrency("USD"))
}

func TestNormalizePlatformCurrencyDropsUnsupportedLegacyProvider(t *testing.T) {
	currency := PlatformCurrency{
		Code:         "rub",
		Name:         "Russian Ruble",
		Symbol:       "₽",
		SyncEnabled:  true,
		SyncProvider: "bybit_p2p",
	}

	NormalizePlatformCurrency(&currency)

	assert.True(t, currency.SyncEnabled)
	assert.Equal(t, "cbr", currency.SyncProvider)
}

func TestNormalizePlatformCurrencyDisablesUnsupportedCurrencySync(t *testing.T) {
	currency := PlatformCurrency{
		Code:         "XYZ",
		Name:         "Unknown",
		Symbol:       "¤",
		SyncEnabled:  true,
		SyncProvider: "cbr",
	}

	NormalizePlatformCurrency(&currency)

	assert.False(t, currency.SyncEnabled)
	assert.Empty(t, currency.SyncProvider)
}

func TestSeedDefaultPlatformCurrenciesReenablesLegacyUSD(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PlatformCurrency{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	require.NoError(t, db.Create(&PlatformCurrency{
		Code: "USD", Name: "US Dollar", Symbol: "$", Enabled: false, ManualRateToUSD: 0, RateToUSD: 0,
	}).Error)

	require.NoError(t, SeedDefaultPlatformCurrencies())
	usd, err := GetPlatformCurrency("USD")
	require.NoError(t, err)
	assert.True(t, usd.Enabled)
	assert.Equal(t, 1.0, usd.ManualRateToUSD)
	assert.Equal(t, 1.0, usd.RateToUSD)
}

func TestSeedDefaultPlatformCurrenciesIsConcurrentAndIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PlatformCurrency{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	const callers = 8
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() { errs <- SeedDefaultPlatformCurrencies() }()
	}
	for i := 0; i < callers; i++ {
		require.NoError(t, <-errs)
	}
	var count int64
	require.NoError(t, db.Model(&PlatformCurrency{}).Count(&count).Error)
	assert.EqualValues(t, 5, count)
}

func TestPlatformCurrencyGuardChecksBeforeMutation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PlatformCurrency{}))
	require.NoError(t, db.Create(&PlatformCurrency{Code: "USD", Name: "US Dollar", Symbol: "$", Enabled: true, ManualRateToUSD: 1, RateToUSD: 1}).Error)
	require.NoError(t, db.Create(&PlatformCurrency{Code: "EUR", Name: "Euro", Symbol: "€", Enabled: true, ManualRateToUSD: 0.92, RateToUSD: 0.92}).Error)
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	called := false
	err = UpdatePlatformCurrencySettingsWithGuard("EUR", map[string]interface{}{"enabled": false}, nil, "", func() error {
		called = true
		return errors.New("payment dependency appeared")
	})
	require.Error(t, err)
	assert.True(t, called)
	currency, getErr := GetPlatformCurrency("EUR")
	require.NoError(t, getErr)
	assert.True(t, currency.Enabled)
}
