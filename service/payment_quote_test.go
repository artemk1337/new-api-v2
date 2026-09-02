package service

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildPaymentQuoteCommissionRule(t *testing.T) {
	previous := operation_setting.PayMethods
	previousRatios := common.TopupGroupRatio2JSONString()
	operation_setting.PayMethods = []map[string]string{{"type": "test", "currency": "USD", "topup_group": "premium"}}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"premium":1.03}`))
	t.Cleanup(func() {
		operation_setting.PayMethods = previous
		_ = common.UpdateTopupGroupRatioByJSONString(previousRatios)
	})
	quote, err := BuildPaymentQuote(10, "test", "default")
	require.NoError(t, err)
	assert.Equal(t, "USD", quote.Currency)
	assert.Equal(t, 1.0, quote.RateToUSD)
	assert.InDelta(t, quote.BaseAmountUSD*0.03, quote.CommissionUSD, 0.000001)
	assert.InDelta(t, quote.BaseAmountUSD*1.03, quote.ChargedAmountUSD, 0.000001)
}

func TestBuildPaymentQuoteUsesReferralCashbackAndSnapshotsBaseQuota(t *testing.T) {
	previousCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	previousMethods := operation_setting.PayMethods
	referralPercent := 20.0
	operation_setting.GetPaymentSetting().AmountCashback = operation_setting.AmountCashbackConfig{{MinAmount: 0, CashbackPercent: 10, ReferralCashbackPercent: &referralPercent}}
	operation_setting.PayMethods = []map[string]string{{"type": "test", "currency": "USD"}}
	t.Cleanup(func() {
		operation_setting.GetPaymentSetting().AmountCashback = previousCashbacks
		operation_setting.PayMethods = previousMethods
	})

	user := &model.User{Id: 99101, Username: "quote-referral-user", AffCode: "quote-referral", InviterId: 99100, ReferralCashbackEligible: true, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	t.Cleanup(func() { _ = model.DB.Delete(&model.User{}, user.Id).Error })

	quote, err := BuildPaymentQuote(100, "test", "default", user.Id)
	require.NoError(t, err)
	assert.Equal(t, 10.0, quote.RegularCashbackPercent)
	assert.Equal(t, 20.0, quote.CashbackPercent)
	assert.True(t, quote.IsReferralCashback)

	topUp := &model.TopUp{RequestedAmount: 100}
	ApplyPaymentQuote(topUp, quote)
	assert.Equal(t, calculateTopUpQuotaAmount(100), topUp.BaseQuotaToAdd)
	assert.Equal(t, calculateTopUpQuotaAmount(120), topUp.QuotaToAdd)
}

func TestBuildPaymentQuoteDirectUSDTRequiresPersistedMethod(t *testing.T) {
	_, err := BuildPaymentQuoteWithPayMethods(10, model.DirectUSDTTRC20Provider, "default", []map[string]string{{"type": "alipay"}})
	require.Error(t, err)

	quote, err := BuildPaymentQuoteWithPayMethods(10, model.DirectUSDTTRC20Provider, "default", []map[string]string{{"type": model.DirectUSDTTRC20Provider}})
	require.NoError(t, err)
	require.Equal(t, "USDT", quote.Currency)
}

func TestBuildPaymentQuoteUsesPersistedPayMethodGroup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousDB := model.DB
	previousMethods := operation_setting.PayMethods
	previousRatios := common.TopupGroupRatio2JSONString()
	model.DB = db
	operation_setting.PayMethods = []map[string]string{{"type": "test", "topup_group": "legacy"}}
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"test","topup_group":"premium"}]`}).Error)
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"legacy":1.01,"premium":1.2}`))
	t.Cleanup(func() {
		model.DB = previousDB
		operation_setting.PayMethods = previousMethods
		_ = common.UpdateTopupGroupRatioByJSONString(previousRatios)
	})

	quote, err := BuildPaymentQuote(10, "test", "default")
	require.NoError(t, err)
	assert.InDelta(t, 1.2, quote.Coefficient, 0.000001)
	assert.InDelta(t, 2.0, quote.CommissionUSD, 0.000001)
	methods, err := model.GetPayMethodsFromDB(db)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "PayMethods").Update("value", `[{"type":"test","topup_group":"legacy"}]`).Error)
	quote, err = BuildPaymentQuoteWithPayMethods(10, "test", "default", methods)
	require.NoError(t, err)
	assert.InDelta(t, 1.2, quote.Coefficient, 0.000001, "payment creation must keep the readiness snapshot")
}

func TestBuildPaymentQuoteFailsClosedOnPayMethodsDatabaseError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	previousDB := model.DB
	previousMethods := operation_setting.PayMethods
	model.DB = db
	operation_setting.PayMethods = []map[string]string{{"type": "test", "topup_group": "stale"}}
	t.Cleanup(func() {
		model.DB = previousDB
		operation_setting.PayMethods = previousMethods
	})

	_, err = BuildPaymentQuote(10, "test", "default")
	require.Error(t, err)
}

func TestBuildPaymentQuoteCreditsBaseAndChargesCommission(t *testing.T) {
	previousMethods := operation_setting.PayMethods
	previousRatios := common.TopupGroupRatio2JSONString()
	previousCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	operation_setting.PayMethods = []map[string]string{{"type": "test", "topup_group": "premium"}}
	operation_setting.GetPaymentSetting().AmountCashback = operation_setting.AmountCashbackConfig{{
		MinAmount: 1, CashbackPercent: 10,
	}}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"premium":1.2}`))
	t.Cleanup(func() {
		operation_setting.PayMethods = previousMethods
		operation_setting.GetPaymentSetting().AmountCashback = previousCashbacks
		_ = common.UpdateTopupGroupRatioByJSONString(previousRatios)
	})

	quote, err := BuildPaymentQuote(1, "test", "default")
	require.NoError(t, err)
	assert.InDelta(t, 1, quote.BaseAmountUSD, 0.000001)
	assert.InDelta(t, 0.2, quote.CommissionUSD, 0.000001)
	assert.InDelta(t, 1.2, quote.ChargedAmountUSD, 0.000001)
	assert.InDelta(t, 1.2, quote.ChargedAmount, 0.000001)
	assert.InDelta(t, 1, quote.CreditedAmountUSD, 0.000001)
	assert.Equal(t, 10.0, quote.CashbackPercent)
	assert.InDelta(t, 0.1, quote.CashbackAmountUSD, 0.000001)
}

