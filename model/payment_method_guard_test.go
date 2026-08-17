package model

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func insertUserForPaymentGuardTest(t *testing.T, id int, quota int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "payment_guard_user",
		Status:   common.UserStatusEnabled,
		Quota:    quota,
	}
	require.NoError(t, DB.Create(user).Error)
}

func insertSubscriptionPlanForPaymentGuardTest(t *testing.T, id int) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:            id,
		Title:         "Guard Plan",
		PriceAmount:   9.99,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertSubscriptionOrderForPaymentGuardTest(t *testing.T, tradeNo string, userID int, planID int, paymentProvider string) {
	t.Helper()
	order := &SubscriptionOrder{
		UserId:          userID,
		PlanId:          planID,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, order.Insert())
}

func insertTopUpForPaymentGuardTest(t *testing.T, tradeNo string, userID int, paymentProvider string) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func getTopUpStatusForPaymentGuardTest(t *testing.T, tradeNo string) string {
	t.Helper()
	topUp := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	return topUp.Status
}

func countUserSubscriptionsForPaymentGuardTest(t *testing.T, userID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
	return count
}

func getUserQuotaForPaymentGuardTest(t *testing.T, userID int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func setupConcurrentTopUpDB(t *testing.T) (*gorm.DB, chan struct{}, chan struct{}) {
	t.Helper()
	originalDB := DB
	originalLogDB := LOG_DB
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
	})

	databasePath := filepath.Join(t.TempDir(), "topup-concurrent.db")
	db, err := gorm.Open(sqlite.Open("file:"+databasePath+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(4)
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &Log{}))
	DB = db
	LOG_DB = db

	arrivedAtCAS := make(chan struct{}, 2)
	releaseCAS := make(chan struct{})
	var barrierCalls atomic.Int32
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:topup-cas-barrier", func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Name != "TopUp" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if !ok || updates["status"] != common.TopUpStatusSuccess || barrierCalls.Add(1) > 2 {
			return
		}
		arrivedAtCAS <- struct{}{}
		<-releaseCAS
	}))

	return db, arrivedAtCAS, releaseCAS
}

func TestRechargeWaffoPancake_RejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 101, 0)
	insertTopUpForPaymentGuardTest(t, "waffo-pancake-guard", 101, PaymentProviderStripe)

	err := RechargeWaffoPancake("waffo-pancake-guard")
	require.Error(t, err)

	topUp := GetTopUpByTradeNo("waffo-pancake-guard")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 101))
}

func TestRechargeYooKassa_IsIdempotent(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 102, 0)
	topUp := &TopUp{
		UserId:          102,
		Amount:          10,
		Money:           100,
		TradeNo:         "yookassa-idempotent",
		PaymentMethod:   PaymentMethodYooKassaSBP,
		PaymentProvider: PaymentProviderYooKassa,
		QuotaToAdd:      12345,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	require.NoError(t, RechargeYooKassa("yookassa-idempotent", "127.0.0.1"))
	require.NoError(t, RechargeYooKassa("yookassa-idempotent", "127.0.0.1"))

	assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, "yookassa-idempotent"))
	assert.Equal(t, 12345, getUserQuotaForPaymentGuardTest(t, 102))

	var topupLogs int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", 102, LogTypeTopup).Count(&topupLogs).Error)
	assert.Equal(t, int64(1), topupLogs)
}

func TestRechargeYooKassaCreditsConfiguredReferralPercentOnce(t *testing.T) {
	truncateTables(t)
	originalPercent := common.ReferralDepositPercent
	common.ReferralDepositPercent = 10
	t.Cleanup(func() { common.ReferralDepositPercent = originalPercent })

	require.NoError(t, DB.Create(&User{
		Id:       1030,
		Username: "referral_inviter",
		AffCode:  "referral-inviter",
		Status:   common.UserStatusEnabled,
		Quota:    0,
	}).Error)
	require.NoError(t, DB.Create(&User{
		Id:        1031,
		Username:  "referral_invitee",
		AffCode:   "referral-invitee",
		Status:    common.UserStatusEnabled,
		InviterId: 1030,
		Quota:     0,
	}).Error)
	require.NoError(t, (&TopUp{
		UserId:          1031,
		Amount:          10,
		Money:           10,
		TradeNo:         "yookassa-referral-percent",
		PaymentMethod:   PaymentMethodYooKassaSBP,
		PaymentProvider: PaymentProviderYooKassa,
		QuotaToAdd:      5000000,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}).Insert())

	require.NoError(t, RechargeYooKassa("yookassa-referral-percent", "127.0.0.1"))
	require.NoError(t, RechargeYooKassa("yookassa-referral-percent", "127.0.0.1"))

	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, 1031))
	var inviter User
	require.NoError(t, DB.Select("quota", "aff_quota", "aff_history").First(&inviter, 1030).Error)
	assert.Zero(t, inviter.Quota)
	assert.Equal(t, 500000, inviter.AffQuota)
	assert.Equal(t, 500000, inviter.AffHistoryQuota)
}

