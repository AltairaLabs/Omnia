/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

func newAgentScopedService(t *testing.T, store *agentScopedStub) (*MemoryService, *mockAuditLogger) {
	t.Helper()
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	audit := newMockAuditLogger()
	svc.SetAuditLogger(audit)
	return svc, audit
}

func TestSaveAgentScopedMemory_ForwardsAndEmitsAudit(t *testing.T) {
	store := &agentScopedStub{saveMemID: "a-1"}
	svc, audit := newAgentScopedService(t, store)

	mem := &memory.Memory{
		Type:    "policy",
		Content: "tool use rule",
		Scope: map[string]string{
			memory.ScopeWorkspaceID: "ws-1",
			memory.ScopeAgentID:     "agent-1",
		},
	}
	require.NoError(t, svc.SaveAgentScopedMemory(context.Background(), mem))

	store.mu.Lock()
	require.Len(t, store.saveCalls, 1)
	store.mu.Unlock()
	assert.Equal(t, "a-1", mem.ID)

	entry := audit.receiveEntry(t)
	assert.Equal(t, eventTypeMemoryCreated, entry.EventType)
	assert.Equal(t, "ws-1", entry.WorkspaceID)
	assert.Equal(t, agentScopedScopeTag, entry.Metadata["scope"])
	assert.Equal(t, saveAgentScopedOp, entry.Metadata["operation"])
	assert.Equal(t, "agent-1", entry.Metadata["agent_id"])
}

func TestSaveAgentScopedMemory_PropagatesStoreError(t *testing.T) {
	store := &agentScopedStub{saveErr: errors.New("boom")}
	svc, audit := newAgentScopedService(t, store)

	err := svc.SaveAgentScopedMemory(context.Background(), &memory.Memory{
		Scope: map[string]string{
			memory.ScopeWorkspaceID: "ws-1",
			memory.ScopeAgentID:     "agent-1",
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")

	select {
	case e := <-audit.entries:
		t.Fatalf("unexpected audit on error: %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestListAgentScopedMemories_ForwardsAndEmitsAudit(t *testing.T) {
	store := &agentScopedStub{
		listResult: []*memory.Memory{{ID: "m-1"}, {ID: "m-2"}},
	}
	svc, audit := newAgentScopedService(t, store)

	mems, err := svc.ListAgentScopedMemories(context.Background(), "ws-1", "agent-1", memory.ListOptions{Limit: 50})
	require.NoError(t, err)
	assert.Len(t, mems, 2)

	store.mu.Lock()
	require.Len(t, store.listCalls, 1)
	store.mu.Unlock()

	entry := audit.receiveEntry(t)
	assert.Equal(t, "agent-1", entry.Metadata["agent_id"])
	assert.Equal(t, listAgentScopedOp, entry.Metadata["operation"])
}

func TestDeleteAgentScopedMemory_ForwardsAndEmitsAudit(t *testing.T) {
	store := &agentScopedStub{}
	svc, audit := newAgentScopedService(t, store)

	require.NoError(t, svc.DeleteAgentScopedMemory(context.Background(), "ws-1", "agent-1", "m-1"))

	store.mu.Lock()
	require.Len(t, store.deleteCalls, 1)
	store.mu.Unlock()

	entry := audit.receiveEntry(t)
	assert.Equal(t, eventTypeMemoryDeleted, entry.EventType)
	assert.Equal(t, "agent-1", entry.Metadata["agent_id"])
	assert.Equal(t, deleteAgentScopedOp, entry.Metadata["operation"])
}

func TestDeleteAgentScopedMemory_PropagatesNotAgentScopedSentinel(t *testing.T) {
	store := &agentScopedStub{deleteErr: memory.ErrNotAgentScoped}
	svc, audit := newAgentScopedService(t, store)

	err := svc.DeleteAgentScopedMemory(context.Background(), "ws-1", "agent-1", "m-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, memory.ErrNotAgentScoped)

	select {
	case e := <-audit.entries:
		t.Fatalf("unexpected audit on error: %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}
