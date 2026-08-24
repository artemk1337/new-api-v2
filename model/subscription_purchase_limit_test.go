package model

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCreatePendingSubscriptionOrderReservesMaxPurchaseSlot(t *testing.T) {
	truncateTables(t)
	userID, planID := 6401, 6401
	require.NoError(t, DB.Create(&User{Id: userID, Username: "pending-limit", Status: common.UserStatusEnabled}).Error)
	first := &SubscriptionOrder{UserId: userID, PlanId: planID, Money: 10, TradeNo: "pending-limit-1", PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
	require.NoError(t, CreatePendingSubscriptionOrder(first, 1))
	second := &SubscriptionOrder{UserId: userID, PlanId: planID, Money: 10, TradeNo: "pending-limit-2", PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
	require.ErrorIs(t, CreatePendingSubscriptionOrder(second, 1), ErrSubscriptionPurchaseLimit)
}

func TestCreatePendingSubscriptionOrderExpiresOnlyStaleReservations(t *testing.T) {
	truncateTables(t)
	userID, planID := 6402, 6402
	require.NoError(t, DB.Create(&User{Id: userID, Username: "stale-limit", Status: common.UserStatusEnabled}).Error)
	stale := &SubscriptionOrder{UserId: userID, PlanId: planID, Money: 10, TradeNo: "pending-stale", PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending, CreateTime: time.Now().Add(-pendingSubscriptionOrderTTL - time.Minute).Unix()}
	require.NoError(t, DB.Create(stale).Error)
	newOrder := &SubscriptionOrder{UserId: userID, PlanId: planID, Money: 10, TradeNo: "pending-after-stale", PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
	require.NoError(t, CreatePendingSubscriptionOrder(newOrder, 1))
	asserted := GetSubscriptionOrderByTradeNo("pending-stale")
	require.NotNil(t, asserted)
	require.Equal(t, common.TopUpStatusExpired, asserted.Status)
}

func TestCreatePendingSubscriptionOrderConcurrentReservation(t *testing.T) {
	truncateTables(t)
	userID, planID := 6403, 6403
	require.NoError(t, DB.Create(&User{Id: userID, Username: "parallel-limit", Status: common.UserStatusEnabled}).Error)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results <- CreatePendingSubscriptionOrder(&SubscriptionOrder{UserId: userID, PlanId: planID, Money: 10, TradeNo: fmt.Sprintf("pending-parallel-%d", index), PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}, 1)
		}(i)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		require.ErrorIs(t, err, ErrSubscriptionPurchaseLimit)
	}
	require.Equal(t, 1, successes)
}

func TestCreateUserSubscriptionFromPlanReturnsTypedPurchaseLimit(t *testing.T) {
	truncateTables(t)
	userID, planID := 6404, 6404
	require.NoError(t, DB.Create(&User{Id: userID, Username: "completion-limit", Status: common.UserStatusEnabled}).Error)
	plan := &SubscriptionPlan{Id: planID, Title: "limited", PriceAmount: 10, Currency: "USD", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, MaxPurchasePerUser: 1}
	plan.NormalizeDefaults()
	require.NoError(t, DB.Create(plan).Error)
	require.NoError(t, DB.Create(&UserSubscription{UserId: userID, PlanId: planID, StartTime: time.Now().Unix(), EndTime: time.Now().Add(time.Hour).Unix()}).Error)

	_, err := CreateUserSubscriptionFromPlanTx(DB, userID, plan, "order")
	require.ErrorIs(t, err, ErrSubscriptionPurchaseLimit)
	require.True(t, IsPermanentSubscriptionOrderError(err))
}

func TestCompleteSubscriptionOrderMarksPaidLimitOrderTerminal(t *testing.T) {
	truncateTables(t)
	userID, planID := 6405, 6405
	require.NoError(t, DB.Create(&User{Id: userID, Username: "completion-limit-order", Status: common.UserStatusEnabled}).Error)
	plan := &SubscriptionPlan{Id: planID, Title: "limited", PriceAmount: 10, Currency: "USD", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, MaxPurchasePerUser: 1, StripePriceId: "price-limit"}
	plan.NormalizeDefaults()
	require.NoError(t, DB.Create(plan).Error)
	require.NoError(t, DB.Create(&UserSubscription{UserId: userID, PlanId: planID, StartTime: time.Now().Unix(), EndTime: time.Now().Add(time.Hour).Unix()}).Error)
	snapshot, err := NewSubscriptionOrderSnapshot(plan, PaymentProviderStripe)
	require.NoError(t, err)
	order := &SubscriptionOrder{UserId: userID, PlanId: planID, Money: 10, TradeNo: "paid-limit-order", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending, PlanSnapshot: snapshot}
	require.NoError(t, order.Insert())

	err = CompleteSubscriptionOrder(order.TradeNo, `{"amount_total":"1000","currency":"usd","stripe_price_id":"price-limit"}`, PaymentProviderStripe, "")
	require.ErrorIs(t, err, ErrSubscriptionPurchaseLimit)
	asserted := GetSubscriptionOrderByTradeNo(order.TradeNo)
	require.NotNil(t, asserted)
	require.Equal(t, common.TopUpStatusFailed, asserted.Status)
}

func TestRefundSubscriptionQuotaRollsBackWithRefundRecordFailure(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	require.NoError(t, DB.Exec("DELETE FROM subscription_pre_consume_records").Error)
	userID, subscriptionID := 6406, 6406
	require.NoError(t, DB.Create(&User{Id: userID, Username: "refund-atomic", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id: userID, UserId: userID, AmountTotal: 100, AmountUsed: 40,
		StartTime: time.Now().Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active",
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
		RequestId: "refund-atomic-record", UserId: userID, UserSubscriptionId: subscriptionID,
		PreConsumed: 10, Status: "consumed",
	}).Error)

	err := DB.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, postConsumeUserSubscriptionDeltaTx(tx, subscriptionID, -10))
		return tx.Exec("UPDATE subscription_pre_consume_records SET missing_refund_status_column = 1 WHERE request_id = ?", "refund-atomic-record").Error
	})
	require.Error(t, err)

	var subscription UserSubscription
	require.NoError(t, DB.First(&subscription, subscriptionID).Error)
	require.Equal(t, int64(40), subscription.AmountUsed)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", "refund-atomic-record").First(&record).Error)
	require.Equal(t, "consumed", record.Status)
}
