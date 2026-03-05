/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package ratelimit provides a per-key token bucket rate limiter backed by
// golang.org/x/time/rate. Stale entries are periodically cleaned up to
// prevent unbounded memory growth.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// staleThreshold is the duration after which an unused limiter entry is
// eligible for cleanup.
const staleThreshold = 5 * time.Minute

// cleanupInterval is how often the background goroutine scans for stale entries.
const cleanupInterval = 1 * time.Minute

// entry holds a limiter and the last time it was accessed.
type entry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// KeyedLimiter maintains per-key token bucket rate limiters. Each unique key
// (e.g. client IP or connection ID) gets its own independent limiter. A
// background goroutine removes entries that have not been accessed within
// staleThreshold to prevent memory leaks.
type KeyedLimiter struct {
	mu       sync.Mutex
	limiters map[string]*entry
	r        rate.Limit
	burst    int
	stop     chan struct{}
	done     chan struct{}
	now      func() time.Time // for testing
}

// NewKeyedLimiter creates a KeyedLimiter with the given rate (events per
// second) and burst size. It starts a background cleanup goroutine that
// must be stopped by calling Stop().
func NewKeyedLimiter(r rate.Limit, burst int) *KeyedLimiter {
	k := &KeyedLimiter{
		limiters: make(map[string]*entry),
		r:        r,
		burst:    burst,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		now:      time.Now,
	}
	go k.cleanupLoop()
	return k
}

// Allow reports whether an event for the given key may happen now.
// It consumes one token from the key's bucket.
func (k *KeyedLimiter) Allow(key string) bool {
	k.mu.Lock()
	e, ok := k.limiters[key]
	if !ok {
		e = &entry{limiter: rate.NewLimiter(k.r, k.burst)}
		k.limiters[key] = e
	}
	e.lastAccess = k.now()
	k.mu.Unlock()

	return e.limiter.Allow()
}

// Len returns the number of tracked keys. Useful for testing.
func (k *KeyedLimiter) Len() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.limiters)
}

// Stop cancels the background cleanup goroutine and waits for it to exit.
func (k *KeyedLimiter) Stop() {
	close(k.stop)
	<-k.done
}

// cleanupLoop periodically removes limiter entries that have not been
// accessed within staleThreshold.
func (k *KeyedLimiter) cleanupLoop() {
	defer close(k.done)
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-k.stop:
			return
		case <-ticker.C:
			k.evictStale()
		}
	}
}

// evictStale removes entries older than staleThreshold.
func (k *KeyedLimiter) evictStale() {
	k.mu.Lock()
	defer k.mu.Unlock()
	cutoff := k.now().Add(-staleThreshold)
	for key, e := range k.limiters {
		if e.lastAccess.Before(cutoff) {
			delete(k.limiters, key)
		}
	}
}
