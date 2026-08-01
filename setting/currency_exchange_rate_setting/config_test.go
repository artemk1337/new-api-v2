package currency_exchange_rate_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateProvider(t *testing.T) {
	assert.NoError(t, ValidateProvider(ProviderBybitP2P))
	assert.NoError(t, ValidateProvider(ProviderCBR))
	assert.Error(t, ValidateProvider("unknown"))
}

func TestValidateUpdateInterval(t *testing.T) {
	for _, interval := range []string{UpdateIntervalMinute, UpdateIntervalHour, UpdateIntervalDay} {
		assert.NoError(t, ValidateUpdateInterval(interval))
	}
	assert.Error(t, ValidateUpdateInterval("week"))
}
