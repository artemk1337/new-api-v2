package service

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureRegistrationRateLimitTest(t *testing.T, attempts, successes string) {
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{
		operation_setting.RegistrationRateLimitEnabled:       "true",
		operation_setting.RegistrationRateLimitAttempts:      attempts,
		operation_setting.RegistrationRateLimitSuccesses:     successes,
		operation_setting.RegistrationRateLimitWindowMinutes: "60",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})
}

func TestRegistrationRateLimitSeparatesAttemptsAndSuccessfulAccountsInMemory(t *testing.T) {
	configureRegistrationRateLimitTest(t, "2", "1")
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })

	assert.True(t, AllowRegistrationAttempt("192.0.2.10"))
	assert.True(t, AllowRegistrationAttempt("192.0.2.10"))
	assert.False(t, AllowRegistrationAttempt("192.0.2.10"))
	assert.True(t, AllowRegistrationAttempt("192.0.2.11"), "attempt limits must be isolated by IP")

	release, allowed := ReserveRegistrationSuccess("192.0.2.20")
	require.True(t, allowed)
	_, allowed = ReserveRegistrationSuccess("192.0.2.20")
	assert.False(t, allowed)

	release()
	_, allowed = ReserveRegistrationSuccess("192.0.2.20")
	require.True(t, allowed, "a failed database transaction must release its reserved slot")
	_, allowed = ReserveRegistrationSuccess("192.0.2.20")
	assert.False(t, allowed, "a committed registration must keep its slot")
}

func TestRegistrationRedisReservationIsAtomicUnderBurst(t *testing.T) {
	configureRegistrationRateLimitTest(t, "100", "3")
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
		require.NoError(t, client.Close())
	})

	var allowed atomic.Int64
	var waitGroup sync.WaitGroup
	for range 20 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, ok := ReserveRegistrationSuccess("198.51.100.30")
			if ok {
				allowed.Add(1)
			}
		}()
	}
	waitGroup.Wait()

	assert.Equal(t, int64(3), allowed.Load())
}

func TestRegistrationRateLimitFailsClosedWhenConfiguredRedisIsUnavailable(t *testing.T) {
	configureRegistrationRateLimitTest(t, "10", "3")
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	require.NoError(t, client.Close())
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
	})

	assert.False(t, AllowRegistrationAttempt("203.0.113.100"))
	_, allowed := ReserveRegistrationSuccess("203.0.113.100")
	assert.False(t, allowed)
}
