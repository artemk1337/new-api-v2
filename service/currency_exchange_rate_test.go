package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFetchCurrencyExchangeRateFromCBR(t *testing.T) {
	saved := currencyExchangeRateHTTPClient
	currencyExchangeRateHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, currencyExchangeRateCBRURL, request.URL.String())
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"Valute":{"USD":{"Nominal":1,"Value":78.2223}}}`)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { currencyExchangeRateHTTPClient = saved })

	quote, err := FetchCurrencyExchangeRate(context.Background(), currencyExchangeRateProviderCBR)
	require.NoError(t, err)
	assert.Equal(t, "USD", quote.BaseCurrency)
	assert.Equal(t, "RUB", quote.QuoteCurrency)
	assert.Equal(t, 78.2223, quote.Rate)
}

func TestFetchCurrencyExchangeRateForPairInvertsUSDTUSD(t *testing.T) {
	saved := currencyExchangeRateHTTPClient
	currencyExchangeRateHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, currencyExchangeRateCoinGeckoURL, request.URL.String())
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"tether":{"usd":1.002}}`)), Header: make(http.Header)}, nil
	})}
	t.Cleanup(func() { currencyExchangeRateHTTPClient = saved })
	quote, err := FetchCurrencyExchangeRateForPair(context.Background(), currencyExchangeRateProviderCoinGecko, "USD", "USDT")
	require.NoError(t, err)
	assert.InDelta(t, 1/1.002, quote.Rate, 0.000001)
}

func TestFetchCurrencyExchangeRateForPairUsesCBRQuotes(t *testing.T) {
	saved := currencyExchangeRateHTTPClient
	currencyExchangeRateHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"Valute":{"USD":{"Nominal":1,"Value":90},"EUR":{"Nominal":1,"Value":99}}}`)), Header: make(http.Header)}, nil
	})}
	t.Cleanup(func() { currencyExchangeRateHTTPClient = saved })
	quote, err := FetchCurrencyExchangeRateForPair(context.Background(), currencyExchangeRateProviderCBR, "USD", "EUR")
	require.NoError(t, err)
	assert.InDelta(t, 90.0/99.0, quote.Rate, 0.000001)
}

func TestFetchCurrencyExchangeRateForPairCBRDirectionContract(t *testing.T) {
	saved := currencyExchangeRateHTTPClient
	currencyExchangeRateHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"Valute":{"USD":{"Nominal":1,"Value":90},"EUR":{"Nominal":1,"Value":99}}}`)), Header: make(http.Header)}, nil
	})}
	t.Cleanup(func() { currencyExchangeRateHTTPClient = saved })
	rub, err := FetchCurrencyExchangeRateForPair(context.Background(), currencyExchangeRateProviderCBR, "USD", "RUB")
	require.NoError(t, err)
	assert.InDelta(t, 90, rub.Rate, 0.000001)
	eur, err := FetchCurrencyExchangeRateForPair(context.Background(), currencyExchangeRateProviderCBR, "USD", "EUR")
	require.NoError(t, err)
	assert.InDelta(t, 90.0/99.0, eur.Rate, 0.000001)
}

func TestCurrencyExchangeRateUpdateInterval(t *testing.T) {
	assert.Equal(t, time.Minute, currencyExchangeRateUpdateInterval("minute"))
	assert.Equal(t, time.Hour, currencyExchangeRateUpdateInterval("hour"))
	assert.Equal(t, 24*time.Hour, currencyExchangeRateUpdateInterval("day"))
	assert.Equal(t, 24*time.Hour, currencyExchangeRateUpdateInterval("unexpected"))
}

func TestCurrencyExchangeRateUpdateDueUsesCurrentInterval(t *testing.T) {
	now := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	lastAttempt := now.Add(-time.Minute)

	assert.True(t, currencyExchangeRateUpdateDue(time.Time{}, now, time.Hour))
	assert.False(t, currencyExchangeRateUpdateDue(lastAttempt, now, 24*time.Hour))
	assert.True(t, currencyExchangeRateUpdateDue(lastAttempt, now, time.Minute))
}

