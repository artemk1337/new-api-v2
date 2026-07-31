package common

import (
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	store              map[string]inMemoryRateLimitEntry
	mutex              sync.Mutex
	expirationDuration time.Duration
}

type inMemoryRateLimitEntry struct {
	requests           []time.Time
	expirationDuration time.Duration
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	if l.store == nil {
		l.mutex.Lock()
		if l.store == nil {
			l.store = make(map[string]inMemoryRateLimitEntry)
			l.expirationDuration = expirationDuration
			if expirationDuration > 0 {
				go l.clearExpiredItems()
			}
		}
		l.mutex.Unlock()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.deleteExpiredItems(time.Now())
	}
}

func (l *InMemoryRateLimiter) deleteExpiredItems(now time.Time) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for key, entry := range l.store {
		if len(entry.requests) == 0 || now.Sub(entry.requests[len(entry.requests)-1]) > entry.expirationDuration {
			delete(l.store, key)
		}
	}
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	return l.request(key, maxRequestNum, time.Duration(duration)*time.Second)
}

func (l *InMemoryRateLimiter) request(key string, maxRequestNum int, duration time.Duration) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	now := time.Now()
	entry := l.store[key]
	firstActive := 0
	for firstActive < len(entry.requests) && now.Sub(entry.requests[firstActive]) >= duration {
		firstActive++
	}
	entry.requests = entry.requests[firstActive:]
	entry.expirationDuration = duration
	if len(entry.requests) >= maxRequestNum {
		l.store[key] = entry
		return false
	}
	entry.requests = append(entry.requests, now)
	l.store[key] = entry
	return true
}

// RequestWithDuration applies a rate limit window represented as time.Duration.
func (l *InMemoryRateLimiter) RequestWithDuration(key string, maxRequestNum int, duration time.Duration) bool {
	return l.request(key, maxRequestNum, duration)
}
