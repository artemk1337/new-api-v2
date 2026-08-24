package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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
	originalPayMethods := operation_setting.PayMethods
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
		operation_setting.PayMethods = originalPayMethods
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

	operation_setting.PayMethods = []map[string]string{{"type": model.PaymentMethodYooKassaSBP}}
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

func TestActivePaymentCurrencyDependenciesUsesPersistedPayMethods(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	for _, option := range []model.Option{
		{Key: "payment_setting.compliance_confirmed", Value: "true"},
		{Key: "YooKassaEnabled", Value: "true"},
		{Key: "YooKassaShopID", Value: "shop"},
		{Key: "YooKassaSecretKey", Value: "secret"},
		// This legacy option is intentionally enabled. PayMethods is the
		// source of truth for the currently exposed checkout methods.
		{Key: "YooKassaPaymentMethods", Value: "sbp"},
		{Key: "PayMethods", Value: `[{"type":"alipay"}]`},
	} {
		require.NoError(t, db.Create(&option).Error)
	}

	originalMethods := operation_setting.PayMethods
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	t.Cleanup(func() { operation_setting.PayMethods = originalMethods })

	dependencies := activePaymentCurrencyDependenciesFromDB(db, "RUB")
	assert.NotContains(t, dependencies, "YooKassa SBP")

	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "PayMethods").Update("value", `[{"type":"alipay"},{"type":"yookassa_sbp"}]`).Error)
	dependencies = activePaymentCurrencyDependenciesFromDB(db, "RUB")
	assert.Contains(t, dependencies, "YooKassa SBP")
}
