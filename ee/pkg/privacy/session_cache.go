/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"sync"
)

// SessionMetadata holds the namespace and agent name for a session.
// These are immutable after session creation, so caching is safe.
type SessionMetadata struct {
	Namespace string
	AgentName string
}

// SessionLookup resolves a session ID to its metadata.
type SessionLookup interface {
	LookupSession(ctx context.Context, sessionID string) (*SessionMetadata, error)
}

// SessionMetadataCache caches session metadata (namespace, agentName) with a
// bounded LRU-style eviction. Session metadata is immutable after creation.
type SessionMetadataCache struct {
	lookup  SessionLookup
	cache   sync.Map
	mu      sync.Mutex
	keys    []string // ordered keys for LRU eviction
	maxSize int
}

// NewSessionMetadataCache creates a cache backed by the given lookup with a
// maximum number of entries.
func NewSessionMetadataCache(lookup SessionLookup, maxSize int) *SessionMetadataCache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &SessionMetadataCache{
		lookup:  lookup,
		keys:    make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

// Resolve returns the namespace and agent name for a session, using the cache
// when available and falling back to the underlying lookup.
func (c *SessionMetadataCache) Resolve(ctx context.Context, sessionID string) (namespace, agentName string, err error) {
	// Check cache first.
	if v, ok := c.cache.Load(sessionID); ok {
		meta := v.(*SessionMetadata)
		return meta.Namespace, meta.AgentName, nil
	}

	// Cache miss — look up from the backing store.
	meta, err := c.lookup.LookupSession(ctx, sessionID)
	if err != nil {
		return "", "", err
	}

	// Store in cache with bounded eviction.
	c.mu.Lock()
	if len(c.keys) >= c.maxSize {
		// Evict oldest entry.
		evictKey := c.keys[0]
		c.keys = c.keys[1:]
		c.cache.Delete(evictKey)
	}
	c.keys = append(c.keys, sessionID)
	c.mu.Unlock()

	c.cache.Store(sessionID, meta)
	return meta.Namespace, meta.AgentName, nil
}
