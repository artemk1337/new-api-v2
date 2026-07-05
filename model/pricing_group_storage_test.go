package model

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitTaskStoresPricingGroupID(t *testing.T) {
	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"Renamed VIP","ratio":1.2,"selectable":true}
	]`))

	task := InitTask(constant.TaskPlatformSuno, &relaycommon.RelayInfo{
		UserId:        1,
		UsingGroup:    "Renamed VIP",
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: 10},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	})

	require.NotNil(t, task)
	assert.Equal(t, "2", task.Group)
}

func TestPricingGroupLogsStoreIDsAndFilterByID(t *testing.T) {
	truncateTables(t)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalDataExportEnabled := common.DataExportEnabled
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.DataExportEnabled = originalDataExportEnabled
	})

	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"Renamed VIP","ratio":1.2,"selectable":true}
	]`))

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("username", "alice")

	RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ModelName: "gpt-log",
		Group:     "Renamed VIP",
		Quota:     10,
	})
	RecordErrorLog(ctx, 1, 0, "gpt-log", "", "error", 0, 0, false, "paid-users", nil)
	RecordTaskBillingLog(RecordTaskBillingLogParams{
		UserId:    1,
		LogType:   LogTypeConsume,
		ModelName: "gpt-log",
		Group:     "2",
		Quota:     5,
	})

	var stored []Log
	require.NoError(t, LOG_DB.Order("id asc").Find(&stored).Error)
	require.Len(t, stored, 3)
	assert.Equal(t, "2", stored[0].Group)
	assert.Equal(t, "1", stored[1].Group)
	assert.Equal(t, "2", stored[2].Group)

	logs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "2", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, logs, 2)

	logs, total, err = GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "1", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
}
