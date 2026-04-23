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

// agentScopedScopeTag tags audit events on the agent-scoped admin path so
// dashboards can distinguish "agent policy" curation from workspace-wide
// institutional curation and from user data.
const agentScopedScopeTag = "agent_scoped"

// operation tags for agent-scoped audit events.
const (
	saveAgentScopedOp   = "save_agent_scoped"
	listAgentScopedOp   = "list_agent_scoped"
	deleteAgentScopedOp = "delete_agent_scoped"
)

// SaveAgentScopedMemory persists a (workspace, agent)-scoped admin memory.
// Provenance is forced to operator_curated by the store; this method adds
// the audit event and keeps the service surface consistent with the
// institutional path.
func (s *MemoryService) SaveAgentScopedMemory(ctx context.Context, mem *memory.Memory) error {
	if err := s.store.SaveAgentScoped(ctx, mem); err != nil {
		return err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    mem.ID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		Kind:        mem.Type,
		Metadata: map[string]string{
			"scope":     agentScopedScopeTag,
			"operation": saveAgentScopedOp,
			"agent_id":  mem.Scope[memory.ScopeAgentID],
		},
	})
	return nil
}

// ListAgentScopedMemories returns the admin-curated memories visible to every
// user of the named agent in the workspace.
func (s *MemoryService) ListAgentScopedMemories(ctx context.Context, workspaceID, agentID string, opts memory.ListOptions) ([]*memory.Memory, error) {
	mems, err := s.store.ListAgentScoped(ctx, workspaceID, agentID, opts)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: workspaceID,
		Metadata: map[string]string{
			"scope":     agentScopedScopeTag,
			"operation": listAgentScopedOp,
			"agent_id":  agentID,
		},
	})
	return mems, nil
}

// DeleteAgentScopedMemory soft-deletes an agent-scoped admin memory. The
// store refuses to delete user- or other-agent-scoped rows via this path,
// returning memory.ErrNotAgentScoped; the error propagates unchanged so the
// HTTP handler maps it to 400.
func (s *MemoryService) DeleteAgentScopedMemory(ctx context.Context, workspaceID, agentID, memoryID string) error {
	if err := s.store.DeleteAgentScoped(ctx, workspaceID, agentID, memoryID); err != nil {
		return err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryDeleted,
		MemoryID:    memoryID,
		WorkspaceID: workspaceID,
		Metadata: map[string]string{
			"scope":     agentScopedScopeTag,
			"operation": deleteAgentScopedOp,
			"agent_id":  agentID,
		},
	})
	return nil
}