func TestRechargeYooKassaLegacyPendingUsesMetadataQuota(t *testing.T) {
	testCases := []struct {
		name     string
		amount   int64
		money    float64
		expected int
	}{
		{
			name:     "usd",
			amount:   100,
			money:    100,
			expected: 5000000,
		},
		{
			name:     "tokens",
			amount:   1,
			money:    777,
			expected: 123456,
		},
	}

	for index, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			userID := 110 + index
			tradeNo := "yookassa-legacy-" + tc.name
			insertUserForPaymentGuardTest(t, userID, 0)
			require.NoError(t, (&TopUp{
				UserId:          userID,
				Amount:          tc.amount,
				Money:           tc.money,
				TradeNo:         tradeNo,
				PaymentMethod:   PaymentMethodYooKassaSBP,
				PaymentProvider: PaymentProviderYooKassa,
				Status:          common.TopUpStatusPending,
				CreateTime:      time.Now().Unix(),
			}).Insert())
			require.NoError(t, (&PaymentMetadata{
				TradeNo:           tradeNo,
				PaymentProvider:   PaymentProviderYooKassa,
				ExternalPaymentID: tradeNo + "-external",
				Metadata:          `{"quota_to_add":"` + fmt.Sprint(tc.expected) + `"}`,
				CreateTime:        time.Now().Unix(),
				UpdateTime:        time.Now().Unix(),
			}).Insert())

			require.NoError(t, RechargeYooKassa(tradeNo, "127.0.0.1"))
			require.NoError(t, RechargeYooKassa(tradeNo, "127.0.0.1"))

			assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, tradeNo))
			assert.Equal(t, tc.expected, getUserQuotaForPaymentGuardTest(t, userID))

			var topupLogs int64
			require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", userID, LogTypeTopup).Count(&topupLogs).Error)
			assert.Equal(t, int64(1), topupLogs)
		})
	}
}

func TestRechargeProviderTopUpLegacyPendingFailsClosedWithoutExactQuota(t *testing.T) {
	testCases := []struct {
		name             string
		provider         string
		paymentMethod    string
		metadataProvider string
		metadataJSON     string
		recharge         func(string, string) error
	}{
		{
			name:          "yookassa without metadata",
			provider:      PaymentProviderYooKassa,
			paymentMethod: PaymentMethodYooKassaSBP,
			recharge:      RechargeYooKassa,
		},
		{
			name:             "yookassa without quota field",
			provider:         PaymentProviderYooKassa,
			paymentMethod:    PaymentMethodYooKassaSBP,
			metadataProvider: PaymentProviderYooKassa,
			metadataJSON:     `{}`,
			recharge:         RechargeYooKassa,
		},
		{
			name:             "yookassa with malformed metadata",
			provider:         PaymentProviderYooKassa,
			paymentMethod:    PaymentMethodYooKassaSBP,
			metadataProvider: PaymentProviderYooKassa,
			metadataJSON:     `{`,
			recharge:         RechargeYooKassa,
		},
		{
			name:             "yookassa with non-positive quota",
			provider:         PaymentProviderYooKassa,
			paymentMethod:    PaymentMethodYooKassaSBP,
			metadataProvider: PaymentProviderYooKassa,
			metadataJSON:     `{"quota_to_add":"0"}`,
			recharge:         RechargeYooKassa,
		},
		{
			name:             "yookassa with mismatched metadata provider",
			provider:         PaymentProviderYooKassa,
			paymentMethod:    PaymentMethodYooKassaSBP,
			metadataProvider: PaymentProviderNOWPayments,
			metadataJSON:     `{"quota_to_add":"5000000"}`,
			recharge:         RechargeYooKassa,
		},
		{
			name:          "nowpayments without persisted quota",
			provider:      PaymentProviderNOWPayments,
			paymentMethod: PaymentMethodNOWPayments,
			recharge:      RechargeNOWPayments,
		},
	}

	for index, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			userID := 120 + index
			tradeNo := tc.provider + "-ambiguous-quota"
			insertUserForPaymentGuardTest(t, userID, 0)
			require.NoError(t, (&TopUp{
				UserId:          userID,
				Amount:          10,
				Money:           100,
				TradeNo:         tradeNo,
				PaymentMethod:   tc.paymentMethod,
				PaymentProvider: tc.provider,
				Status:          common.TopUpStatusPending,
				CreateTime:      time.Now().Unix(),
			}).Insert())
			if tc.metadataJSON != "" {
				require.NoError(t, (&PaymentMetadata{
					TradeNo:           tradeNo,
					PaymentProvider:   tc.metadataProvider,
					ExternalPaymentID: tradeNo + "-external",
					Metadata:          tc.metadataJSON,
					CreateTime:        time.Now().Unix(),
					UpdateTime:        time.Now().Unix(),
				}).Insert())
			}

			require.Error(t, tc.recharge(tradeNo, "127.0.0.1"))
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, tradeNo))
			assert.Zero(t, getUserQuotaForPaymentGuardTest(t, userID))
		})
	}
}

