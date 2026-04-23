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

package api

import (
	"context"

	"github.com/altairalabs/omnia/internal/memory"
)

// institutionalScopeTag tags audit events on the institutional admin path so
// dashboards can filter workspace-knowledge activity from user-data activity.
const institutionalScopeTag = "institutional"

// operation tags emitted on the Metadata.operation key for institutional audit
// events. Kept as constants so dashboards and tests share the same strings.
const (
	saveInstitutionalOp   = "save_institutional"
	listInstitutionalOp   = "list_institutional"
	deleteInstitutionalOp = "delete_institutional"
)

// SaveInstitutionalMemory persists a workspace-scoped memory (no user_id, no
// agent_id) and emits a memory_created audit tagged scope=institutional.
// Audit fires only on success, matching Save/List/Search semantics.
func (s *MemoryService) SaveInstitutionalMemory(ctx context.Context, mem *memory.Memory) error {
	if err := s.store.SaveInstitutional(ctx, mem); err != nil {
		return err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    mem.ID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		Kind:        mem.Type,
		Metadata: map[string]string{
			"scope":     institutionalScopeTag,
			"operation": saveInstitutionalOp,
		},
	})
	return nil
}

// ListInstitutionalMemories returns all institutional memories in a workspace
// and emits a memory_accessed audit tagged operation=list_institutional.
func (s *MemoryService) ListInstitutionalMemories(ctx context.Context, workspaceID string, opts memory.ListOptions) ([]*memory.Memory, error) {
	mems, err := s.store.ListInstitutional(ctx, workspaceID, opts)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: workspaceID,
		Metadata: map[string]string{
			"scope":     institutionalScopeTag,
			"operation": listInstitutionalOp,
		},
	})
	return mems, nil
}

// DeleteInstitutionalMemory soft-deletes an institutional memory. The store
// layer refuses to delete user- or agent-scoped rows via this path, returning
// memory.ErrNotInstitutional; the error propagates unchanged so the HTTP
// handler can map it to a 400 response.
func (s *MemoryService) DeleteInstitutionalMemory(ctx context.Context, workspaceID, memoryID string) error {
	if err := s.store.DeleteInstitutional(ctx, workspaceID, memoryID); err != nil {
		return err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryDeleted,
		MemoryID:    memoryID,
		WorkspaceID: workspaceID,
		Metadata: map[string]string{
			"scope":     institutionalScopeTag,
			"operation": deleteInstitutionalOp,
		},
	})
	return nil
}
