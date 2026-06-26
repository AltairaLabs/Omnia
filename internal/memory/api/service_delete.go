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
	"time"

	"github.com/altairalabs/omnia/internal/memory"
)

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
		s.safeGo("event_publish", func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, logMemoryEventPublishFailed, "eventType", event.EventType, "memoryID", event.MemoryID)
			}
		})
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
		Metadata:    map[string]string{metaKeyOperation: "delete_all"},
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
		s.safeGo("event_publish_batch_delete", func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory batch delete event publish failed", "eventType", event.EventType, "count", n)
			}
		})
	}
	if n > 0 {
		s.emitAuditEvent(ctx, &MemoryAuditEntry{
			EventType:   eventTypeMemoryDeleted,
			WorkspaceID: scope[memory.ScopeWorkspaceID],
			UserID:      scope[memory.ScopeUserID],
			Metadata:    map[string]string{metaKeyOperation: "batch_delete"},
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
