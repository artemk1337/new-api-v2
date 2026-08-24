package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPaymentMethodCurrencyTestDB(t *testing.T, value string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: value}).Error)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestGetPaymentMethodCurrencyUsesProviderCurrencyForYooKassa(t *testing.T) {
	assert.Equal(t, "RUB", getPaymentMethodCurrency(map[string]string{
		"type": model.PaymentMethodYooKassaSBP,
	}))
	// A stale legacy value must not leak into metadata for a fixed gateway.
	assert.Equal(t, "RUB", getPaymentMethodCurrency(map[string]string{
		"type":     model.PaymentMethodYooKassaSBP,
		"currency": "USD",
	}))
}

func TestGetPaymentMethodCurrencyPreservesLegacyConfiguredCurrency(t *testing.T) {
	setupPaymentMethodCurrencyTestDB(t, `[{"type":"alipay"}]`)
	previous := operation_setting.PayMethods
	previousWaffoCurrency := setting.WaffoCurrency
	t.Cleanup(func() {
		operation_setting.PayMethods = previous
		setting.WaffoCurrency = previousWaffoCurrency
	})

	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	assert.Equal(t, "USD", getPaymentMethodCurrency(map[string]string{"type": "alipay"}))
	assert.Equal(t, "USD", getPaymentMethodCurrency(map[string]string{
		"type":     "alipay",
		"currency": "eur",
	}))
	assert.Equal(t, "EUR", getPaymentMethodCurrency(map[string]string{
		"type":     "unregistered",
		"currency": "eur",
	}))

	setting.WaffoCurrency = "rub"
	assert.Equal(t, "RUB", getPaymentMethodCurrency(map[string]string{
		"type": model.PaymentMethodWaffo,
	}))
}
