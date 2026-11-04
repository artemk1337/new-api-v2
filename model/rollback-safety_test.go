package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func rollbackSafetyDB(t *testing.T, expanded bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "rollback.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	require.NoError(t, db.AutoMigrate(&Channel{}, &Task{}, &BillingOutbox{}, &Option{}))
	if expanded {
		require.NoError(t, db.Exec("ALTER TABLE channels ADD COLUMN supplier_id INTEGER").Error)
		require.NoError(t, db.Exec("ALTER TABLE channels ADD COLUMN supplier_profile_id INTEGER").Error)
		require.NoError(t, db.Exec("CREATE TABLE pricing_group_supplier_bindings (group_id INTEGER PRIMARY KEY, supplier_id INTEGER, profile_id INTEGER)").Error)
	}
	return db
}

func TestLegacyPricingRollbackAllowsOldAndUnusedExpandedSchema(t *testing.T) {
	for _, expanded := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "unused expanded"}[expanded], func(t *testing.T) {
			db := rollbackSafetyDB(t, expanded)
			require.NoError(t, db.Create(&Channel{Id: 1, Status: common.ChannelStatusEnabled, Key: "unchanged-key", Group: "1"}).Error)
			if expanded {
				require.NoError(t, db.Table("channels").Where("id = ?", 1).Updates(map[string]any{"supplier_id": 0, "supplier_profile_id": 0}).Error)
			}
			require.NoError(t, db.Exec("INSERT INTO tasks (id, private_data) VALUES (?, ?)", 1, `{"transport_fingerprint":"legacy-transport","billing_context":{"price_snapshot":{"SupplierTariff":null},"model_price":0.01}}`).Error)
			data, err := common.Marshal(billingLogOutboxPayload{Log: Log{Other: `{"admin_info":{"proxy_supplier_id":0},"model_ratio":2}`}})
			require.NoError(t, err)
			require.NoError(t, db.Create(&BillingOutbox{EventID: "legacy-log", Kind: BillingOutboxKindLog, Payload: string(data)}).Error)

			require.NoError(t, checkLegacyPricingRollbackSafety(db))
		})
	}
}

func TestLegacyPricingRollbackBlocksNativeContractsWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		name  string
		seed  func(*testing.T, *gorm.DB)
		block string
	}{
		{"dormant group binding", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Exec("INSERT INTO pricing_group_supplier_bindings VALUES (1, 7, 8)").Error)
		}, "group bindings"},
		{"disabled supplier channel", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Create(&Channel{Id: 1, Status: 2, Key: "unchanged-key"}).Error)
			require.NoError(t, db.Table("channels").Where("id = ?", 1).Update("supplier_id", 7).Error)
		}, "channel bindings"},
		{"partial profile channel", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Create(&Channel{Id: 1, Status: 1, Key: "unchanged-key"}).Error)
			require.NoError(t, db.Table("channels").Where("id = ?", 1).Update("supplier_profile_id", 8).Error)
		}, "channel bindings"},
		{"unfinished native task", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Exec("INSERT INTO tasks (id, status, private_data) VALUES (?, ?, ?)", 1, "SUBMITTED", `{"billing_context":{"supplier_id":7,"profile_id":8}}`).Error)
		}, "native pricing contract"},
		{"completed native task", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Exec("INSERT INTO tasks (id, status, private_data) VALUES (?, ?, ?)", 1, "SUCCESS", `{"billing_context":{"price_snapshot":{"SupplierTariff":{"input_usd":0.01}}}}`).Error)
		}, "native pricing contract"},
		{"failed native task", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Exec("INSERT INTO tasks (id, status, private_data) VALUES (?, ?, ?)", 1, "FAILURE", `{"billing_context":{"profile_id":8}}`).Error)
		}, "native pricing contract"},
		{"unverifiable task snapshot", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Exec("INSERT INTO tasks (id, private_data) VALUES (?, ?)", 1, `{"key":"secret-never-print",`).Error)
		}, "unverifiable snapshot"},
		{"pending native billing recovery", func(t *testing.T, db *gorm.DB) {
			data, err := common.Marshal(billingLogOutboxPayload{Log: Log{Other: `{"admin_info":{"proxy_supplier_id":7},"private":"secret-never-print"}`}})
			require.NoError(t, err)
			require.NoError(t, db.Create(&BillingOutbox{EventID: "native", Kind: BillingOutboxKindLog, Payload: string(data)}).Error)
		}, "native supplier metadata"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := rollbackSafetyDB(t, true)
			test.seed(t, db)
			const groups = `[{"id":1,"name":"Retail","ratio":2}]`
			require.NoError(t, db.Create(&Option{Key: "PricingGroups", Value: groups}).Error)
			writes := 0
			for _, callback := range []struct {
				register func(string, func(*gorm.DB)) error
			}{
				{db.Callback().Create().Before("gorm:create").Register},
				{db.Callback().Update().Before("gorm:update").Register},
				{db.Callback().Delete().Before("gorm:delete").Register},
			} {
				require.NoError(t, callback.register("rollback-no-write", func(*gorm.DB) { writes++ }))
			}

			err := checkLegacyPricingRollbackSafety(db)

			require.ErrorContains(t, err, test.block)
			require.ErrorContains(t, err, "no data was changed")
			require.NotContains(t, err.Error(), "secret-never-print")
			require.Zero(t, writes)
			var saved Option
			require.NoError(t, db.Where("key = ?", "PricingGroups").First(&saved).Error)
			require.Equal(t, groups, saved.Value)
		})
	}
}

func TestLegacyPricingRollbackFailsWhenDatabaseCannotBeVerified(t *testing.T) {
	db := rollbackSafetyDB(t, false)
	connection, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, connection.Close())
	require.ErrorContains(t, checkLegacyPricingRollbackSafety(db), "cannot verify")
}
