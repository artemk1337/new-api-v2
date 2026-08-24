package model

import (
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPendingTopUpTTLDefaultsAndOverrides(t *testing.T) {
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	originalMethods := operation_setting.PayMethods
	originalOptions := map[string]string{}
	for _, key := range []string{operation_setting.PaymentPendingTTLMinutes} {
		originalOptions[key] = common.OptionMap[key]
	}
	t.Cleanup(func() {
		operation_setting.PayMethods = originalMethods
		for key, value := range originalOptions {
			common.OptionMap[key] = value
		}
	})

	operation_setting.PayMethods = nil
	delete(common.OptionMap, operation_setting.PaymentPendingTTLMinutes)
	assert.Equal(t, 24*time.Hour, operation_setting.PendingTopUpTTL(PaymentMethodStripe))
	assert.Equal(t, 15*time.Minute, operation_setting.PendingTopUpTTL(PaymentMethodYooKassaSBP))

	operation_setting.PayMethods = []map[string]string{{"type": PaymentMethodStripe, "pending_ttl_minutes": "7"}}
	assert.Equal(t, 7*time.Minute, operation_setting.PendingTopUpTTL(PaymentMethodStripe))
	common.OptionMap[operation_setting.PaymentPendingTTLMinutes] = "9"
	assert.Equal(t, 7*time.Minute, operation_setting.PendingTopUpTTL(PaymentMethodStripe))
	assert.Equal(t, 9*time.Minute, operation_setting.PendingTopUpTTL(PaymentMethodNOWPayments))
	assert.Equal(t, 15*time.Minute, operation_setting.PendingTopUpTTL(PaymentMethodYooKassaSBP))

	common.OptionMap[operation_setting.PaymentPendingTTLMinutes] = strconv.Itoa(operation_setting.MaxPaymentPendingTTLMinutes + 1)
	assert.Equal(t, time.Duration(operation_setting.MaxPaymentPendingTTLMinutes)*time.Minute, operation_setting.PendingTopUpTTL(PaymentMethodNOWPayments))
	operation_setting.PayMethods = []map[string]string{{"type": PaymentMethodStripe, "pending_ttl_minutes": strconv.Itoa(operation_setting.MaxPaymentPendingTTLMinutes + 1)}}
	assert.Equal(t, time.Duration(operation_setting.MaxPaymentPendingTTLMinutes)*time.Minute, operation_setting.PendingTopUpTTL(PaymentMethodStripe))
}

func TestPaymentCreationRateLimitOptionsAreServerValidated(t *testing.T) {
	for _, key := range []string{
		operation_setting.PaymentCreationRateLimit,
		operation_setting.PaymentCreationRateLimitDurationMinutes,
	} {
		for _, value := range []string{"1", "5", "525600"} {
			if key == operation_setting.PaymentCreationRateLimitDurationMinutes && value == "525600" {
				require.Error(t, validateOptionValue(key, value), "%s=%s", key, value)
				continue
			}
			if key == operation_setting.PaymentCreationRateLimit && value == "525600" {
				require.Error(t, validateOptionValue(key, value), "%s=%s", key, value)
				continue
			}
			require.NoError(t, validateOptionValue(key, value), "%s=%s", key, value)
		}
		for _, value := range []string{"0", "-1", "1001", "525601", "one minute", "1.5"} {
			require.Error(t, validateOptionValue(key, value), "%s=%s", key, value)
		}
	}
	require.NoError(t, validateOptionValue(operation_setting.PaymentCreationRateLimit, "1000"))
	require.NoError(t, validateOptionValue(operation_setting.PaymentCreationRateLimitDurationMinutes, "60"))
}

func TestExpiredPendingTopUpCannotCreditOnCallback(t *testing.T) {
	truncateTables(t)
	userID := 9411
	insertUserForPaymentGuardTest(t, userID, 0)
	tradeNo := "expired-callback-topup"
	require.NoError(t, DB.Create(&TopUp{
		UserId: userID, Amount: 100, Money: 100, TradeNo: tradeNo,
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay,
		QuotaToAdd: 100, Status: common.TopUpStatusPending,
		CreateTime: time.Now().Add(-48 * time.Hour).Unix(),
	}).Error)

	err := RechargeEpay(tradeNo, "alipay", "127.0.0.1")
	assert.Error(t, err)
	assert.Equal(t, common.TopUpStatusExpired, GetTopUpByTradeNo(tradeNo).Status)
	assert.Zero(t, getUserQuotaForPaymentGuardTest(t, userID))
}
