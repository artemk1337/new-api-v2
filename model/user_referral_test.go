package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInsertWithTxPersistsInviterID(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{
		Id:       1200,
		Username: "oauth-referral-inviter",
		AffCode:  "oauth-referral-inviter-code",
		Status:   common.UserStatusEnabled,
	}).Error)

	user := &User{Username: "oauth-referral-invitee", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return user.InsertWithTx(tx, 1200)
	}))

	var stored User
	require.NoError(t, DB.Select("inviter_id").First(&stored, user.Id).Error)
	assert.Equal(t, 1200, stored.InviterId)
}

func TestTransferAffQuotaToQuotaInvalidatesCacheAfterCommit(t *testing.T) {
	truncateTables(t)
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	previousClient := common.RDB
	previousEnabled := common.RedisEnabled
	common.RDB = redisClient
	common.RedisEnabled = true
	t.Cleanup(func() {
		common.RDB = previousClient
		common.RedisEnabled = previousEnabled
		require.NoError(t, redisClient.Close())
	})

	user := &User{
		Id:       1201,
		Username: "affiliate-transfer-user",
		AffCode:  "affiliate-transfer-code",
		Status:   common.UserStatusEnabled,
		AffQuota: int(common.QuotaPerUnit),
	}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, updateUserCache(*user))

	cacheKey := getUserCacheKey(user.Id)
	exists, err := redisClient.Exists(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), exists)

	require.NoError(t, user.TransferAffQuotaToQuota(int(common.QuotaPerUnit)))
	var stored User
	require.NoError(t, DB.Select("quota").First(&stored, user.Id).Error)
	assert.Equal(t, int(common.QuotaPerUnit), stored.Quota)
	exists, err = redisClient.Exists(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)

	rollbackUser := &User{
		Id:       1202,
		Username: "affiliate-transfer-rollback-user",
		AffCode:  "affiliate-transfer-rollback-code",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(rollbackUser).Error)
	require.NoError(t, updateUserCache(*rollbackUser))
	require.Error(t, rollbackUser.TransferAffQuotaToQuota(int(common.QuotaPerUnit)))
	exists, err = redisClient.Exists(context.Background(), getUserCacheKey(rollbackUser.Id)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)
}
