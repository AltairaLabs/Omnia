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

// institutionalStub extends mockMemoryStore with configurable
// institutional-path responses and call recording.
type institutionalStub struct {
	mockMemoryStore
	mu sync.Mutex

	saveCalls []*memory.Memory
	saveErr   error
	saveMemID string // ID written back on the passed Memory

	listCalls  []string
	listResult []*memory.Memory
	listErr    error

	deleteCalls []struct{ ws, id string }
	deleteErr   error
}

func (i *institutionalStub) SaveInstitutional(_ context.Context, mem *memory.Memory) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.saveCalls = append(i.saveCalls, mem)
	if i.saveErr != nil {
		return i.saveErr
	}
	if i.saveMemID != "" {
		mem.ID = i.saveMemID
	}
	return nil
}

func (i *institutionalStub) ListInstitutional(_ context.Context, ws string, _ memory.ListOptions) ([]*memory.Memory, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.listCalls = append(i.listCalls, ws)
	return i.listResult, i.listErr
}

func (i *institutionalStub) DeleteInstitutional(_ context.Context, ws, id string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.deleteCalls = append(i.deleteCalls, struct{ ws, id string }{ws, id})
	return i.deleteErr
}

func newInstitutionalService(t *testing.T, store *institutionalStub) (*MemoryService, *mockAuditLogger) {
	t.Helper()
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	audit := newMockAuditLogger()
	svc.SetAuditLogger(audit)
	return svc, audit
}

func TestSaveInstitutionalMemory_ForwardsAndEmitsAudit(t *testing.T) {
	store := &institutionalStub{saveMemID: "inst-1"}
	svc, audit := newInstitutionalService(t, store)

	mem := &memory.Memory{
		Type:    "policy",
		Content: "API uses snake_case",
		Scope:   map[string]string{memory.ScopeWorkspaceID: "ws-1"},
	}
	require.NoError(t, svc.SaveInstitutionalMemory(context.Background(), mem))

	store.mu.Lock()
	require.Len(t, store.saveCalls, 1)
	store.mu.Unlock()
	assert.Equal(t, "inst-1", mem.ID)

	entry := audit.receiveEntry(t)
	assert.Equal(t, eventTypeMemoryCreated, entry.EventType)
	assert.Equal(t, "ws-1", entry.WorkspaceID)
	assert.Equal(t, institutionalScopeTag, entry.Metadata["scope"])
	assert.Equal(t, saveInstitutionalOp, entry.Metadata["operation"])
}

func TestSaveInstitutionalMemory_PropagatesStoreError(t *testing.T) {
	store := &institutionalStub{saveErr: errors.New("boom")}
	svc, audit := newInstitutionalService(t, store)

	err := svc.SaveInstitutionalMemory(context.Background(), &memory.Memory{
		Scope: map[string]string{memory.ScopeWorkspaceID: "ws-1"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")

	select {
	case e := <-audit.entries:
		t.Fatalf("unexpected audit on error: %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestListInstitutionalMemories_ForwardsAndEmitsAudit(t *testing.T) {
	store := &institutionalStub{
		listResult: []*memory.Memory{{ID: "m-1"}, {ID: "m-2"}},
	}
	svc, audit := newInstitutionalService(t, store)

	got, err := svc.ListInstitutionalMemories(context.Background(), "ws-1", memory.ListOptions{Limit: 20})
	require.NoError(t, err)
	require.Len(t, got, 2)

	entry := audit.receiveEntry(t)
	assert.Equal(t, auditEventMemoryAccessed, entry.EventType)
	assert.Equal(t, listInstitutionalOp, entry.Metadata["operation"])
	assert.Equal(t, institutionalScopeTag, entry.Metadata["scope"])
}

func TestListInstitutionalMemories_PropagatesStoreError(t *testing.T) {
	store := &institutionalStub{listErr: errors.New("db down")}
	svc, _ := newInstitutionalService(t, store)

	_, err := svc.ListInstitutionalMemories(context.Background(), "ws-1", memory.ListOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

func TestDeleteInstitutionalMemory_ForwardsAndEmitsAudit(t *testing.T) {
	store := &institutionalStub{}
	svc, audit := newInstitutionalService(t, store)

	require.NoError(t, svc.DeleteInstitutionalMemory(context.Background(), "ws-1", "m-1"))

	store.mu.Lock()
	require.Len(t, store.deleteCalls, 1)
	assert.Equal(t, "ws-1", store.deleteCalls[0].ws)
	assert.Equal(t, "m-1", store.deleteCalls[0].id)
	store.mu.Unlock()

	entry := audit.receiveEntry(t)
	assert.Equal(t, eventTypeMemoryDeleted, entry.EventType)
	assert.Equal(t, deleteInstitutionalOp, entry.Metadata["operation"])
	assert.Equal(t, "m-1", entry.MemoryID)
}

func TestDeleteInstitutionalMemory_PropagatesNotInstitutional(t *testing.T) {
	store := &institutionalStub{deleteErr: memory.ErrNotInstitutional}
	svc, audit := newInstitutionalService(t, store)

	err := svc.DeleteInstitutionalMemory(context.Background(), "ws-1", "m-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, memory.ErrNotInstitutional)

	select {
	case e := <-audit.entries:
		t.Fatalf("unexpected audit on error: %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}
