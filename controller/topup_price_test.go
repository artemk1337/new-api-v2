package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPayMoneyUsesPaymentMethodTopupGroup(t *testing.T) {
	originalPayMethods := operation_setting.PayMethods
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()
	originalPrice := operation_setting.Price
	t.Cleanup(func() {
		operation_setting.PayMethods = originalPayMethods
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
		operation_setting.Price = originalPrice
	})

	operation_setting.PayMethods = []map[string]string{{
		"type":        "yookassa_sbp",
		"topup_group": "sbp",
	}}
	operation_setting.Price = 80
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"vip":1.1,"sbp":1.1}`))

	assert.InDelta(t, 88, getPayMoney(1, "yookassa_sbp", "vip"), 0.0001)
	assert.InDelta(t, 80, getPayMoney(1, "unknown", "default"), 0.0001)
}
