package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryRateLimiterKeepsEntriesForTheirOwnWindow(t *testing.T) {
	limiter := InMemoryRateLimiter{store: make(map[string]inMemoryRateLimitEntry)}
	require.True(t, limiter.RequestWithDuration("model:42", 1, time.Hour))

	limiter.deleteExpiredItems(time.Now().Add(2 * time.Minute))
	assert.Contains(t, limiter.store, "model:42")

	limiter.deleteExpiredItems(time.Now().Add(time.Hour + time.Second))
	assert.NotContains(t, limiter.store, "model:42")
}

func TestInMemoryRateLimiterUpdatesWindowForDeniedRequest(t *testing.T) {
	limiter := InMemoryRateLimiter{store: make(map[string]inMemoryRateLimitEntry)}
	require.True(t, limiter.RequestWithDuration("model:42", 1, 10*time.Second))
	require.False(t, limiter.RequestWithDuration("model:42", 1, time.Hour))

	limiter.deleteExpiredItems(time.Now().Add(11 * time.Second))
	assert.Contains(t, limiter.store, "model:42")
}
