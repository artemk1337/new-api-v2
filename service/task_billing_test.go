package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true

	if err := db.AutoMigrate(
		&model.Task{},
		&model.User{},
		&model.Token{},
		&model.Log{},
		&model.Channel{},
		&model.TopUp{},
		&model.UserSubscription{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
		&model.BillingOutbox{},
	); err != nil {
		panic("failed to migrate: " + err.Error())
	}

	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Seed helpers
// ---------------------------------------------------------------------------

func truncate(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM tasks")
		model.DB.Exec("DELETE FROM users")
		model.DB.Exec("DELETE FROM tokens")
		model.DB.Exec("DELETE FROM logs")
		model.DB.Exec("DELETE FROM channels")
		model.DB.Exec("DELETE FROM top_ups")
		model.DB.Exec("DELETE FROM user_subscriptions")
		model.DB.Exec("DELETE FROM system_task_locks")
		model.DB.Exec("DELETE FROM system_tasks")
		model.DB.Exec("DELETE FROM billing_outboxes")
	})
}

func seedUser(t *testing.T, id int, quota int) {
	t.Helper()
	user := &model.User{Id: id, Username: "test_user", Quota: quota, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
}

func seedToken(t *testing.T, id int, userId int, key string, remainQuota int) {
	t.Helper()
	token := &model.Token{
		Id:          id,
		UserId:      userId,
		Key:         key,
		Name:        "test_token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: remainQuota,
		UsedQuota:   0,
	}
	require.NoError(t, model.DB.Create(token).Error)
}

func TestTaskBillingModelNameUsesMappedUpstreamModel(t *testing.T) {
	task := &model.Task{
		Properties: model.Properties{
			OriginModelName:   "alias-model",
			UpstreamModelName: "provider-model",
			IsModelMapped:     true,
		},
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{OriginModelName: "alias-model"},
		},
	}

	assert.Equal(t, "provider-model", taskBillingModelName(task))
	assert.Equal(t, "alias-model", taskModelName(task))
}

func TestTaskBillingModelNameKeepsOriginForUnmappedUpstreamNormalization(t *testing.T) {
	task := &model.Task{
		Properties: model.Properties{
			OriginModelName:   "requested-model",
			UpstreamModelName: "adapter-normalized-model",
		},
	}

	assert.Equal(t, "requested-model", taskBillingModelName(task))
}

func TestTaskBillingModelNameUsesPersistedCompactBillingTarget(t *testing.T) {
	task := &model.Task{
		Properties: model.Properties{
			OriginModelName:   "alias-openai-compact",
			UpstreamModelName: "provider-model",
			IsModelMapped:     true,
			BillingModelName:  "provider-model-openai-compact",
		},
	}

	assert.Equal(t, "provider-model-openai-compact", taskBillingModelName(task))
}

func TestTaskBillingModelNameUsesLegacyMappedTargetWithoutMappingFlag(t *testing.T) {
	oldPrice := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"provider-model":0.5}`))

	task := &model.Task{Properties: model.Properties{
		OriginModelName:   "legacy-alias",
		UpstreamModelName: "provider-model",
	}}

	assert.Equal(t, "provider-model", taskBillingModelName(task))
	other := taskBillingOther(task)
	assert.Equal(t, true, other["is_model_mapped"])
	assert.Equal(t, "provider-model", other["upstream_model_name"])
}

func TestTaskBillingModelNameKeepsLegacyNormalizedOriginWhenItHasPrice(t *testing.T) {
	oldPrice := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"requested-model":0.5,"adapter-normalized-model":1}`))

	task := &model.Task{Properties: model.Properties{
		OriginModelName:   "requested-model",
		UpstreamModelName: "adapter-normalized-model",
	}}

	assert.Equal(t, "requested-model", taskBillingModelName(task))
}