func TestPlatformCurrencySyncCandidatesOnlyIncludeSyncEnabled(t *testing.T) {
	candidates := platformCurrencySyncCandidates([]model.PlatformCurrency{
		{Code: "USD", SyncEnabled: false},
		{Code: "RUB", SyncEnabled: true},
		{Code: "EUR", SyncEnabled: false},
		{Code: "USDT", SyncEnabled: true},
	})
	assert.Equal(t, []string{"RUB", "USDT"}, candidates)
}

func TestUpdatePlatformCurrenciesSkipsOverlappingBatch(t *testing.T) {
	platformCurrencySyncRunning.Store(true)
	t.Cleanup(func() { platformCurrencySyncRunning.Store(false) })
	require.NoError(t, UpdatePlatformCurrencies(context.Background()))
}

func TestSyncPlatformCurrencyDiscardsOldProviderQuoteAfterConfigurationChange(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}, &model.CurrencyExchangeRate{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "RUB", Name: "Russian Ruble", Symbol: "₽", Enabled: true,
		SyncEnabled: true, SyncProvider: "cbr", ManualRateToUSD: 90, RateToUSD: 90,
	}).Error)

	previousFetch := currencyExchangeRateFetchForPair
	started := make(chan struct{})
	release := make(chan struct{})
	currencyExchangeRateFetchForPair = func(ctx context.Context, provider, baseCurrency, quoteCurrency string) (currencyExchangeRateQuote, error) {
		close(started)
		<-release
		return currencyExchangeRateQuote{BaseCurrency: baseCurrency, QuoteCurrency: quoteCurrency, Rate: 95}, nil
	}
	t.Cleanup(func() { currencyExchangeRateFetchForPair = previousFetch })

	done := make(chan error, 1)
	go func() { done <- SyncPlatformCurrency(context.Background(), "RUB") }()
	<-started

	require.NoError(t, db.Model(&model.PlatformCurrency{}).Where("code = ?", "RUB").Updates(map[string]interface{}{
		"sync_provider":   "replacement-provider",
		"rate_to_usd":     0,
		"last_sync_at":    nil,
		"last_sync_error": "",
	}).Error)
	close(release)
	require.NoError(t, <-done)

	updated, err := model.GetPlatformCurrency("RUB")
	require.NoError(t, err)
	assert.Equal(t, "replacement-provider", updated.SyncProvider)
	assert.Zero(t, updated.RateToUSD)
	assert.Nil(t, updated.LastSyncAt)
	var historyCount int64
	require.NoError(t, db.Model(&model.CurrencyExchangeRate{}).Where("quote_currency = ?", "RUB").Count(&historyCount).Error)
	assert.Zero(t, historyCount)
}

func TestSyncPlatformCurrencyKeepsCommittedRateDuringConcurrentAdminFieldEdit(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}, &model.CurrencyExchangeRate{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "RUB", Name: "Russian Ruble", Symbol: "₽", Enabled: true,
		SyncEnabled: true, SyncProvider: "cbr", ManualRateToUSD: 90, RateToUSD: 90,
	}).Error)

	previousFetch := currencyExchangeRateFetchForPair
	started := make(chan struct{})
	release := make(chan struct{})
	currencyExchangeRateFetchForPair = func(ctx context.Context, provider, baseCurrency, quoteCurrency string) (currencyExchangeRateQuote, error) {
		close(started)
		<-release
		return currencyExchangeRateQuote{BaseCurrency: baseCurrency, QuoteCurrency: quoteCurrency, Rate: 95}, nil
	}
	t.Cleanup(func() { currencyExchangeRateFetchForPair = previousFetch })

	done := make(chan error, 1)
	go func() { done <- SyncPlatformCurrency(context.Background(), "RUB") }()
	<-started

	adminCurrency, err := model.GetPlatformCurrency("RUB")
	require.NoError(t, err)
	adminCurrency.Name = "Russian Ruble (edited)"
	require.NoError(t, model.UpdatePlatformCurrencySettings(adminCurrency.Code, map[string]interface{}{
		"name": adminCurrency.Name,
	}, nil, ""))
	close(release)
	require.NoError(t, <-done)

	updated, err := model.GetPlatformCurrency("RUB")
	require.NoError(t, err)
	assert.Equal(t, "Russian Ruble (edited)", updated.Name)
	assert.Equal(t, 95.0, updated.RateToUSD)
	assert.NotNil(t, updated.LastSyncAt)
}

