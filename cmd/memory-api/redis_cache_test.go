/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	goredis "github.com/redis/go-redis/v9"

	"github.com/altairalabs/omnia/internal/memory"
)

// TestBuildRedisClient_EmptyReturnsNil proves an unset --redis-url
// flag produces no client without surfacing an error. The cache is
// opt-in: empty URL means "no Redis", not "broken config".
func TestBuildRedisClient_EmptyReturnsNil(t *testing.T) {
	for _, url := range []string{"", "   "} {
		c, err := buildRedisClient(url)
		if err != nil {
			t.Errorf("buildRedisClient(%q): unexpected error %v", url, err)
		}
		if c != nil {
			t.Errorf("buildRedisClient(%q): expected nil client, got %v", url, c)
		}
	}
}

// TestBuildRedisClient_ParsesURL proves redis:// URLs parse correctly
// and the resulting client carries the parsed addr/db. Validates the
// canonical form documented in chart values.
func TestBuildRedisClient_ParsesURL(t *testing.T) {
	c, err := buildRedisClient("redis://omnia-redis-master.omnia-system.svc.cluster.local:6379/3")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if c == nil {
		t.Fatal("expected a client for a valid URL")
	}
	if got := c.Options().Addr; got != "omnia-redis-master.omnia-system.svc.cluster.local:6379" {
		t.Errorf("addr: got %q", got)
	}
	if got := c.Options().DB; got != 3 {
		t.Errorf("db: got %d, want 3", got)
	}
	_ = c.Close()
}

// TestBuildRedisClient_RejectsInvalidURL proves bad input is a startup
// error, not silently dropped. A typo in the chart values shouldn't
// degrade into "cache disabled" — operators need a clear signal.
func TestBuildRedisClient_RejectsInvalidURL(t *testing.T) {
	_, err := buildRedisClient("not-a-url")
	if err == nil {
		t.Fatal("expected an error for an invalid URL")
	}
}

// TestParseCacheTTL covers every meaningful path of the helper:
// the explicit-off values ("" and "0"), parse failures, non-positive
// durations, and the happy path. Each case has its own table row so a
// regression points at exactly which input class broke.
func TestParseCacheTTL(t *testing.T) {
	cases := []struct {
		raw     string
		wantDur time.Duration
		wantOK  bool
	}{
		{"", 0, false},
		{"   ", 0, false},
		{"0", 0, false},
		{"-5m", 0, false},
		{"not-a-duration", 0, false},
		{"5m", 5 * time.Minute, true},
		{"30s", 30 * time.Second, true},
		{"  10m  ", 10 * time.Minute, true},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			d, ok := parseCacheTTL(tc.raw)
			if ok != tc.wantOK {
				t.Errorf("parseCacheTTL(%q): ok=%v, want %v", tc.raw, ok, tc.wantOK)
			}
			if d != tc.wantDur {
				t.Errorf("parseCacheTTL(%q): dur=%v, want %v", tc.raw, d, tc.wantDur)
			}
		})
	}
}

// TestResolveAPIStore_NoRedisReturnsInner proves that without a Redis
// client the API store is the unwrapped inner Store — the cache must
// be opt-in. A wrapper here would mean every Retrieve/List goes
// through cache code paths that would NPE on the missing client.
func TestResolveAPIStore_NoRedisReturnsInner(t *testing.T) {
	inner := fakeMemoryStore{}
	got, enabled := resolveAPIStore(inner, nil, "5m", logr.Discard())
	if enabled {
		t.Error("expected cache disabled when redis client is nil")
	}
	if _, ok := got.(*memory.CachedStore); ok {
		t.Error("expected unwrapped inner Store, got *memory.CachedStore")
	}
}

// TestResolveAPIStore_NoTTLReturnsInner proves the TTL gate: a
// configured Redis client with no/zero/invalid TTL leaves the store
// uncached. Operators sometimes provision Redis ahead of enabling the
// cache; this gate keeps that intermediate state correct.
func TestResolveAPIStore_NoTTLReturnsInner(t *testing.T) {
	rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:0"})
	t.Cleanup(func() { _ = rdb.Close() })

	for _, ttl := range []string{"", "0", "garbage"} {
		t.Run("ttl="+ttl, func(t *testing.T) {
			got, enabled := resolveAPIStore(fakeMemoryStore{}, rdb, ttl, logr.Discard())
			if enabled {
				t.Errorf("expected cache disabled for ttl=%q", ttl)
			}
			if _, ok := got.(*memory.CachedStore); ok {
				t.Errorf("expected unwrapped Store for ttl=%q", ttl)
			}
		})
	}
}

// TestResolveAPIStore_WrapsWhenConfigured proves the happy path: a
// client + valid TTL produces a *memory.CachedStore. This is the
// wiring assertion that #692 (Phase 3) was missing — the cache
// implementation existed and was tested, but no cmd/* binary actually
// constructed one.
func TestResolveAPIStore_WrapsWhenConfigured(t *testing.T) {
	rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:0"})
	t.Cleanup(func() { _ = rdb.Close() })

	got, enabled := resolveAPIStore(fakeMemoryStore{}, rdb, "5m", logr.Discard())
	if !enabled {
		t.Fatal("expected cache enabled with rdb + valid TTL")
	}
	if _, ok := got.(*memory.CachedStore); !ok {
		t.Errorf("expected *memory.CachedStore, got %T", got)
	}
}
