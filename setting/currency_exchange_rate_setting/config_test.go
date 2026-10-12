package currency_exchange_rate_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateProvider(t *testing.T) {
	assert.NoError(t, ValidateProvider(ProviderBybitP2P))
	assert.NoError(t, ValidateProvider(ProviderCBR))
	assert.NoError(t, ValidateProvider(ProviderCoinGecko))
	assert.Error(t, ValidateProvider("unknown"))
}

func TestValidateUpdateInterval(t *testing.T) {
	for _, interval := range []string{UpdateIntervalMinute, UpdateIntervalHour, UpdateIntervalDay} {
		assert.NoError(t, ValidateUpdateInterval(interval))
	}
	assert.Error(t, ValidateUpdateInterval("week"))
}

func TestSupportedProvidersForCurrencyUsesUSDQuotes(t *testing.T) {
	assert.Equal(t, []string{ProviderCBR, ProviderBybitP2P}, SupportedProvidersForCurrency("rub"))
	assert.Equal(t, []string{ProviderCoinGecko}, SupportedProvidersForCurrency("USDT"))
	assert.Empty(t, SupportedProvidersForCurrency("USD"))
	assert.True(t, IsProviderSupportedForCurrency(ProviderBybitP2P, "RUB"))
}
