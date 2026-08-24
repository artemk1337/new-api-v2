package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/waffo-com/waffo-go/core"
	"gorm.io/gorm"
)

func TestValidateEpayPaymentAmountUsesSnapshot(t *testing.T) {
	topUp := &model.TopUp{PaymentCurrency: "USD", PaymentChargedAmount: 1.23}
	require.NoError(t, validateEpayPaymentAmount(topUp, "1.23"))
	assert.Error(t, validateEpayPaymentAmount(topUp, "1.24"))
	assert.Error(t, validateEpayPaymentAmount(topUp, "not-a-number"))
}

func TestEpayTopUpPaymentMethodMatchesSnapshot(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		callback string
		want     bool
	}{
		{name: "same method", method: "alipay", callback: "alipay", want: true},
		{name: "case insensitive", method: "Alipay", callback: "alipay", want: true},
		{name: "different method", method: "alipay", callback: "wxpay", want: false},
		{name: "missing snapshot", method: "", callback: "alipay", want: false},
		{name: "missing callback method", method: "alipay", callback: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, epayTopUpPaymentMethodMatches(&model.TopUp{PaymentMethod: tt.method}, tt.callback))
		})
	}
}

func TestWaffoPancakeStoreMatchesConfiguredStore(t *testing.T) {
	event := &service.WaffoPancakeWebhookEvent{StoreID: "store-1"}
	assert.True(t, waffoPancakeStoreMatches(event, "store-1"))
	assert.False(t, waffoPancakeStoreMatches(event, "store-2"))
	assert.False(t, waffoPancakeStoreMatches(event, ""))
	assert.False(t, waffoPancakeStoreMatches(nil, "store-1"))
}

func TestValidateWaffoPaymentResultUsesRoundedProviderSnapshot(t *testing.T) {
	topUp := &model.TopUp{PaymentCurrency: "RUB", PaymentChargedAmount: 123.45}
	result := &core.PaymentNotificationResult{OrderCurrency: "rub", OrderAmount: "123.45"}
	require.NoError(t, validateWaffoPaymentResult(result, topUp))
	result.OrderAmount = "123.46"
	assert.Error(t, validateWaffoPaymentResult(result, topUp))
	result.OrderCurrency = "USD"
	assert.Error(t, validateWaffoPaymentResult(result, topUp))
}

func TestValidateWaffoPancakeAndCreemPaymentSnapshots(t *testing.T) {
	pancakeTopUp := &model.TopUp{PaymentCurrency: "USD", PaymentChargedAmount: 1.23}
	pancakeEvent := &service.WaffoPancakeWebhookEvent{Data: service.WaffoPancakeWebhookData{Currency: "USD", Amount: "1.23"}}
	actual, err := decimal.NewFromString(pancakeEvent.Data.Amount)
	require.NoError(t, err)
	require.NoError(t, service.ValidatePaymentSnapshot(pancakeTopUp, pancakeEvent.Data.Currency, actual.InexactFloat64()))
	pancakeEvent.Data.Amount = "1.24"
	actual, err = decimal.NewFromString(pancakeEvent.Data.Amount)
	require.NoError(t, err)
	assert.Error(t, service.ValidatePaymentSnapshot(pancakeTopUp, pancakeEvent.Data.Currency, actual.InexactFloat64()))

	creemTopUp := &model.TopUp{PaymentCurrency: "EUR", PaymentChargedAmount: 12.34, CreemProductID: "prod_eur"}
	creemEvent := &CreemWebhookEvent{}
	creemEvent.Object.Order.AmountPaid = 1234
	creemEvent.Object.Order.Currency = "eur"
	creemEvent.Object.Order.Product = "prod_eur"
	require.NoError(t, validateCreemPayment(creemTopUp, creemEvent))
	creemEvent.Object.Order.AmountPaid = 1235
	assert.Error(t, validateCreemPayment(creemTopUp, creemEvent))

	for _, tc := range []struct {
		currency string
		amount   float64
		paid     int
	}{
		{currency: "JPY", amount: 100, paid: 100},
		{currency: "KWD", amount: 12.345, paid: 12345},
	} {
		topUp := &model.TopUp{PaymentCurrency: tc.currency, PaymentChargedAmount: tc.amount, CreemProductID: "prod_" + tc.currency}
		event := &CreemWebhookEvent{}
		event.Object.Order.AmountPaid = tc.paid
		event.Object.Order.Currency = tc.currency
		event.Object.Order.Product = "prod_" + tc.currency
		require.NoError(t, validateCreemPayment(topUp, event), tc.currency)
	}
}

func TestValidateCreemPaymentRejectsProductSubstitution(t *testing.T) {
	topUp := &model.TopUp{PaymentCurrency: "USD", PaymentChargedAmount: 10, CreemProductID: "prod_expected"}
	event := &CreemWebhookEvent{}
	event.Object.Order.AmountPaid = 1000
	event.Object.Order.Currency = "USD"
	event.Object.Order.Product = "prod_other"
	assert.Error(t, validateCreemPayment(topUp, event))

	event.Object.Order.Product = "prod_expected"
	event.Object.Product.Id = "prod_other"
	assert.Error(t, validateCreemPayment(topUp, event))
}

