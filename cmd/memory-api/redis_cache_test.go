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

// TestBuildRedisClient_EmptyReturnsNil proves an unset --redis-addrs
// flag produces no client. This is the gate the rest of the cache
// wiring depends on — a non-nil client with no real backend would
// turn every Retrieve/List into a connection-refused round trip.
func TestBuildRedisClient_EmptyReturnsNil(t *testing.T) {
	if buildRedisClient("") != nil {
		t.Error("expected nil client for empty addrs")
	}
	if buildRedisClient("   ") != nil {
		t.Error("expected nil client for whitespace-only addrs")
	}
}

// TestBuildRedisClient_TakesFirstAddr proves the comma-separated input
// shape is tolerated even though the production go-redis client is
// single-master. Operators don't have to remember which one of
// "Sentinel-shaped or single-shaped" the binary expects.
func TestBuildRedisClient_TakesFirstAddr(t *testing.T) {
	c := buildRedisClient("redis-a:6379,redis-b:6379")
	if c == nil {
		t.Fatal("expected a client for non-empty addrs")
	}
	if got := c.Options().Addr; got != "redis-a:6379" {
		t.Errorf("expected addr=redis-a:6379, got %q", got)
	}
	_ = c.Close()
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
