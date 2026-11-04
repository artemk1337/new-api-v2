package model

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type logFilterLegacyRow struct {
	ID        int `gorm:"primaryKey"`
	UserID    int
	CreatedAt int64
	Type      int
	Username  string
	ModelName string
	Quota     int
	ChannelID int
	Group     string
	RequestID string
	Other     string
}

func (logFilterLegacyRow) TableName() string { return "logs" }

func openLogFilterDialectDB(t *testing.T, env string, expected common.DatabaseType) *gorm.DB {
	t.Helper()
	var db *gorm.DB
	var err error
	if env == "" {
		db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{})
	} else {
		if os.Getenv(env) == "" {
			t.Skip(env + " is not configured")
		}
		var actual common.DatabaseType
		db, actual, err = chooseDB(env, true)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Silent)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var tables []string
	if expected == common.DatabaseTypeClickHouse {
		err = db.Raw("SELECT name FROM system.tables WHERE database = currentDatabase()").Scan(&tables).Error
	} else {
		tables, err = db.Migrator().GetTables()
	}
	require.NoError(t, err)
	require.Empty(t, tables, "log dialect tests require an empty isolated database")
	return db
}

// This is the pre-supplier ClickHouse log contract. Keep it independent of the
// current CREATE helper so the test exercises an existing table's migration.
const logFilterLegacyClickHouseSchema = `CREATE TABLE logs (
	id Int64 DEFAULT 0, user_id Int32 DEFAULT 0, created_at Int64 DEFAULT 0,
	type Int32 DEFAULT 0, content String DEFAULT '', username String DEFAULT '',
	token_name String DEFAULT '', model_name String DEFAULT '', quota Int32 DEFAULT 0,
	prompt_tokens Int32 DEFAULT 0, completion_tokens Int32 DEFAULT 0,
	use_time Int32 DEFAULT 0, is_stream UInt8 DEFAULT 0, channel_id Int32 DEFAULT 0,
	token_id Int32 DEFAULT 0, ` + "`group`" + ` String DEFAULT '', ip String DEFAULT '',
	request_id String DEFAULT '', upstream_request_id String DEFAULT '', other String DEFAULT ''
) ENGINE = MergeTree() PARTITION BY toYYYYMM(toDateTime(created_at)) ORDER BY (created_at, request_id)`

