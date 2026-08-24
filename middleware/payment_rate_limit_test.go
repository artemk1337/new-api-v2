package middleware

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentCreationRateLimitIsPerUserAndDynamic(t *testing.T) {
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	originalRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedis })
	original := map[string]string{}
	for _, key := range []string{operation_setting.PaymentCreationRateLimit, operation_setting.PaymentCreationRateLimitDurationMinutes} {
		original[key] = common.OptionMap[key]
	}
	t.Cleanup(func() {
		for key, value := range original {
			common.OptionMap[key] = value
		}
	})
	common.OptionMap[operation_setting.PaymentCreationRateLimit] = "5"
	common.OptionMap[operation_setting.PaymentCreationRateLimitDurationMinutes] = "1"

	limiter := PaymentCreationRateLimit()
	for attempt := 0; attempt < 5; attempt++ {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Set("id", 9200)
		limiter(ctx)
		assert.False(t, ctx.IsAborted())
	}
	blocked, _ := gin.CreateTestContext(httptest.NewRecorder())
	blocked.Set("id", 9200)
	limiter(blocked)
	assert.True(t, blocked.IsAborted())

	common.OptionMap[operation_setting.PaymentCreationRateLimit] = "1"
	otherUser, _ := gin.CreateTestContext(httptest.NewRecorder())
	otherUser.Set("id", 9201)
	limiter(otherUser)
	require.False(t, otherUser.IsAborted())
}

func TestPaymentCreationRateLimitRetainsAttemptsWhenWindowIncreases(t *testing.T) {
	originalRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedis })

	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	originalCount := common.OptionMap[operation_setting.PaymentCreationRateLimit]
	originalWindow := common.OptionMap[operation_setting.PaymentCreationRateLimitDurationMinutes]
	t.Cleanup(func() {
		common.OptionMap[operation_setting.PaymentCreationRateLimit] = originalCount
		common.OptionMap[operation_setting.PaymentCreationRateLimitDurationMinutes] = originalWindow
	})
	common.OptionMap[operation_setting.PaymentCreationRateLimit] = "1"
	common.OptionMap[operation_setting.PaymentCreationRateLimitDurationMinutes] = "1"

	limiter := PaymentCreationRateLimit()
	first, _ := gin.CreateTestContext(httptest.NewRecorder())
	first.Set("id", 9203)
	limiter(first)
	require.False(t, first.IsAborted())

	common.OptionMap[operation_setting.PaymentCreationRateLimitDurationMinutes] = "60"
	second, _ := gin.CreateTestContext(httptest.NewRecorder())
	second.Set("id", 9203)
	limiter(second)
	assert.True(t, second.IsAborted(), "an attempt from the old one-minute window must survive a larger active window")
}

func TestPaymentCreationLimitClampsLegacyOversizedWindow(t *testing.T) {
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	originalCount := common.OptionMap[operation_setting.PaymentCreationRateLimit]
	original := common.OptionMap[operation_setting.PaymentCreationRateLimitDurationMinutes]
	t.Cleanup(func() {
		common.OptionMap[operation_setting.PaymentCreationRateLimit] = originalCount
		common.OptionMap[operation_setting.PaymentCreationRateLimitDurationMinutes] = original
	})
	common.OptionMap[operation_setting.PaymentCreationRateLimit] = "525600"
	common.OptionMap[operation_setting.PaymentCreationRateLimitDurationMinutes] = "525600"

	count, window := operation_setting.PaymentCreationLimit()
	assert.Equal(t, operation_setting.MaxPaymentCreationRateLimit, count)
	assert.Equal(t, time.Hour, window)
}

func TestPaymentRedisRateLimitRetainsAttemptsAcrossWindowIncrease(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previous := common.RDB
	common.RDB = client
	t.Cleanup(func() {
		common.RDB = previous
		require.NoError(t, client.Close())
	})

	key := "rateLimit:PAY:user:9202"
	require.NoError(t, client.ZAdd(context.Background(), key, &redis.Z{Score: float64(time.Now().Add(-2 * time.Minute).UnixMilli()), Member: "older"}).Err())
	require.NoError(t, client.Expire(context.Background(), key, time.Minute).Err())
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	userRedisSlidingRateLimiter(ctx, 1, int64(time.Hour/time.Second), int64(operation_setting.PaymentCreationRateLimitRetention/time.Second), key)
	assert.True(t, ctx.IsAborted())
	assert.Equal(t, operation_setting.PaymentCreationRateLimitRetention+time.Second, server.TTL(key))
}

func TestPaymentRedisRateLimitSetsTTLOnFirstAttempt(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previous := common.RDB
	common.RDB = client
	t.Cleanup(func() {
		common.RDB = previous
		require.NoError(t, client.Close())
	})

	key := "rateLimit:PAY:user:first-attempt"
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	userRedisSlidingRateLimiter(ctx, 1, int64(time.Minute/time.Second), int64(operation_setting.PaymentCreationRateLimitRetention/time.Second), key)
	require.False(t, ctx.IsAborted())
	assert.Equal(t, operation_setting.PaymentCreationRateLimitRetention+time.Second, server.TTL(key))
}
