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
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/memory"
)

// Audit event type constants for memory operations.
const (
	// eventTypeMemoryCreated is the event type published when a memory is saved.
	eventTypeMemoryCreated = "memory_created"
	// auditEventMemoryAccessed is the event type emitted when memories are read.
	auditEventMemoryAccessed = "memory_accessed"
	// auditEventMemoryExported is the event type emitted on DSAR export.
	auditEventMemoryExported = "memory_exported"
)

// eventTypeMemoryDeleted is the event type published when a memory is deleted.
const eventTypeMemoryDeleted = "memory_deleted"

// Sentinel errors returned by the memory service and handler.
var (
	ErrMissingWorkspace = errors.New("workspace parameter is required")
	ErrMissingUserID    = errors.New("user_id in scope is required — memories must be owned by a user")
	ErrMissingQuery     = errors.New("search query parameter is required")
	ErrMissingMemoryID  = errors.New("memory ID is required")
	ErrMissingBody      = errors.New("request body is required")
	ErrBodyTooLarge     = errors.New("request body too large")
	ErrExpiresAtInPast  = errors.New("expires_at must be in the future")
	ErrMissingAgentID   = errors.New("agent_id is required for agent-scoped admin operations")
)

// MemoryServiceConfig holds runtime configuration for the MemoryService.
type MemoryServiceConfig struct {
	// DefaultTTL is applied to new memories that do not carry an explicit ExpiresAt.
	// Zero means no default TTL.
	DefaultTTL time.Duration
	// Purpose is the default purpose tag sourced from the CRD configuration.
	Purpose string
}

// MemoryAuditLogger is the audit logging interface for memory operations.
// Implemented in ee/pkg/audit for enterprise deployments.
type MemoryAuditLogger interface {
	// LogEvent records an audit entry. Implementations must be non-blocking —
	// entries may be dropped if the internal buffer is full.
	LogEvent(ctx context.Context, entry *MemoryAuditEntry)
}

// MemoryAuditEntry represents a single audit log entry for a memory operation.
type MemoryAuditEntry struct {
	EventType   string
	MemoryID    string
	WorkspaceID string
	UserID      string
	Kind        string
	IPAddress   string
	UserAgent   string
	Metadata    map[string]string
}

// MemoryService wraps the memory store with business logic for the HTTP layer.
type MemoryService struct {
	store          memory.Store
	embeddingSvc   *memory.EmbeddingService // nil if embeddings not configured
	eventPublisher MemoryEventPublisher     // nil if event publishing not configured
	auditLogger    MemoryAuditLogger        // nil if audit logging not configured
	policyLoader   memory.PolicyLoader      // nil if no MemoryPolicy resolution wired
	config         MemoryServiceConfig
	log            logr.Logger
}

// NewMemoryService creates a new MemoryService backed by the given store.
// embeddingSvc may be nil when embedding is not configured.
func NewMemoryService(store memory.Store, embeddingSvc *memory.EmbeddingService, cfg MemoryServiceConfig, log logr.Logger) *MemoryService {
	return &MemoryService{
		store:        store,
		embeddingSvc: embeddingSvc,
		config:       cfg,
		log:          log.WithName("memory-service"),
	}
}

// SetEventPublisher configures the event publisher for the service.
// It may be called at most once before the service begins handling requests.
func (s *MemoryService) SetEventPublisher(p MemoryEventPublisher) {
	s.eventPublisher = p
}

// SetPolicyLoader wires a MemoryPolicy loader so retrieval can build a
// per-tier ranker from the workspace's bound policy. May be called at
// most once before the service begins handling requests. Optional —
// without a loader the service uses the identity ranker (no per-tier
// score adjustment).
func (s *MemoryService) SetPolicyLoader(loader memory.PolicyLoader) {
	s.policyLoader = loader
}

// SetAuditLogger configures the audit logger for the service.
// It may be called at most once before the service begins handling requests.
func (s *MemoryService) SetAuditLogger(l MemoryAuditLogger) {
	s.auditLogger = l
}

// emitAuditEvent fires an audit log entry asynchronously. If no audit logger is
// configured the call is a no-op. Request metadata (IP, User-Agent) is extracted
// from the context when present.
func (s *MemoryService) emitAuditEvent(ctx context.Context, entry *MemoryAuditEntry) {
	if s.auditLogger == nil {
		return
	}
	if meta, ok := requestMetaFromCtx(ctx); ok {
		entry.IPAddress = meta.IPAddress
		entry.UserAgent = meta.UserAgent
	}
	logger := s.auditLogger
	// Detached background context is intentional: the audit-log write must
	// complete even if the request context is cancelled (client disconnect,
	// deadline exceeded). Losing an audit event is worse than wasting work.
	go logger.LogEvent(context.Background(), entry)
}