func TestLogFilterPersistenceDialects(t *testing.T) {
	for _, dialect := range []struct {
		name string
		env  string
		kind common.DatabaseType
	}{
		{"sqlite", "", common.DatabaseTypeSQLite},
		{"postgres", "LOG_FILTER_TEST_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
		{"mysql", "LOG_FILTER_TEST_MYSQL_DSN", common.DatabaseTypeMySQL},
		{"clickhouse", "LOG_FILTER_TEST_CLICKHOUSE_DSN", common.DatabaseTypeClickHouse},
	} {
		t.Run(dialect.name, func(t *testing.T) {
			logDB := openLogFilterDialectDB(t, dialect.env, dialect.kind)
			mainDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "main.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			mainSQL, err := mainDB.DB()
			require.NoError(t, err)
			mainSQL.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, mainSQL.Close()) })
			previousDB, previousLogDB := DB, LOG_DB
			previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
			previousGroups := ratio_setting.PricingGroups2JSONString()
			previousCache := common.MemoryCacheEnabled
			DB, LOG_DB = mainDB, logDB
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, dialect.kind)
			common.MemoryCacheEnabled = false
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMain, previousLog)
				common.MemoryCacheEnabled = previousCache
				initCol()
				require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups))
			})
			t.Cleanup(func() {
				invalidateLogSupplierColumnCache(logDB)
				require.NoError(t, logDB.Migrator().DropTable("logs"))
			})
			t.Setenv("LOG_SQL_CLICKHOUSE_TTL_DAYS", "0")
			require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":7,"name":"Legacy-Coding","ratio":2,"selectable":true}]`))
			require.NoError(t, mainDB.AutoMigrate(&Supplier{}, &Channel{}, &User{}, &Token{}, &BillingOutbox{}))
			supplierA := Supplier{Code: "historical-a", Name: "Historical supplier A", Adapter: "manual", Enabled: true}
			supplierB := Supplier{Code: "current-b", Name: "Current supplier B", Adapter: "manual", Enabled: true}
			require.NoError(t, mainDB.Create(&supplierA).Error)
			require.NoError(t, mainDB.Create(&supplierB).Error)
			channel := Channel{Id: 41, Name: "Same channel after rebinding", Key: "private-test-key", Group: "7", SupplierID: &supplierB.ID}
			require.NoError(t, mainDB.Create(&channel).Error)

			if dialect.kind == common.DatabaseTypeClickHouse {
				require.NoError(t, logDB.Exec(logFilterLegacyClickHouseSchema).Error)
			} else {
				require.NoError(t, logDB.AutoMigrate(&logFilterLegacyRow{}))
			}
			now := time.Now().Unix()
			require.NoError(t, logDB.Create(&logFilterLegacyRow{UserID: 11, CreatedAt: now - 1, Type: LogTypeConsume, Username: "old-user", ModelName: "old-model", Quota: 13, ChannelID: channel.Id, Group: "7", RequestID: "legacy-unknown", Other: `{}`}).Error)
			for pass := 0; pass < 2; pass++ {
				require.NoError(t, migrateLOGDB(), "existing log migrations must be additive and repeatable")
			}
			var old Log
			require.NoError(t, logDB.Where("request_id = ?", "legacy-unknown").First(&old).Error)
			require.Zero(t, old.ProxySupplierID, "historical rows cannot inherit the channel's current supplier")
			require.Equal(t, 13, old.Quota)

			makeRow := func(requestID string, supplierID, quota int, modelName, group string) *Log {
				t.Helper()
				other := map[string]interface{}{"requested_model": modelName}
				SetLogProxySupplier(other, supplierID)
				row := &Log{UserId: 11, CreatedAt: now, Type: LogTypeConsume, Username: "name_under", TokenName: "token!literal", TokenId: 303, ModelName: modelName, Quota: quota, PromptTokens: 10, CompletionTokens: 3, ChannelId: channel.Id, Group: group, RequestId: requestID, Other: common.MapToJsonStr(other)}
				require.NoError(t, createLog(row))
				return row
			}
			makeRow("a-first", supplierA.ID, 17, "model_under", "7")
			makeRow("a-second", supplierA.ID, 23, "model_under", "Legacy-Coding")
			makeRow("b-current", supplierB.ID, 31, "modelXunder", "7")
			makeRow("new-unbound", 0, 37, "model_under", "7")
			require.NoError(t, mainDB.Delete(&Supplier{}, supplierA.ID).Error)
			require.False(t, logDB.Migrator().HasTable(&Supplier{}), "supplier names must be resolved using the separate main DB")

			filter := LogFilter{ProxySupplierID: &supplierA.ID}
			rows, count, err := GetAllLogsWithFilter(filter, 0, 1)
			require.NoError(t, err)
			require.EqualValues(t, 2, count)
			require.Len(t, rows, 1)
			require.Equal(t, channel.Name, rows[0].ChannelName)
			if dialect.kind == common.DatabaseTypeClickHouse {
				require.Equal(t, 1, rows[0].Id)
			}
			firstRequestID := rows[0].RequestId
			rows, count, err = GetAllLogsWithFilter(filter, 1, 1)
			require.NoError(t, err)
			require.EqualValues(t, 2, count)
			require.Len(t, rows, 1)
			require.NotEqual(t, firstRequestID, rows[0].RequestId, "pagination must be deterministic across identical timestamps")
			if dialect.kind == common.DatabaseTypeClickHouse {
				require.Equal(t, 2, rows[0].Id, "ClickHouse IDs remain display IDs after excluding the physical ID column")
			}
			stat, err := SumUsedQuotaWithFilter(filter)
			require.NoError(t, err)
			require.Equal(t, Stat{Quota: 40, Rpm: 2, Tpm: 26}, stat)

			zero := 0
			rows, count, err = GetAllLogsWithFilter(LogFilter{ProxySupplierID: &zero}, 0, 10)
			require.NoError(t, err)
			require.EqualValues(t, 2, count, "supplier ID zero is an explicit unknown/unbound filter")
			require.Len(t, rows, 2)
			for _, row := range rows {
				require.Zero(t, row.ProxySupplierID)
			}
			stat, err = SumUsedQuotaWithFilter(LogFilter{ProxySupplierID: &zero})
			require.NoError(t, err)
			require.Equal(t, Stat{Quota: 50, Rpm: 2, Tpm: 13}, stat)

			for _, text := range []string{"model_under", "model_under%"} {
				rows, count, err = GetAllLogsWithFilter(LogFilter{ModelName: text, Username: "name_under%", TokenName: "token!literal"}, 0, 10)
				require.NoError(t, err)
				require.EqualValues(t, 3, count, "underscore remains literal for exact and explicit wildcard filters")
				require.Len(t, rows, 3)
			}
			rows, count, err = GetAllLogsWithFilter(LogFilter{ModelName: "' OR 1=1 --"}, 0, 10)
			require.NoError(t, err)
			require.Zero(t, count, "search text must be a bound SQL parameter")
			require.Empty(t, rows)
			aliasFilter := LogFilter{ProxySupplierID: &supplierA.ID, Group: "Legacy-Coding"}
			rows, count, err = GetAllLogsWithFilter(aliasFilter, 0, 10)
			require.NoError(t, err)
			require.EqualValues(t, 2, count, "canonical group IDs and historical names must both match")
			require.Len(t, rows, 2)
			stat, err = SumUsedQuotaWithFilter(aliasFilter)
			require.NoError(t, err)
			require.Equal(t, Stat{Quota: 40, Rpm: 2, Tpm: 26}, stat)
			aliasFilter.RequestID = "a-second"
			rows, count, err = GetAllLogsWithFilter(aliasFilter, 0, 10)
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			require.Len(t, rows, 1)

			page, err := GetLogSuggestions(context.Background(), LogFilter{}, "model_name", "under", 0, 1)
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			require.NotNil(t, page.NextCursor)
			nextOffset, err := strconv.Atoi(*page.NextCursor)
			require.NoError(t, err)
			next, err := GetLogSuggestions(context.Background(), LogFilter{}, "model_name", "under", nextOffset, 1)
			require.NoError(t, err)
			require.Len(t, next.Items, 1)
			require.Nil(t, next.NextCursor)
			require.NotEqual(t, page.Items[0].Value, next.Items[0].Value)
			for _, query := range []string{"_", "%", "' OR 1=1 --"} {
				page, err = GetLogSuggestions(context.Background(), filter, "model_name", query, 0, 10)
				require.NoError(t, err)
				if query == "_" {
					require.Equal(t, []LogSuggestion{{Value: "model_under", Label: "model_under"}}, page.Items)
				} else {
					require.Empty(t, page.Items, "suggestion input is literal text, including percent and SQL syntax")
				}
			}
			page, err = GetLogSuggestions(context.Background(), filter, "group", "coding", 0, 10)
			require.NoError(t, err)
			require.ElementsMatch(t, []LogSuggestion{{Value: "7", Label: "Legacy-Coding"}, {Value: "Legacy-Coding", Label: "Legacy-Coding"}}, page.Items)
			_, err = GetLogSuggestions(context.Background(), filter, "username) OR 1=1 --", "", 0, 10)
			require.Error(t, err, "suggestion column names must be restricted to a fixed allowlist")

			supplierLookups := 0
			require.NoError(t, mainDB.Callback().Query().After("gorm:query").Register("log_filter_supplier_lookup", func(tx *gorm.DB) {
				if tx.Statement.Table == "suppliers" {
					supplierLookups++
				}
			}))
			choices, err := GetLogSupplierChoices(context.Background(), LogFilter{}, "", 0, 1)
			require.NoError(t, err)
			require.Equal(t, 1, supplierLookups, "supplier labels must be resolved in a main-DB batch")
			require.Equal(t, []LogSupplierChoice{{ID: supplierA.ID, Name: "#" + strconv.Itoa(supplierA.ID)}}, choices.Items)
			require.NotNil(t, choices.NextCursor)
			nextOffset, err = strconv.Atoi(*choices.NextCursor)
			require.NoError(t, err)
			choices, err = GetLogSupplierChoices(context.Background(), LogFilter{}, "", nextOffset, 1)
			require.NoError(t, err)
			require.Equal(t, []LogSupplierChoice{{ID: supplierB.ID, Name: supplierB.Name}}, choices.Items)
			require.Nil(t, choices.NextCursor)
			choices, err = GetLogSupplierChoices(context.Background(), LogFilter{ModelName: "modelXunder"}, "Current", 0, 10)
			require.NoError(t, err)
			require.Equal(t, []LogSupplierChoice{{ID: supplierB.ID, Name: supplierB.Name}}, choices.Items, "choice candidates must respect the other active log filters")

			rows, count, err = GetAllLogs(LogTypeConsume, 0, 0, "", "", "", 0, 10, 0, "", "", "")
			require.NoError(t, err)
			require.EqualValues(t, 5, count)
			require.Len(t, rows, 5)
			stat, err = SumUsedQuota(LogTypeConsume, 0, 0, "", "", "", 0, "")
			require.NoError(t, err)
			require.Equal(t, Stat{Quota: 121, Rpm: 5, Tpm: 52}, stat)
			rows, count, err = GetUserLogs(11, LogTypeConsume, 0, 0, "", "", 0, 10, "", "", "")
			require.NoError(t, err)
			require.EqualValues(t, 5, count)
			require.Len(t, rows, 5)
			userJSON, err := common.Marshal(rows)
			require.NoError(t, err)
			require.NotContains(t, string(userJSON), "proxy_supplier_id")
			require.NotContains(t, string(userJSON), "admin_info")
			rows, err = GetLogByTokenId(303)
			require.NoError(t, err)
			require.Len(t, rows, 4)

			metadataQueries := 0
			countMetadata := func(tx *gorm.DB) {
				query := strings.ToLower(tx.Statement.SQL.String())
				if strings.Contains(query, "sqlite_master") || strings.Contains(query, "information_schema.columns") || strings.Contains(query, "system.columns") {
					metadataQueries++
				}
			}
			require.NoError(t, logDB.Callback().Query().After("gorm:query").Register("log_filter_metadata_query", countMetadata))
			require.NoError(t, logDB.Callback().Row().After("gorm:row").Register("log_filter_metadata_row", countMetadata))
			require.NoError(t, logDB.Callback().Raw().After("gorm:raw").Register("log_filter_metadata_raw", countMetadata))
			require.NoError(t, createLog(&Log{CreatedAt: now, Type: LogTypeSystem, RequestId: "performance-first"}))
			firstMetadataQueries := metadataQueries
			metadataQueries = 0
			for index := 0; index < 100; index++ {
				require.NoError(t, createLog(&Log{CreatedAt: now, Type: LogTypeSystem, RequestId: "performance-stable-" + strconv.Itoa(index)}))
			}
			t.Logf("schema metadata queries for 1/100 stable writes: %d/%d", firstMetadataQueries, metadataQueries)
			require.LessOrEqual(t, metadataQueries, 1, "a stable log schema cannot require a metadata round trip per write")

			// Freeze the outbox payload, then remove the live channel entirely.
			// Delivery and replacement must use only the captured attribution.
			requestContext, _ := gin.CreateTestContext(httptest.NewRecorder())
			requestContext.Set("username", "name_under")
			requestContext.Set("proxy_supplier_id", supplierB.ID)
			other := map[string]interface{}{}
			SetLogProxySupplier(other, supplierA.ID)
			params := RecordConsumeLogParams{ChannelId: channel.Id, ModelName: "model_under", Group: "7", Quota: 43, Other: other}
			const eventID = "dialect-frozen-attribution"
			require.NoError(t, UpsertConsumeLogOutboxIntent(requestContext, 11, eventID, params))
			require.NoError(t, mainDB.Delete(&Channel{}, channel.Id).Error)
			require.NoError(t, DeliverBillingOutboxEvent(eventID))
			var delivered Log
			require.NoError(t, logDB.Where("request_id = ?", eventID).First(&delivered).Error)
			require.Equal(t, supplierA.ID, delivered.ProxySupplierID)
			require.Equal(t, 43, delivered.Quota)
			originalCreatedAt := delivered.CreatedAt
			params.Quota = 47
			SetLogProxySupplier(other, 0)
			require.NoError(t, UpsertConsumeLogOutboxIntent(requestContext, 11, eventID, params))
			var replacement BillingOutbox
			require.NoError(t, mainDB.Where("event_id = ?", eventID).First(&replacement).Error)
			var finalPayload billingLogOutboxPayload
			require.NoError(t, common.UnmarshalJsonStr(replacement.Payload, &finalPayload))
			finalPayload.Log.CreatedAt = originalCreatedAt + 5
			encoded, err := common.Marshal(finalPayload)
			require.NoError(t, err)
			require.NoError(t, mainDB.Model(&replacement).Update("payload", string(encoded)).Error)
			require.NoError(t, DeliverBillingOutboxEvent(eventID))
			require.Eventually(t, func() bool {
				var stored Log
				if logDB.Where("request_id = ?", eventID).First(&stored).Error != nil {
					return false
				}
				delivered = stored
				return stored.ProxySupplierID == 0 && stored.Quota == 47
			}, 3*time.Second, 10*time.Millisecond, "ClickHouse mutations must finish without losing the final payload")
			require.Zero(t, delivered.ProxySupplierID, "final payloads must replace a previous positive supplier ID with explicit zero")
			require.Equal(t, 47, delivered.Quota)
			if dialect.kind == common.DatabaseTypeClickHouse {
				require.Equal(t, originalCreatedAt, delivered.CreatedAt, "ClickHouse sorting and partition keys remain immutable during outbox replacement")
			} else {
				require.Equal(t, originalCreatedAt+5, delivered.CreatedAt)
			}
			require.NoError(t, logDB.Model(&Log{}).Where("request_id = ?", eventID).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}