func TestValidateCreemPaymentAllowsLegacySnapshotWithoutProductID(t *testing.T) {
	topUp := &model.TopUp{PaymentCurrency: "EUR", PaymentChargedAmount: 12.34}
	event := &CreemWebhookEvent{}
	event.Object.Order.AmountPaid = 1234
	event.Object.Order.Currency = "EUR"
	// Legacy callbacks may not contain product fields; amount/currency remain
	// the only available immutable checks for those pre-migration orders.
	require.NoError(t, validateCreemPayment(topUp, event))
}

func TestValidateStripeTopUpPaymentUsesCentsAndCurrency(t *testing.T) {
	topUp := &model.TopUp{PaymentCurrency: "USD", PaymentChargedAmount: 12.34}
	event := stripe.Event{Data: &stripe.EventData{Object: map[string]interface{}{
		"amount_total": "1234",
		"currency":     "usd",
	}}}
	require.NoError(t, validateStripeTopUpPayment(topUp, event))
	event.Data.Object["amount_total"] = "1235"
	assert.Error(t, validateStripeTopUpPayment(topUp, event))
	event.Data.Object["amount_total"] = "1234"
	event.Data.Object["currency"] = "eur"
	assert.Error(t, validateStripeTopUpPayment(topUp, event))
}

func TestNOWPaymentsInvoiceCurrencyUsesSnapshotCurrency(t *testing.T) {
	original := setting.NOWPaymentsPriceCurrency
	setting.NOWPaymentsPriceCurrency = "eur"
	t.Cleanup(func() { setting.NOWPaymentsPriceCurrency = original })

	assert.Equal(t, "usdt", nowPaymentsInvoicePriceCurrency(&model.TopUp{PaymentCurrency: "USDT"}))
	assert.Equal(t, "usdt", nowPaymentsInvoicePriceCurrency(&model.TopUp{}))
}

func TestProviderAmountFormattingMatchesSnapshotPrecision(t *testing.T) {
	assert.Equal(t, "123.46", formatWaffoAmount(123.456, "USD"))
	assert.Equal(t, "123", formatWaffoAmount(123.456, "JPY"))
	assert.Equal(t, "1.23", formatWaffoPancakeAmount(1.234))
	stripeItem := newStripeTopUpLineItemExact(1.23)
	assert.Nil(t, stripeItem.Price)
	require.NotNil(t, stripeItem.PriceData)
	assert.Equal(t, int64(123), *stripeItem.PriceData.UnitAmount)
}

func TestRoundedProviderAmountKeepsUnroundedQuoteEconomics(t *testing.T) {
	originalCashback := operation_setting.GetPaymentSetting().AmountCashback
	operation_setting.GetPaymentSetting().AmountCashback = operation_setting.AmountCashbackConfig{{MinAmount: 1, CashbackPercent: 10}}
	t.Cleanup(func() { operation_setting.GetPaymentSetting().AmountCashback = originalCashback })

	// A provider rounds $1.005 to $1.01, but commission/cashback are based on
	// the same unrounded gross quote that the wallet displays.
	topUp := &model.TopUp{RequestedAmount: 1}
	service.ApplyPaymentSnapshot(topUp, "USD", 1, 1.005, 1, 1.01)
	credit := service.CalculateTopUpCredit(1.005, 0)
	assert.InDelta(t, 1.005, topUp.PaymentBaseAmount, 0.0000001)
	assert.InDelta(t, 0, topUp.PaymentCommission, 0.0000001)
	assert.Equal(t, int(credit.TotalAmountUSD*common.QuotaPerUnit), topUp.QuotaToAdd)
	require.NoError(t, service.ValidatePaymentSnapshot(topUp, "USD", 1.01))
	assert.Error(t, service.ValidatePaymentSnapshot(topUp, "USD", 1.00))
}

func TestWaffoPancakeSnapshotUsesPaymentMethodGroup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"waffo_pancake","topup_group":"premium"}]`}).Error)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	originalMethods := operation_setting.PayMethods
	originalRatios := common.TopupGroupRatio2JSONString()
	originalUnitPrice := setting.WaffoPancakeUnitPrice
	t.Cleanup(func() {
		operation_setting.PayMethods = originalMethods
		setting.WaffoPancakeUnitPrice = originalUnitPrice
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalRatios))
	})
	operation_setting.PayMethods = []map[string]string{{"type": model.PaymentMethodWaffoPancake, "topup_group": "premium"}}
	setting.WaffoPancakeUnitPrice = 1
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"premium":1.2}`))

	payMoney := getWaffoPancakePayMoney(1, "default")
	coefficient := common.GetTopupGroupRatio(getPaymentTopupGroup(model.PaymentMethodWaffoPancake, "default"))
	topUp := &model.TopUp{RequestedAmount: 1}
	service.ApplyPaymentSnapshot(topUp, "USD", 1, payMoney/coefficient, coefficient, payMoney)
	assert.InDelta(t, 1, topUp.PaymentBaseAmount, 0.0000001)
	assert.InDelta(t, 1.2, topUp.PaymentChargedAmount, 0.0000001)
	assert.InDelta(t, 0.2, topUp.PaymentCommission, 0.0000001)
}
