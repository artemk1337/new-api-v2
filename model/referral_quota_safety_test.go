package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateQuotaPerUnitRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"0", "-1", "NaN", "+Inf", "not-a-number"} {
		require.Error(t, validateOptionValue("QuotaPerUnit", value), value)
	}
	for _, value := range []string{"0.1", "500000"} {
		require.NoError(t, validateOptionValue("QuotaPerUnit", value), value)
	}
}

func TestReferralRewardWithInvalidQuotaPerUnitDoesNotPanic(t *testing.T) {
	truncateTables(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	originalPercent := common.GetReferralDepositPercent()
	common.QuotaPerUnit = 0
	common.SetReferralDepositPercent(10)
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.SetReferralDepositPercent(originalPercent)
	})

	require.NoError(t, DB.Create(&User{Id: 7211, Username: "quota-safe-inviter", AffCode: "quota-safe-inviter", InviterId: 0, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 7212, Username: "quota-safe-invitee", AffCode: "quota-safe-invitee", InviterId: 7211, Status: common.UserStatusEnabled}).Error)
	topUp := &TopUp{UserId: 7212, TradeNo: "quota-safe-referral", Status: common.TopUpStatusSuccess, BaseQuotaToAdd: 1000}

	require.NotPanics(t, func() {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return creditReferralDepositReward(tx, topUp, 1000)
		}))
	})
	var reward TopUp
	require.NoError(t, DB.Where("user_id = ? AND source = ?", 7211, "referral_income").First(&reward).Error)
	assert.Zero(t, reward.RequestedAmount)
	assert.Equal(t, int64(100), reward.Amount)
}

func TestPromoTopUpWithInvalidQuotaPerUnitDoesNotPanic(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Redemption{}))
	t.Cleanup(func() { _ = DB.Exec("DELETE FROM redemptions").Error })
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 0
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	require.NoError(t, DB.Create(&User{Id: 7221, Username: "promo-quota-safe", AffCode: "promo-quota-safe", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&Redemption{
		Key: "promo-quota-safe-code", Quota: 100, Status: common.RedemptionCodeStatusEnabled,
		CreatedTime: time.Now().Unix(),
	}).Error)

	var quota int
	require.NotPanics(t, func() {
		var err error
		quota, err = Redeem("promo-quota-safe-code", 7221)
		require.NoError(t, err)
	})
	assert.Equal(t, 100, quota)
	var promo TopUp
	require.NoError(t, DB.Where("user_id = ? AND source = ?", 7221, "promo_code").First(&promo).Error)
	assert.Zero(t, promo.RequestedAmount)
}
