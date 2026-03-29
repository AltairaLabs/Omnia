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

// eventTypeMemoryCreated is the event type published when a memory is saved.
const eventTypeMemoryCreated = "memory_created"

// eventTypeMemoryDeleted is the event type published when a memory is deleted.
const eventTypeMemoryDeleted = "memory_deleted"

// Sentinel errors returned by the memory service and handler.
var (
	ErrMissingWorkspace = errors.New("workspace parameter is required")
	ErrMissingQuery     = errors.New("search query parameter is required")
	ErrMissingMemoryID  = errors.New("memory ID is required")
	ErrMissingBody      = errors.New("request body is required")
	ErrBodyTooLarge     = errors.New("request body too large")
)

// MemoryServiceConfig holds runtime configuration for the MemoryService.
type MemoryServiceConfig struct {
	// DefaultTTL is applied to new memories that do not carry an explicit ExpiresAt.
	// Zero means no default TTL.
	DefaultTTL time.Duration
	// Purpose is the default purpose tag sourced from the CRD configuration.
	Purpose string
}

// MemoryService wraps the memory store with business logic for the HTTP layer.
type MemoryService struct {
	store          memory.Store
	embeddingSvc   *memory.EmbeddingService // nil if embeddings not configured
	eventPublisher MemoryEventPublisher     // nil if event publishing not configured
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

// SaveMemory persists a memory entry and, if an embedding service is configured,
// asynchronously generates and stores its embedding.
func (s *MemoryService) SaveMemory(ctx context.Context, mem *memory.Memory) error {
	if mem.ExpiresAt == nil && s.config.DefaultTTL > 0 {
		exp := time.Now().Add(s.config.DefaultTTL)
		mem.ExpiresAt = &exp
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
	return nil
}

// SearchMemories retrieves memories matching a query and scope.
func (s *MemoryService) SearchMemories(ctx context.Context, scope map[string]string, query string, opts memory.RetrieveOptions) ([]*memory.Memory, error) {
	return s.store.Retrieve(ctx, scope, query, opts)
}

// ListMemories returns memories for a given scope with pagination.
func (s *MemoryService) ListMemories(ctx context.Context, scope map[string]string, opts memory.ListOptions) ([]*memory.Memory, error) {
	return s.store.List(ctx, scope, opts)
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
	return nil
}

// DeleteAllMemories hard-deletes all memories for the given scope (DSAR).
func (s *MemoryService) DeleteAllMemories(ctx context.Context, scope map[string]string) error {
	return s.store.DeleteAll(ctx, scope)
}

// ExportMemories returns all memories for a scope without pagination (DSAR export).
func (s *MemoryService) ExportMemories(ctx context.Context, scope map[string]string) ([]*memory.Memory, error) {
	memories, err := s.store.ExportAll(ctx, scope)
	if err != nil {
		return nil, err
	}
	s.log.V(1).Info("memories exported", "workspace", scope[memory.ScopeWorkspaceID], "count", len(memories))
	return memories, nil
}