// SaveMemory persists a memory entry and, if an embedding service is
// configured, asynchronously generates and stores its embedding.
// Backwards-compatible thin wrapper around SaveMemoryWithResult that
// discards the dedup result. New callers prefer SaveMemoryWithResult
// so the agent sees auto_superseded / potential_duplicates info.
func (s *MemoryService) SaveMemory(ctx context.Context, mem *memory.Memory) error {
	_, err := s.SaveMemoryWithResult(ctx, mem)
	return err
}

// SaveMemoryWithResult is the rich Omnia write API. Returns a
// SaveResult describing how the dedup pipeline resolved the write
// (added vs auto_superseded; supersedes ids; reason). The HTTP
// handler surfaces this to the agent so its reply ("Got it" vs
// "Updated your name from X to Y") and follow-up tool calls can be
// honest about what happened.
//
// Two dedup paths run before the write commits:
//
//  1. Structured key. When the caller set about_kind+about_key in
//     metadata the store's ON CONFLICT path supersedes any prior
//     observation under the same entity (handled inside
//     store.SaveWithResult).
//  2. Embedding similarity. When no about key is set AND an
//     embedding service is configured, the service embeds the new
//     content and queries for similar active observations under the
//     same scope. cosine ≥ 0.95 routes through
//     AppendObservationToEntity to atomically supersede the match;
//     0.85 ≤ cosine < 0.95 lands the write normally and surfaces the
//     matches as PotentialDuplicates so the agent can decide on a
//     later turn.
func (s *MemoryService) SaveMemoryWithResult(ctx context.Context, mem *memory.Memory) (*memory.SaveResult, error) {
	if mem.ExpiresAt == nil && s.config.DefaultTTL > 0 {
		exp := time.Now().Add(s.config.DefaultTTL)
		mem.ExpiresAt = &exp
	}
	// Stamp the service-configured purpose when the caller didn't set one.
	// Same shape as DefaultTTL — the store reads Metadata[MetaKeyPurpose]
	// into memory_entities.purpose at insert time.
	if s.config.Purpose != "" {
		if mem.Metadata == nil {
			mem.Metadata = map[string]any{}
		}
		if _, ok := mem.Metadata[memory.MetaKeyPurpose]; !ok {
			mem.Metadata[memory.MetaKeyPurpose] = s.config.Purpose
		}
	}

	// Embedding-similarity dedup — only when no structured about key
	// (the structured path is more reliable) AND embedding service is
	// available. Failures here log + fall through to a normal insert
	// rather than failing the write.
	var preMatches []memory.SimilarObservation
	if !hasAboutKeyInMetadata(mem) && s.embeddingSvc != nil {
		matches, simErr := s.findSimilarForDedup(ctx, mem)
		if simErr != nil {
			s.log.V(1).Info("similarity dedup skipped",
				"reason", "embedding/query failed",
				"error", simErr.Error())
		} else {
			preMatches = matches
		}
	}
	if len(preMatches) > 0 && preMatches[0].Similarity >= memory.DefaultAutoSupersedeSimilarity {
		return s.applyAutoSupersedeViaSimilarity(ctx, mem, preMatches[0])
	}

	res, err := s.store.SaveWithResult(ctx, mem)
	if err != nil {
		return nil, err
	}
	for _, m := range preMatches {
		res.PotentialDuplicates = append(res.PotentialDuplicates, memory.DuplicateCandidate{
			ID:         m.ObservationID,
			Content:    m.Content,
			Similarity: m.Similarity,
		})
	}
	if s.eventPublisher != nil {
		event := MemoryEvent{
			EventType:   eventTypeMemoryCreated,
			MemoryID:    mem.ID,
			WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
			UserID:      mem.Scope[memory.ScopeUserID],
			Kind:        mem.Type,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		go func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory event publish failed", "eventType", event.EventType, "memoryID", event.MemoryID)
			}
		}()
	}
	if s.embeddingSvc != nil {
		go func() {
			embedCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.embeddingSvc.EmbedMemory(embedCtx, mem); err != nil {
				s.log.Error(err, "async embedding failed", "memoryID", mem.ID)
			}
		}()
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    mem.ID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeUserID],
		Kind:        mem.Type,
	})
	return res, nil
}

