package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuccessfulTopUpRewardsOnlyDirectInviter(t *testing.T) {
	truncateTables(t)
	originalPercent := common.ReferralDepositPercent
	common.ReferralDepositPercent = 10
	t.Cleanup(func() { common.ReferralDepositPercent = originalPercent })

	// A invited B, and B invited C. C's deposit must reward B only.
	require.NoError(t, DB.Create(&User{Id: 7101, Username: "direct-inviter", AffCode: "direct-7101", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 7102, Username: "middle-invitee", AffCode: "direct-7102", InviterId: 7101, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 7103, Username: "deposit-user", AffCode: "direct-7103", InviterId: 7102, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId: 7103, Amount: 100, Money: 100, TradeNo: "direct-referral-topup",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
		QuotaToAdd: 1000, CreateTime: time.Now().Unix(),
	}).Error)

	require.NoError(t, RechargeEpay("direct-referral-topup", "sbp", "127.0.0.1"))

	var inviter, ancestor User
	require.NoError(t, DB.Select("aff_quota", "aff_history").First(&inviter, 7102).Error)
	require.NoError(t, DB.Select("aff_quota", "aff_history").First(&ancestor, 7101).Error)
	assert.Equal(t, 100, inviter.AffQuota)
	assert.Equal(t, 100, inviter.AffHistoryQuota)
	assert.Zero(t, ancestor.AffQuota)
	assert.Zero(t, ancestor.AffHistoryQuota)

	var reward TopUp
	require.NoError(t, DB.Where("user_id = ? AND source = ?", 7102, "referral_income").First(&reward).Error)
	assert.Equal(t, "referral", reward.PaymentMethod)
	assert.Equal(t, common.TopUpStatusSuccess, reward.Status)
}

func TestUserCreationPersistsDirectInviter(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7121, Username: "registration-inviter", AffCode: "registration-7121", Status: common.UserStatusEnabled}).Error)
	invitee := &User{Id: 7122, Username: "registration-invitee", Status: common.UserStatusEnabled}
	tx := DB.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, invitee.InsertWithTx(tx, 7121))
	require.NoError(t, tx.Commit().Error)

	var stored User
	require.NoError(t, DB.Select("inviter_id").First(&stored, 7122).Error)
	assert.Equal(t, 7121, stored.InviterId)
}

func TestTopUpHistorySourceDoesNotExposeUnknownConfiguredNames(t *testing.T) {
	assert.Equal(t, "Stripe", PaymentMethodDisplayName(PaymentMethodStripe))
	assert.Empty(t, PaymentMethodDisplayName("custom_gateway"))
	unknown := &TopUp{Source: "alice", PaymentMethod: "custom_gateway"}
	annotateTopupSources([]*TopUp{unknown})
	assert.Empty(t, unknown.Source)
}

func TestTopUpHistoryUsesConfiguredYooKassaDisplayName(t *testing.T) {
	originalMethods := operation_setting.PayMethods
	operation_setting.PayMethods = []map[string]string{{"type": PaymentMethodYooKassaSBP, "name": "СБП"}}
	t.Cleanup(func() { operation_setting.PayMethods = originalMethods })

	record := &TopUp{PaymentMethod: PaymentMethodYooKassaSBP}
	annotateTopupSources([]*TopUp{record})
	assert.Empty(t, record.Source)
	assert.Equal(t, "СБП", record.PaymentMethodName)
	technical := &TopUp{PaymentMethod: PaymentMethodYooKassaSBP, PaymentMethodName: "YooKassa"}
	annotateTopupSources([]*TopUp{technical})
	assert.Equal(t, "СБП", technical.PaymentMethodName)
}

func TestPaymentMethodDisplayNameUsesSafeConfiguredAndCanonicalNames(t *testing.T) {
	originalMethods := operation_setting.PayMethods
	operation_setting.PayMethods = []map[string]string{{"type": "custom1", "name": "FastPay"}}
	t.Cleanup(func() { operation_setting.PayMethods = originalMethods })

	assert.Equal(t, "СБП", PaymentMethodDisplayName(PaymentMethodYooKassaSBP))
	assert.Equal(t, "Crypto / NOWPayments", PaymentMethodDisplayName(PaymentMethodNOWPayments))
	assert.Equal(t, "FastPay", PaymentMethodDisplayName("custom1"))
	assert.Empty(t, PaymentMethodDisplayName("provider_12345"))
}

func TestPaymentMethodDisplayNamePrefersConfiguredBuiltinName(t *testing.T) {
	originalMethods := operation_setting.PayMethods
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "Alipay QR"}}
	t.Cleanup(func() { operation_setting.PayMethods = originalMethods })

	assert.Equal(t, "Alipay QR", PaymentMethodDisplayName("alipay"))
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	assert.Equal(t, "Alipay", PaymentMethodDisplayName("alipay"))
}

func TestPaymentMethodDisplayNameUsesSnapshotDuringUpdates(t *testing.T) {
	originalMethods := operation_setting.PayMethods
	t.Cleanup(func() { operation_setting.PayMethods = originalMethods })
	require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(`[{"type":"alipay","name":"Alipay A"}]`))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_ = PaymentMethodDisplayName("alipay")
		}
	}()
	for i := 0; i < 100; i++ {
		require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(`[{"type":"alipay","name":"Alipay B"}]`))
	}
	<-done
}

