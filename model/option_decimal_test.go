package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionMapLoadsFractionalMinTopUp(t *testing.T) {
	originalMinTopUp := operation_setting.MinTopUp
	t.Cleanup(func() {
		operation_setting.MinTopUp = originalMinTopUp
	})

	require.NoError(t, updateOptionMapFromDatabase("MinTopUp", "0.1"))
	assert.Equal(t, 0.1, operation_setting.MinTopUp)
	assert.Equal(t, "0.1", common.OptionMap["MinTopUp"])
}

func TestManualCompleteTopUpUsesPersistedQuotaForFractionalAmount(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{
		Id:       901,
		Username: "fractional_topup_user",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:          901,
		Amount:          0,
		Money:           0.1,
		TradeNo:         "fractional-topup",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		QuotaToAdd:      50000,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}).Error)

	require.NoError(t, ManualCompleteTopUp("fractional-topup", "127.0.0.1"))

	var user User
	require.NoError(t, DB.First(&user, 901).Error)
	assert.Equal(t, 50000, user.Quota)
	assert.Equal(t, common.TopUpStatusSuccess, GetTopUpByTradeNo("fractional-topup").Status)
}

func TestTopUpPersistsAndSerializesFractionalRequestedAmount(t *testing.T) {
	truncateTables(t)
	topUp := &TopUp{
		UserId:          902,
		Amount:          0,
		RequestedAmount: 0.1,
		Money:           0.1,
		TradeNo:         "fractional-requested-amount",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		QuotaToAdd:      50000,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	stored := GetTopUpByTradeNo(topUp.TradeNo)
	require.NotNil(t, stored)
	assert.Equal(t, int64(0), stored.Amount)
	assert.Equal(t, 0.1, stored.RequestedAmount)

	payload, err := common.Marshal(stored)
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, common.Unmarshal(payload, &response))
	assert.Equal(t, 0.1, response["requested_amount"])
}
