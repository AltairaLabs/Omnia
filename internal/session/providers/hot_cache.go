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

package providers

import (
	"context"
	"time"

	"github.com/altairalabs/omnia/internal/session"
)

// HotCacheProvider defines the interface for ephemeral, low-latency session
// storage (e.g. Redis). It acts as a read-through cache for active sessions.
type HotCacheProvider interface {
	// GetSession retrieves a cached session by ID.
	// Returns session.ErrSessionNotFound if the session is not in the cache.
	GetSession(ctx context.Context, sessionID string) (*session.Session, error)

	// SetSession stores or replaces a session in the cache with the given TTL.
	// A zero TTL means the entry does not expire.
	SetSession(ctx context.Context, s *session.Session, ttl time.Duration) error

	// DeleteSession permanently removes a session from the cache.
	DeleteSession(ctx context.Context, sessionID string) error

	// AppendMessage adds a message to the cached session's message list.
	// Returns session.ErrSessionNotFound if the session is not in the cache.
	AppendMessage(ctx context.Context, sessionID string, msg *session.Message) error

	// GetRecentMessages returns the most recent messages for a session,
	// ordered chronologically (oldest first). Limit controls the max count.
	// Returns session.ErrSessionNotFound if the session is not in the cache.
	GetRecentMessages(ctx context.Context, sessionID string, limit int) ([]*session.Message, error)

	// RefreshTTL extends the expiration of a cached session.
	// A zero TTL removes the expiration.
	// Returns session.ErrSessionNotFound if the session is not in the cache.
	RefreshTTL(ctx context.Context, sessionID string, ttl time.Duration) error

	// Invalidate removes a session from the cache, signaling cache eviction
	// rather than permanent deletion. Functionally equivalent to DeleteSession
	// but semantically distinct for observability.
	Invalidate(ctx context.Context, sessionID string) error

	// Ping checks connectivity to the underlying store.
	Ping(ctx context.Context) error

	// Close releases resources held by the provider.
	Close() error
}
