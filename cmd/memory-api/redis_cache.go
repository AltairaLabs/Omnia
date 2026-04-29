/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	goredis "github.com/redis/go-redis/v9"

	"github.com/altairalabs/omnia/internal/memory"
)

// buildRedisClient parses a Redis connection URL and returns a client.
// Empty URL → (nil, nil) so callers can branch on "Redis configured"
// without tripping on parse errors. Invalid URL → (nil, error) — bad
// config is a startup error, not a silently-disabled cache.
//
// Accepts redis:// and rediss:// URLs. TLS, AUTH, DB index, ACL user
// all encoded in the URL per RFC 7595. goredis.ParseURL handles the
// edge cases (URL-encoded passwords, IPv6 hosts, etc.).
func buildRedisClient(url string) (*goredis.Client, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, nil
	}
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}
	return goredis.NewClient(opts), nil
}

// resolveAPIStore returns the Store the HTTP API should use.
//
// Workers (compaction, retention, tombstone, re-embed) take the raw
// *PostgresMemoryStore directly — they need concrete-type methods like
// ListWorkspaceIDs and they always want live data. The HTTP API
// benefits from a Redis read-through cache on Retrieve/List, so when a
// Redis client and a positive TTL are both available we wrap inner in
// a CachedStore.
//
// The bool return is purely informational (so the caller can log
// "cache enabled" once at startup); the wrap decision itself is fully
// reflected in the returned Store.
//
// Failure modes:
//   - rdb == nil → no cache, return inner
//   - cacheTTLRaw empty / "0" / unparseable / non-positive → no cache,
//     return inner. Bad input is treated like "off"; we log later in
//     run() that the cache is enabled, so a missing log line is a
//     readable signal that the operator's configured TTL didn't parse.
func resolveAPIStore(inner memory.Store, rdb *goredis.Client, cacheTTLRaw string, log logr.Logger) (memory.Store, bool) {
	if rdb == nil {
		return inner, false
	}
	ttl, ok := parseCacheTTL(cacheTTLRaw)
	if !ok {
		return inner, false
	}
	return memory.NewCachedStore(inner, rdb, ttl, log), true
}

// parseCacheTTL accepts the raw flag/env value and returns (duration,
// enabled). "" and "0" are explicitly off; anything that fails to
// parse is also off (callers don't get a half-configured cache from a
// typo). Negative durations are rejected.
func parseCacheTTL(raw string) (time.Duration, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || s == "0" {
		return 0, false
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}
