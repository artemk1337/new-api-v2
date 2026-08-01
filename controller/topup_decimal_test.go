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
	t.Cleanup(func() {
		operation_setting.MinTopUp = originalMinTopUp
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
	})

	operation_setting.MinTopUp = 0.1
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	assert.Equal(t, 0.1, getMinTopup())
	assert.Equal(t, int(0.1*common.QuotaPerUnit), getYooKassaQuotaToAdd(0.1))

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	assert.Equal(t, 0.1*common.QuotaPerUnit, getMinTopup())
	assert.Equal(t, 50000, getYooKassaQuotaToAdd(50000))

	var request AmountRequest
	require.NoError(t, common.Unmarshal([]byte(`{"amount":0.1}`), &request))
	assert.Equal(t, 0.1, request.Amount)
}
