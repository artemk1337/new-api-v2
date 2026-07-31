package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordRedisRequestUsesRateLimitWindowForTTL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	window := 10 * time.Second
	key := "rateLimit:MRRLS:42"
	recordRedisRequest(t.Context(), client, key, 2, window)

	ttl := server.TTL(key)
	assert.Equal(t, window, ttl)
}

func TestRedisRateLimitUsesLegacyKeysUntilCanonicalDurationIsSaved(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousRDB := common.RDB
	common.RDB = client
	t.Cleanup(func() {
		common.RDB = previousRDB
		require.NoError(t, client.Close())
	})

	request := func(window time.Duration, windowText string, canonical bool, totalMaxCount, successMaxCount int) int {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("id", 42)
		})
		router.Use(redisRateLimitHandler(window, windowText, canonical, totalMaxCount, successMaxCount))
		router.GET("/", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		return recorder.Code
	}

	require.Equal(t, http.StatusOK, request(time.Minute, "1m", false, 1, 1))
	assert.True(t, server.Exists("rateLimit:42"))
	assert.Zero(t, server.TTL("rateLimit:42"))
	assert.True(t, server.Exists("rateLimit:MRRLS:42"))
	require.Equal(t, http.StatusTooManyRequests, request(time.Minute, "1m", false, 1, 1))

	server.Set("rateLimit:42", "legacy")
	require.Equal(t, http.StatusOK, request(10*time.Second, "10s", true, 0, 100))
	assert.True(t, server.Exists("rateLimit:42"))
	assert.Equal(t, 10*time.Second, server.TTL("rateLimit:42"))

	require.Equal(t, http.StatusOK, request(10*time.Second, "10s", true, 1, 100))
	require.Equal(t, http.StatusTooManyRequests, request(10*time.Second, "10s", true, 1, 100))
	require.Equal(t, http.StatusTooManyRequests, request(10*time.Second, "10s", true, 2, 100))
	require.Equal(t, http.StatusTooManyRequests, request(time.Hour, "1h", true, 1, 100))
	assert.Equal(t, time.Hour, server.TTL("rateLimit:MRRL:v2:42"))
	require.NoError(t, client.Del(t.Context(), "rateLimit:MRRLS:v2:42").Err())
	require.Equal(t, http.StatusOK, request(10*time.Second, "10s", true, 0, 1))
	require.Equal(t, http.StatusTooManyRequests, request(10*time.Second, "10s", true, 0, 1))
	require.Equal(t, http.StatusOK, request(10*time.Second, "10s", true, 0, 2))
	require.Equal(t, http.StatusTooManyRequests, request(time.Hour, "1h", true, 0, 2))
	assert.Equal(t, time.Hour, server.TTL("rateLimit:MRRLS:v2:42"))
	assert.True(t, server.Exists("rateLimit:MRRL:v2:42"))
	assert.True(t, server.Exists("rateLimit:MRRLS:v2:42"))
	assert.False(t, server.Exists("rateLimit:MRRL:42:1:10s"))
	assert.False(t, server.Exists("rateLimit:MRRL:42:2:10s"))
	assert.False(t, server.Exists("rateLimit:MRRL:42:1:1h"))
	assert.False(t, server.Exists("rateLimit:MRRLS:42:1:10s"))
	assert.False(t, server.Exists("rateLimit:MRRLS:42:2:10s"))
}
