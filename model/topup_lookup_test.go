package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdatePendingTopUpStatusPreservesDatabaseFailure(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	DB = db
	t.Cleanup(func() { DB = previousDB })

	err = UpdatePendingTopUpStatus("stripe-db-error", PaymentProviderStripe, "expired")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrTopUpNotFound, "database failures must not be treated as missing orders")
}
