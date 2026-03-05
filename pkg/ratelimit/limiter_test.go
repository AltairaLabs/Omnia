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

package ratelimit

import (
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestAllow_BasicPermit(t *testing.T) {
	kl := NewKeyedLimiter(10, 10)
	defer kl.Stop()

	if !kl.Allow("key1") {
		t.Fatal("expected first request to be allowed")
	}
}

func TestAllow_ExceedsBurst(t *testing.T) {
	// rate=1/s, burst=3 → first 3 allowed, 4th denied
	kl := NewKeyedLimiter(1, 3)
	defer kl.Stop()

	for i := 0; i < 3; i++ {
		if !kl.Allow("key1") {
			t.Fatalf("request %d should be allowed within burst", i)
		}
	}

	if kl.Allow("key1") {
		t.Fatal("request beyond burst should be denied")
	}
}

func TestAllow_PerKeyIsolation(t *testing.T) {
	kl := NewKeyedLimiter(1, 1)
	defer kl.Stop()

	// Exhaust key1
	if !kl.Allow("key1") {
		t.Fatal("key1 first request should be allowed")
	}
	if kl.Allow("key1") {
		t.Fatal("key1 second request should be denied")
	}

	// key2 should still be allowed
	if !kl.Allow("key2") {
		t.Fatal("key2 first request should be allowed (independent bucket)")
	}
}

func TestCleanup_RemovesStaleEntries(t *testing.T) {
	kl := NewKeyedLimiter(10, 10)
	defer kl.Stop()

	// Override time function to simulate passage of time
	fakeNow := time.Now()
	kl.mu.Lock()
	kl.now = func() time.Time { return fakeNow }
	kl.mu.Unlock()

	kl.Allow("stale-key")
	kl.Allow("fresh-key")

	if kl.Len() != 2 {
		t.Fatalf("expected 2 keys, got %d", kl.Len())
	}

	// Advance time past stale threshold for stale-key
	kl.mu.Lock()
	fakeNow = fakeNow.Add(staleThreshold + time.Second)
	kl.mu.Unlock()

	// Touch fresh-key so it stays alive
	kl.Allow("fresh-key")

	// Run cleanup
	kl.evictStale()

	if kl.Len() != 1 {
		t.Fatalf("expected 1 key after cleanup, got %d", kl.Len())
	}

	// fresh-key should still work
	if !kl.Allow("fresh-key") {
		t.Fatal("fresh-key should still be usable after cleanup")
	}
}

func TestConcurrentAccess(t *testing.T) {
	kl := NewKeyedLimiter(rate.Limit(1000), 1000)
	defer kl.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				kl.Allow(key)
			}
		}(string(rune('A' + i%26)))
	}
	wg.Wait()

	// No panics or races — success
	if kl.Len() == 0 {
		t.Fatal("expected at least one tracked key")
	}
}

func TestStop_TerminatesCleanup(t *testing.T) {
	kl := NewKeyedLimiter(10, 10)
	kl.Stop()

	// Calling Stop should return promptly. If the goroutine didn't exit,
	// the test would hang and time out.
}

func TestAllow_ReplenishesTokens(t *testing.T) {
	// rate=1000/s, burst=1 — tokens should replenish quickly
	kl := NewKeyedLimiter(1000, 1)
	defer kl.Stop()

	if !kl.Allow("key") {
		t.Fatal("first request should be allowed")
	}

	// Sleep briefly to allow token replenishment
	time.Sleep(5 * time.Millisecond)

	if !kl.Allow("key") {
		t.Fatal("request after replenishment should be allowed")
	}
}

func TestLen_TracksKeys(t *testing.T) {
	kl := NewKeyedLimiter(10, 10)
	defer kl.Stop()

	if kl.Len() != 0 {
		t.Fatalf("expected 0 keys initially, got %d", kl.Len())
	}

	kl.Allow("a")
	kl.Allow("b")
	kl.Allow("c")

	if kl.Len() != 3 {
		t.Fatalf("expected 3 keys, got %d", kl.Len())
	}

	// Same key doesn't create duplicate
	kl.Allow("a")
	if kl.Len() != 3 {
		t.Fatalf("expected 3 keys after duplicate, got %d", kl.Len())
	}
}
