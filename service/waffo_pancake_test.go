package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestResolveWaffoPancakeSubscriptionTradeNoRetriesOnDatabaseFailure(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	_, err = ResolveWaffoPancakeSubscriptionTradeNo(&WaffoPancakeWebhookEvent{
		Data: WaffoPancakeWebhookData{OrderMerchantExternalID: "WAFFO_PANCAKE_SUB-db-error"},
	})
	require.Error(t, err)
	require.False(t, IsPermanentWaffoPancakeWebhookResolutionError(err), "database failures must be retried")
}

func TestResolveWaffoPancakeSubscriptionTradeNoAcknowledgesMissingOrder(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionOrder{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	_, err = ResolveWaffoPancakeSubscriptionTradeNo(&WaffoPancakeWebhookEvent{
		Data: WaffoPancakeWebhookData{OrderMerchantExternalID: "WAFFO_PANCAKE_SUB-missing"},
	})
	require.Error(t, err)
	require.True(t, IsPermanentWaffoPancakeWebhookResolutionError(err))
}

func TestResolveWaffoPancakeTradeNoRetriesOnDatabaseFailure(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	_, err = ResolveWaffoPancakeTradeNo(&WaffoPancakeWebhookEvent{
		Data: WaffoPancakeWebhookData{OrderMerchantExternalID: "WAFFO_PANCAKE-db-error"},
	})
	require.Error(t, err)
	require.False(t, IsPermanentWaffoPancakeWebhookResolutionError(err), "database failures must be retried")
}