func TestRechargeEpay_IsAtomicAndIdempotent(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 103, 0)
	require.NoError(t, (&TopUp{
		UserId:          103,
		Amount:          10,
		Money:           100,
		TradeNo:         "epay-idempotent",
		PaymentMethod:   "initial",
		PaymentProvider: PaymentProviderEpay,
		QuotaToAdd:      5050000,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}).Insert())

	require.NoError(t, RechargeEpay("epay-idempotent", "actual", "127.0.0.1"))
	require.NoError(t, RechargeEpay("epay-idempotent", "actual", "127.0.0.1"))

	topUp := GetTopUpByTradeNo("epay-idempotent")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, "actual", topUp.PaymentMethod)
	assert.Equal(t, 5050000, getUserQuotaForPaymentGuardTest(t, 103))

	var topupLogs int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", 103, LogTypeTopup).Count(&topupLogs).Error)
	assert.Equal(t, int64(1), topupLogs)
}

func TestRechargeEpayCreditsReferralRewardOnce(t *testing.T) {
	truncateTables(t)
	originalPercent := common.ReferralDepositPercent
	common.ReferralDepositPercent = 10
	t.Cleanup(func() { common.ReferralDepositPercent = originalPercent })

	require.NoError(t, DB.Create(&User{
		Id:       1032,
		Username: "epay-referral-inviter",
		AffCode:  "epay-referral-inviter-code",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, DB.Create(&User{
		Id:        1033,
		Username:  "epay-referral-invitee",
		AffCode:   "epay-referral-invitee-code",
		InviterId: 1032,
		Status:    common.UserStatusEnabled,
	}).Error)
	require.NoError(t, (&TopUp{
		UserId:          1033,
		Amount:          10,
		Money:           10,
		TradeNo:         "epay-referral-percent",
		PaymentProvider: PaymentProviderEpay,
		QuotaToAdd:      5000000,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}).Insert())

	require.NoError(t, RechargeEpay("epay-referral-percent", "alipay", "127.0.0.1"))
	require.NoError(t, RechargeEpay("epay-referral-percent", "alipay", "127.0.0.1"))

	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, 1033))
	var inviter User
	require.NoError(t, DB.Select("quota", "aff_quota", "aff_history").First(&inviter, 1032).Error)
	assert.Zero(t, inviter.Quota)
	assert.Equal(t, 500000, inviter.AffQuota)
	assert.Equal(t, 500000, inviter.AffHistoryQuota)
}

