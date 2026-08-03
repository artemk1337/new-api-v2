package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFractionalMinimumTopUpPreservesTokenConversion(t *testing.T) {
	originalMinTopUp := operation_setting.MinTopUp
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	t.Cleanup(func() {
		operation_setting.MinTopUp = originalMinTopUp
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountCashback = originalCashbacks
	})

	operation_setting.MinTopUp = 0.1
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	assert.Equal(t, 0.1, getMinTopup())
	assert.Equal(t, int(0.1*common.QuotaPerUnit), getTopUpQuotaToAdd(0.1))

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	assert.Equal(t, 0.1*common.QuotaPerUnit, getMinTopup())
	assert.Equal(t, 50000, getTopUpQuotaToAdd(50000))

	var request AmountRequest
	require.NoError(t, common.Unmarshal([]byte(`{"amount":0.1}`), &request))
	assert.Equal(t, 0.1, request.Amount)
}

func TestTopUpQuotaToAddIncludesHighestMatchingCashback(t *testing.T) {
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountCashback = originalCashbacks
	})

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.GetPaymentSetting().AmountCashback = operation_setting.AmountCashbackConfig{
		{MinAmount: 10, CashbackPercent: 1},
		{MinAmount: 100, CashbackPercent: 3},
	}

	assert.Equal(t, int(9*common.QuotaPerUnit), getTopUpQuotaToAdd(9))
	assert.Equal(t, int(10*common.QuotaPerUnit*1.01), getTopUpQuotaToAdd(10))
	assert.Equal(t, int(100*common.QuotaPerUnit*1.03), getTopUpQuotaToAdd(100))
}
