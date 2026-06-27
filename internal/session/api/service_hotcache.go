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

package api

import (
	"context"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// This file holds the tiered-store accessors and hot-cache write-through
// helpers for SessionService. They are split out of service.go so each file
// keeps to a single responsibility (see issue #1325).

// getFromHot attempts to retrieve a session from the hot cache.
func (s *SessionService) getFromHot(ctx context.Context, sessionID string) (*session.Session, error) {
	hot, err := s.registry.HotCache()
	if err != nil {
		return nil, err
	}
	return hot.GetSession(ctx, sessionID)
}

// getFromWarm attempts to retrieve a session from the warm store.
func (s *SessionService) getFromWarm(ctx context.Context, sessionID string) (*session.Session, error) {
	warm, err := s.registry.WarmStore()
	if err != nil {
		return nil, err
	}
	return warm.GetSession(ctx, sessionID)
}

// getFromCold attempts to retrieve a session from the cold archive.
func (s *SessionService) getFromCold(ctx context.Context, sessionID string) (*session.Session, error) {
	cold, err := s.registry.ColdArchive()
	if err != nil {
		return nil, err
	}
	return cold.GetSession(ctx, sessionID)
}

// populateHotCache stores a session in the hot cache on a best-effort basis.
func (s *SessionService) populateHotCache(ctx context.Context, sess *session.Session) {
	hot, err := s.registry.HotCache()
	if err != nil {
		s.requestLog(ctx).V(1).Info("hot cache unavailable, skipping populate", "error", err.Error())
		return
	}
	if err := hot.SetSession(ctx, sess, s.cacheTTL); err != nil {
		s.requestLog(ctx).Error(err, "failed to populate hot cache", "sessionID", sess.ID)
		return
	}
	s.requestLog(ctx).V(2).Info("hot cache populated", "sessionID", sess.ID)
}

// pushToHotCache runs a hot-cache write operation in a bounded goroutine.
// If no hot cache is configured or the concurrency limit is reached, the call is dropped.
func (s *SessionService) pushToHotCache(fn func(ctx context.Context, hot providers.HotCacheProvider)) {
	hot, err := s.registry.HotCache()
	if err != nil {
		return // Hot cache not configured — no-op.
	}
	select {
	case s.hotCacheSem <- struct{}{}:
		go func() {
			defer func() { <-s.hotCacheSem }()
			ctx, cancel := context.WithTimeout(context.Background(), hotCacheTimeout)
			defer cancel()
			fn(ctx, hot)
		}()
	default:
		s.log.V(1).Info("hot cache push dropped", "reason", "backpressure")
	}
}

// refreshHotCacheTTL extends the hot cache TTL for an active session.
func (s *SessionService) refreshHotCacheTTL(sessionID string) {
	s.pushToHotCache(func(ctx context.Context, hot providers.HotCacheProvider) {
		if err := hot.RefreshTTL(ctx, sessionID, s.cacheTTL); err != nil {
			s.log.V(2).Info("hot cache TTL refresh skipped", "sessionID", sessionID, "reason", err.Error())
		}
	})
}

// refreshHotCacheSession re-reads the session metadata from the warm store and
// writes it through to the hot cache so the cached blob's aggregate counters
// (message_count, token totals, cost, tool_call_count) don't go stale. Those
// counters are mutated only in the warm store (by AppendMessage /
// RecordProviderCall / RecordToolCall), so without this the hot cache keeps
// serving the zero-valued blob seeded at creation until its TTL expires.
// SetSession writes only the session key, leaving the cached message list
// intact. Best-effort and fire-and-forget.
func (s *SessionService) refreshHotCacheSession(sessionID string) {
	warm, err := s.registry.WarmStore()
	if err != nil {
		return
	}
	s.pushToHotCache(func(ctx context.Context, hot providers.HotCacheProvider) {
		sess, err := warm.GetSession(ctx, sessionID)
		if err != nil {
			s.log.V(2).Info("hot cache session refresh skipped", "sessionID", sessionID, "reason", err.Error())
			return
		}
		if err := hot.SetSession(ctx, sess, s.cacheTTL); err != nil {
			s.log.V(2).Info("hot cache session refresh failed", "sessionID", sessionID, "reason", err.Error())
		}
	})
}
