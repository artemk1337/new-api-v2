package limiter

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisLimiterUsesConfiguredExpiration(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	window := time.Hour
	allowed, err := New(t.Context(), client).Allow(
		t.Context(),
		"rateLimit:MRRL:42",
		WithCapacity(10),
		WithRate(1),
		WithRequested(1),
		WithExpiration(window),
	)

	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, window, server.TTL("rateLimit:MRRL:42"))
}
