package common

import (
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	store              map[string]inMemoryRateLimitEntry
	mutex              sync.Mutex
	expirationDuration time.Duration
	nextReservationID  uint64
}

type inMemoryRateLimitEntry struct {
	requests           []inMemoryRateLimitRequest
	expirationDuration time.Duration
}

type inMemoryRateLimitRequest struct {
	at            time.Time
	reservationID uint64
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.store != nil {
		return
	}
	l.store = make(map[string]inMemoryRateLimitEntry)
	l.expirationDuration = expirationDuration
	if expirationDuration > 0 {
		go l.clearExpiredItems()
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
		expirationDuration := entry.expirationDuration
		if expirationDuration <= 0 {
			expirationDuration = l.expirationDuration
		}
		if len(entry.requests) == 0 || now.Sub(entry.requests[len(entry.requests)-1].at) > expirationDuration {
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
	for firstActive < len(entry.requests) && now.Sub(entry.requests[firstActive].at) >= duration {
		firstActive++
	}
	entry.requests = entry.requests[firstActive:]
	entry.expirationDuration = duration
	if len(entry.requests) >= maxRequestNum {
		l.store[key] = entry
		return false
	}
	entry.requests = append(entry.requests, inMemoryRateLimitRequest{at: now})
	l.store[key] = entry
	return true
}

// RequestWithDuration applies a rate limit window represented as time.Duration.
func (l *InMemoryRateLimiter) RequestWithDuration(key string, maxRequestNum int, duration time.Duration) bool {
	return l.request(key, maxRequestNum, duration)
}

// RequestWithRetention keeps attempts for retention while applying duration as
// the active window. It is for limits whose window can be changed at runtime:
// a later larger window still sees attempts made under the earlier setting.
func (l *InMemoryRateLimiter) RequestWithRetention(key string, maxRequestNum int, duration, retention time.Duration) bool {
	_, allowed := l.ReserveWithRetention(key, maxRequestNum, duration, retention)
	return allowed
}

// ReserveWithRetention returns an identifier that can release this exact
// reservation if the protected operation fails.
func (l *InMemoryRateLimiter) ReserveWithRetention(key string, maxRequestNum int, duration, retention time.Duration) (uint64, bool) {
	if maxRequestNum <= 0 || duration <= 0 || retention < duration {
		return 0, false
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	now := time.Now()
	entry := l.store[key]
	firstRetained := 0
	for firstRetained < len(entry.requests) && now.Sub(entry.requests[firstRetained].at) >= retention {
		firstRetained++
	}
	entry.requests = entry.requests[firstRetained:]
	entry.expirationDuration = retention
	active := 0
	for i := len(entry.requests) - 1; i >= 0 && now.Sub(entry.requests[i].at) < duration; i-- {
		active++
	}
	if active >= maxRequestNum {
		l.store[key] = entry
		return 0, false
	}
	l.nextReservationID++
	if l.nextReservationID == 0 {
		l.nextReservationID++
	}
	reservationID := l.nextReservationID
	entry.requests = append(entry.requests, inMemoryRateLimitRequest{at: now, reservationID: reservationID})
	l.store[key] = entry
	return reservationID, true
}

// ReleaseReservation removes one previously accepted reservation.
func (l *InMemoryRateLimiter) ReleaseReservation(key string, reservationID uint64) {
	if reservationID == 0 {
		return
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	entry, ok := l.store[key]
	if !ok || len(entry.requests) == 0 {
		return
	}
	reservationIndex := -1
	for i := len(entry.requests) - 1; i >= 0; i-- {
		if entry.requests[i].reservationID == reservationID {
			reservationIndex = i
			break
		}
	}
	if reservationIndex == -1 {
		return
	}
	entry.requests = append(entry.requests[:reservationIndex], entry.requests[reservationIndex+1:]...)
	if len(entry.requests) == 0 {
		delete(l.store, key)
		return
	}
	l.store[key] = entry
}
