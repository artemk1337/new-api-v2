package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestSnapshotTopUpMissingUserIsTerminalAcrossProviders(t *testing.T) {
	providers := []struct {
		name     string
		provider string
		settle   func(string) error
	}{
		{name: "waffo", provider: PaymentProviderWaffo, settle: func(tradeNo string) error {
			return RechargeWaffo(tradeNo, "127.0.0.1")
		}},
		{name: "waffo pancake", provider: PaymentProviderWaffoPancake, settle: RechargeWaffoPancake},
		{name: "nowpayments", provider: PaymentProviderNOWPayments, settle: func(tradeNo string) error {
			return RechargeNOWPayments(tradeNo, "127.0.0.1")
		}},
	}

	for index, tc := range providers {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			tradeNo := fmt.Sprintf("deleted-user-snapshot-%d", index)
			require.NoError(t, (&TopUp{
				UserId:               9900 + index,
				RequestedAmount:      10,
				Money:                10,
				TradeNo:              tradeNo,
				PaymentMethod:        tc.provider,
				PaymentProvider:      tc.provider,
				PaymentCurrency:      "USD",
				PaymentBaseAmount:    10,
				PaymentChargedAmount: 10,
				PaymentCoefficient:   1,
				QuotaToAdd:           1000,
				CreateTime:           time.Now().Unix(),
				Status:               common.TopUpStatusPending,
			}).Insert())

			err := tc.settle(tradeNo)
			require.ErrorIs(t, err, ErrTopUpUserNotFound)
			asserted := GetTopUpByTradeNo(tradeNo)
			require.NotNil(t, asserted)
			require.Equal(t, common.TopUpStatusFailed, asserted.Status)
		})
	}
}

func TestSnapshotSubscriptionMissingUserIsTerminal(t *testing.T) {
	truncateTables(t)
	plan := &SubscriptionPlan{
		Id: 9950, Title: "Deleted-user plan", PriceAmount: 10, Currency: "USD",
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1, StripePriceId: "price_deleted_user",
		Enabled: true,
	}
	snapshot, err := NewSubscriptionOrderSnapshot(plan, PaymentProviderStripe)
	require.NoError(t, err)
	order := &SubscriptionOrder{
		UserId: 9951, PlanId: plan.Id, Money: 10, TradeNo: "deleted-user-subscription",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(), PlanSnapshot: snapshot,
	}
	require.NoError(t, order.Insert())

	err = CompleteSubscriptionOrder(order.TradeNo, `{"amount_total":"1000","currency":"USD","stripe_price_id":"price_deleted_user"}`, PaymentProviderStripe, "")
	require.ErrorIs(t, err, ErrSubscriptionOrderUserNotFound)
	require.True(t, IsPermanentSubscriptionOrderError(err))
	require.Equal(t, common.TopUpStatusFailed, GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
}
