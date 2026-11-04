package model

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLogSupplierTestDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	mainDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "main.db")), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = mainDB, logDB
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		initCol()
		for _, db := range []*gorm.DB{mainDB, logDB} {
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())
		}
	})
	require.NoError(t, mainDB.AutoMigrate(&User{}, &Token{}, &Channel{}, &Task{}, &BillingOutbox{}))
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	require.NoError(t, mainDB.Create(&User{Id: 1, Username: "log-user", Quota: 100}).Error)
	require.NoError(t, mainDB.Create(&Token{Id: 1, UserId: 1, Key: "private-test-key", Status: common.TokenStatusEnabled}).Error)
	supplierID := 22
	require.NoError(t, mainDB.Create(&Channel{Id: 1, Name: "rebound", SupplierID: &supplierID}).Error)
	return mainDB, logDB
}

func TestLogSupplierSnapshotSurvivesEveryOutboxWriter(t *testing.T) {
	for _, writer := range []string{"staged", "failed-log", "violation-fee"} {
		t.Run(writer, func(t *testing.T) {
			mainDB, logDB := setupLogSupplierTestDB(t)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("proxy_supplier_id", 22)
			other := map[string]interface{}{}
			SetLogProxySupplier(other, 11)
			params := RecordConsumeLogParams{ChannelId: 1, ModelName: "public-model", TokenId: 1, Quota: 7, Other: other, Force: true}
			switch writer {
			case "staged":
				require.NoError(t, StageConsumeLogOutboxIntent(ctx, 1, "staged-event", params))
			case "failed-log":
				missingDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "missing.db")), &gorm.Config{})
				require.NoError(t, err)
				LOG_DB = missingDB
				require.Error(t, RecordConsumeLog(ctx, 1, params))
				LOG_DB = logDB
				sqlDB, err := missingDB.DB()
				require.NoError(t, err)
				require.NoError(t, sqlDB.Close())
			case "violation-fee":
				require.NoError(t, CommitViolationFeeWithOutbox(ctx, "violation-event", ViolationFeeCommitParams{UserID: 1, Quota: 7, SkipTokenQuota: true}, params))
			}
			var event BillingOutbox
			require.NoError(t, mainDB.First(&event).Error)
			var payload billingLogOutboxPayload
			require.NoError(t, common.UnmarshalJsonStr(event.Payload, &payload))
			require.Zero(t, payload.Log.ProxySupplierID, "the persisted private sidecar restores the JSON-hidden column")
			require.Contains(t, payload.Log.Other, `"proxy_supplier_id":11`)
			require.NoError(t, mainDB.Delete(&Channel{}, 1).Error)
			require.NoError(t, processBillingLogOutbox(&event))
			var stored Log
			require.NoError(t, logDB.Where("request_id = ?", event.EventID).First(&stored).Error)
			require.Equal(t, 11, stored.ProxySupplierID)
			require.Equal(t, 7, stored.Quota)
			logs, err := GetLogByTokenId(1)
			require.NoError(t, err)
			public, err := common.Marshal(logs)
			require.NoError(t, err)
			require.NotContains(t, string(public), "supplier")
			require.NotContains(t, string(public), "admin_info")
		})
	}
}

func TestLogSupplierUnknownAndExplicitZeroNeverInferCurrentChannel(t *testing.T) {
	mainDB, logDB := setupLogSupplierTestDB(t)
	stored := &Log{ChannelId: 1, RequestId: "same-charge", ProxySupplierID: 11, Other: `{}`}
	require.NoError(t, createLog(stored))
	for _, other := range []map[string]interface{}{{}, {"admin_info": map[string]interface{}{"proxy_supplier_id": 0}}} {
		params := RecordConsumeLogParams{ChannelId: 1, Other: other, Force: true}
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Set("proxy_supplier_id", 22)
		log := &Log{ChannelId: 1, RequestId: "same-charge", Other: common.MapToJsonStr(other)}
		if len(other) > 0 {
			log = buildConsumeLog(ctx, 1, params)
			log.RequestId = "same-charge"
		}
		payload, err := common.Marshal(billingLogOutboxPayload{Log: *log})
		require.NoError(t, err)
		event := BillingOutbox{EventID: "same-charge", Kind: BillingOutboxKindLog, Payload: string(payload)}
		require.NoError(t, mainDB.Create(&event).Error)
		require.NoError(t, processBillingLogOutbox(&event))
		var updated Log
		require.NoError(t, logDB.Where("request_id = ?", "same-charge").First(&updated).Error)
		require.Zero(t, updated.ProxySupplierID)
	}
}