func TestApplyPaymentQuoteSnapshotKeepsOriginalRateAfterRateChanges(t *testing.T) {
	// The quote was calculated at 90 RUB/USD. If the current rate becomes 80
	// before the payment order is created, deriving the base again from the
	// already quoted provider amount would incorrectly derive a $135 base.
	quote := PaymentQuote{
		Currency:          "RUB",
		RateToUSD:         90,
		Coefficient:       1.2,
		BaseAmountUSD:     100,
		CommissionUSD:     20,
		CashbackAmountUSD: 10,
		CreditedAmountUSD: 100,
		ChargedAmountUSD:  120,
		ChargedAmount:     10800,
	}
	rateAfterQuote := 80.0
	topUp := &model.TopUp{RequestedAmount: 100}

	ApplyPaymentQuoteSnapshot(topUp, quote, quote.ChargedAmount)

	assert.InDelta(t, 10800, topUp.PaymentChargedAmount, 0.000001)
	assert.InDelta(t, 90, topUp.PaymentRateToUSD, 0.000001)
	assert.InDelta(t, 100, topUp.PaymentBaseAmount, 0.000001)
	assert.InDelta(t, 20, topUp.PaymentCommission, 0.000001)
	assert.NotEqual(t, quote.ChargedAmount/rateAfterQuote, topUp.PaymentBaseAmount)
	assert.Equal(t, calculateTopUpQuotaAmount(110), topUp.QuotaToAdd)
}

func TestBuildPaymentQuoteIgnoresLegacyEpayPrice(t *testing.T) {
	previousMethods := operation_setting.PayMethods
	previousPrice := operation_setting.Price
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "currency": "RUB"}}
	operation_setting.Price = 80
	t.Cleanup(func() {
		operation_setting.PayMethods = previousMethods
		operation_setting.Price = previousPrice
	})

	quote, err := BuildPaymentQuote(10, "alipay", "default")
	require.NoError(t, err)
	assert.Equal(t, 10.0, quote.BaseAmountUSD)
	assert.Equal(t, "USD", quote.Currency, "legacy PayMethods currency must not select EPay settlement")
	assert.Equal(t, 10.0, quote.ChargedAmount)
}

func TestBuildPaymentQuoteRejectsNonFiniteAmount(t *testing.T) {
	_, err := BuildPaymentQuote(math.NaN(), model.PaymentMethodStripe, "default")
	assert.Error(t, err)
	_, err = BuildPaymentQuote(math.Inf(1), model.PaymentMethodStripe, "default")
	assert.Error(t, err)
}

func TestPaymentMethodCurrencyDefaultsToUSD(t *testing.T) {
	previous := operation_setting.PayMethods
	operation_setting.PayMethods = []map[string]string{{"type": "legacy"}}
	t.Cleanup(func() { operation_setting.PayMethods = previous })
	currency, err := PaymentMethodCurrency("legacy")
	require.NoError(t, err)
	assert.Equal(t, "USD", currency)
}

