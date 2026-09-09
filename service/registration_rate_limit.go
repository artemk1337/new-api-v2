package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const registrationRateLimitScript = `
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local retention = tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - retention)
local count = redis.call('ZCOUNT', KEYS[1], now - window, '+inf')
redis.call('EXPIRE', KEYS[1], math.floor(retention / 1000) + 1)
if count >= tonumber(ARGV[4]) then return '' end
local sequence = redis.call('INCR', KEYS[2])
local member = tostring(now) .. ':' .. tostring(sequence)
redis.call('ZADD', KEYS[1], now, member)
redis.call('EXPIRE', KEYS[1], math.floor(retention / 1000) + 1)
redis.call('EXPIRE', KEYS[2], math.floor(retention / 1000) + 1)
return member
`

var registrationMemoryRateLimiter common.InMemoryRateLimiter

func AllowRegistrationAttempt(clientIP string) bool {
	config := operation_setting.RegistrationRateLimit()
	if !config.Enabled {
		return true
	}
	_, allowed := reserveRegistrationLimit("attempt", clientIP, config.Attempts, config.Window)
	return allowed
}

// ReserveRegistrationSuccess reserves one successful-account slot. The caller
// must invoke release if the database transaction does not create the user.
func ReserveRegistrationSuccess(clientIP string) (release func(), allowed bool) {
	config := operation_setting.RegistrationRateLimit()
	if !config.Enabled {
		return func() {}, true
	}
	return reserveRegistrationLimit("success", clientIP, config.Successes, config.Window)
}

func reserveRegistrationLimit(kind, clientIP string, maximum int, window time.Duration) (func(), bool) {
	key := "rateLimit:REG:" + kind + ":ip:" + clientIP
	retention := operation_setting.RegistrationRateLimitRetention
	if common.RedisEnabled {
		if common.RDB == nil {
			common.SysLog("registration rate limit unavailable: Redis client is not configured")
			return func() {}, false
		}
		now := time.Now().UnixMilli()
		sequenceKey := key + ":seq"
		result, err := common.RDB.Eval(
			context.Background(),
			registrationRateLimitScript,
			[]string{key, sequenceKey},
			now,
			window.Milliseconds(),
			retention.Milliseconds(),
			maximum,
		).Text()
		if err == nil {
			if result == "" {
				return func() {}, false
			}
			rdb := common.RDB
			return func() {
				if err := rdb.ZRem(context.Background(), key, result).Err(); err != nil {
					common.SysLog("failed to release registration rate limit reservation: " + err.Error())
				}
			}, true
		}
		common.SysLog("registration rate limit Redis failure: " + err.Error())
		return func() {}, false
	}

	registrationMemoryRateLimiter.Init(operation_setting.RegistrationRateLimitRetention)
	memoryKey := kind + ":ip:" + clientIP
	reservationID, allowed := registrationMemoryRateLimiter.ReserveWithRetention(memoryKey, maximum, window, retention)
	if !allowed {
		return func() {}, false
	}
	return func() { registrationMemoryRateLimiter.ReleaseReservation(memoryKey, reservationID) }, true
}
