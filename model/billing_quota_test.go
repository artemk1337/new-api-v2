package model

import (
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReserveUserQuotaForBillingIsAtomic(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7001, Username: "billing-atomic", Quota: 100}).Error)

	var succeeded atomic.Int32
	var insufficient atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := ReserveUserQuotaForBilling(7001, 10)
			switch {
			case err == nil:
				succeeded.Add(1)
			case errors.Is(err, ErrBillingUserQuotaInsufficient):
				insufficient.Add(1)
			default:
				unexpected.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(10), succeeded.Load())
	assert.Equal(t, int32(10), insufficient.Load())
	assert.Zero(t, unexpected.Load())
	var user User
	require.NoError(t, DB.First(&user, 7001).Error)
	assert.Zero(t, user.Quota)
}

func TestBillingDebitAllowsNegativeAndRefunds(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7002, Username: "billing-negative", Quota: 5}).Error)

	require.NoError(t, DebitUserQuotaForBilling(7002, 9))
	var user User
	require.NoError(t, DB.First(&user, 7002).Error)
	assert.Equal(t, -4, user.Quota)

	require.NoError(t, RefundUserQuotaForBilling(7002, 3))
	require.NoError(t, DB.First(&user, 7002).Error)
	assert.Equal(t, -1, user.Quota)
}

func TestBillingTokenWritesWorkWithRedisDisabled(t *testing.T) {
	truncateTables(t)
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
	})
	require.NoError(t, DB.Create(&Token{
		Id:          7003,
		UserId:      7002,
		Key:         "billing-token-no-redis",
		Name:        "billing-token",
		RemainQuota: 5,
	}).Error)

	require.ErrorIs(t,
		ReserveTokenQuotaForBilling(7003, "billing-token-no-redis", 6, false),
		ErrBillingTokenQuotaInsufficient,
	)
	require.NoError(t, DebitTokenQuotaForBilling(7003, "billing-token-no-redis", 9))
	require.NoError(t, RefundTokenQuotaForBilling(7003, "billing-token-no-redis", 3))

	var token Token
	require.NoError(t, DB.First(&token, 7003).Error)
	assert.Equal(t, -1, token.RemainQuota)
	assert.Equal(t, 6, token.UsedQuota)
}

func TestBillingReserveReturnsDatabaseFailure(t *testing.T) {
	previousDB := DB
	failedDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "closed.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, failedDB.AutoMigrate(&User{}))
	sqlDB, err := failedDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	DB = failedDB
	t.Cleanup(func() {
		DB = previousDB
	})

	require.Error(t, ReserveUserQuotaForBilling(1, 1))
}
