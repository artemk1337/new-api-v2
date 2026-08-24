package common

import (
	"sync"
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

func TestInMemoryRateLimiterRetainsAttemptsAcrossWindowIncrease(t *testing.T) {
	now := time.Now()
	limiter := InMemoryRateLimiter{store: map[string]inMemoryRateLimitEntry{
		"payment:42": {
			requests:           []time.Time{now.Add(-2 * time.Minute)},
			expirationDuration: time.Minute,
		},
	}}
	assert.False(t, limiter.RequestWithRetention("payment:42", 1, time.Hour, 24*time.Hour))
	assert.Equal(t, 24*time.Hour, limiter.store["payment:42"].expirationDuration)
}

func TestInMemoryRateLimiterCleanupUsesEntryRetention(t *testing.T) {
	now := time.Now()
	limiter := InMemoryRateLimiter{
		store:              make(map[string]inMemoryRateLimitEntry),
		expirationDuration: 20 * time.Minute,
	}
	limiter.store["payment:42"] = inMemoryRateLimitEntry{
		requests:           []time.Time{now.Add(-30 * time.Minute)},
		expirationDuration: time.Hour,
	}

	limiter.deleteExpiredItems(now)
	assert.Contains(t, limiter.store, "payment:42", "payment attempts must survive the global 20-minute cleanup interval")

	limiter.deleteExpiredItems(now.Add(time.Hour + time.Second))
	assert.NotContains(t, limiter.store, "payment:42")
}

func TestInMemoryRateLimiterConcurrentInit(t *testing.T) {
	var limiter InMemoryRateLimiter
	const workers = 32

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			limiter.Init(0)
			limiter.RequestWithRetention("payment:42", 1, time.Minute, time.Hour)
		}()
	}
	wg.Wait()

	require.NotNil(t, limiter.store)
}
