package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInsertWithTxPersistsInviterID(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{
		Id:       1200,
		Username: "oauth-referral-inviter",
		AffCode:  "oauth-referral-inviter-code",
		Status:   common.UserStatusEnabled,
	}).Error)

	user := &User{Username: "oauth-referral-invitee", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return user.InsertWithTx(tx, 1200)
	}))

	var stored User
	require.NoError(t, DB.Select("inviter_id").First(&stored, user.Id).Error)
	assert.Equal(t, 1200, stored.InviterId)
}

func TestReferralQualificationAndEntitlementAreSeparate(t *testing.T) {
	truncateTables(t)
	inviter := &User{Id: 1210, Username: "qualified-inviter", AffCode: "qualified-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId: inviter.Id, TradeNo: "qualification-99", Status: common.TopUpStatusSuccess,
		PaymentProvider: PaymentProviderStripe, PaymentCurrency: "USD", PaymentBaseAmount: 99,
		QuotaToAdd: 99, BaseQuotaToAdd: 99,
	}).Error)
	require.NoError(t, PromoteReferralCashbackEligibility(DB, inviter.Id))
	_, err := ResolveQualifiedInviter(inviter.AffCode)
	require.ErrorIs(t, err, ErrReferralInviterNotQualified)

	require.NoError(t, DB.Create(&TopUp{
		UserId: inviter.Id, TradeNo: "qualification-1", Status: common.TopUpStatusSuccess,
		PaymentProvider: PaymentProviderStripe, PaymentCurrency: "USD", PaymentBaseAmount: 1,
		QuotaToAdd: 1, BaseQuotaToAdd: 1,
	}).Error)
	require.NoError(t, PromoteReferralCashbackEligibility(DB, inviter.Id))
	inviterId, err := ResolveQualifiedInviter(inviter.AffCode)
	require.NoError(t, err)
	assert.Equal(t, inviter.Id, inviterId)

	invitee := &User{Id: 1211, Username: "entitled-invitee", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return invitee.InsertWithTx(tx, inviterId)
	}))
	var stored User
	require.NoError(t, DB.Select("inviter_id", "referral_cashback_eligible", "referral_program_qualified").First(&stored, invitee.Id).Error)
	assert.Equal(t, inviter.Id, stored.InviterId)
	assert.True(t, stored.ReferralCashbackEligible)
	assert.False(t, stored.ReferralProgramQualified)
}

func TestReferralQualificationUsesConfiguredTopUpThreshold(t *testing.T) {
	truncateTables(t)
	originalThreshold := common.GetReferralRequiredTopUpUSD()
	common.SetReferralRequiredTopUpUSD(125)
	t.Cleanup(func() { common.SetReferralRequiredTopUpUSD(originalThreshold) })

	inviter := &User{Id: 1212, Username: "configured-threshold-inviter", AffCode: "configured-threshold-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId: inviter.Id, TradeNo: "configured-threshold-124", Status: common.TopUpStatusSuccess,
		PaymentProvider: PaymentProviderStripe, PaymentCurrency: "USD", PaymentBaseAmount: 124.99,
		QuotaToAdd: 124, BaseQuotaToAdd: 124,
	}).Error)
	require.NoError(t, PromoteReferralCashbackEligibility(DB, inviter.Id))
	_, err := ResolveQualifiedInviter(inviter.AffCode)
	require.ErrorIs(t, err, ErrReferralInviterNotQualified)

	require.NoError(t, DB.Create(&TopUp{
		UserId: inviter.Id, TradeNo: "configured-threshold-1", Status: common.TopUpStatusSuccess,
		PaymentProvider: PaymentProviderStripe, PaymentCurrency: "USD", PaymentBaseAmount: 0.01,
		QuotaToAdd: 1, BaseQuotaToAdd: 1,
	}).Error)
	require.NoError(t, PromoteReferralCashbackEligibility(DB, inviter.Id))
	_, err = ResolveQualifiedInviter(inviter.AffCode)
	require.NoError(t, err)

	common.SetReferralRequiredTopUpUSD(126)
	_, err = ResolveQualifiedInviter(inviter.AffCode)
	require.ErrorIs(t, err, ErrReferralInviterNotQualified)
}

