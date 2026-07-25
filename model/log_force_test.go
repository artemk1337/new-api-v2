package model

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecordConsumeLogForcePersistsChargedErrorWhenConsumeLogsDisabled(t *testing.T) {
	truncateTables(t)
	original := common.LogConsumeEnabled
	t.Cleanup(func() {
		common.LogConsumeEnabled = original
	})
	common.LogConsumeEnabled = false

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.NoError(t, RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ModelName: "gpt-test",
		Quota:     25,
		Content:   "ordinary",
	}))
	require.NoError(t, RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ModelName: "gpt-test",
		Quota:     25,
		Content:   "charged on error",
		Force:     true,
	}))

	var logs []Log
	require.NoError(t, LOG_DB.Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, LogTypeConsume, logs[0].Type)
	assert.Equal(t, 25, logs[0].Quota)
	assert.Equal(t, "charged on error", logs[0].Content)
}

func TestRecordConsumeLogQueuesSuccessfulChargeWhenLogDBFails(t *testing.T) {
	truncateTables(t)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set(common.RequestIdKey, "req-success-log-retry")
	originalLogDB := LOG_DB
	t.Cleanup(func() { LOG_DB = originalLogDB })
	missingLogDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB = missingLogDB

	err = RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ModelName: "gpt-test",
		Quota:     75,
		Content:   "successful settlement",
	})
	require.Error(t, err)

	var event BillingOutbox
	require.NoError(t, DB.Where("kind = ?", BillingOutboxKindLog).First(&event).Error)
	LOG_DB = originalLogDB
	require.NoError(t, DB.Model(&BillingOutbox{}).Where("id = ?", event.ID).Update("next_attempt_at", 0).Error)
	result := ProcessBillingOutbox(context.Background(), 10)
	assert.Equal(t, 1, result.Processed)

	var stored Log
	require.NoError(t, LOG_DB.Where("request_id = ?", event.EventID).First(&stored).Error)
	assert.Equal(t, 75, stored.Quota)
	assert.Equal(t, "successful settlement", stored.Content)
}
