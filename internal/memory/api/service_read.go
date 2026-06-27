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

// Package api provides the HTTP API layer for the memory-api service.
package api

import (
	"context"
	"errors"

	"github.com/altairalabs/omnia/internal/memory"
)

// OpenMemory returns the full content of a single memory by entity
// ID. Mirrors the recall scope filter; the active-only filter
// applies so superseded observations are not returned. Used by
// memory__open when the agent needs the body of a large memory.
func (s *MemoryService) OpenMemory(ctx context.Context, scope map[string]string, entityID string) (*memory.Memory, error) {
	mem, err := s.store.GetMemory(ctx, scope, entityID)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		MemoryID:    entityID,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
		Metadata:    map[string]string{metaKeyOperation: "open"},
	})
	return mem, nil
}

// FindConflicts returns entities whose dedup machinery missed —
// i.e. those holding more than one active observation. The dashboard
// renders this as a triage queue so operators can see when the
// `about` discipline (or the embedding-similarity threshold) has
// drifted.
func (s *MemoryService) FindConflicts(ctx context.Context, workspaceID string, limit int) ([]memory.ConflictedEntity, error) {
	out, err := s.store.FindConflictedEntities(ctx, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: workspaceID,
		Metadata:    map[string]string{metaKeyOperation: "list_conflicts"},
	})
	return out, nil
}

// LinkMemories inserts a directed edge in memory_relations so
// derived facts (preferences, notes) attached to an anchor entity
// (the user identity) survive renames of the target. Returns the
// new relation ID.
func (s *MemoryService) LinkMemories(ctx context.Context, scope map[string]string,
	sourceEntityID, targetEntityID, relationType string, weight float64,
) (string, error) {
	id, err := s.store.LinkEntities(ctx, scope, sourceEntityID, targetEntityID, relationType, weight)
	if err != nil {
		return "", err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    id,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
		Metadata: map[string]string{
			metaKeyOperation: "link",
			"source_id":      sourceEntityID,
			"target_id":      targetEntityID,
			"relation_type":  relationType,
		},
	})
	return id, nil
}

// SearchMemories retrieves memories matching a query and scope.
// When an embedding service is configured and the query is non-empty,
// the call routes through Store.RetrieveHybrid so semantic-only
// matches (e.g. "what do I prefer?" → "user likes dark mode") surface
// alongside lexical hits via Reciprocal Rank Fusion. Without an
// embedder, or for empty queries, it falls through to the FTS-only
// Retrieve path.
func (s *MemoryService) SearchMemories(ctx context.Context, scope map[string]string, query string, opts memory.RetrieveOptions) ([]*memory.Memory, error) {
	results, err := s.searchMemoriesInner(ctx, scope, query, opts)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
		Metadata:    map[string]string{metaKeyOperation: "search"},
	})
	return results, nil
}

// searchMemoriesInner picks between the FTS-only Retrieve and the
// hybrid RRF path based on embedder availability. If the embedding
// call fails the call falls back to FTS so a transient embedder
// outage degrades recall quality rather than hard-failing the
// request — recall is too central to the agent loop to make brittle.
func (s *MemoryService) searchMemoriesInner(ctx context.Context, scope map[string]string, query string, opts memory.RetrieveOptions) ([]*memory.Memory, error) {
	if s.embeddingSvc == nil || query == "" {
		return s.store.Retrieve(ctx, scope, query, opts)
	}
	embeddings, err := s.embeddingSvc.Provider().Embed(ctx, []string{query})
	if err != nil || len(embeddings) == 0 || len(embeddings[0]) == 0 {
		s.log.V(1).Info("hybrid recall fallback to FTS",
			"reason", "embed_query_failed", "error", err)
		return s.store.Retrieve(ctx, scope, query, opts)
	}
	return s.store.RetrieveHybrid(ctx, scope, query, embeddings[0], opts)
}

// RelatedForMemories returns a map keyed by source entity ID, with each
// value being the relations originating from that entity. Used by the
// recall path to attach `related[]` to each result so the agent can
// navigate the memory graph and reason about supersession candidates
// without making a second round-trip per memory.
//
// Returns an empty map (not nil) when there are no memories so the
// handler can call this unconditionally and look up by ID without nil
// guards.
func (s *MemoryService) RelatedForMemories(ctx context.Context, scope map[string]string, mems []*memory.Memory) map[string][]memory.EntityRelation {
	out := make(map[string][]memory.EntityRelation, len(mems))
	if len(mems) == 0 {
		return out
	}
	ids := make([]string, 0, len(mems))
	for _, m := range mems {
		if m != nil && m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return out
	}
	rels, err := s.store.FindRelatedEntities(ctx, scope, ids, s.maxRelatedPerMemory(ctx))
	if err != nil {
		s.log.V(1).Info("recall related lookup failed", "error", err, "ids", len(ids))
		return out
	}
	for _, r := range rels {
		out[r.SourceEntityID] = append(out[r.SourceEntityID], r)
	}
	return out
}

// ListMemories returns memories for a given scope with pagination.
func (s *MemoryService) ListMemories(ctx context.Context, scope map[string]string, opts memory.ListOptions) ([]*memory.Memory, error) {
	results, err := s.store.List(ctx, scope, opts)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
		Metadata:    map[string]string{metaKeyOperation: "list"},
	})
	return results, nil
}

// AggregateMemories runs a workspace-scoped aggregate over memory_entities.
// Thin pass-through to the store; kept here for symmetry with other Service
// methods so handlers always go through one indirection. Asserts to the
// memory.Aggregator interface rather than the concrete *PostgresMemoryStore:
// the Redis-cleanup work wraps the store in a *CachedStore, which delegates
// Aggregate to its inner store, so a concrete-type assertion 500s on every
// request with the cache on (issue #1253). Test fakes that don't implement
// Aggregate still fail the assertion and surface a clear error.
func (s *MemoryService) AggregateMemories(ctx context.Context, opts memory.AggregateOptions) ([]memory.AggregateRow, error) {
	agg, ok := s.store.(memory.Aggregator)
	if !ok {
		return nil, errors.New("memory service: aggregate requires a store that supports Aggregate")
	}
	return agg.Aggregate(ctx, opts)
}