func TestStaleNormalAdminEditCannotRestoreSwitchedSyncProvider(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}, &model.CurrencyExchangeRate{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "RUB", Name: "Russian Ruble", Symbol: "₽", Enabled: true,
		SyncEnabled: true, SyncProvider: "cbr", ManualRateToUSD: 90, RateToUSD: 90,
	}).Error)

	staleEditor, err := model.GetPlatformCurrency("RUB")
	require.NoError(t, err)
	previousFetch := currencyExchangeRateFetchForPair
	started := make(chan struct{})
	release := make(chan struct{})
	currencyExchangeRateFetchForPair = func(ctx context.Context, provider, baseCurrency, quoteCurrency string) (currencyExchangeRateQuote, error) {
		close(started)
		<-release
		return currencyExchangeRateQuote{BaseCurrency: baseCurrency, QuoteCurrency: quoteCurrency, Rate: 95}, nil
	}
	t.Cleanup(func() { currencyExchangeRateFetchForPair = previousFetch })

	done := make(chan error, 1)
	go func() { done <- SyncPlatformCurrency(context.Background(), "RUB") }()
	<-started

	expectedEnabled := true
	require.NoError(t, model.UpdatePlatformCurrencySettings("RUB", map[string]interface{}{
		"sync_enabled":    true,
		"sync_provider":   "replacement-provider",
		"rate_to_usd":     0,
		"last_sync_at":    nil,
		"last_sync_error": "",
	}, &expectedEnabled, "cbr"))
	require.ErrorIs(t, model.UpdatePlatformCurrencySettings("RUB", map[string]interface{}{
		"sync_provider": "stale-provider",
	}, &expectedEnabled, "cbr"), model.ErrPlatformCurrencySyncConfigChanged)
	require.NoError(t, model.UpdatePlatformCurrencySettings(staleEditor.Code, map[string]interface{}{
		"name": "Russian Ruble (stale editor)",
	}, nil, ""))
	close(release)
	require.NoError(t, <-done)

	updated, err := model.GetPlatformCurrency("RUB")
	require.NoError(t, err)
	assert.Equal(t, "Russian Ruble (stale editor)", updated.Name)
	assert.Equal(t, "replacement-provider", updated.SyncProvider)
	assert.True(t, updated.SyncEnabled)
	assert.Zero(t, updated.RateToUSD)
	assert.Nil(t, updated.LastSyncAt)
	var historyCount int64
	require.NoError(t, db.Model(&model.CurrencyExchangeRate{}).Where("quote_currency = ?", "RUB").Count(&historyCount).Error)
	assert.Zero(t, historyCount)
}

func TestStaleManualRateUpdateCannotOverwriteEnabledSyncRate(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}, &model.CurrencyExchangeRate{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "RUB", Name: "Russian Ruble", Symbol: "₽", Enabled: true,
		SyncEnabled: false, ManualRateToUSD: 90, RateToUSD: 90,
	}).Error)

	staleEditor, err := model.GetPlatformCurrency("RUB")
	require.NoError(t, err)
	expectedDisabled := false
	require.NoError(t, model.UpdatePlatformCurrencySettings("RUB", map[string]interface{}{
		"sync_enabled":    true,
		"sync_provider":   "cbr",
		"rate_to_usd":     0,
		"last_sync_at":    nil,
		"last_sync_error": "",
	}, &expectedDisabled, ""))
	now := time.Now().UTC()
	require.NoError(t, model.CommitPlatformCurrencySyncQuote("RUB", "cbr", 95, now))
	require.ErrorIs(t, model.UpdatePlatformCurrencySettings(staleEditor.Code, map[string]interface{}{
		"manual_rate_to_usd": 91,
		"rate_to_usd":        91,
	}, &expectedDisabled, staleEditor.SyncProvider), model.ErrPlatformCurrencySyncConfigChanged)

	updated, err := model.GetPlatformCurrency("RUB")
	require.NoError(t, err)
	assert.True(t, updated.SyncEnabled)
	assert.Equal(t, "cbr", updated.SyncProvider)
	assert.Equal(t, 95.0, updated.RateToUSD)
	assert.NotNil(t, updated.LastSyncAt)
	assert.Equal(t, 90.0, updated.ManualRateToUSD)
}