func TestRechargeEpayConcurrentCompletionsCreditOnce(t *testing.T) {
	originalDB := DB
	originalLogDB := LOG_DB
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
	})

	databasePath := filepath.Join(t.TempDir(), "epay-concurrent.db")
	db, err := gorm.Open(sqlite.Open("file:"+databasePath+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(4)
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &Log{}))
	DB = db
	LOG_DB = db

	arrivedAtCAS := make(chan struct{}, 2)
	releaseCAS := make(chan struct{})
	var barrierCalls atomic.Int32
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:epay-cas-barrier", func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Name != "TopUp" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if !ok || updates["status"] != common.TopUpStatusSuccess || barrierCalls.Add(1) > 2 {
			return
		}
		arrivedAtCAS <- struct{}{}
		<-releaseCAS
	}))

	truncateTables(t)
	insertUserForPaymentGuardTest(t, 105, 0)
	require.NoError(t, (&TopUp{
		UserId:          105,
		Amount:          10,
		Money:           100,
		TradeNo:         "epay-concurrent",
		PaymentMethod:   "initial",
		PaymentProvider: PaymentProviderEpay,
		QuotaToAdd:      5050000,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}).Insert())

	start := make(chan struct{})
	errorsByCompletion := make([]error, 2)
	var completions sync.WaitGroup
	completions.Add(len(errorsByCompletion))
	for index := range errorsByCompletion {
		go func() {
			defer completions.Done()
			<-start
			errorsByCompletion[index] = RechargeEpay("epay-concurrent", "actual", "127.0.0.1")
		}()
	}
	close(start)
	for range errorsByCompletion {
		select {
		case <-arrivedAtCAS:
		case <-time.After(5 * time.Second):
			close(releaseCAS)
			require.FailNow(t, "both completions did not reach the CAS barrier")
		}
	}
	close(releaseCAS)
	completions.Wait()

	for _, err := range errorsByCompletion {
		require.NoError(t, err)
	}
	assert.Equal(t, 5050000, getUserQuotaForPaymentGuardTest(t, 105))
	assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, "epay-concurrent"))

	var successfulTopUps int64
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ? AND status = ?", "epay-concurrent", common.TopUpStatusSuccess).Count(&successfulTopUps).Error)
	assert.Equal(t, int64(1), successfulTopUps)

	var topupLogs int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", 105, LogTypeTopup).Count(&topupLogs).Error)
	assert.Equal(t, int64(1), topupLogs)
}

func TestTopUpCompletionsConcurrentCreditOnce(t *testing.T) {
	testCases := []struct {
		name     string
		provider string
		complete func(string) error
		verify   func(*testing.T, *User)
	}{
		{
			name:     "stripe",
			provider: PaymentProviderStripe,
			complete: func(tradeNo string) error { return Recharge(tradeNo, "stripe-customer", "127.0.0.1") },
			verify: func(t *testing.T, user *User) {
				assert.Equal(t, "stripe-customer", user.StripeCustomer)
			},
		},
		{
			name:     "waffo",
			provider: PaymentProviderWaffo,
			complete: func(tradeNo string) error { return RechargeWaffo(tradeNo, "127.0.0.1") },
		},
		{
			name:     "waffo pancake",
			provider: PaymentProviderWaffoPancake,
			complete: RechargeWaffoPancake,
		},
		{
			name:     "creem",
			provider: PaymentProviderCreem,
			complete: func(tradeNo string) error {
				return RechargeCreem(tradeNo, "paid@example.com", "Paid User", "127.0.0.1")
			},
			verify: func(t *testing.T, user *User) {
				assert.Equal(t, "paid@example.com", user.Email)
			},
		},
		{
			name:     "manual",
			provider: PaymentProviderEpay,
			complete: func(tradeNo string) error { return ManualCompleteTopUp(tradeNo, "127.0.0.1") },
		},
	}

	for index, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, arrivedAtCAS, releaseCAS := setupConcurrentTopUpDB(t)
			userID := 300 + index
			tradeNo := "concurrent-" + tc.provider + fmt.Sprint(index)
			insertUserForPaymentGuardTest(t, userID, 0)
			require.NoError(t, (&TopUp{
				UserId:          userID,
				Amount:          10,
				Money:           10,
				TradeNo:         tradeNo,
				PaymentMethod:   tc.provider,
				PaymentProvider: tc.provider,
				QuotaToAdd:      5050000,
				Status:          common.TopUpStatusPending,
				CreateTime:      time.Now().Unix(),
			}).Insert())

			start := make(chan struct{})
			errorsByCompletion := make([]error, 2)
			var completions sync.WaitGroup
			completions.Add(len(errorsByCompletion))
			for completionIndex := range errorsByCompletion {
				go func() {
					defer completions.Done()
					<-start
					errorsByCompletion[completionIndex] = tc.complete(tradeNo)
				}()
			}
			close(start)
			for range errorsByCompletion {
				select {
				case <-arrivedAtCAS:
				case <-time.After(5 * time.Second):
					close(releaseCAS)
					require.FailNow(t, "both completions did not reach the CAS barrier")
				}
			}
			close(releaseCAS)
			completions.Wait()

			for _, err := range errorsByCompletion {
				require.NoError(t, err)
			}
			var user User
			require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
			assert.Equal(t, 5050000, user.Quota)
			assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, tradeNo))
			if tc.verify != nil {
				tc.verify(t, &user)
			}

			var topupLogs int64
			require.NoError(t, db.Model(&Log{}).Where("user_id = ? AND type = ?", userID, LogTypeTopup).Count(&topupLogs).Error)
			assert.Equal(t, int64(1), topupLogs)
		})
	}
}