func TestTopUpHistoryPreservesConfiguredPaymentMethodSnapshot(t *testing.T) {
	originalMethods := operation_setting.PayMethods
	operation_setting.PayMethods = []map[string]string{{"type": "custom1", "name": "FastPay"}}
	t.Cleanup(func() { operation_setting.PayMethods = originalMethods })

	assert.Equal(t, "FastPay", PaymentMethodDisplayName("custom1"))
	record := &TopUp{PaymentMethod: "custom1", PaymentMethodName: "FastPay"}
	operation_setting.PayMethods = []map[string]string{{"type": "custom1", "name": "Renamed payment"}}
	annotateTopupSources([]*TopUp{record})
	assert.Equal(t, "FastPay", record.PaymentMethodName)
}

func TestTopUpHistoryPreservesSelectedWaffoMethodSnapshot(t *testing.T) {
	record := &TopUp{PaymentMethod: PaymentMethodWaffo, PaymentMethodName: "Apple Pay"}
	annotateTopupSources([]*TopUp{record})
	assert.Equal(t, "Apple Pay", record.PaymentMethodName)
}

func TestTopUpHistoryRejectsUnsafePaymentMethodSnapshot(t *testing.T) {
	record := &TopUp{PaymentMethod: "unknown_gateway", PaymentMethodName: "raw\nprovider-value"}
	annotateTopupSources([]*TopUp{record})
	assert.Empty(t, record.PaymentMethodName)
}

func TestTopUpHistoryUsesImmutableAccountingUSDForProviderCurrency(t *testing.T) {
	creem := &TopUp{
		PaymentCurrency:      "EUR",
		PaymentBaseAmount:    8,
		PaymentChargedAmount: 10,
		RequestedAmount:      10,
		Money:                10,
	}
	annotateTopupSources([]*TopUp{creem})
	assert.InDelta(t, 8, creem.AccountingAmountUSD, 0.000001)

	// A legacy USD row can still use its captured requested amount as a
	// presentation fallback when no provider conversion is involved.
	tokens := &TopUp{
		PaymentCurrency: "USD", RequestedAmount: 100, Money: 100,
		QuotaToAdd: int(common.QuotaPerUnit / 2),
	}
	annotateTopupSources([]*TopUp{tokens})
	assert.InDelta(t, 100, tokens.AccountingAmountUSD, 0.000001)
}

func TestTopUpHistoryDoesNotTreatUnknownProviderCurrencyAsUSD(t *testing.T) {
	record := &TopUp{PaymentCurrency: "EUR", RequestedAmount: 12.5, Money: 12.5}
	annotateTopupSources([]*TopUp{record})
	assert.Zero(t, record.AccountingAmountUSD)

	legacyTokens := &TopUp{
		PaymentCurrency: "USD", PaymentProvider: PaymentProviderNOWPayments,
		RequestedAmount: 500000, Money: 10,
		QuotaToAdd: int(common.QuotaPerUnit),
	}
	annotateTopupSources([]*TopUp{legacyTokens})
	assert.Zero(t, legacyTokens.AccountingAmountUSD)

	legacyWaffo := &TopUp{
		PaymentCurrency: "USD", PaymentProvider: PaymentProviderWaffo,
		RequestedAmount: 500000, Money: 10,
	}
	annotateTopupSources([]*TopUp{legacyWaffo})
	assert.Zero(t, legacyWaffo.AccountingAmountUSD)

	legacyEpayTokens := &TopUp{
		PaymentCurrency: "USD", PaymentProvider: PaymentProviderEpay,
		RequestedAmount: 500000, Money: 10,
	}
	annotateTopupSources([]*TopUp{legacyEpayTokens})
	assert.InDelta(t, 10, legacyEpayTokens.AccountingAmountUSD, 0.000001)
}

func TestQuotaOnlyHistoryKeepsAccountingAmountAfterQuotaUnitChange(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	record := &TopUp{
		PaymentCurrency:   "USD",
		PaymentBaseAmount: quotaToUSD(250),
		QuotaToAdd:        250,
	}
	common.QuotaPerUnit = 500
	annotateTopupSources([]*TopUp{record})
	assert.InDelta(t, 2.5, record.AccountingAmountUSD, 0.000001)
}

func TestPromoCodeTopUpRewardsDirectInviterAndHasNeutralSource(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Redemption{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM redemptions") })
	originalPercent := common.ReferralDepositPercent
	common.ReferralDepositPercent = 10
	t.Cleanup(func() { common.ReferralDepositPercent = originalPercent })

	require.NoError(t, DB.Create(&User{Id: 7111, Username: "promo-inviter", AffCode: "promo-7111", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 7112, Username: "promo-user", AffCode: "promo-7112", InviterId: 7111, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&Redemption{
		Key: "promo-referral-code", Quota: 500, Status: common.RedemptionCodeStatusEnabled,
		CreatedTime: time.Now().Unix(),
	}).Error)

	quota, err := Redeem("promo-referral-code", 7112)
	require.NoError(t, err)
	assert.Equal(t, 500, quota)

	var promo TopUp
	require.NoError(t, DB.Where("user_id = ? AND source = ?", 7112, "promo_code").First(&promo).Error)
	assert.Equal(t, "promo_code", promo.PaymentMethod)

	var inviter User
	require.NoError(t, DB.Select("aff_quota", "aff_history").First(&inviter, 7111).Error)
	assert.Equal(t, 50, inviter.AffQuota)
	assert.Equal(t, 50, inviter.AffHistoryQuota)
}
