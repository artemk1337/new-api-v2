package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLegacyPaymentSnapshotBackfillDatabaseFailureIsRetryable(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	err = ValidateAndBackfillLegacyPaymentSnapshot(&model.TopUp{
		Id: 1, Money: 12.35,
	}, model.PaymentProviderNOWPayments, "USDT", 12.35)
	require.Error(t, err)
	require.False(t, IsPermanentPaymentSnapshotError(err), "database backfill failures must be retried")
}

func TestPaymentSnapshotMismatchIsPermanent(t *testing.T) {
	err := ValidatePaymentSnapshot(&model.TopUp{PaymentCurrency: "USD", PaymentChargedAmount: 10}, "USD", 9)
	require.Error(t, err)
	require.True(t, IsPermanentPaymentSnapshotError(err))

	err = ValidateAndBackfillLegacyPaymentSnapshot(&model.TopUp{Money: 10}, model.PaymentProviderNOWPayments, "USDT", 9)
	require.Error(t, err)
	require.True(t, IsPermanentPaymentSnapshotError(err))
}
