package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
)

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
