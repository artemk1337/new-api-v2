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

func TestReferralDepositPercentOptionValidation(t *testing.T) {
	originalPercent := common.GetReferralDepositPercent()
	t.Cleanup(func() { common.SetReferralDepositPercent(originalPercent) })

	require.NoError(t, validateOptionValue("ReferralDepositPercent", "12.5"))
	require.Error(t, validateOptionValue("ReferralDepositPercent", "-1"))
	require.Error(t, validateOptionValue("ReferralDepositPercent", "100.1"))
	require.Error(t, validateOptionValue("ReferralDepositPercent", "not-a-number"))
	require.NoError(t, updateOptionMapFromDatabase("ReferralDepositPercent", "12.5"))
	assert.Equal(t, 12.5, common.GetReferralDepositPercent())
}

func TestReferralRequiredTopUpUSDOptionValidation(t *testing.T) {
	originalAmount := common.GetReferralRequiredTopUpUSD()
	t.Cleanup(func() { common.SetReferralRequiredTopUpUSD(originalAmount) })

	require.NoError(t, validateOptionValue("ReferralRequiredTopUpUSD", "125.5"))
	require.Error(t, validateOptionValue("ReferralRequiredTopUpUSD", "0"))
	require.Error(t, validateOptionValue("ReferralRequiredTopUpUSD", "-1"))
	require.Error(t, validateOptionValue("ReferralRequiredTopUpUSD", "NaN"))
	require.NoError(t, updateOptionMapFromDatabase("ReferralRequiredTopUpUSD", "125.5"))
	assert.Equal(t, 125.5, common.GetReferralRequiredTopUpUSD())
}

func TestUpdateOptionPersistsReferralRequiredTopUpUSD(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	originalAmount := common.GetReferralRequiredTopUpUSD()
	common.OptionMapRWMutex.RLock()
	originalMapValue, hadOriginalMapValue := common.OptionMap["ReferralRequiredTopUpUSD"]
	common.OptionMapRWMutex.RUnlock()
	t.Cleanup(func() {
		common.SetReferralRequiredTopUpUSD(originalAmount)
		common.OptionMapRWMutex.Lock()
		if hadOriginalMapValue {
			common.OptionMap["ReferralRequiredTopUpUSD"] = originalMapValue
		} else {
			delete(common.OptionMap, "ReferralRequiredTopUpUSD")
		}
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, UpdateOption("ReferralRequiredTopUpUSD", "250"))
	assert.Equal(t, 250.0, common.GetReferralRequiredTopUpUSD())
	var option Option
	require.NoError(t, DB.First(&option, "key = ?", "ReferralRequiredTopUpUSD").Error)
	assert.Equal(t, "250", option.Value)
}

func TestOptionMapLoadsHundredthMinTopUp(t *testing.T) {
	originalMinTopUp := operation_setting.MinTopUp
	originalOptionValue := common.OptionMap["MinTopUp"]
	t.Cleanup(func() {
		operation_setting.MinTopUp = originalMinTopUp
		common.OptionMap["MinTopUp"] = originalOptionValue
	})

	require.NoError(t, updateOptionMapFromDatabase("MinTopUp", "0.01"))
	assert.Equal(t, 0.01, operation_setting.MinTopUp)
	assert.Equal(t, "0.01", common.OptionMap["MinTopUp"])
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