func TestValidatePaymentMethodCurrencyRejectsUnsupportedProviderCurrency(t *testing.T) {
	require.Error(t, ValidatePaymentMethodCurrency("alipay", "RUB"))
	require.NoError(t, ValidatePaymentMethodCurrency("alipay", "USD"))
	require.NoError(t, ValidatePaymentMethodCurrency("yookassa_sbp", "RUB"))
	assert.Error(t, ValidatePaymentMethodCurrency("stripe", "RUB"))
	require.NoError(t, ValidatePaymentMethodCurrency(model.PaymentMethodNOWPayments, "USDT"))
	require.Error(t, ValidatePaymentMethodCurrency(model.PaymentMethodNOWPayments, "USD"))
}

func TestNOWPaymentsCurrencyIsAlwaysUSDT(t *testing.T) {
	previous := operation_setting.PayMethods
	operation_setting.PayMethods = []map[string]string{{"type": model.PaymentMethodNOWPayments, "currency": "USD"}}
	t.Cleanup(func() { operation_setting.PayMethods = previous })
	currency, err := PaymentMethodCurrency(model.PaymentMethodNOWPayments)
	require.NoError(t, err)
	assert.Equal(t, "USDT", currency)
}

func TestPaymentMethodCurrencyPrefersBuiltInContractsOverStalePayMethods(t *testing.T) {
	previous := operation_setting.PayMethods
	operation_setting.PayMethods = []map[string]string{
		{"type": model.PaymentMethodStripe, "currency": "RUB"},
		{"type": model.PaymentMethodYooKassaSBP, "currency": "USD"},
	}
	t.Cleanup(func() { operation_setting.PayMethods = previous })

	stripeCurrency, err := PaymentMethodCurrency(model.PaymentMethodStripe)
	require.NoError(t, err)
	assert.Equal(t, "USD", stripeCurrency)
	yooKassaCurrency, err := PaymentMethodCurrency(model.PaymentMethodYooKassaSBP)
	require.NoError(t, err)
	assert.Equal(t, "RUB", yooKassaCurrency)
}

func TestPaymentMethodCurrencyNormalizesLegacyTypeAndCurrency(t *testing.T) {
	previous := operation_setting.PayMethods
	operation_setting.PayMethods = []map[string]string{{"type": " CustomPay ", "currency": "rub"}}
	t.Cleanup(func() { operation_setting.PayMethods = previous })

	currency, err := PaymentMethodCurrency(" custompay ")
	require.NoError(t, err)
	assert.Equal(t, "USD", currency)

	assert.EqualError(t, ValidatePaymentMethodCurrency(" STRIPE ", "RUB"), "Stripe top-up supports USD only")
}

func TestApplyPaymentQuoteStoresImmutableSnapshot(t *testing.T) {
	topUp := &model.TopUp{}
	ApplyPaymentQuote(topUp, PaymentQuote{Currency: "RUB", RateToUSD: 90, Coefficient: 1.03, BaseAmountUSD: 10, CommissionUSD: .3, ChargedAmount: 927})
	assert.Equal(t, "RUB", topUp.PaymentCurrency)
	assert.Equal(t, 90.0, topUp.PaymentRateToUSD)
	assert.Equal(t, 927.0, topUp.PaymentChargedAmount)
	assert.Equal(t, 927.0, topUp.Money)
}

func TestApplyPaymentSnapshotClearsCommissionForNoCommissionCoefficient(t *testing.T) {
	previousCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	operation_setting.GetPaymentSetting().AmountCashback = nil
	t.Cleanup(func() { operation_setting.GetPaymentSetting().AmountCashback = previousCashbacks })

	topUp := &model.TopUp{
		RequestedAmount:   1,
		PaymentCommission: 9,
	}
	ApplyPaymentSnapshot(topUp, "USD", 1, 1, 0.8, 1)

	assert.Equal(t, 1.0, topUp.PaymentCoefficient)
	assert.Zero(t, topUp.PaymentCommission)
	assert.Equal(t, int(common.QuotaPerUnit), topUp.QuotaToAdd)
}

