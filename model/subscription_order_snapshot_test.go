package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCompleteSubscriptionOrderUsesImmutableSnapshotAfterPlanEdit(t *testing.T) {
	truncateTables(t)
	const userID, planID = 6101, 6101
	require.NoError(t, DB.Create(&User{Id: userID, Username: "snapshot-user", Status: common.UserStatusEnabled}).Error)
	allowOverflow := true
	plan := &SubscriptionPlan{
		Id: planID, Title: "Original plan", PriceAmount: 9.99, Currency: "USD",
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 1200,
		QuotaResetPeriod: SubscriptionResetMonthly, UpgradeGroup: "pro", AllowWalletOverflow: &allowOverflow,
		StripePriceId: "price_original", Enabled: true,
	}
	require.NoError(t, DB.Create(plan).Error)
	snapshot, err := NewSubscriptionOrderSnapshot(plan, PaymentProviderStripe)
	require.NoError(t, err)
	order := &SubscriptionOrder{
		UserId: userID, PlanId: planID, Money: 9.99, TradeNo: "snapshot-order-1",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(), PlanSnapshot: snapshot,
	}
	require.NoError(t, order.Insert())

	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", planID).Updates(map[string]interface{}{
		"title": "Edited plan", "price_amount": 99.99, "total_amount": 9999,
		"duration_value": 12, "upgrade_group": "enterprise", "stripe_price_id": "price_edited",
	}).Error)

	payload := `{"amount_total":"999","currency":"usd","stripe_price_id":"price_original"}`
	require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, payload, PaymentProviderStripe, ""))

	var sub UserSubscription
	require.NoError(t, DB.Where("user_id = ?", userID).First(&sub).Error)
	assert.Equal(t, int64(1200), sub.AmountTotal)
	assert.Equal(t, "pro", sub.UpgradeGroup)
	assert.Equal(t, "order", sub.Source)
	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo).First(&topUp).Error)
	assert.Equal(t, PaymentProviderStripe, topUp.PaymentProvider)
	assert.Equal(t, "Stripe", topUp.PaymentMethodName)
}

func TestUpsertSubscriptionTopUpSnapshotsPaymentMethodName(t *testing.T) {
	truncateTables(t)
	order := &SubscriptionOrder{
		UserId:          6199,
		Money:           10,
		TradeNo:         "subscription-topup-name",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return upsertSubscriptionTopUpTx(tx, order)
	}))

	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo).First(&topUp).Error)
	assert.Equal(t, "Alipay", topUp.PaymentMethodName)
	assert.Equal(t, order.Money, topUp.RequestedAmount)
}

func TestUpsertSubscriptionTopUpBackfillsLegacyDisplayAmount(t *testing.T) {
	truncateTables(t)
	order := &SubscriptionOrder{
		UserId:          6200,
		Money:           12.5,
		TradeNo:         "subscription-topup-legacy-amount",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, DB.Create(&TopUp{
		UserId:          order.UserId,
		TradeNo:         order.TradeNo,
		Amount:          0,
		RequestedAmount: 0,
		Money:           order.Money,
		PaymentMethod:   order.PaymentMethod,
		PaymentProvider: order.PaymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      order.CreateTime,
	}).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return upsertSubscriptionTopUpTx(tx, order)
	}))

	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo).First(&topUp).Error)
	assert.Equal(t, order.Money, topUp.RequestedAmount)
}

func TestUpsertSubscriptionTopUpPreservesProviderCurrencyForHistory(t *testing.T) {
	truncateTables(t)
	plan := &SubscriptionPlan{
		Id: 6201, Title: "Creem EUR plan", PriceAmount: 10, Currency: "EUR",
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true,
	}
	snapshot, err := NewSubscriptionOrderSnapshotWithProvider(plan, PaymentProviderCreem, 10, "EUR", "prod_eur")
	require.NoError(t, err)
	order := &SubscriptionOrder{
		UserId: 6201, PlanId: plan.Id, Money: 10, TradeNo: "subscription-creem-eur-history",
		PaymentMethod: PaymentMethodCreem, PaymentProvider: PaymentProviderCreem,
		CreateTime: time.Now().Unix(), PlanSnapshot: snapshot,
	}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return upsertSubscriptionTopUpTx(tx, order)
	}))

	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo).First(&topUp).Error)
	assert.Equal(t, "EUR", topUp.PaymentCurrency)
	assert.Equal(t, 10.0, topUp.RequestedAmount)
	assert.Zero(t, topUp.PaymentBaseAmount)
	annotateTopupSources([]*TopUp{&topUp})
	assert.Zero(t, topUp.AccountingAmountUSD)
}

