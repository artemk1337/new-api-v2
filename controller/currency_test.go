package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePlatformCurrencyAllowsZeroManualRateForSyncedCurrency(t *testing.T) {
	currency := &model.PlatformCurrency{
		Code: "RUB", Name: "Russian Ruble", Symbol: "₽",
		SyncEnabled: true, SyncProvider: "cbr", ManualRateToUSD: 0,
	}
	require.NoError(t, validatePlatformCurrency(currency))

	currency.SyncEnabled = false
	model.NormalizePlatformCurrency(currency)
	require.Error(t, validatePlatformCurrency(currency))
}

func TestInvalidatePlatformCurrencySyncStateOnProviderChange(t *testing.T) {
	lastSync := time.Now().UTC()
	currency := &model.PlatformCurrency{
		Code: "RUB", Name: "Russian Ruble", Symbol: "₽",
		SyncEnabled: true, SyncProvider: "cbr", RateToUSD: 95,
		LastSyncAt: &lastSync, LastSyncError: "old provider error",
	}

	currency.SyncProvider = "replacement-provider"
	invalidatePlatformCurrencySyncState(currency)

	assert.Zero(t, currency.RateToUSD)
	assert.Nil(t, currency.LastSyncAt)
	assert.Empty(t, currency.LastSyncError)
}

func TestValidatePlatformCurrencyRejectsDisabledUSD(t *testing.T) {
	currency := &model.PlatformCurrency{
		Code: "USD", Name: "US Dollar", Symbol: "$", Enabled: false,
		ManualRateToUSD: 1, RateToUSD: 1,
	}
	require.Error(t, validatePlatformCurrency(currency))
	currency.Enabled = true
	require.NoError(t, validatePlatformCurrency(currency))
}

func TestActivePaymentCurrencyDependencies(t *testing.T) {
	originalYooKassaConfig := setting.GetYooKassaConfig()
	originalNOWEnabled := setting.NOWPaymentsEnabled
	originalNOWAPIKey := setting.NOWPaymentsAPIKey
	originalNOWSecret := setting.NOWPaymentsIPNSecret
	originalNOWCallback := setting.NOWPaymentsIPNCallbackURL
	originalWaffoEnabled := setting.WaffoEnabled
	originalWaffoSandbox := setting.WaffoSandbox
	originalWaffoAPIKey := setting.WaffoSandboxApiKey
	originalWaffoPrivateKey := setting.WaffoSandboxPrivateKey
	originalWaffoPublicCert := setting.WaffoSandboxPublicCert
	originalWaffoCurrency := setting.WaffoCurrency
	t.Cleanup(func() {
		setting.PublishYooKassaConfig(originalYooKassaConfig)
		setting.NOWPaymentsEnabled = originalNOWEnabled
		setting.NOWPaymentsAPIKey = originalNOWAPIKey
		setting.NOWPaymentsIPNSecret = originalNOWSecret
		setting.NOWPaymentsIPNCallbackURL = originalNOWCallback
		setting.WaffoEnabled = originalWaffoEnabled
		setting.WaffoSandbox = originalWaffoSandbox
		setting.WaffoSandboxApiKey = originalWaffoAPIKey
		setting.WaffoSandboxPrivateKey = originalWaffoPrivateKey
		setting.WaffoSandboxPublicCert = originalWaffoPublicCert
		setting.WaffoCurrency = originalWaffoCurrency
	})

	setting.PublishYooKassaConfig(setting.YooKassaConfig{Enabled: true, ShopID: "shop", SecretKey: "secret", PaymentMethods: "sbp"})
	require.Error(t, ensurePlatformCurrencyCanBeDisabledOrDeleted("RUB"))

	setting.NOWPaymentsEnabled = true
	setting.NOWPaymentsAPIKey = "api-key"
	setting.NOWPaymentsIPNSecret = "ipn-secret"
	setting.NOWPaymentsIPNCallbackURL = "https://example.test/ipn"
	require.Error(t, ensurePlatformCurrencyCanBeDisabledOrDeleted("USDT"))

	config := setting.GetYooKassaConfig()
	config.Enabled = false
	setting.PublishYooKassaConfig(config)
	setting.WaffoEnabled = true
	setting.WaffoSandbox = true
	setting.WaffoSandboxApiKey = "api-key"
	setting.WaffoSandboxPrivateKey = "private-key"
	setting.WaffoSandboxPublicCert = "public-cert"
	setting.WaffoCurrency = "EUR"
	require.Error(t, ensurePlatformCurrencyCanBeDisabledOrDeleted("EUR"))

	setting.WaffoEnabled = false
	require.NoError(t, ensurePlatformCurrencyCanBeDisabledOrDeleted("EUR"))
}