func TestCalculateTopUpQuotaCreditsBaseAndAddsCommission(t *testing.T) {
	originalCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	t.Cleanup(func() {
		operation_setting.GetPaymentSetting().AmountCashback = originalCashbacks
	})
	operation_setting.GetPaymentSetting().AmountCashback = operation_setting.AmountCashbackConfig{{MinAmount: 1, CashbackPercent: 10}}

	topUp := &model.TopUp{RequestedAmount: 1}
	ApplyPaymentQuote(topUp, PaymentQuote{
		Currency:          "USD",
		RateToUSD:         1,
		BaseAmountUSD:     1,
		CommissionUSD:     0.2,
		CashbackPercent:   10,
		CashbackAmountUSD: 0.1,
		CreditedAmountUSD: 1,
		ChargedAmountUSD:  1.2,
		ChargedAmount:     1.2,
	})

	credit := CalculateTopUpCredit(1, 0.2)
	assert.InDelta(t, 1, credit.CreditedAmountUSD, 0.0000001)
	assert.InDelta(t, 0.1, credit.CashbackAmountUSD, 0.0000001)
	assert.InDelta(t, 1.1, credit.TotalAmountUSD, 0.0000001)

	// The user enters $1.00 to credit, pays $1.20 gross, and receives
	// $1.00 plus 10% cashback: ($1.00 + $0.10) * 500,000.
	assert.Equal(t, int(1.1*common.QuotaPerUnit), topUp.QuotaToAdd)
}

func TestCalculateTopUpCreditUsesDecimalCashbackArithmetic(t *testing.T) {
	previousCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	t.Cleanup(func() {
		operation_setting.GetPaymentSetting().AmountCashback = previousCashbacks
	})
	operation_setting.GetPaymentSetting().AmountCashback = operation_setting.AmountCashbackConfig{{MinAmount: 0, CashbackPercent: 3}}

	credit := CalculateTopUpCredit(0.07, 0)
	assert.Equal(t, 0.07, credit.CreditedAmountUSD)
	assert.Equal(t, 3.0, credit.CashbackPercent)
	assert.Equal(t, 0.0021, credit.CashbackAmountUSD)
	assert.Equal(t, 0.0721, credit.TotalAmountUSD)
}

func TestCalculateTopUpQuotaHandlesBoundariesAndTruncatesOnce(t *testing.T) {
	originalCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	t.Cleanup(func() {
		operation_setting.GetPaymentSetting().AmountCashback = originalCashbacks
	})
	operation_setting.GetPaymentSetting().AmountCashback = operation_setting.AmountCashbackConfig{{MinAmount: 1, CashbackPercent: 10}}

	assert.Equal(t, int(1.1*common.QuotaPerUnit), CalculateTopUpQuota(1, 2), "commission is charged separately and cannot reduce the credited base")
	assert.Equal(t, int(1.1*common.QuotaPerUnit), CalculateTopUpQuota(1, -1), "negative commission is treated as zero")
	operation_setting.GetPaymentSetting().AmountCashback = nil
	assert.Equal(t, 500000, CalculateTopUpQuota(1.000001, 0.2), "fractional quota is truncated only after base credit is combined with cashback")
	assert.Equal(t, 0, CalculateTopUpQuota(0, 0))
	assert.Equal(t, 0, CalculateTopUpQuota(-1, 0))
	assert.Equal(t, 0, CalculateTopUpQuota(math.NaN(), 0))
}

func TestBuildPaymentQuoteUsesProviderUnitPrice(t *testing.T) {
	previousMethods := operation_setting.PayMethods
	previousUnitPrice := setting.StripeUnitPrice
	operation_setting.PayMethods = nil
	setting.StripeUnitPrice = 1.25
	t.Cleanup(func() {
		operation_setting.PayMethods = previousMethods
		setting.StripeUnitPrice = previousUnitPrice
	})
	quote, err := BuildPaymentQuote(10, "stripe", "default")
	require.NoError(t, err)
	assert.Equal(t, 12.5, quote.BaseAmountUSD)
	assert.Equal(t, 12.5, quote.ChargedAmount)
	displayConfig, err := GetPaymentQuoteDisplayConfig("stripe", "default")
	require.NoError(t, err)
	assert.Equal(t, "USD", displayConfig.Currency)
	assert.Equal(t, 1.0, displayConfig.RateToUSD)
	assert.Equal(t, 1.25, displayConfig.BaseAmountMultiplier)
	assert.InDelta(t, 10*displayConfig.BaseAmountMultiplier*displayConfig.Coefficient*displayConfig.RateToUSD, quote.ChargedAmount, 0.000001)
}

