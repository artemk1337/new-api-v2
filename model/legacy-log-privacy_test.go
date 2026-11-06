package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestHistoricalSupplierLogPrivacySurvivesRollback(t *testing.T) {
	for _, marker := range []string{
		`"supplier_pricing":true`,
		`"admin_info":{"proxy_supplier_id":7}`,
		`"billing_context":{"price_snapshot":{"SupplierTariff":{"input_usd":0.01}}}`,
		`"supplier_id":7`,
	} {
		log := &Log{Type: LogTypeError, Content: "private-source credit failure", ModelName: "private-upstream-model", ChannelId: 91, UpstreamRequestId: "private-upstream-id",
			Other: `{` + marker + `,"requested_model":"public-alias","cache_tokens":9,"billing_source":"wallet","model_price":0.006,"billing_expr":"private-expression","future_private":{"key":"private-secret"}}`}

		formatUserLogs([]*Log{log}, 0)

		require.Equal(t, "public-alias", log.ModelName)
		require.Equal(t, "Request failed", log.Content)
		require.Zero(t, log.ChannelId)
		require.Empty(t, log.UpstreamRequestId)
		require.JSONEq(t, `{"requested_model":"public-alias","cache_tokens":9,"billing_source":"wallet"}`, log.Other)
		data, err := common.Marshal(log)
		require.NoError(t, err)
		require.NotContains(t, string(data), "private-")
		require.NotContains(t, string(data), "supplier")
	}
}

func TestHistoricalSupplierLogColumnIsReadOnlyAndProtectsTokenAndUserViews(t *testing.T) {
	db := rollbackSafetyDB(t, false)
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.False(t, db.Migrator().HasColumn(&Log{}, "proxy_supplier_id"), "compatibility reads must not recreate the removed feature schema")
	require.NoError(t, db.Exec("ALTER TABLE logs ADD COLUMN proxy_supplier_id INTEGER DEFAULT 0").Error)
	previous := LOG_DB
	LOG_DB = db
	t.Cleanup(func() { LOG_DB = previous })
	stored := Log{Id: 1, UserId: 1, TokenId: 7, Type: LogTypeConsume, ChannelId: 91, Content: "private-source", ModelName: "private-model", UpstreamRequestId: "private-id", Other: "invalid private-secret"}
	require.NoError(t, db.Create(&stored).Error)
	require.NoError(t, db.Table("logs").Where("id = ?", 1).Update("proxy_supplier_id", 7).Error)

	tokenLogs, err := GetLogByTokenId(7)
	require.NoError(t, err)
	pagedTokenLogs, tokenTotal, err := GetTokenLogs(7, 0, 50)
	require.NoError(t, err)
	require.EqualValues(t, 1, tokenTotal)
	userLogs, total, err := GetUserLogs(1, LogTypeUnknown, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	for _, logs := range [][]*Log{tokenLogs, pagedTokenLogs, userLogs} {
		require.Len(t, logs, 1)
		require.Empty(t, logs[0].ModelName)
		require.Empty(t, logs[0].Content)
		require.Zero(t, logs[0].ChannelId)
		require.Empty(t, logs[0].UpstreamRequestId)
		require.JSONEq(t, `{}`, logs[0].Other)
	}
	var retained Log
	require.NoError(t, db.First(&retained).Error)
	require.Equal(t, stored.Other, retained.Other)
	require.Equal(t, stored.Content, retained.Content)
	require.Equal(t, stored.ModelName, retained.ModelName)
	require.Equal(t, 7, retained.LegacyProxySupplierID)
}

func TestLegacyUnboundLogsKeepTheirExistingPublicPricingFields(t *testing.T) {
	log := &Log{ModelName: "public-model", Content: "completed", Other: `{"admin_info":{"proxy_supplier_id":0},"model_ratio":2,"group_ratio":0.5,"completion_ratio":5}`}
	formatUserLogs([]*Log{log}, 0)
	require.Equal(t, "completed", log.Content)
	require.Equal(t, "public-model", log.ModelName)
	require.JSONEq(t, `{"model_ratio":2,"group_ratio":0.5,"completion_ratio":5}`, log.Other)
}