// hasAboutKeyInMetadata reports whether the caller set the
// structured-dedup metadata keys; if so the store handles dedup
// via the unique index path and the embedding-similarity path is
// skipped.
func hasAboutKeyInMetadata(mem *memory.Memory) bool {
	if mem == nil || mem.Metadata == nil {
		return false
	}
	kind, _ := mem.Metadata[memory.MetaKeyAboutKind].(string)
	key, _ := mem.Metadata[memory.MetaKeyAboutKey].(string)
	return kind != "" && key != ""
}

// findSimilarForDedup embeds the new content (synchronously — adds
// ~one embedding-API roundtrip to the write path) and asks the
// store for active observations within DefaultSurfaceDuplicateSimilarity.
// Returns nil when embedding fails or yields no result.
func (s *MemoryService) findSimilarForDedup(ctx context.Context, mem *memory.Memory) ([]memory.SimilarObservation, error) {
	embeddings, err := s.embeddingSvc.Provider().Embed(ctx, []string{mem.Content})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return nil, errors.New("embed returned empty result")
	}
	return s.store.FindSimilarObservations(ctx, mem.Scope, embeddings[0],
		memory.DefaultDuplicateCandidateLimit,
		memory.DefaultSurfaceDuplicateSimilarity)
}

// applyAutoSupersedeViaSimilarity attaches the new observation to
// the matched entity and supersedes the entity's prior active
// observations atomically. Returns a SaveResult marked
// auto_superseded with reason=high_similarity. The agent uses this
// to phrase its reply honestly ("I already had something like that
// — refreshed").
func (s *MemoryService) applyAutoSupersedeViaSimilarity(
	ctx context.Context,
	mem *memory.Memory,
	match memory.SimilarObservation,
) (*memory.SaveResult, error) {
	supersededIDs, err := s.store.AppendObservationToEntity(ctx, match.EntityID, mem)
	if err != nil {
		return nil, err
	}

	// Audit + async embed for the new observation, mirroring the
	// happy-path behaviour. Event publish + audit fire even on
	// supersede so dashboards see "this entity was updated".
	if s.eventPublisher != nil {
		event := MemoryEvent{
			EventType:   eventTypeMemoryCreated,
			MemoryID:    mem.ID,
			WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
			UserID:      mem.Scope[memory.ScopeUserID],
			Kind:        mem.Type,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		go func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory event publish failed", "eventType", event.EventType, "memoryID", event.MemoryID)
			}
		}()
	}
	if s.embeddingSvc != nil {
		go func() {
			embedCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.embeddingSvc.EmbedMemory(embedCtx, mem); err != nil {
				s.log.Error(err, "async embedding failed", "memoryID", mem.ID)
			}
		}()
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    mem.ID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeUserID],
		Kind:        mem.Type,
		Metadata: map[string]string{
			"dedup_reason": string(memory.ReasonHighSimilarity),
		},
	})

	return &memory.SaveResult{
		ID:                       mem.ID,
		Action:                   memory.SaveActionAutoSuperseded,
		SupersededObservationIDs: supersededIDs,
		SupersedeReason:          memory.ReasonHighSimilarity,
	}, nil
}

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
		Metadata:    map[string]string{"operation": "open"},
	})
	return mem, nil
}

// UpdateMemory atomically supersedes the prior active observation
// under the given entity and inserts a new one with the supplied
// content. Returns a SaveResult shaped for the agent's reply
// (action=auto_superseded with reason=explicit). The agent uses
// memory__update for cases where it knows the entity ID — most
// commonly when recall surfaced the prior observation and the agent
// decides this is a replacement.
func (s *MemoryService) UpdateMemory(ctx context.Context, entityID string, mem *memory.Memory) (*memory.SaveResult, error) {
	supersededIDs, err := s.store.AppendObservationToEntity(ctx, entityID, mem)
	if err != nil {
		return nil, err
	}

	if s.eventPublisher != nil {
		event := MemoryEvent{
			EventType:   eventTypeMemoryCreated,
			MemoryID:    mem.ID,
			WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
			UserID:      mem.Scope[memory.ScopeUserID],
			Kind:        mem.Type,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		go func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory event publish failed",
					"eventType", event.EventType, "memoryID", event.MemoryID)
			}
		}()
	}
	if s.embeddingSvc != nil {
		go func() {
			embedCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.embeddingSvc.EmbedMemory(embedCtx, mem); err != nil {
				s.log.Error(err, "async embedding failed", "memoryID", mem.ID)
			}
		}()
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    mem.ID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeUserID],
		Kind:        mem.Type,
		Metadata:    map[string]string{"dedup_reason": "explicit"},
	})

	return &memory.SaveResult{
		ID:                       mem.ID,
		Action:                   memory.SaveActionAutoSuperseded,
		SupersededObservationIDs: supersededIDs,
		SupersedeReason:          memory.SaveSupersedeReason("explicit"),
	}, nil
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
			"operation":     "link",
			"source_id":     sourceEntityID,
			"target_id":     targetEntityID,
			"relation_type": relationType,
		},
	})
	return id, nil
}