func TestBuildPaymentQuoteAppliesMinimumInProviderCurrency(t *testing.T) {
	previousMethods := operation_setting.PayMethods
	previousWaffoUnitPrice := setting.WaffoUnitPrice
	previousWaffoCurrency := setting.WaffoCurrency
	previousWaffoMinTopUp := setting.WaffoMinTopUp
	operation_setting.PayMethods = nil
	setting.WaffoUnitPrice = 1
	setting.WaffoCurrency = "RUB"
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}))
	previousDB := model.DB
	model.DB = db
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "RUB", Name: "Ruble", Symbol: "₽", Enabled: true,
		ManualRateToUSD: 90, RateToUSD: 90,
	}).Error)
	t.Cleanup(func() {
		model.DB = previousDB
		operation_setting.PayMethods = previousMethods
		setting.WaffoUnitPrice = previousWaffoUnitPrice
		setting.WaffoCurrency = previousWaffoCurrency
		setting.WaffoMinTopUp = previousWaffoMinTopUp
	})

	setting.WaffoMinTopUp = 100
	_, err = BuildPaymentQuote(1, model.PaymentMethodWaffo, "default")
	assert.Error(t, err, "$1 -> ₽90 must be rejected by a ₽100 minimum")
	_, err = BuildPaymentQuote(0.99, model.PaymentMethodWaffo, "default")
	assert.Error(t, err, "minimum is in RUB, so $0.99 -> ₽89.10 must be rejected")
	quote, err := BuildPaymentQuote(1.12, model.PaymentMethodWaffo, "default")
	require.NoError(t, err)
	assert.InDelta(t, 100.8, quote.ChargedAmount, 0.000001)
}

func TestBuildPaymentQuoteAppliesLegacyMethodMinimumInSettlementCurrency(t *testing.T) {
	previousMethods := operation_setting.PayMethods
	operation_setting.PayMethods = []map[string]string{{
		"type": "alipay", "min_topup": "10.50",
	}}
	t.Cleanup(func() { operation_setting.PayMethods = previousMethods })

	_, err := BuildPaymentQuote(10, "alipay", "default")
	assert.Error(t, err)
	quote, err := BuildPaymentQuote(10.5, "alipay", "default")
	require.NoError(t, err)
	assert.Equal(t, 10.5, quote.ChargedAmount)
}

func TestBuildPaymentQuoteUsesPerMethodMinimumForBuiltInGateway(t *testing.T) {
	previousMethods := operation_setting.PayMethods
	previousMin := setting.StripeMinTopUp
	previousUnitPrice := setting.StripeUnitPrice
	operation_setting.PayMethods = []map[string]string{{"type": model.PaymentMethodStripe, "min_topup": "25"}}
	setting.StripeMinTopUp = 1
	setting.StripeUnitPrice = 1
	t.Cleanup(func() {
		operation_setting.PayMethods = previousMethods
		setting.StripeMinTopUp = previousMin
		setting.StripeUnitPrice = previousUnitPrice
	})

	_, err := BuildPaymentQuote(24, model.PaymentMethodStripe, "default")
	assert.Error(t, err)
	quote, err := BuildPaymentQuote(25, model.PaymentMethodStripe, "default")
	require.NoError(t, err)
	assert.Equal(t, 25.0, quote.ChargedAmount)
}

func TestBuildPaymentQuoteReturnsAuthoritativeCashbackAndCreditedAmount(t *testing.T) {
	previousMethods := operation_setting.PayMethods
	previousUnitPrice := setting.StripeUnitPrice
	previousCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	operation_setting.PayMethods = nil
	setting.StripeUnitPrice = 1.25
	// The provider unit price makes the server gross amount $12.50 while the
	// user-entered amount is only $10. Cashback must use that authoritative
	// gross amount, not the raw form value.
	operation_setting.GetPaymentSetting().AmountCashback = operation_setting.AmountCashbackConfig{{
		MinAmount:       12,
		CashbackPercent: 10,
	}}
	t.Cleanup(func() {
		operation_setting.PayMethods = previousMethods
		setting.StripeUnitPrice = previousUnitPrice
		operation_setting.GetPaymentSetting().AmountCashback = previousCashbacks
	})

	quote, err := BuildPaymentQuote(10, model.PaymentMethodStripe, "default")
	require.NoError(t, err)
	assert.Equal(t, 12.5, quote.BaseAmountUSD)
	assert.Equal(t, 0.0, quote.CommissionUSD)
	assert.Equal(t, 10.0, quote.CashbackPercent)
	assert.InDelta(t, 1.25, quote.CashbackAmountUSD, 0.0000001)
	assert.InDelta(t, 12.5, quote.CreditedAmountUSD, 0.0000001)
}

func TestValidatePaymentSnapshotUsesPersistedCurrencyAndAmount(t *testing.T) {
	topUp := &model.TopUp{PaymentCurrency: "RUB", PaymentChargedAmount: 927}
	require.NoError(t, ValidatePaymentSnapshot(topUp, "RUB", 927))
	assert.Error(t, ValidatePaymentSnapshot(topUp, "USD", 927))
	assert.Error(t, ValidatePaymentSnapshot(topUp, "RUB", 926))
}

