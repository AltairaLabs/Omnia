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

// SaveMemory persists a memory entry and, if an embedding service is configured,
// asynchronously generates and stores its embedding.
func (s *MemoryService) SaveMemory(ctx context.Context, mem *memory.Memory) error {
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
	if err := s.store.Save(ctx, mem); err != nil {
		return err
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
	return nil
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