func seedSubscription(t *testing.T, id int, userId int, amountTotal int64, amountUsed int64) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:          id,
		UserId:      userId,
		AmountTotal: amountTotal,
		AmountUsed:  amountUsed,
		Status:      "active",
		StartTime:   time.Now().Unix(),
		EndTime:     time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func seedChannel(t *testing.T, id int) {
	t.Helper()
	ch := &model.Channel{Id: id, Name: "test_channel", Key: "sk-test", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(ch).Error)
}

func makeTask(userId, channelId, quota, tokenId int, billingSource string, subscriptionId int) *model.Task {
	return &model.Task{
		TaskID:    "task_" + time.Now().Format("150405.000"),
		UserId:    userId,
		ChannelId: channelId,
		Quota:     quota,
		Status:    model.TaskStatus(model.TaskStatusInProgress),
		Group:     "default",
		Data:      json.RawMessage(`{}`),
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		Properties: model.Properties{
			OriginModelName: "test-model",
		},
		PrivateData: model.TaskPrivateData{
			BillingSource:  billingSource,
			SubscriptionId: subscriptionId,
			TokenId:        tokenId,
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      0.02,
				GroupRatio:      1.0,
				OriginModelName: "test-model",
			},
		},
	}
}

func TestResolveTaskPricingGroupKeyDoesNotUseUserGroup(t *testing.T) {
	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"Renamed VIP","ratio":1.2,"selectable":true,"description":"vip"}
	]`))

	assert.Equal(t, "1", ResolveTaskPricingGroupKey(&model.Task{}))
	assert.Equal(t, "2", ResolveTaskPricingGroupKey(&model.Task{Group: "Renamed VIP"}))
	assert.Equal(t, "2", ResolveTaskPricingGroupKey(&model.Task{Group: "2"}))
	assert.InDelta(t, 1.2, ResolveTaskGroupRatio(&model.Task{Group: "2"}), 1e-9)
	assert.InDelta(t, 0.75, ResolveTaskGroupRatio(&model.Task{
		Group: "2",
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{GroupRatio: 0.75},
		},
	}), 1e-9)
}

// ---------------------------------------------------------------------------
// Read-back helpers
// ---------------------------------------------------------------------------

func getUserQuota(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&user).Error)
	return user.Quota
}

func getTokenRemainQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("remain_quota").Where("id = ?", id).First(&token).Error)
	return token.RemainQuota
}

func getTokenUsedQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("used_quota").Where("id = ?", id).First(&token).Error)
	return token.UsedQuota
}

func getSubscriptionUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", id).First(&sub).Error)
	return sub.AmountUsed
}

func getLastLog(t *testing.T) *model.Log {
	t.Helper()
	var log model.Log
	err := model.LOG_DB.Order("id desc").First(&log).Error
	if err != nil {
		return nil
	}
	return &log
}

func countLogs(t *testing.T) int64 {
	t.Helper()
	var count int64
	model.LOG_DB.Model(&model.Log{}).Count(&count)
	return count
}

func queueTerminalTaskRefund(t *testing.T, task *model.Task, reason string) string {
	t.Helper()
	fromStatus := task.Status
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FailReason = reason
	won, eventID, err := task.UpdateWithStatusAndTaskRefund(fromStatus, reason)
	require.NoError(t, err)
	require.True(t, won)
	return eventID
}

func makeBillingOutboxDue(t *testing.T, eventID string) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.BillingOutbox{}).
		Where("event_id = ?", eventID).
		Update("next_attempt_at", 0).Error)
}

// ===========================================================================
// RefundTaskQuota tests
// ===========================================================================

func TestRefundTaskQuota_Wallet(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 1, 1, 1
	const initQuota, preConsumed = 10000, 3000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-test-key", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RefundTaskQuota(ctx, task, "task failed: upstream error")

	// User quota should increase by preConsumed
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))

	// Token remain_quota should increase, used_quota should decrease
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, -preConsumed, getTokenUsedQuota(t, tokenID))

	// A refund log should be created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed, log.Quota)
	assert.Equal(t, "test-model", log.ModelName)
}

func TestRefundTaskQuota_Subscription(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 2, 2, 2, 1
	const preConsumed = 2000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-key", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)

	RefundTaskQuota(ctx, task, "subscription task failed")

	// Subscription used should decrease by preConsumed
	assert.Equal(t, subUsed-int64(preConsumed), getSubscriptionUsed(t, subID))

	// Token should also be refunded
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestRefundTaskQuota_SubscriptionWalletOverage(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 66, 66, 66, 66
	const totalQuota, walletOverage = 3000, 800
	seedUser(t, userID, 5000)
	seedToken(t, tokenID, userID, "sk-direct-split", 1000)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, 10000, 5000)
	task := makeTask(userID, channelID, totalQuota, tokenID, BillingSourceSubscription, subID)
	task.PrivateData.BillingOverageSource = BillingSourceWallet
	task.PrivateData.BillingOverageQuota = walletOverage

	RefundTaskQuota(ctx, task, "subscription task failed")

	assert.Equal(t, 5000+walletOverage, getUserQuota(t, userID))
	assert.Equal(t, int64(5000-(totalQuota-walletOverage)), getSubscriptionUsed(t, subID))
	assert.Equal(t, 1000+totalQuota, getTokenRemainQuota(t, tokenID))
}

func TestRefundTaskQuota_ZeroQuota(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 3
	seedUser(t, userID, 5000)

	task := makeTask(userID, 0, 0, 0, BillingSourceWallet, 0)

	RefundTaskQuota(ctx, task, "zero quota task")

	// No change to user quota
	assert.Equal(t, 5000, getUserQuota(t, userID))

	// No log created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRefundTaskQuota_NoToken(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 4, 4
	const initQuota, preConsumed = 10000, 1500

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0) // TokenId=0

	RefundTaskQuota(ctx, task, "no token task failed")

	// User quota refunded
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))

	// Log created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestHandleTaskTerminalFailureBillingRetainsAmbiguousCharge(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 5, 5, 5
	const retainedQuota = 3000
	seedUser(t, userID, 7000)
	seedToken(t, tokenID, userID, "sk-ambiguous-task", 2000)
	seedChannel(t, channelID)
	task := makeTask(userID, channelID, retainedQuota, tokenID, BillingSourceWallet, 0)

	originalLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() {
		common.LogConsumeEnabled = originalLogConsumeEnabled
	})

	outcome := HandleTaskTerminalFailureBilling(ctx, task, "terminal timeout", nil)

	assert.Equal(t, types.AttemptFinancialOutcomeAmbiguous, outcome)
	assert.Equal(t, 7000, getUserQuota(t, userID))
	assert.Equal(t, 2000, getTokenRemainQuota(t, tokenID))
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeConsume, log.Type)
	assert.Zero(t, log.Quota)
	var other map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(log.Other), &other))
	assert.Equal(t, float64(retainedQuota), other["retained_quota"])
	assert.Equal(t, true, other["charged_on_error"])
	assert.Equal(t, true, other["charge_retained"])
	assert.Equal(t, true, other["usage_missing"])
	assert.Equal(t, string(types.AttemptFinancialOutcomeAmbiguous), other["financial_outcome"])
}

func TestHandleTaskTerminalFailureBillingRefundsAuthoritativeNoCharge(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 6, 6, 6
	const preConsumed = 3000
	seedUser(t, userID, 7000)
	seedToken(t, tokenID, userID, "sk-free-task", 2000)
	seedChannel(t, channelID)
	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	outcome := HandleTaskTerminalFailureBilling(ctx, task, "rejected without charge", []byte(`{"error":{"message":"rejected"},"cost":0}`))

	assert.Equal(t, types.AttemptFinancialOutcomeNonBillable, outcome)
	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Equal(t, 2000+preConsumed, getTokenRemainQuota(t, tokenID))
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed, log.Quota)
}

func TestDurableTaskRefundRetriesFundingFailureWithoutDoubleRefund(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID, preConsumed = 61, 61, 61, 3000
	seedToken(t, tokenID, userID, "sk-durable-funding", 2000)
	seedChannel(t, channelID)
	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	eventID := queueTerminalTaskRefund(t, task, "free upstream failure")

	require.Error(t, model.ProcessBillingOutboxEvent(eventID))
	assert.Equal(t, 2000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, countLogs(t))

	seedUser(t, userID, 7000)
	makeBillingOutboxDue(t, eventID)
	require.NoError(t, model.ProcessBillingOutboxEvent(eventID))
	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Equal(t, 5000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(1), countLogs(t))

	makeBillingOutboxDue(t, eventID)
	require.NoError(t, model.ProcessBillingOutboxEvent(eventID))
	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Equal(t, 5000, getTokenRemainQuota(t, tokenID))
}

func TestDurableTaskRefundRetriesTokenFailureWithoutDoubleFundingRefund(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID, preConsumed = 62, 62, 62, 2500
	seedUser(t, userID, 7500)
	seedToken(t, tokenID, userID, "sk-durable-token", 1500)
	seedChannel(t, channelID)
	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	eventID := queueTerminalTaskRefund(t, task, "free upstream failure")

	callbackName := "test:fail_durable_token_refund"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(errors.New("token update unavailable"))
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })
	require.Error(t, model.ProcessBillingOutboxEvent(eventID))
	require.NoError(t, model.DB.Callback().Update().Remove(callbackName))

	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Equal(t, 1500, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, countLogs(t))

	makeBillingOutboxDue(t, eventID)
	require.NoError(t, model.ProcessBillingOutboxEvent(eventID))
	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Equal(t, 4000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(1), countLogs(t))
}

func TestDurableTaskRefundRetriesLogFailureWithoutDoubleRefund(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID, preConsumed = 63, 63, 63, 2000
	seedUser(t, userID, 8000)
	seedToken(t, tokenID, userID, "sk-durable-log", 1000)
	seedChannel(t, channelID)
	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	eventID := queueTerminalTaskRefund(t, task, "free upstream failure")

	originalLogDB := model.LOG_DB
	t.Cleanup(func() { model.LOG_DB = originalLogDB })
	missingLogDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.LOG_DB = missingLogDB
	require.Error(t, model.ProcessBillingOutboxEvent(eventID))
	model.LOG_DB = originalLogDB

	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Equal(t, 3000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, countLogs(t))

	makeBillingOutboxDue(t, eventID)
	require.NoError(t, model.ProcessBillingOutboxEvent(eventID))
	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Equal(t, 3000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(1), countLogs(t))
}

func TestDurableTaskRefundReturnsSubscriptionWalletSplitToOriginalSources(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID, subID = 64, 64, 64, 64
	const totalQuota, walletOverage = 3000, 800
	seedUser(t, userID, 5000)
	seedToken(t, tokenID, userID, "sk-split-refund", 1000)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, 10000, 5000)
	task := makeTask(userID, channelID, totalQuota, tokenID, BillingSourceSubscription, subID)
	task.PrivateData.BillingOverageSource = BillingSourceWallet
	task.PrivateData.BillingOverageQuota = walletOverage
	require.NoError(t, model.DB.Create(task).Error)
	eventID := queueTerminalTaskRefund(t, task, "free upstream failure")

	require.NoError(t, model.ProcessBillingOutboxEvent(eventID))
	assert.Equal(t, 5000+walletOverage, getUserQuota(t, userID))
	assert.Equal(t, int64(5000-(totalQuota-walletOverage)), getSubscriptionUsed(t, subID))
	assert.Equal(t, 1000+totalQuota, getTokenRemainQuota(t, tokenID))

	log := getLastLog(t)
	require.NotNil(t, log)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, BillingSourceSubscription, other["billing_source"])
	assert.Equal(t, BillingSourceWallet, other["billing_overage_source"])
	assert.Equal(t, float64(walletOverage), other["billing_overage_quota"])
}

func TestDurableTaskRefundSubscriptionConditionalUpdateRollsBackWalletPart(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID, subID = 65, 65, 65, 65
	const totalQuota, walletOverage = 3000, 800
	seedUser(t, userID, 5000)
	seedToken(t, tokenID, userID, "sk-split-atomic", 1000)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, 10000, 1000)
	task := makeTask(userID, channelID, totalQuota, tokenID, BillingSourceSubscription, subID)
	task.PrivateData.BillingOverageSource = BillingSourceWallet
	task.PrivateData.BillingOverageQuota = walletOverage
	require.NoError(t, model.DB.Create(task).Error)
	eventID := queueTerminalTaskRefund(t, task, "free upstream failure")

	require.Error(t, model.ProcessBillingOutboxEvent(eventID))
	assert.Equal(t, 5000, getUserQuota(t, userID))
	assert.Equal(t, int64(1000), getSubscriptionUsed(t, subID))
	assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))

	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("id = ?", subID).Update("amount_used", 5000).Error)
	makeBillingOutboxDue(t, eventID)
	require.NoError(t, model.ProcessBillingOutboxEvent(eventID))
	assert.Equal(t, 5000+walletOverage, getUserQuota(t, userID))
	assert.Equal(t, int64(5000-(totalQuota-walletOverage)), getSubscriptionUsed(t, subID))
	assert.Equal(t, 1000+totalQuota, getTokenRemainQuota(t, tokenID))
}

// ===========================================================================
// RecalculateTaskQuota tests
// ===========================================================================

func TestRecalculate_PositiveDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 10, 10, 10
	const initQuota, preConsumed = 10000, 2000
	const actualQuota = 3000 // under-charged by 1000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-pos", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	// User quota should decrease by the delta (1000 additional charge)
	assert.Equal(t, initQuota-(actualQuota-preConsumed), getUserQuota(t, userID))

	// Token should also be charged the delta
	assert.Equal(t, tokenRemain-(actualQuota-preConsumed), getTokenRemainQuota(t, tokenID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)

	// Log type should be Consume (additional charge)
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeConsume, log.Type)
	assert.Equal(t, actualQuota-preConsumed, log.Quota)
}

func TestRecalculate_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 11, 11, 11
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged by 2000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-neg", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	// User quota should increase by abs(delta) = 2000 (refund overpayment)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))

	// Token should be refunded the difference
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	// task.Quota updated
	assert.Equal(t, actualQuota, task.Quota)

	// Log type should be Refund
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed-actualQuota, log.Quota)
}

func TestRecalculate_ZeroDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 12
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, preConsumed, "exact match")

	// No change to user quota
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No log created (delta is zero)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_ActualQuotaZero(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 13
	const initQuota = 10000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, 5000, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, 0, "zero actual")

	// No change (early return)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_Subscription_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 14, 14, 14, 2
	const preConsumed = 5000
	const actualQuota = 2000 // over-charged by 3000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-recalc", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)

	RecalculateTaskQuota(ctx, task, actualQuota, "subscription over-charge")

	// Subscription used should decrease by delta (refund 3000)
	assert.Equal(t, subUsed-int64(preConsumed-actualQuota), getSubscriptionUsed(t, subID))

	// Token refunded
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	assert.Equal(t, actualQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

// ===========================================================================
// CAS + Billing integration tests
// Simulates the flow in updateVideoSingleTask (service/task_polling.go)
// ===========================================================================

// simulatePollBilling reproduces the CAS + billing logic from updateVideoSingleTask.
// It takes a persisted task (already in DB), applies the new status, and performs
// the conditional update + billing exactly as the polling loop does.
func simulatePollBilling(ctx context.Context, task *model.Task, newStatus model.TaskStatus, actualQuota int) {
	snap := task.Snapshot()

	shouldRefund := false
	shouldSettle := false
	quota := task.Quota

	task.Status = newStatus
	switch string(newStatus) {
	case model.TaskStatusSuccess:
		task.Progress = "100%"
		task.FinishTime = 9999
		shouldSettle = true
	case model.TaskStatusFailure:
		task.Progress = "100%"
		task.FinishTime = 9999
		task.FailReason = "upstream error"
		if quota != 0 {
			shouldRefund = true
		}
	default:
		task.Progress = "50%"
	}

	isDone := task.Status == model.TaskStatus(model.TaskStatusSuccess) || task.Status == model.TaskStatus(model.TaskStatusFailure)
	if isDone && snap.Status != task.Status {
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			shouldRefund = false
			shouldSettle = false
		} else if !won {
			shouldRefund = false
			shouldSettle = false
		}
	} else if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	if shouldSettle && actualQuota > 0 {
		RecalculateTaskQuota(ctx, task, actualQuota, "test settle")
	}
	if shouldRefund {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
}

func TestCASGuardedRefund_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 20, 20, 20
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-win", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS wins: task in DB should now be FAILURE
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, reloaded.Status)

	// Refund should have happened
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestCASGuardedRefund_Lose(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 21, 21, 21
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-lose", tokenRemain)
	seedChannel(t, channelID)

	// Create task with IN_PROGRESS in DB
	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate another process already transitioning to FAILURE
	model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Update("status", model.TaskStatusFailure)

	// Our process still has the old in-memory state (IN_PROGRESS) and tries to transition
	// task.Status is still IN_PROGRESS in the snapshot
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS lost: user quota should NOT change (no double refund)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))

	// No billing log should be created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestCASGuardedSettle_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 22, 22, 22
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged, should get partial refund
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-settle-win", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusSuccess), actualQuota)

	// CAS wins: task should be SUCCESS
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)

	// Settlement should refund the over-charge (5000 - 3000 = 2000 back to user)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)
}

func TestNonTerminalUpdate_NoBilling(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 23, 23
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	task.Progress = "20%"
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate a non-terminal poll update (still IN_PROGRESS, progress changed)
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusInProgress), 0)

	// User quota should NOT change
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No billing log
	assert.Equal(t, int64(0), countLogs(t))

	// Task progress should be updated in DB
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, "50%", reloaded.Progress)
}

// ===========================================================================
// Mock adaptor for settleTaskBillingOnComplete tests
// ===========================================================================

type mockAdaptor struct {
	adjustReturn int
}

func (m *mockAdaptor) Init(_ *relaycommon.RelayInfo) {}
func (m *mockAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return nil, nil
}
func (m *mockAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) { return nil, nil }
func (m *mockAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return m.adjustReturn
}

// ===========================================================================
// PerCallBilling tests — settleTaskBillingOnComplete
// ===========================================================================

func TestSettle_PerCallBilling_SkipsAdaptorAdjust(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 30, 30, 30
	const initQuota, preConsumed = 10000, 5000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-adaptor", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 2000}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no adjustment despite adaptor returning 2000
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_PerCallBilling_SkipsTotalTokens(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 31, 31, 31
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 7000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-tokens", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 0}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, TotalTokens: 9999}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no recalculation by tokens
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_NonPerCallBilling_AppliesAdaptorAdjustment(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 32, 32, 32
	const initQuota, preConsumed = 10000, 5000
	const adaptorQuota = 3000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-nonpercall-adj", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	// PerCallBilling defaults to false

	adaptor := &mockAdaptor{adjustReturn: adaptorQuota}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Non-per-call: adaptor adjustment applies (refund 2000)
	assert.Equal(t, initQuota+(preConsumed-adaptorQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-adaptorQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, adaptorQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}