func TestValidateAndBackfillLegacyNOWPaymentsSnapshot(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	for _, callbackCurrency := range []string{"USDT", "USD", "EUR"} {
		topUp := &model.TopUp{
			PaymentProvider: model.PaymentProviderNOWPayments,
			Money:           12.345,
			PaymentCurrency: "USD",
			TradeNo:         "legacy-now-" + strings.ToLower(callbackCurrency),
		}
		require.NoError(t, db.Create(topUp).Error)

		require.NoError(t, ValidateAndBackfillLegacyPaymentSnapshot(topUp, model.PaymentProviderNOWPayments, strings.ToLower(callbackCurrency), 12.35))
		assert.Equal(t, callbackCurrency, topUp.PaymentCurrency)
		assert.InDelta(t, 12.35, topUp.PaymentChargedAmount, 0.000001)
		stored := &model.TopUp{}
		require.NoError(t, db.First(stored, topUp.Id).Error)
		assert.Equal(t, callbackCurrency, stored.PaymentCurrency)
		assert.InDelta(t, 12.35, stored.PaymentChargedAmount, 0.000001)
	}

	invalidTopUp := &model.TopUp{PaymentProvider: model.PaymentProviderNOWPayments, Money: 12.34, PaymentCurrency: "USD", TradeNo: "legacy-now-invalid"}
	require.NoError(t, db.Create(invalidTopUp).Error)
	assert.Error(t, ValidateAndBackfillLegacyPaymentSnapshot(invalidTopUp, model.PaymentProviderNOWPayments, "DOGE", 12.34))
}

func TestValidateAndBackfillLegacyWaffoAndCreemUseCallbackCurrency(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))

	waffoTopUp := &model.TopUp{PaymentProvider: model.PaymentProviderWaffo, Money: 1234.567, PaymentCurrency: "USD", TradeNo: "legacy-waffo"}
	require.NoError(t, db.Create(waffoTopUp).Error)
	require.NoError(t, ValidateAndBackfillLegacyPaymentSnapshot(waffoTopUp, model.PaymentProviderWaffo, "rub", 1234.57))
	assert.Equal(t, "RUB", waffoTopUp.PaymentCurrency)

	creemTopUp := &model.TopUp{PaymentProvider: model.PaymentProviderCreem, Money: 12.34, PaymentCurrency: "USD", TradeNo: "legacy-creem"}
	require.NoError(t, db.Create(creemTopUp).Error)
	require.NoError(t, ValidateAndBackfillLegacyPaymentSnapshot(creemTopUp, model.PaymentProviderCreem, "eur", 12.34))
	assert.Equal(t, "EUR", creemTopUp.PaymentCurrency)
	assert.Error(t, ValidateAndBackfillLegacyPaymentSnapshot(creemTopUp, model.PaymentProviderCreem, "eur", 12.35))
}

func TestValidateAndBackfillPaymentSnapshotKeepsNewRowsStrict(t *testing.T) {
	topUp := &model.TopUp{PaymentProvider: model.PaymentProviderWaffo, PaymentCurrency: "RUB", PaymentBaseAmount: 10, PaymentChargedAmount: 1000, Money: 1000}
	assert.Error(t, ValidateAndBackfillLegacyPaymentSnapshot(topUp, model.PaymentProviderWaffo, "USD", 1000))
	assert.Error(t, ValidateAndBackfillLegacyPaymentSnapshot(topUp, model.PaymentProviderWaffo, "RUB", 999))
	require.NoError(t, ValidateAndBackfillLegacyPaymentSnapshot(topUp, model.PaymentProviderWaffo, "RUB", 1000))
}

func TestGetPlatformCurrencyRateRejectsStaleSyncQuote(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}, &model.CurrencyExchangeRate{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	lastSync := time.Now().UTC().Add(-49 * time.Hour)
	require.NoError(t, db.Create(&model.PlatformCurrency{Code: "RUB", Name: "Ruble", Symbol: "₽", Enabled: true, SyncEnabled: true, SyncProvider: "cbr", RateToUSD: 90, LastSyncAt: &lastSync}).Error)
	require.NoError(t, db.Create(&model.CurrencyExchangeRate{BaseCurrency: "USD", QuoteCurrency: "RUB", Provider: "cbr", Rate: 90, RecordedAt: lastSync}).Error)
	_, err = GetPlatformCurrencyRate("RUB")
	assert.Error(t, err)
}

