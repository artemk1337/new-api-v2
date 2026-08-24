package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderSettlementsKeepValidZeroSnapshotCredit(t *testing.T) {
	testCases := []struct {
		name     string
		provider string
		method   string
		complete func(string) error
	}{
		{name: "stripe", provider: PaymentProviderStripe, method: PaymentMethodStripe, complete: func(tradeNo string) error { return Recharge(tradeNo, "cus_zero", "127.0.0.1") }},
		{name: "epay", provider: PaymentProviderEpay, method: "alipay", complete: func(tradeNo string) error { return RechargeEpay(tradeNo, "alipay", "127.0.0.1") }},
		{name: "waffo", provider: PaymentProviderWaffo, method: PaymentMethodWaffo, complete: func(tradeNo string) error { return RechargeWaffo(tradeNo, "127.0.0.1") }},
		{name: "waffo pancake", provider: PaymentProviderWaffoPancake, method: PaymentMethodWaffoPancake, complete: RechargeWaffoPancake},
		{name: "creem", provider: PaymentProviderCreem, method: PaymentMethodCreem, complete: func(tradeNo string) error {
			return RechargeCreem(tradeNo, "paid@example.com", "Paid User", "127.0.0.1")
		}},
		{name: "nowpayments", provider: PaymentProviderNOWPayments, method: PaymentMethodNOWPayments, complete: func(tradeNo string) error { return RechargeNOWPayments(tradeNo, "127.0.0.1") }},
	}

	for index, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			userID := 300 + index
			tradeNo := "zero-snapshot-" + tc.name
			insertUserForPaymentGuardTest(t, userID, 0)
			require.NoError(t, (&TopUp{
				UserId: userID, Amount: 10, RequestedAmount: 1, Money: 1.2,
				TradeNo: tradeNo, PaymentMethod: tc.method, PaymentProvider: tc.provider,
				PaymentCurrency: "USD", PaymentBaseAmount: 1, PaymentCommission: 1.2,
				PaymentChargedAmount: 1.2, QuotaToAdd: 0,
				Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
			}).Insert())

			require.NoError(t, tc.complete(tradeNo))
			assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, tradeNo))
			assert.Zero(t, getUserQuotaForPaymentGuardTest(t, userID))
		})
	}
}