// SearchMemories retrieves memories matching a query and scope.
func (s *MemoryService) SearchMemories(ctx context.Context, scope map[string]string, query string, opts memory.RetrieveOptions) ([]*memory.Memory, error) {
	results, err := s.store.Retrieve(ctx, scope, query, opts)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
		Metadata:    map[string]string{"operation": "search"},
	})
	return results, nil
}

// defaultRelatedPerMemory caps the per-memory related[] list. Three keeps
// the recall payload lean while still letting the agent see the strongest
// graph neighbours (an identity entity's preferences, a workspace doc's
// related skills) it might want to update or supersede.
const defaultRelatedPerMemory = 3

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
	rels, err := s.store.FindRelatedEntities(ctx, scope, ids, defaultRelatedPerMemory)
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
		Metadata:    map[string]string{"operation": "list"},
	})
	return results, nil
}

// AggregateMemories runs a workspace-scoped aggregate over memory_entities.
// Thin pass-through to the store; kept here for symmetry with other Service
// methods so handlers always go through one indirection. Type-asserts to
// *PostgresMemoryStore because the fake stores in the test suite don't
// implement Aggregate; an interface addition would break every fake for
// no real benefit.
func (s *MemoryService) AggregateMemories(ctx context.Context, opts memory.AggregateOptions) ([]memory.AggregateRow, error) {
	pgStore, ok := s.store.(*memory.PostgresMemoryStore)
	if !ok {
		return nil, errors.New("memory service: aggregate requires a PostgresMemoryStore")
	}
	return pgStore.Aggregate(ctx, opts)
}

// DeleteMemory performs a soft delete (forget) of a single memory.
func (s *MemoryService) DeleteMemory(ctx context.Context, scope map[string]string, memoryID string) error {
	if err := s.store.Delete(ctx, scope, memoryID); err != nil {
		return err
	}
	if s.eventPublisher != nil {
		event := MemoryEvent{
			EventType:   eventTypeMemoryDeleted,
			MemoryID:    memoryID,
			WorkspaceID: scope[memory.ScopeWorkspaceID],
			UserID:      scope[memory.ScopeUserID],
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		go func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory event publish failed", "eventType", event.EventType, "memoryID", event.MemoryID)
			}
		}()
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryDeleted,
		MemoryID:    memoryID,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
	})
	return nil
}

// DeleteAllMemories hard-deletes all memories for the given scope (DSAR).
func (s *MemoryService) DeleteAllMemories(ctx context.Context, scope map[string]string) error {
	if err := s.store.DeleteAll(ctx, scope); err != nil {
		return err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryDeleted,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
		Metadata:    map[string]string{"operation": "delete_all"},
	})
	return nil
}

// BatchDeleteMemories hard-deletes up to limit memories for the given scope (paginated DSAR).
// Returns the count of deleted rows so the caller can loop until 0.
func (s *MemoryService) BatchDeleteMemories(ctx context.Context, scope map[string]string, limit int) (int, error) {
	n, err := s.store.BatchDelete(ctx, scope, limit)
	if err != nil {
		return 0, err
	}
	if n > 0 && s.eventPublisher != nil {
		event := MemoryEvent{
			EventType:   eventTypeMemoryDeleted,
			WorkspaceID: scope[memory.ScopeWorkspaceID],
			UserID:      scope[memory.ScopeUserID],
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		go func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory batch delete event publish failed", "eventType", event.EventType, "count", n)
			}
		}()
	}
	if n > 0 {
		s.emitAuditEvent(ctx, &MemoryAuditEntry{
			EventType:   eventTypeMemoryDeleted,
			WorkspaceID: scope[memory.ScopeWorkspaceID],
			UserID:      scope[memory.ScopeUserID],
			Metadata:    map[string]string{"operation": "batch_delete"},
		})
	}
	return n, nil
}

// ExportMemories returns all memories for a scope without pagination (DSAR export).
func (s *MemoryService) ExportMemories(ctx context.Context, scope map[string]string) ([]*memory.Memory, error) {
	memories, err := s.store.ExportAll(ctx, scope)
	if err != nil {
		return nil, err
	}
	s.log.V(1).Info("memories exported", "workspace", scope[memory.ScopeWorkspaceID], "count", len(memories))
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryExported,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
	})
	return memories, nil
}