func TestBuildPaymentQuoteUsesWaffoNonUSDRate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}))
	previousDB := model.DB
	model.DB = db
	previousMethods := operation_setting.PayMethods
	previousUnitPrice := setting.WaffoUnitPrice
	previousCurrency := setting.WaffoCurrency
	operation_setting.PayMethods = nil
	setting.WaffoUnitPrice = 1
	commonOption := model.PlatformCurrency{Code: "RUB", Name: "Ruble", Symbol: "₽", Enabled: true, ManualRateToUSD: 90, RateToUSD: 90}
	require.NoError(t, db.Create(&commonOption).Error)
	// Waffo currency is configured by the integration setting; use the public
	// helper only to verify the central quote once the row is available.
	t.Cleanup(func() {
		model.DB = previousDB
		operation_setting.PayMethods = previousMethods
		setting.WaffoUnitPrice = previousUnitPrice
		setting.WaffoCurrency = previousCurrency
	})
	setting.WaffoCurrency = "RUB"
	quote, err := BuildPaymentQuote(2, model.PaymentMethodWaffo, "default")
	require.NoError(t, err)
	assert.Equal(t, 180.0, quote.ChargedAmount)
	assert.InDelta(t, quote.ChargedAmountUSD*quote.RateToUSD, quote.ChargedAmount, 0.000001)
	assert.InDelta(t, quote.BaseAmountUSD*90, quote.ChargedAmountUSD*quote.RateToUSD, 0.000001)
}

func TestBuildPaymentQuoteMatchesDecimalPreviewFormula(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}))
	previousDB := model.DB
	previousMethods := operation_setting.PayMethods
	previousRatios := common.TopupGroupRatio2JSONString()
	previousCurrency := setting.WaffoCurrency
	previousUnitPrice := setting.WaffoUnitPrice
	model.DB = db
	operation_setting.PayMethods = nil
	setting.WaffoCurrency = "RUB"
	setting.WaffoUnitPrice = 1
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1.1}`))
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "RUB", Name: "Ruble", Symbol: "₽", Enabled: true,
		ManualRateToUSD: 90, RateToUSD: 90,
	}).Error)
	t.Cleanup(func() {
		model.DB = previousDB
		operation_setting.PayMethods = previousMethods
		setting.WaffoCurrency = previousCurrency
		setting.WaffoUnitPrice = previousUnitPrice
		_ = common.UpdateTopupGroupRatioByJSONString(previousRatios)
	})

	// The browser evaluates the decimal inputs as 0.1 × 1.1 × 90 = 9.90.
	// Float multiplication before decimal conversion produces 9.91 here.
	quote, err := BuildPaymentQuote(0.1, model.PaymentMethodWaffo, "default")
	require.NoError(t, err)
	assert.Equal(t, 9.9, quote.ChargedAmount)
	assert.Equal(t, 0.1, quote.BaseAmountUSD)
	assert.Equal(t, 0.01, quote.CommissionUSD)
}

func TestWaffoDisplayConfigUsesZeroDecimalProviderCurrency(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}))
	previousDB := model.DB
	previousMethods := operation_setting.PayMethods
	previousCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	previousUnitPrice := setting.WaffoUnitPrice
	previousCurrency := setting.WaffoCurrency
	previousRatios := common.TopupGroupRatio2JSONString()
	model.DB = db
	operation_setting.PayMethods = nil
	operation_setting.GetPaymentSetting().AmountCashback = nil
	setting.WaffoUnitPrice = 1
	setting.WaffoCurrency = "JPY"
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "JPY", Name: "Japanese Yen", Symbol: "¥", Enabled: true,
		ManualRateToUSD: 150, RateToUSD: 150,
	}).Error)
	t.Cleanup(func() {
		model.DB = previousDB
		operation_setting.PayMethods = previousMethods
		operation_setting.GetPaymentSetting().AmountCashback = previousCashbacks
		setting.WaffoUnitPrice = previousUnitPrice
		setting.WaffoCurrency = previousCurrency
		_ = common.UpdateTopupGroupRatioByJSONString(previousRatios)
	})

	config, err := GetPaymentQuoteDisplayConfig(model.PaymentMethodWaffo, "default")
	require.NoError(t, err)
	assert.Equal(t, "JPY", config.Currency)
	assert.Equal(t, 0, config.RoundingDecimals)
	quote, err := BuildPaymentQuote(1.234, model.PaymentMethodWaffo, "default")
	require.NoError(t, err)
	assert.Equal(t, 186.0, quote.ChargedAmount)
	assert.Equal(t, 186.0, math.Round(quote.ChargedAmount))
}

func TestBuildPaymentQuoteRoundsProviderAmountUpward(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}))
	previousDB := model.DB
	previousMethods := operation_setting.PayMethods
	previousCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	previousUnitPrice := setting.WaffoUnitPrice
	previousCurrency := setting.WaffoCurrency
	model.DB = db
	operation_setting.PayMethods = nil
	operation_setting.GetPaymentSetting().AmountCashback = nil
	setting.WaffoUnitPrice = 1
	setting.WaffoCurrency = "RUB"
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "RUB", Name: "Ruble", Symbol: "₽", Enabled: true,
		ManualRateToUSD: 1.234, RateToUSD: 1.234,
	}).Error)
	t.Cleanup(func() {
		model.DB = previousDB
		operation_setting.PayMethods = previousMethods
		operation_setting.GetPaymentSetting().AmountCashback = previousCashbacks
		setting.WaffoUnitPrice = previousUnitPrice
		setting.WaffoCurrency = previousCurrency
	})

	// A whole wallet dollar produces 1.234 provider units. The provider must
	// receive the safe upward-rounded 1.24, never a rounded-down 1.23.
	quote, err := BuildPaymentQuote(1, model.PaymentMethodWaffo, "default")
	require.NoError(t, err)
	assert.Equal(t, 1.24, quote.ChargedAmount)
	assert.InDelta(t, 1.24/1.234, quote.ChargedAmountUSD, 0.0000001)
	assert.GreaterOrEqual(t, quote.ChargedAmountUSD, quote.BaseAmountUSD)
	assert.LessOrEqual(t, quote.CreditedAmountUSD, quote.ChargedAmountUSD)
}

func TestBuildPaymentQuoteKeepsBaseWhenProviderAmountIsRounded(t *testing.T) {
	previousMethods := operation_setting.PayMethods
	previousRatios := common.TopupGroupRatio2JSONString()
	previousCashbacks := operation_setting.GetPaymentSetting().AmountCashback
	operation_setting.PayMethods = nil
	operation_setting.GetPaymentSetting().AmountCashback = nil
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1.2}`))
	t.Cleanup(func() {
		operation_setting.PayMethods = previousMethods
		operation_setting.GetPaymentSetting().AmountCashback = previousCashbacks
		_ = common.UpdateTopupGroupRatioByJSONString(previousRatios)
	})

	quote, err := BuildPaymentQuote(1.001, model.PaymentMethodWaffoPancake, "default")
	require.NoError(t, err)
	assert.Equal(t, 1.21, quote.ChargedAmount)
	assert.InDelta(t, 1.001, quote.BaseAmountUSD, 0.0000001)
	assert.InDelta(t, 0.209, quote.CommissionUSD, 0.0000001)
	topUp := &model.TopUp{RequestedAmount: 1.001}
	ApplyPaymentQuoteSnapshot(topUp, quote, quote.ChargedAmount)
	assert.InDelta(t, quote.BaseAmountUSD, topUp.PaymentBaseAmount, 0.0000001)
	assert.InDelta(t, quote.CommissionUSD, topUp.PaymentCommission, 0.0000001)
	assert.LessOrEqual(t, topUp.PaymentBaseAmount, topUp.PaymentChargedAmount)
}