func TestLogSupplierPreMigrationWritesKeepPrivateSnapshot(t *testing.T) {
	_, logDB := setupLogSupplierTestDB(t)
	require.NoError(t, logDB.Migrator().DropIndex(&Log{}, "idx_logs_proxy_supplier_id"))
	require.NoError(t, logDB.Exec("ALTER TABLE logs DROP COLUMN proxy_supplier_id").Error)
	other := map[string]interface{}{}
	SetLogProxySupplier(other, 11)
	row := &Log{RequestId: "before-migration", Other: common.MapToJsonStr(other)}
	require.NoError(t, createLog(row))
	supplierID, unknownID := 11, 0
	filter := LogFilter{ProxySupplierID: &supplierID}
	rows, total, err := GetAllLogsWithFilter(filter, 0, 10)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows)
	filter.ProxySupplierID = &unknownID
	rows, total, err = GetAllLogsWithFilter(filter, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	require.Contains(t, rows[0].Other, `"proxy_supplier_id":11`)
	require.NoError(t, migrateLOGDB())
	var migrated Log
	require.NoError(t, logDB.Where("request_id = ?", row.RequestId).First(&migrated).Error)
	require.Zero(t, migrated.ProxySupplierID, "schema migration does not guess historical attribution")
}

func TestLogSupplierLegacyPrivacyClassificationDoesNotAttributeHistory(t *testing.T) {
	mainDB, logDB := setupLogSupplierTestDB(t)
	for _, writer := range []string{"direct", "old-outbox"} {
		row := &Log{UserId: 1, TokenId: 1, ChannelId: 1, Type: LogTypeError, ModelName: "public-model", RequestId: writer, Content: "private upstream failure", Other: `{"model_price":0.006}`}
		if writer == "direct" {
			require.NoError(t, createLog(row))
		} else {
			payload, err := common.Marshal(billingLogOutboxPayload{Log: *row})
			require.NoError(t, err)
			event := BillingOutbox{EventID: writer, Kind: BillingOutboxKindLog, Payload: string(payload)}
			require.NoError(t, mainDB.Create(&event).Error)
			require.NoError(t, processBillingLogOutbox(&event))
		}
		var stored Log
		require.NoError(t, logDB.Where("request_id = ?", writer).First(&stored).Error)
		require.Zero(t, stored.ProxySupplierID)
		require.Contains(t, stored.Other, `"supplier_pricing":true`)
	}
	logs, err := GetLogByTokenId(1)
	require.NoError(t, err)
	public, err := common.Marshal(logs)
	require.NoError(t, err)
	require.NotContains(t, string(public), "model_price")
	require.NotContains(t, string(public), "private upstream failure")
	other := map[string]interface{}{}
	SetLogProxySupplier(other, 0)
	explicit := &Log{ChannelId: 1, RequestId: "known-unbound", Other: common.MapToJsonStr(other)}
	require.NoError(t, createLog(explicit))
	require.Zero(t, explicit.ProxySupplierID)
	require.NotContains(t, explicit.Other, "supplier_pricing", "a later binding cannot reclassify an explicitly unbound request")
}

func TestLogSuggestionQueriesRespectCanceledContext(t *testing.T) {
	setupLogSupplierTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetLogSuggestions(ctx, LogFilter{}, "model_name", "", 0, 50)
	require.ErrorIs(t, err, context.Canceled)
	_, err = GetLogSupplierChoices(ctx, LogFilter{}, "", 0, 50)
	require.ErrorIs(t, err, context.Canceled)
}
