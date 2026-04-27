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

package memory

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Cache + Redis-dependency metrics.
//
// The recall cache is fail-open: every Redis error path falls
// through to the inner Postgres store. That makes it correct under
// Redis outage, but invisible — without these counters operators
// can't tell whether the cache is doing useful work, or whether
// Redis is even up.
//
// The cache_lookups counter splits hit/miss/error so we can spot
// the H-P12 thrash failure mode (hits ≈ 0 while misses climb in
// step with version_bumps). The redis_dependency_errors counter
// surfaces Redis trouble even before logs do — alert on rate > 0
// sustained.
var (
	cacheLookupsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_memory_cache_lookups_total",
		Help: "Cache lookups by operation (retrieve|list) and outcome (hit|miss|error).",
	}, []string{"op", "result"})

	cacheVersionBumpsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "omnia_memory_cache_version_bumps_total",
		Help: "Scope-version bumps issued by writes; high rate vs cache_lookups{result=hit} indicates the cache-thrash failure mode under write-heavy workloads.",
	})

	redisDependencyErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_memory_redis_dependency_errors_total",
		Help: "Errors from Redis dependency operations, by op (get_version|get_cache|set_cache|bump_version).",
	}, []string{"op"})

	redisUp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "omnia_memory_redis_up",
		Help: "1 when the most recent CachedStore Redis op succeeded; 0 otherwise. Lagging signal — pair with rate(omnia_memory_redis_dependency_errors_total) for alerting.",
	})
)

// recordCacheLookup increments the cache_lookups counter for a
// (op, result) pair. Result is one of hit / miss / error.
func recordCacheLookup(op, result string) {
	cacheLookupsTotal.WithLabelValues(op, result).Inc()
}

// recordCacheVersionBump bumps the version-bumps counter. Called
// by every write path that invalidates the cache.
func recordCacheVersionBump() {
	cacheVersionBumpsTotal.Inc()
}

// recordRedisError counts a Redis dependency error and flips the
// up gauge to 0. The next successful op flips it back to 1.
func recordRedisError(op string) {
	redisDependencyErrorsTotal.WithLabelValues(op).Inc()
	redisUp.Set(0)
}

// recordRedisOK flips the up gauge to 1 after a successful op. The
// gauge is the most recent-result signal; the counters give rate.
func recordRedisOK() {
	redisUp.Set(1)
}