func TestBuildPaymentQuoteUsesPersistedMethodCurrencyAndGroup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}))
	previousDB := model.DB
	previousMethods := operation_setting.PayMethods
	previousRatios := common.TopupGroupRatio2JSONString()
	model.DB = db
	operation_setting.PayMethods = []map[string]string{{
		"type": "yookassa_sbp", "currency": "RUB", "topup_group": "premium",
	}}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"premium":1.1}`))
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "RUB", Name: "Ruble", Symbol: "₽", Enabled: true,
		ManualRateToUSD: 90, RateToUSD: 90,
	}).Error)
	t.Cleanup(func() {
		model.DB = previousDB
		operation_setting.PayMethods = previousMethods
		_ = common.UpdateTopupGroupRatioByJSONString(previousRatios)
	})

	quote, err := BuildPaymentQuote(10, model.PaymentMethodYooKassaSBP, "default")
	require.NoError(t, err)
	assert.Equal(t, "RUB", quote.Currency)
	assert.Equal(t, 1.1, quote.Coefficient)
	assert.InDelta(t, 990, quote.ChargedAmount, 0.000001)
	displayConfig, err := GetPaymentQuoteDisplayConfig(model.PaymentMethodYooKassaSBP, "default")
	require.NoError(t, err)
	assert.Equal(t, "RUB", displayConfig.Currency)
	assert.Equal(t, 90.0, displayConfig.RateToUSD)
	assert.Equal(t, 1.0, displayConfig.BaseAmountMultiplier)
	assert.Equal(t, 1.1, displayConfig.Coefficient)
	assert.Equal(t, 2, displayConfig.RoundingDecimals)
	assert.InDelta(t, 10*displayConfig.BaseAmountMultiplier*displayConfig.Coefficient*displayConfig.RateToUSD, quote.ChargedAmount, 0.000001)
}