func TestTopUpCompletionsRollBackWhenUserMissing(t *testing.T) {
	testCases := []struct {
		name     string
		provider string
		complete func(string) error
	}{
		{name: "stripe", provider: PaymentProviderStripe, complete: func(tradeNo string) error { return Recharge(tradeNo, "customer", "127.0.0.1") }},
		{name: "waffo", provider: PaymentProviderWaffo, complete: func(tradeNo string) error { return RechargeWaffo(tradeNo, "127.0.0.1") }},
		{name: "waffo pancake", provider: PaymentProviderWaffoPancake, complete: RechargeWaffoPancake},
		{name: "creem", provider: PaymentProviderCreem, complete: func(tradeNo string) error {
			return RechargeCreem(tradeNo, "paid@example.com", "Paid User", "127.0.0.1")
		}},
		{name: "manual", provider: PaymentProviderEpay, complete: func(tradeNo string) error { return ManualCompleteTopUp(tradeNo, "127.0.0.1") }},
	}

	for index, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			tradeNo := "missing-user-" + tc.provider + fmt.Sprint(index)
			require.NoError(t, (&TopUp{
				UserId:          999,
				Amount:          10,
				Money:           10,
				TradeNo:         tradeNo,
				PaymentMethod:   tc.provider,
				PaymentProvider: tc.provider,
				QuotaToAdd:      5050000,
				Status:          common.TopUpStatusPending,
				CreateTime:      time.Now().Unix(),
			}).Insert())

			require.Error(t, tc.complete(tradeNo))
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, tradeNo))

			var topupLogs int64
			require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", 999, LogTypeTopup).Count(&topupLogs).Error)
			assert.Zero(t, topupLogs)
		})
	}
}

func TestManualCompleteTopUpRetryDoesNotWriteZeroUserLog(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 401, 0)
	require.NoError(t, (&TopUp{
		UserId:          401,
		Amount:          10,
		Money:           10,
		TradeNo:         "manual-idempotent-log",
		PaymentMethod:   PaymentMethodBalance,
		PaymentProvider: PaymentProviderBalance,
		QuotaToAdd:      500000,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}).Insert())

	require.NoError(t, ManualCompleteTopUp("manual-idempotent-log", "127.0.0.1"))
	require.NoError(t, ManualCompleteTopUp("manual-idempotent-log", "127.0.0.1"))
	assert.Equal(t, 500000, getUserQuotaForPaymentGuardTest(t, 401))

	var userLogs int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", 401, LogTypeTopup).Count(&userLogs).Error)
	assert.Equal(t, int64(1), userLogs)
	var zeroUserLogs int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", 0, LogTypeTopup).Count(&zeroUserLogs).Error)
	assert.Zero(t, zeroUserLogs)
}

func TestRechargeEpayLegacyPendingUsesBaseQuota(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 104, 0)
	require.NoError(t, (&TopUp{
		UserId:          104,
		Amount:          10,
		Money:           100,
		TradeNo:         "epay-legacy",
		PaymentMethod:   "epay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}).Insert())

	require.NoError(t, RechargeEpay("epay-legacy", "epay", "127.0.0.1"))
	assert.Equal(t, int(10*common.QuotaPerUnit), getUserQuotaForPaymentGuardTest(t, 104))
}

func TestRechargeEpayRollsBackWhenUserMissing(t *testing.T) {
	truncateTables(t)
	require.NoError(t, (&TopUp{
		UserId:          999,
		Amount:          10,
		Money:           100,
		TradeNo:         "epay-missing-user",
		PaymentMethod:   "epay",
		PaymentProvider: PaymentProviderEpay,
		QuotaToAdd:      5050000,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}).Insert())

	require.Error(t, RechargeEpay("epay-missing-user", "epay", "127.0.0.1"))
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, "epay-missing-user"))
}