func TestSyncPlatformCurrencyDiscardsOldProviderErrorAfterConfigurationChange(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}, &model.CurrencyExchangeRate{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "RUB", Name: "Russian Ruble", Symbol: "₽", Enabled: true,
		SyncEnabled: true, SyncProvider: "cbr", ManualRateToUSD: 90, RateToUSD: 90,
	}).Error)

	previousFetch := currencyExchangeRateFetchForPair
	started := make(chan struct{})
	release := make(chan struct{})
	currencyExchangeRateFetchForPair = func(ctx context.Context, provider, baseCurrency, quoteCurrency string) (currencyExchangeRateQuote, error) {
		close(started)
		<-release
		return currencyExchangeRateQuote{}, assert.AnError
	}
	t.Cleanup(func() { currencyExchangeRateFetchForPair = previousFetch })

	done := make(chan error, 1)
	go func() { done <- SyncPlatformCurrency(context.Background(), "RUB") }()
	<-started
	require.NoError(t, db.Model(&model.PlatformCurrency{}).Where("code = ?", "RUB").Updates(map[string]interface{}{
		"sync_provider":   "replacement-provider",
		"rate_to_usd":     0,
		"last_sync_at":    nil,
		"last_sync_error": "",
	}).Error)
	close(release)
	require.NoError(t, <-done)

	updated, err := model.GetPlatformCurrency("RUB")
	require.NoError(t, err)
	assert.Equal(t, "replacement-provider", updated.SyncProvider)
	assert.Zero(t, updated.RateToUSD)
	assert.Nil(t, updated.LastSyncAt)
	assert.Empty(t, updated.LastSyncError)
}

func TestGetPlatformCurrencyRateDoesNotUseHistoryAfterSyncInvalidation(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}, &model.CurrencyExchangeRate{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "RUB", Name: "Russian Ruble", Symbol: "₽", Enabled: true,
		SyncEnabled: true, SyncProvider: "cbr", RateToUSD: 0,
	}).Error)
	require.NoError(t, db.Create(&model.CurrencyExchangeRate{
		BaseCurrency: "USD", QuoteCurrency: "RUB", Provider: "cbr", Rate: 95, RecordedAt: time.Now().UTC(),
	}).Error)

	_, err = GetPlatformCurrencyRate("RUB")
	require.Error(t, err)
}

func TestParseBybitP2PUSDTRUBUsesMedianOfValidPrices(t *testing.T) {
	rate, err := parseBybitP2PUSDTRUB([]byte(`{
		"ret_code": 0,
		"ret_msg": "SUCCESS",
		"result": {"items": [
			{"price": "95.00"},
			{"price": "91.21"},
			{"price": "not-a-price"},
			{"price": "90.50"},
			{"price": "92.00"}
		]}
	}`))
	require.NoError(t, err)
	assert.InDelta(t, 91.605, rate, 0.000001)
}

func TestFetchCurrencyExchangeRateFromBybitP2P(t *testing.T) {
	saved := currencyExchangeRateHTTPClient
	currencyExchangeRateHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, currencyExchangeRateBybitP2PURL, request.URL.String())
		var payload bybitP2PRequest
		require.NoError(t, common.DecodeJson(request.Body, &payload))
		assert.Equal(t, "USDT", payload.TokenID)
		assert.Equal(t, "RUB", payload.CurrencyID)
		assert.Equal(t, currencyExchangeRateBybitP2PSideBuyUSDTForRUB, payload.Side)
		assert.Equal(t, currencyExchangeRateBybitP2PRequestSize, payload.Size)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ret_code":0,"result":{"items":[{"price":"91.00"},{"price":"92.00"},{"price":"93.00"}]}}`)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { currencyExchangeRateHTTPClient = saved })

	quote, err := FetchCurrencyExchangeRate(context.Background(), currencyExchangeRateProviderBybitP2P)
	require.NoError(t, err)
	assert.Equal(t, "USDT", quote.BaseCurrency)
	assert.Equal(t, "RUB", quote.QuoteCurrency)
	assert.Equal(t, 92.0, quote.Rate)
}