func TestCompleteSubscriptionOrderRejectsStripeSnapshotMismatch(t *testing.T) {
	truncateTables(t)
	const userID, planID = 6102, 6102
	require.NoError(t, DB.Create(&User{Id: userID, Username: "snapshot-mismatch-user", Status: common.UserStatusEnabled}).Error)
	plan := &SubscriptionPlan{
		Id: planID, Title: "Mismatch plan", PriceAmount: 10, Currency: "USD",
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1, StripePriceId: "price_expected", Enabled: true,
	}
	require.NoError(t, DB.Create(plan).Error)
	snapshot, err := NewSubscriptionOrderSnapshot(plan, PaymentProviderStripe)
	require.NoError(t, err)
	order := &SubscriptionOrder{
		UserId: userID, PlanId: planID, Money: 10, TradeNo: "snapshot-order-2",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(), PlanSnapshot: snapshot,
	}
	require.NoError(t, order.Insert())

	err = CompleteSubscriptionOrder(order.TradeNo, `{"amount_total":"1000","currency":"usd","stripe_price_id":"price_other"}`, PaymentProviderStripe, "")
	require.ErrorIs(t, err, ErrSubscriptionOrderSnapshotInvalid)
	assert.Equal(t, common.TopUpStatusPending, GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
}

func TestFailSubscriptionOrderOnlyChangesPendingOrders(t *testing.T) {
	truncateTables(t)
	order := &SubscriptionOrder{
		UserId: 6103, PlanId: 6103, Money: 1, TradeNo: "failed-sub-order",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, order.Insert())
	require.NoError(t, FailSubscriptionOrder(order.TradeNo, PaymentProviderStripe))
	assert.Equal(t, common.TopUpStatusFailed, GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
	require.NoError(t, FailSubscriptionOrder(order.TradeNo, PaymentProviderStripe))
	assert.Equal(t, common.TopUpStatusFailed, GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
}

func TestSubscriptionProviderSnapshotsRejectMismatches(t *testing.T) {
	providers := []struct {
		name     string
		provider string
		valid    string
		payload  string
		currency string
		priceID  string
	}{
		{name: "creem", provider: PaymentProviderCreem, currency: "USD", priceID: "prod_creem", valid: `{"object":{"order":{"amount_paid":1000,"currency":"USD","product":"prod_creem"},"product":{"id":"prod_creem"}}}`, payload: `{"object":{"order":{"amount_paid":1000,"currency":"USD","product":"prod_other"},"product":{"id":"prod_other"}}}`},
		{name: "epay", provider: PaymentProviderEpay, currency: "USD", priceID: "alipay", valid: `{"Money":"10.00","Type":"alipay"}`, payload: `{"Money":"9.00","Type":"alipay"}`},
		{name: "waffo_pancake", provider: PaymentProviderWaffoPancake, currency: "USD", priceID: "PROD_expected", valid: `{"data":{"amount":"10.00","currency":"USD","productMetadata":{"product_id":"PROD_expected"}}}`, payload: `{"data":{"amount":"9.00","currency":"USD","productMetadata":{"product_id":"PROD_other"}}}`},
	}
	for _, tc := range providers {
		t.Run(tc.name, func(t *testing.T) {
			plan := &SubscriptionPlan{Id: 6200, Title: "provider plan", PriceAmount: 10, Currency: tc.currency, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true}
			snapshot, err := NewSubscriptionOrderSnapshotWithProvider(plan, tc.provider, 10, tc.currency, tc.priceID)
			require.NoError(t, err)
			var order SubscriptionOrder
			order.PlanSnapshot = snapshot
			decoded, err := order.Snapshot()
			require.NoError(t, err)
			require.NoError(t, validateSubscriptionProviderSnapshot(decoded, tc.valid, tc.provider))
			err = validateSubscriptionProviderSnapshot(decoded, tc.payload, tc.provider)
			require.ErrorIs(t, err, ErrSubscriptionOrderSnapshotInvalid)
		})
	}
}

func TestSubscriptionProviderSnapshotCreemRejectsContradictoryProductIDs(t *testing.T) {
	plan := &SubscriptionPlan{Id: 6210, Title: "Creem product plan", PriceAmount: 10, Currency: "USD", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true}
	snapshotJSON, err := NewSubscriptionOrderSnapshotWithProvider(plan, PaymentProviderCreem, 10, "USD", "prod_expected")
	require.NoError(t, err)
	var order SubscriptionOrder
	order.PlanSnapshot = snapshotJSON
	snapshot, err := order.Snapshot()
	require.NoError(t, err)

	validWithBothIDs := `{"object":{"order":{"amount_paid":1000,"currency":"USD","product":"prod_expected"},"product":{"id":"prod_expected"}}}`
	require.NoError(t, validateSubscriptionProviderSnapshot(snapshot, validWithBothIDs, PaymentProviderCreem))

	contradictoryIDs := `{"object":{"order":{"amount_paid":1000,"currency":"USD","product":"prod_other"},"product":{"id":"prod_expected"}}}`
	err = validateSubscriptionProviderSnapshot(snapshot, contradictoryIDs, PaymentProviderCreem)
	require.ErrorIs(t, err, ErrSubscriptionOrderSnapshotInvalid)
}

func TestSubscriptionProviderSnapshotsUseZeroDecimalCurrencies(t *testing.T) {
	plan := &SubscriptionPlan{Id: 6250, Title: "JPY plan", PriceAmount: 1000, Currency: "JPY", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true}
	snapshotJSON, err := NewSubscriptionOrderSnapshotWithProvider(plan, PaymentProviderStripe, 1000, "JPY", "price_jpy")
	require.NoError(t, err)
	var order SubscriptionOrder
	order.PlanSnapshot = snapshotJSON
	snapshot, err := order.Snapshot()
	require.NoError(t, err)
	require.NoError(t, validateSubscriptionProviderSnapshot(snapshot, `{"amount_total":"1000","currency":"jpy","stripe_price_id":"price_jpy"}`, PaymentProviderStripe))

	creemJSON, err := NewSubscriptionOrderSnapshotWithProvider(plan, PaymentProviderCreem, 1000, "JPY", "prod_jpy")
	require.NoError(t, err)
	order.PlanSnapshot = creemJSON
	snapshot, err = order.Snapshot()
	require.NoError(t, err)
	require.NoError(t, validateSubscriptionProviderSnapshot(snapshot, `{"object":{"order":{"amount_paid":1000,"currency":"JPY","product":"prod_jpy"},"product":{"id":"prod_jpy"}}}`, PaymentProviderCreem))
}

func TestSubscriptionProviderAmountRepresentableRejectsImplicitRounding(t *testing.T) {
	assert.True(t, SubscriptionProviderAmountRepresentable(10, "USD"))
	assert.False(t, SubscriptionProviderAmountRepresentable(10.005, "USD"))
	assert.True(t, SubscriptionProviderAmountRepresentable(1000, "JPY"))
	assert.False(t, SubscriptionProviderAmountRepresentable(1000.5, "JPY"))
}

func TestCompleteSubscriptionOrderPreservesRetryableDatabaseError(t *testing.T) {
	originalDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	DB = db
	t.Cleanup(func() { DB = originalDB })

	err = CompleteSubscriptionOrder("db-error-order", "{}", PaymentProviderStripe, "")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrSubscriptionOrderNotFound)
	assert.False(t, IsPermanentSubscriptionOrderError(err))
}

func TestCompleteSubscriptionOrderRejectsLegacyPlanUpdatedInSameSecond(t *testing.T) {
	truncateTables(t)
	const userID, planID = 6301, 6301
	require.NoError(t, DB.Create(&User{Id: userID, Username: "legacy-same-second", Status: common.UserStatusEnabled}).Error)
	createdAt := time.Now().Unix()
	plan := &SubscriptionPlan{
		Id: planID, Title: "Legacy plan", PriceAmount: 10, Currency: "USD",
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 100,
		UpdatedAt: createdAt, Enabled: true, StripePriceId: "price_legacy",
	}
	require.NoError(t, DB.Create(plan).Error)
	order := &SubscriptionOrder{
		UserId: userID, PlanId: planID, Money: 10, TradeNo: "legacy-same-second-order",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, CreateTime: createdAt,
	}
	require.NoError(t, order.Insert())
	err := CompleteSubscriptionOrder(order.TradeNo, `{"amount_total":"1000","currency":"usd","stripe_price_id":"price_legacy"}`, PaymentProviderStripe, "")
	require.ErrorIs(t, err, ErrSubscriptionOrderSnapshotMissing)
}