func TestRechargeProviders_UsePersistedQuotaForFractionalTopUp(t *testing.T) {
	for _, provider := range []string{PaymentProviderStripe, PaymentProviderWaffo, PaymentProviderWaffoPancake} {
		t.Run(provider, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 201, 0)
			require.NoError(t, (&TopUp{
				UserId:          201,
				Amount:          0,
				Money:           0.1,
				TradeNo:         "fractional-" + provider,
				PaymentMethod:   provider,
				PaymentProvider: provider,
				QuotaToAdd:      50000,
				Status:          common.TopUpStatusPending,
				CreateTime:      time.Now().Unix(),
			}).Insert())

			var err error
			if provider == PaymentProviderStripe {
				err = Recharge("fractional-"+provider, "", "127.0.0.1")
			} else if provider == PaymentProviderWaffo {
				err = RechargeWaffo("fractional-"+provider, "127.0.0.1")
			} else {
				err = RechargeWaffoPancake("fractional-" + provider)
			}
			require.NoError(t, err)
			assert.Equal(t, 50000, getUserQuotaForPaymentGuardTest(t, 201))
		})
	}
}

func TestRechargeStripe_UsesLegacyMoneyForIntegerTopUp(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 202, 0)
	require.NoError(t, (&TopUp{
		UserId:          202,
		Amount:          10,
		Money:           20,
		TradeNo:         "stripe-group-ratio",
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}).Insert())

	require.NoError(t, Recharge("stripe-group-ratio", "", "127.0.0.1"))
	assert.Equal(t, int(20*common.QuotaPerUnit), getUserQuotaForPaymentGuardTest(t, 202))
}

func TestRechargeYooKassa_RollsBackWhenUserMissing(t *testing.T) {
	truncateTables(t)

	topUp := &TopUp{
		UserId:          999,
		Amount:          10,
		Money:           100,
		TradeNo:         "yookassa-missing-user",
		PaymentMethod:   PaymentMethodYooKassaSBP,
		PaymentProvider: PaymentProviderYooKassa,
		QuotaToAdd:      12345,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	require.Error(t, RechargeYooKassa("yookassa-missing-user", "127.0.0.1"))
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, "yookassa-missing-user"))

	var topupLogs int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", 999, LogTypeTopup).Count(&topupLogs).Error)
	assert.Zero(t, topupLogs)
}

func TestUpdatePendingTopUpStatus_RejectsMismatchedPaymentProvider(t *testing.T) {
	testCases := []struct {
		name                    string
		tradeNo                 string
		storedPaymentProvider   string
		expectedPaymentProvider string
		targetStatus            string
	}{
		{
			name:                    "stripe expire",
			tradeNo:                 "stripe-expire-guard",
			storedPaymentProvider:   PaymentProviderCreem,
			expectedPaymentProvider: PaymentProviderStripe,
			targetStatus:            common.TopUpStatusExpired,
		},
		{
			name:                    "waffo failed",
			tradeNo:                 "waffo-failed-guard",
			storedPaymentProvider:   PaymentProviderStripe,
			expectedPaymentProvider: PaymentProviderWaffo,
			targetStatus:            common.TopUpStatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 150, 0)
			insertTopUpForPaymentGuardTest(t, tc.tradeNo, 150, tc.storedPaymentProvider)

			err := UpdatePendingTopUpStatus(tc.tradeNo, tc.expectedPaymentProvider, tc.targetStatus)
			require.ErrorIs(t, err, ErrPaymentMethodMismatch)
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, tc.tradeNo))
		})
	}
}

func TestCompleteSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 202, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 301)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-guard-order", 202, plan.Id, PaymentProviderStripe)

	err := CompleteSubscriptionOrder("sub-guard-order", `{"provider":"epay"}`, PaymentProviderEpay, "alipay")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-guard-order")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, 202))

	topUp := GetTopUpByTradeNo("sub-guard-order")
	assert.Nil(t, topUp)
}

func TestExpireSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 303, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 401)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-expire-guard", 303, plan.Id, PaymentProviderStripe)

	err := ExpireSubscriptionOrder("sub-expire-guard", PaymentProviderCreem)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-expire-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}
