package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacyDirectCryptoPaymentMigrationRow struct {
	Id      uint   `gorm:"primaryKey"`
	TradeNo string `gorm:"uniqueIndex"`
}

func (legacyDirectCryptoPaymentMigrationRow) TableName() string { return "direct_crypto_payments" }

type legacyTopUpMigrationRow struct {
	Id      uint   `gorm:"primaryKey"`
	TradeNo string `gorm:"uniqueIndex"`
}

func (legacyTopUpMigrationRow) TableName() string { return "top_ups" }

func TestMigrateDirectCryptoBooleanColumnsBackfillsExistingRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "direct_crypto_migration.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&legacyDirectCryptoPaymentMigrationRow{}, &legacyTopUpMigrationRow{}))
	require.NoError(t, db.Create(&legacyDirectCryptoPaymentMigrationRow{TradeNo: "legacy-direct"}).Error)
	require.NoError(t, db.Create(&legacyTopUpMigrationRow{TradeNo: "legacy-topup"}).Error)

	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	require.NoError(t, migrateDirectCryptoBooleanColumns())
	assert.True(t, db.Migrator().HasColumn(&DirectCryptoPayment{}, "round_to_cents"))
	assert.True(t, db.Migrator().HasColumn(&DirectCryptoPayment{}, "round_policy_captured"))
	assert.True(t, db.Migrator().HasColumn(&TopUp{}, "payment_round_to_cents"))

	var directValues struct {
		RoundToCents        bool
		RoundPolicyCaptured bool
	}
	require.NoError(t, db.Table("direct_crypto_payments").Select("round_to_cents, round_policy_captured").Where("trade_no = ?", "legacy-direct").Scan(&directValues).Error)
	assert.False(t, directValues.RoundToCents)
	assert.False(t, directValues.RoundPolicyCaptured)
	var topUpValue bool
	require.NoError(t, db.Table("top_ups").Select("payment_round_to_cents").Where("trade_no = ?", "legacy-topup").Scan(&topUpValue).Error)
	assert.False(t, topUpValue)
	// A rollback to the previous binary omits the new columns on INSERT. The
	// additive nullable schema must continue accepting those writes.
	require.NoError(t, db.Create(&legacyDirectCryptoPaymentMigrationRow{TradeNo: "rollback-direct"}).Error)
	require.NoError(t, db.Create(&legacyTopUpMigrationRow{TradeNo: "rollback-topup"}).Error)
}