func TestBackfillReferralCashbackEligibilityUsesConfiguredThreshold(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	originalThreshold := common.GetReferralRequiredTopUpUSD()
	common.SetReferralRequiredTopUpUSD(125)
	t.Cleanup(func() { common.SetReferralRequiredTopUpUSD(originalThreshold) })

	inviter := &User{Id: 1214, Username: "backfill-threshold-inviter", AffCode: "backfill-threshold-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId: inviter.Id, TradeNo: "backfill-threshold-100", Status: common.TopUpStatusSuccess,
		PaymentProvider: PaymentProviderStripe, PaymentCurrency: "USD", PaymentBaseAmount: 100,
		QuotaToAdd: 100, BaseQuotaToAdd: 100,
	}).Error)
	invitee := &User{Id: 1215, Username: "backfill-threshold-invitee", Status: common.UserStatusEnabled, InviterId: inviter.Id}
	require.NoError(t, DB.Create(invitee).Error)

	require.NoError(t, BackfillReferralCashbackEligibility())
	var storedInviter, storedInvitee User
	require.NoError(t, DB.Select("referral_program_qualified").First(&storedInviter, inviter.Id).Error)
	require.NoError(t, DB.Select("referral_cashback_eligible").First(&storedInvitee, invitee.Id).Error)
	assert.False(t, storedInviter.ReferralProgramQualified)
	assert.False(t, storedInvitee.ReferralCashbackEligible)
	var marker Option
	require.NoError(t, DB.First(&marker, "key = ?", referralEligibilityBackfillOption).Error)
}

func TestReferralRegistrationDoesNotGrantLegacyQuotaRewards(t *testing.T) {
	truncateTables(t)
	originalInviterQuota := common.QuotaForInviter
	originalInviteeQuota := common.QuotaForInvitee
	originalNewUserQuota := common.QuotaForNewUser
	common.QuotaForInviter = 900
	common.QuotaForInvitee = 800
	common.QuotaForNewUser = 0
	t.Cleanup(func() {
		common.QuotaForInviter = originalInviterQuota
		common.QuotaForInvitee = originalInviteeQuota
		common.QuotaForNewUser = originalNewUserQuota
	})

	inviter := &User{Id: 1213, Username: "no-legacy-reward-inviter", AffCode: "no-legacy-reward-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{Username: "no-legacy-reward-invitee", Status: common.UserStatusEnabled}
	require.NoError(t, invitee.Insert(inviter.Id))

	var storedInviter, storedInvitee User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&storedInvitee, invitee.Id).Error)
	assert.Equal(t, 1, storedInviter.AffCount)
	assert.Zero(t, storedInviter.AffQuota)
	assert.Zero(t, storedInviter.AffHistoryQuota)
	assert.Zero(t, storedInvitee.Quota)
}

func TestTransferAffQuotaToQuotaInvalidatesCacheAfterCommit(t *testing.T) {
	truncateTables(t)
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	previousClient := common.RDB
	previousEnabled := common.RedisEnabled
	common.RDB = redisClient
	common.RedisEnabled = true
	t.Cleanup(func() {
		common.RDB = previousClient
		common.RedisEnabled = previousEnabled
		require.NoError(t, redisClient.Close())
	})

	user := &User{
		Id:       1201,
		Username: "affiliate-transfer-user",
		AffCode:  "affiliate-transfer-code",
		Status:   common.UserStatusEnabled,
		AffQuota: int(common.QuotaPerUnit),
	}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, updateUserCache(*user))

	cacheKey := getUserCacheKey(user.Id)
	exists, err := redisClient.Exists(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), exists)

	require.NoError(t, user.TransferAffQuotaToQuota(int(common.QuotaPerUnit)))
	var stored User
	require.NoError(t, DB.Select("quota").First(&stored, user.Id).Error)
	assert.Equal(t, int(common.QuotaPerUnit), stored.Quota)
	exists, err = redisClient.Exists(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)

	rollbackUser := &User{
		Id:       1202,
		Username: "affiliate-transfer-rollback-user",
		AffCode:  "affiliate-transfer-rollback-code",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(rollbackUser).Error)
	require.NoError(t, updateUserCache(*rollbackUser))
	require.Error(t, rollbackUser.TransferAffQuotaToQuota(int(common.QuotaPerUnit)))
	exists, err = redisClient.Exists(context.Background(), getUserCacheKey(rollbackUser.Id)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)
}
