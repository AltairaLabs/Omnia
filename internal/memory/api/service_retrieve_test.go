/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

// multiTierStoreStub extends mockMemoryStore-like behavior with configurable
// multi-tier responses. Inline to avoid leaking test-only state into the
// shared mockMemoryStore type.
type multiTierStoreStub struct {
	mockMemoryStore
	mu       sync.Mutex
	mtCalls  []memory.MultiTierRequest
	mtResult *memory.MultiTierResult
	mtErr    error
}

func (m *multiTierStoreStub) RetrieveMultiTier(_ context.Context, req memory.MultiTierRequest) (*memory.MultiTierResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mtCalls = append(m.mtCalls, req)
	if m.mtErr != nil {
		return nil, m.mtErr
	}
	return m.mtResult, nil
}

func TestRetrieveMultiTier_PassesThroughAndEmitsAudit(t *testing.T) {
	store := &multiTierStoreStub{
		mtResult: &memory.MultiTierResult{
			Memories: []*memory.MultiTierMemory{
				{Memory: &memory.Memory{ID: "m-1"}, Tier: memory.TierUser},
			},
			Total: 1,
		},
	}
	audit := newMockAuditLogger()
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetAuditLogger(audit)

	got, err := svc.RetrieveMultiTier(context.Background(), memory.MultiTierRequest{
		WorkspaceID: "ws-1",
		UserID:      "u-1",
		AgentID:     "a-1",
		Query:       "dark",
		Limit:       10,
	})
	require.NoError(t, err)
	require.Equal(t, 1, got.Total)
	assert.Equal(t, "m-1", got.Memories[0].ID)

	store.mu.Lock()
	require.Len(t, store.mtCalls, 1)
	call := store.mtCalls[0]
	store.mu.Unlock()
	assert.Equal(t, "ws-1", call.WorkspaceID)
	assert.Equal(t, "a-1", call.AgentID)

	entry := audit.receiveEntry(t)
	assert.Equal(t, auditEventMemoryAccessed, entry.EventType)
	assert.Equal(t, "retrieve_multi_tier", entry.Metadata["operation"])
	assert.Equal(t, "ws-1", entry.WorkspaceID)
	assert.Equal(t, "u-1", entry.UserID)
}

func TestRetrieveMultiTier_PropagatesStoreError(t *testing.T) {
	store := &multiTierStoreStub{mtErr: errors.New("boom")}
	audit := newMockAuditLogger()
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetAuditLogger(audit)

	_, err := svc.RetrieveMultiTier(context.Background(), memory.MultiTierRequest{WorkspaceID: "ws-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")

	// No audit on failure.
	select {
	case e := <-audit.entries:
		t.Fatalf("unexpected audit entry on store error: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// expected — no event
	}
}
