package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFractionalProviderRequestsAndPricing(t *testing.T) {
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalStripeUnitPrice := setting.StripeUnitPrice
	originalWaffoUnitPrice := setting.WaffoUnitPrice
	originalWaffoPancakeUnitPrice := setting.WaffoPancakeUnitPrice
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		setting.StripeUnitPrice = originalStripeUnitPrice
		setting.WaffoUnitPrice = originalWaffoUnitPrice
		setting.WaffoPancakeUnitPrice = originalWaffoPancakeUnitPrice
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
	})

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	setting.StripeUnitPrice = 8
	setting.WaffoUnitPrice = 1
	setting.WaffoPancakeUnitPrice = 1
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

	var stripeRequest StripePayRequest
	var waffoRequest WaffoPayRequest
	var pancakeRequest WaffoPancakePayRequest
	require.NoError(t, common.Unmarshal([]byte(`{"amount":0.1}`), &stripeRequest))
	require.NoError(t, common.Unmarshal([]byte(`{"amount":0.1}`), &waffoRequest))
	require.NoError(t, common.Unmarshal([]byte(`{"amount":0.1}`), &pancakeRequest))

	assert.Equal(t, 0.1, stripeRequest.Amount)
	assert.Equal(t, 0.1, waffoRequest.Amount)
	assert.Equal(t, 0.1, pancakeRequest.Amount)
	assert.InDelta(t, 0.8, getStripePayMoney(stripeRequest.Amount, "default"), 0.000001)
	assert.InDelta(t, 0.1, getWaffoPayMoney(waffoRequest.Amount, "default"), 0.000001)
	assert.InDelta(t, 0.1, getWaffoPancakePayMoney(pancakeRequest.Amount, "default"), 0.000001)

	fractionalStripeItem := newStripeTopUpLineItem(0.1, 0.8)
	require.NotNil(t, fractionalStripeItem.PriceData)
	assert.Nil(t, fractionalStripeItem.Price)
	assert.Equal(t, int64(80), *fractionalStripeItem.PriceData.UnitAmount)
	assert.Equal(t, int64(1), *fractionalStripeItem.Quantity)

	wholeStripeItem := newStripeTopUpLineItem(1, 1)
	assert.Nil(t, wholeStripeItem.PriceData)
	assert.Equal(t, setting.StripePriceId, *wholeStripeItem.Price)
	assert.Equal(t, int64(1), *wholeStripeItem.Quantity)
}

func TestTopUpPaymentAmountRepresentableInCents(t *testing.T) {
	assert.True(t, isTopUpPaymentAmountRepresentable(0.1, 2))
	assert.False(t, isTopUpPaymentAmountRepresentable(0.104, 2))
}
