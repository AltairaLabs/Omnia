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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

// --- Mock store ---

type mockStore struct {
	memories       []*memory.Memory
	saveErr        error
	retErr         error
	listErr        error
	delErr         error
	delAllErr      error
	batchDeleteErr error
	batchDeleteN   int
	exportAllErr   error
	savedMem       *memory.Memory
	// nextSaveResult overrides the default SaveActionAdded result.
	// Tests use this to exercise the auto_superseded surface without
	// standing up a real Postgres.
	nextSaveResult *memory.SaveResult
	// relatedBySource lets tests pre-populate the FindRelatedEntities
	// response per source entity ID — used to assert the recall
	// handler attaches related[] correctly.
	relatedBySource map[string][]memory.EntityRelation
	// conflicts canned response for FindConflictedEntities so the
	// /memories/conflicts handler test can assert end-to-end shape
	// without a real DB.
	conflicts []memory.ConflictedEntity
}

func (m *mockStore) Save(_ context.Context, mem *memory.Memory) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	mem.ID = "mock-id-001"
	m.savedMem = mem
	return nil
}

func (m *mockStore) SaveWithResult(ctx context.Context, mem *memory.Memory) (*memory.SaveResult, error) {
	if err := m.Save(ctx, mem); err != nil {
		return nil, err
	}
	if m.nextSaveResult != nil {
		// Stamp the freshly-allocated mock id so the caller sees a
		// consistent SaveResult.ID.
		out := *m.nextSaveResult
		if out.ID == "" {
			out.ID = mem.ID
		}
		return &out, nil
	}
	return &memory.SaveResult{ID: mem.ID, Action: memory.SaveActionAdded}, nil
}

func (m *mockStore) FindSimilarObservations(_ context.Context, _ map[string]string,
	_ []float32, _ int, _ float64,
) ([]memory.SimilarObservation, error) {
	return nil, nil
}

func (m *mockStore) AppendObservationToEntity(_ context.Context, entityID string, mem *memory.Memory) ([]string, error) {
	mem.ID = entityID
	return []string{"prior-obs"}, nil
}

func (m *mockStore) GetMemory(_ context.Context, _ map[string]string, entityID string) (*memory.Memory, error) {
	for _, mem := range m.memories {
		if mem.ID == entityID {
			return mem, nil
		}
	}
	return nil, memory.ErrNotFound
}

func (m *mockStore) LinkEntities(_ context.Context, _ map[string]string,
	_, _, _ string, _ float64,
) (string, error) {
	return "rel-mock", nil
}

func (m *mockStore) FindRelatedEntities(_ context.Context, _ map[string]string,
	entityIDs []string, _ int,
) ([]memory.EntityRelation, error) {
	if m.relatedBySource == nil {
		return nil, nil
	}
	out := make([]memory.EntityRelation, 0)
	for _, id := range entityIDs {
		out = append(out, m.relatedBySource[id]...)
	}
	return out, nil
}

func (m *mockStore) RetrieveHybrid(_ context.Context, _ map[string]string,
	_ string, _ []float32, _ memory.RetrieveOptions,
) ([]*memory.Memory, error) {
	if m.retErr != nil {
		return nil, m.retErr
	}
	return m.memories, nil
}

func (m *mockStore) SupersedeMany(_ context.Context, sourceIDs []string, mem *memory.Memory) (string, []string, error) {
	if len(sourceIDs) == 0 {
		return "", nil, nil
	}
	mem.ID = sourceIDs[0]
	// Return canned superseded IDs so handler tests can assert them.
	out := make([]string, len(sourceIDs))
	for i, id := range sourceIDs {
		out[i] = "obs-" + id
	}
	return sourceIDs[0], out, nil
}

func (m *mockStore) FindConflictedEntities(_ context.Context, _ string, _ int) ([]memory.ConflictedEntity, error) {
	return m.conflicts, nil
}

func (m *mockStore) Retrieve(_ context.Context, _ map[string]string, _ string, _ memory.RetrieveOptions) ([]*memory.Memory, error) {
	if m.retErr != nil {
		return nil, m.retErr
	}
	return m.memories, nil
}

func (m *mockStore) List(_ context.Context, _ map[string]string, _ memory.ListOptions) ([]*memory.Memory, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.memories, nil
}

func (m *mockStore) RetrieveMultiTier(_ context.Context, _ memory.MultiTierRequest) (*memory.MultiTierResult, error) {
	return &memory.MultiTierResult{Memories: []*memory.MultiTierMemory{}, Total: 0}, nil
}

func (m *mockStore) SaveInstitutional(_ context.Context, _ *memory.Memory) error { return nil }

func (m *mockStore) ListInstitutional(_ context.Context, _ string, _ memory.ListOptions) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *mockStore) DeleteInstitutional(_ context.Context, _, _ string) error { return nil }

func (m *mockStore) SaveAgentScoped(_ context.Context, _ *memory.Memory) error { return nil }

func (m *mockStore) ListAgentScoped(_ context.Context, _, _ string, _ memory.ListOptions) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *mockStore) DeleteAgentScoped(_ context.Context, _, _, _ string) error { return nil }

func (m *mockStore) FindCompactionCandidates(_ context.Context, _ memory.FindCompactionCandidatesOptions) ([]memory.CompactionCandidate, error) {
	return nil, nil
}

func (m *mockStore) SaveCompactionSummary(_ context.Context, _ memory.CompactionSummary) (string, error) {
	return "", nil
}

func (m *mockStore) Delete(_ context.Context, _ map[string]string, _ string) error {
	return m.delErr
}

func (m *mockStore) DeleteAll(_ context.Context, _ map[string]string) error {
	return m.delAllErr
}

func (m *mockStore) ExportAll(_ context.Context, _ map[string]string) ([]*memory.Memory, error) {
	if m.exportAllErr != nil {
		return nil, m.exportAllErr
	}
	return m.memories, nil
}

func (m *mockStore) BatchDelete(_ context.Context, _ map[string]string, _ int) (int, error) {
	if m.batchDeleteErr != nil {
		return 0, m.batchDeleteErr
	}
	return m.batchDeleteN, nil
}

func newTestHandler(store memory.Store) *Handler {
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	return NewHandler(svc, logr.Discard())
}

func setupMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

// TestHandler_RegisterRoutes_IncludesRetrieveMultiTier pins the multi-tier
// retrieval route to the mux. A regression that drops the route (e.g. stale
// merge or accidental deletion) shows up here as a 404 — every other status
// proves the route is registered, even if the handler rejects the body.
func TestHandler_RegisterRoutes_IncludesRetrieveMultiTier(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/memories/retrieve",
		strings.NewReader(`{"workspace_id":"ws"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatalf("POST /api/v1/memories/retrieve returned 404 — route not registered")
	}
}

// --- List memories tests ---

func TestHandleListMemories_Success(t *testing.T) {
	store := &mockStore{
		memories: []*memory.Memory{
			{ID: "1", Type: "preference", Content: "likes Go"},
			{ID: "2", Type: "fact", Content: "uses Linux"},
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp MemoryListResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Memories, 2)
}

// TestHandleListMemories_IncludesTier verifies the derived tier field appears
// on each row. Tier is computed from the scope map: virtual_user_id → "user",
// agent_id (no user) → "agent", neither → "institutional".
func TestHandleListMemories_IncludesTier(t *testing.T) {
	store := &mockStore{
		memories: []*memory.Memory{
			{ID: "u-1", Scope: map[string]string{
				memory.ScopeWorkspaceID: "ws", memory.ScopeUserID: "alice",
			}},
			{ID: "a-1", Scope: map[string]string{
				memory.ScopeWorkspaceID: "ws", memory.ScopeAgentID: "support",
			}},
			{ID: "i-1", Scope: map[string]string{
				memory.ScopeWorkspaceID: "ws",
			}},
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories?workspace=ws", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var raw struct {
		Memories []map[string]any `json:"memories"`
		Total    int              `json:"total"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&raw))
	require.Len(t, raw.Memories, 3)
	want := map[string]string{"u-1": "user", "a-1": "agent", "i-1": "institutional"}
	for _, m := range raw.Memories {
		id, _ := m["id"].(string)
		tier, _ := m["tier"].(string)
		assert.Equal(t, want[id], tier, "tier for %s", id)
	}
}

func TestHandleListMemories_MissingWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Contains(t, resp.Error, "workspace")
}

func TestHandleListMemories_WithFilters(t *testing.T) {
	store := &mockStore{memories: []*memory.Memory{}}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories?workspace=ws1&user_id=u1&agent=a1&type=preference&limit=10&offset=5", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleListMemories_StoreError(t *testing.T) {
	store := &mockStore{listErr: fmt.Errorf("db connection lost")}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// --- Search memories tests ---

func TestHandleSearchMemories_Success(t *testing.T) {
	store := &mockStore{
		memories: []*memory.Memory{
			{ID: "1", Content: "prefers dark mode"},
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/search?workspace=ws1&q=dark+mode", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp MemoryListResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 1, resp.Total)
}

func TestHandleSearchMemories_MissingWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/search?q=test", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleSearchMemories_MissingQuery(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/search?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Contains(t, resp.Error, "query")
}

func TestHandleSearchMemories_StoreError(t *testing.T) {
	store := &mockStore{retErr: fmt.Errorf("search failed")}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/search?workspace=ws1&q=test", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestHandleSaveMemory_RejectsMissingAboutForRequiredKind proves
// the wiring of MemoryServiceConfig.RequireAboutForKinds through
// the handler: a save for a configured kind without about returns
// 400 with ErrAboutRequired's message. The agent retries with about
// populated; without this the agent silently drops identity-class
// updates.
func TestHandleSaveMemory_RejectsMissingAboutForRequiredKind(t *testing.T) {
	store := &mockStore{}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{
		RequireAboutForKinds: []string{"fact"},
	}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := SaveMemoryRequest{
		Type:       "fact",
		Content:    "no anchor",
		Confidence: 0.9,
		Scope:      map[string]string{"workspace_id": "ws1", "user_id": "u1"},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Contains(t, resp.Error, "about")
}

// TestHandleSearchMemories_LargeBodyReturnsPreview proves the recall
// handler swaps the full content for a preview + has_full_body=true
// when an observation's body_size_bytes is over the inline
// threshold. The agent then calls memory__open to fetch the full
// body when needed — short memories still ride inline.
func TestHandleSearchMemories_LargeBodyReturnsPreview(t *testing.T) {
	largeBody := strings.Repeat("x", InlineBodyThresholdBytes*2)
	store := &mockStore{
		memories: []*memory.Memory{
			{ID: "doc-1", Type: "document", Content: largeBody, Confidence: 0.9,
				Metadata: map[string]any{
					memory.MetaKeyTitle:    "Engineering handbook",
					memory.MetaKeySummary:  "Coding standards and review process",
					memory.MetaKeyBodySize: len(largeBody),
				}},
			{ID: "pref-1", Type: "preference", Content: "prefers dark mode", Confidence: 0.9,
				Metadata: map[string]any{memory.MetaKeyBodySize: len("prefers dark mode")}},
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/search?workspace=ws1&q=anything", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp MemoryListResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.Len(t, resp.Memories, 2)

	var doc, pref *MemoryWithTier
	for _, m := range resp.Memories {
		switch m.ID {
		case "doc-1":
			doc = m
		case "pref-1":
			pref = m
		}
	}
	require.NotNil(t, doc)
	require.NotNil(t, pref)

	assert.True(t, doc.HasFullBody, "large memory must advertise has_full_body")
	assert.Empty(t, doc.Content, "large memory's full body must not be inlined")
	assert.NotEmpty(t, doc.ContentPreview, "large memory must carry preview")
	assert.LessOrEqual(t, len([]rune(doc.ContentPreview)), previewRunes)
	assert.Equal(t, "Engineering handbook", doc.Title)
	assert.Equal(t, "Coding standards and review process", doc.Summary)

	assert.False(t, pref.HasFullBody, "small memory keeps full content inline")
	assert.Equal(t, "prefers dark mode", pref.Content)
	assert.Empty(t, pref.ContentPreview, "small memory has no preview")
}

// TestHandleSearchMemories_AttachesRelated proves the recall response
// carries the per-memory `related[]` slice the agent uses to navigate
// the memory graph and decide which memories share an entity (the
// user identity, a project) so it can update / supersede them
// correctly.
func TestHandleSearchMemories_AttachesRelated(t *testing.T) {
	store := &mockStore{
		memories: []*memory.Memory{
			{ID: "ent-user", Content: "name: Phil"},
			{ID: "ent-pref", Content: "prefers dark mode"},
		},
		relatedBySource: map[string][]memory.EntityRelation{
			"ent-user": {
				{SourceEntityID: "ent-user", TargetEntityID: "ent-pref",
					RelationType: "MENTIONS", Weight: 1.0},
			},
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/search?workspace=ws1&q=phil", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp MemoryListResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.Len(t, resp.Memories, 2)

	var userMem, prefMem *MemoryWithTier
	for _, m := range resp.Memories {
		switch m.ID {
		case "ent-user":
			userMem = m
		case "ent-pref":
			prefMem = m
		}
	}
	require.NotNil(t, userMem)
	require.NotNil(t, prefMem)
	require.Len(t, userMem.Related, 1, "user identity memory should carry its outgoing relation")
	assert.Equal(t, "ent-pref", userMem.Related[0].TargetEntityID)
	assert.Equal(t, "MENTIONS", userMem.Related[0].RelationType)
	assert.Empty(t, prefMem.Related, "preference memory has no outgoing relations in this fixture")
}

// --- Save memory tests ---

func TestHandleSaveMemory_Success(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := SaveMemoryRequest{
		Type:       "preference",
		Content:    "likes Go",
		Confidence: 0.9,
		Scope:      map[string]string{"workspace_id": "ws1", "user_id": "test-user"},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)

	var resp MemoryResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "mock-id-001", resp.Memory.ID)
}

// TestHandleSaveMemory_SurfacesAutoSupersedeAction proves the
// HTTP response carries the dedup result so the agent can phrase
// its reply ("Updated your name from Slim Shard to Phil") and
// know which observation IDs the server superseded.
func TestHandleSaveMemory_SurfacesAutoSupersedeAction(t *testing.T) {
	store := &mockStore{
		nextSaveResult: &memory.SaveResult{
			Action:                   memory.SaveActionAutoSuperseded,
			SupersededObservationIDs: []string{"obs-old"},
			SupersedeReason:          memory.ReasonStructuredKey,
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := SaveMemoryRequest{
		Type:       "fact",
		Content:    "User's name is Phil",
		Confidence: 1.0,
		Scope:      map[string]string{"workspace_id": "ws1", "user_id": "u1"},
		About:      &AboutKey{Kind: "user", Key: "name"},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)

	var resp SaveMemoryResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, memory.SaveActionAutoSuperseded, resp.Action)
	assert.Equal(t, []string{"obs-old"}, resp.SupersededObservationIDs)
	assert.Equal(t, memory.ReasonStructuredKey, resp.SupersedeReason)
}

// TestHandleSaveMemory_PropagatesAboutKeyToMetadata proves the
// top-level `about` field on the request is translated into the
// store's metadata keys (MetaKeyAboutKind / MetaKeyAboutKey) so
// PostgresMemoryStore.SaveWithResult engages the structured-key
// dedup path. Without this translation the dedup index never fires.
func TestHandleSaveMemory_PropagatesAboutKeyToMetadata(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := SaveMemoryRequest{
		Type:       "fact",
		Content:    "User's name is Phil",
		Confidence: 1.0,
		Scope:      map[string]string{"workspace_id": "ws1", "user_id": "u1"},
		About:      &AboutKey{Kind: "User", Key: "Name"}, // mixed case → store normalizes
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	require.NotNil(t, store.savedMem, "Save not called")
	require.NotNil(t, store.savedMem.Metadata)
	assert.Equal(t, "User", store.savedMem.Metadata[memory.MetaKeyAboutKind])
	assert.Equal(t, "Name", store.savedMem.Metadata[memory.MetaKeyAboutKey])
}

// TestHandleOpenMemory_ReturnsFullContent proves GET /memories/{id}
// returns the full content even for large memories where recall
// would have returned only the title + summary + preview. This is
// what the agent calls when its working set decides it needs the
// body of a workspace document.
func TestHandleOpenMemory_ReturnsFullContent(t *testing.T) {
	store := &mockStore{
		memories: []*memory.Memory{
			{ID: "doc-1", Type: "document", Content: "long body content",
				Scope: map[string]string{memory.ScopeWorkspaceID: "ws1", memory.ScopeUserID: "u1"}},
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/doc-1?workspace=ws1&user_id=u1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp MemoryResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.NotNil(t, resp.Memory)
	assert.Equal(t, "doc-1", resp.Memory.ID)
	assert.Equal(t, "long body content", resp.Memory.Content)
}

// TestHandleUpdateMemory_AtomicSupersede proves PATCH /memories/{id}
// routes through AppendObservationToEntity, returning a SaveResult
// shaped for the agent to phrase its reply.
func TestHandleUpdateMemory_AtomicSupersede(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := UpdateMemoryRequest{
		Content:    "User's name is Phil",
		Type:       "fact",
		Confidence: 1.0,
		Scope:      map[string]string{"workspace_id": "ws1", "user_id": "u1"},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/memories/entity-x",
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var resp SaveMemoryResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, memory.SaveActionAutoSuperseded, resp.Action)
}

// TestHandleListConflicts proves GET /memories/conflicts returns
// the dedup-bypass triage rows in the documented response shape.
func TestHandleListConflicts(t *testing.T) {
	store := &mockStore{
		conflicts: []memory.ConflictedEntity{
			{EntityID: "ent-1", Kind: "fact", UserID: "u1", ActiveCount: 3},
			{EntityID: "ent-2", Kind: "preference", UserID: "u1", ActiveCount: 2},
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/conflicts?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp ConflictsResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, "ent-1", resp.Conflicts[0].EntityID)
	assert.Equal(t, 3, resp.Conflicts[0].ActiveCount)
}

// TestHandleListConflicts_RequiresWorkspace proves the workspace
// guard fires — the endpoint is admin-class and must not leak
// across tenants.
func TestHandleListConflicts_RequiresWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/conflicts", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleSupersedeMemories proves POST /memories/supersede maps
// the multi-id supersede flow to the wire response. The mock store
// returns canned superseded observation IDs so the handler test can
// assert the agent sees them.
func TestHandleSupersedeMemories(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := SupersedeRequest{
		SourceIDs:  []string{"ent-a", "ent-b", "ent-c"},
		Content:    "User's name is Phil",
		Type:       "fact",
		Confidence: 1.0,
		Scope:      map[string]string{"workspace_id": "ws1", "user_id": "u1"},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/supersede",
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp SaveMemoryResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, memory.SaveActionAutoSuperseded, resp.Action)
	assert.Len(t, resp.SupersededObservationIDs, 3)
}

// TestHandleSupersedeMemories_RequiresFields proves the validation
// guards fire when source_ids or content are missing.
func TestHandleSupersedeMemories_RequiresFields(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	cases := []struct {
		name string
		body SupersedeRequest
	}{
		{"no_sources", SupersedeRequest{
			Content: "x", Scope: map[string]string{"workspace_id": "ws1", "user_id": "u1"},
		}},
		{"no_content", SupersedeRequest{
			SourceIDs: []string{"a"},
			Scope:     map[string]string{"workspace_id": "ws1", "user_id": "u1"},
		}},
		{"no_user", SupersedeRequest{
			SourceIDs: []string{"a"}, Content: "x",
			Scope: map[string]string{"workspace_id": "ws1"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/memories/supersede", bytes.NewReader(b))
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

// TestHandleLinkMemories proves POST /relations writes a row into
// memory_relations with the requested type. Relations attach
// derived facts (preferences, notes) to anchor entities (the user
// identity) so name changes don't strand them.
func TestHandleLinkMemories(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := LinkRequest{
		SourceID:     "obs-1",
		TargetID:     "user-entity",
		RelationType: "ABOUT",
		Scope:        map[string]string{"workspace_id": "ws1", "user_id": "u1"},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/relations",
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
}

// TestHandleOpenMemory_NotFound proves the 404 path: an unknown
// entity ID produces a 404 with a meaningful body, not a 500.
func TestHandleOpenMemory_NotFound(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/missing-id?workspace=ws1&user_id=u1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// TestHandleUpdateMemory_RequiresContent proves the validation
// guard fires when the agent sends an empty content string —
// updating to nothing isn't a valid operation.
func TestHandleUpdateMemory_RequiresContent(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	body := UpdateMemoryRequest{
		Scope: map[string]string{"workspace_id": "ws1", "user_id": "u1"},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/memories/x",
		bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleLinkMemories_RequiresFields proves the source / target /
// relation_type validation fires.
func TestHandleLinkMemories_RequiresFields(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	body := LinkRequest{Scope: map[string]string{"workspace_id": "ws1", "user_id": "u1"}}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/relations",
		bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleSaveMemory_BadJSON(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories",
		strings.NewReader("not-json"))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleSaveMemory_StoreError(t *testing.T) {
	store := &mockStore{saveErr: fmt.Errorf("insert failed")}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := SaveMemoryRequest{
		Type:    "fact",
		Content: "test",
		Scope:   map[string]string{"workspace_id": "ws1", "user_id": "test-user"},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleSaveMemory_PropagatesCategoryToMetadata(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := SaveMemoryRequest{
		Type:     "fact",
		Content:  "user lives in Edinburgh",
		Scope:    map[string]string{memory.ScopeWorkspaceID: "ws1", memory.ScopeUserID: "user-1"},
		Category: "memory:location",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	require.NotNil(t, store.savedMem)
	got, _ := store.savedMem.Metadata[memory.MetaKeyConsentCategory].(string)
	assert.Equal(t, "memory:location", got, "Category from request should land in metadata")
}

func TestHandleSaveMemory_EmptyCategoryLeavesMetadataUntouched(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := SaveMemoryRequest{
		Type:    "fact",
		Content: "no category here",
		Scope:   map[string]string{memory.ScopeWorkspaceID: "ws1", memory.ScopeUserID: "user-1"},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	require.NotNil(t, store.savedMem)
	_, present := store.savedMem.Metadata[memory.MetaKeyConsentCategory]
	assert.False(t, present, "no Category in request → no consent_category in metadata")
}

func TestHandleSaveMemory_ExplicitMetadataCategoryWins(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := SaveMemoryRequest{
		Type:    "fact",
		Content: "x",
		Scope:   map[string]string{memory.ScopeWorkspaceID: "ws1", memory.ScopeUserID: "user-1"},
		Metadata: map[string]any{
			memory.MetaKeyConsentCategory: "memory:health",
		},
		Category: "memory:preferences",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	require.NotNil(t, store.savedMem)
	got := store.savedMem.Metadata[memory.MetaKeyConsentCategory].(string)
	assert.Equal(t, "memory:health", got, "explicit metadata wins over req.Category")
}

func TestHandleSaveMemory_BodyTooLarge(t *testing.T) {
	store := &mockStore{}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	h := &Handler{
		service:     svc,
		log:         logr.Discard(),
		maxBodySize: 10, // very small limit
	}
	mux := setupMux(h)

	// Use valid JSON that exceeds the limit to trigger MaxBytesError.
	body := `{"type":"preference","content":"` + strings.Repeat("x", 100) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
}

// --- Delete memory tests ---

func TestHandleDeleteMemory_Success(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories/mem-123?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleDeleteMemory_StoreError(t *testing.T) {
	store := &mockStore{delErr: fmt.Errorf("not found")}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories/mem-123?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// --- DeleteAll tests ---

func TestHandleDeleteAllMemories_Success(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories?workspace=ws1&user_id=u1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleDeleteAllMemories_MissingWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleDeleteAllMemories_StoreError(t *testing.T) {
	store := &mockStore{delAllErr: fmt.Errorf("cascade failed")}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// --- Healthz test ---

func TestHandleHealthz(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "ok", rr.Body.String())
}

// --- Helper tests ---

func TestParseTypes(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"preference", []string{"preference"}},
		{"preference,fact", []string{"preference", "fact"}},
		{" preference , fact ", []string{"preference", "fact"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseTypes(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMinMax(t *testing.T) {
	// Verify the min/max clamping pattern used in handlers.
	assert.Equal(t, 1, min(max(0, 1), 100))
	assert.Equal(t, 50, min(max(50, 1), 100))
	assert.Equal(t, 100, min(max(200, 1), 100))
}

func TestTruncateParam(t *testing.T) {
	assert.Equal(t, "abc", truncateParam("abc"))
	// String longer than maxStringParamLen gets truncated.
	long := strings.Repeat("x", maxStringParamLen+10)
	assert.Len(t, truncateParam(long), maxStringParamLen)
}

func TestBuildScope(t *testing.T) {
	q := fakeQuery(map[string]string{
		"workspace": "ws1",
		"user_id":   "u1",
		"agent":     "a1",
	})
	scope := buildScope(q)
	assert.Equal(t, "ws1", scope[memory.ScopeWorkspaceID])
	assert.Equal(t, "u1", scope[memory.ScopeUserID])
	assert.Equal(t, "a1", scope[memory.ScopeAgentID])
}

func TestBuildScope_MinimalParams(t *testing.T) {
	q := fakeQuery(map[string]string{"workspace": "ws1"})
	scope := buildScope(q)
	assert.Equal(t, "ws1", scope[memory.ScopeWorkspaceID])
	_, hasUser := scope[memory.ScopeUserID]
	assert.False(t, hasUser)
}

func TestParseIntParam_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?limit=abc", nil)
	assert.Equal(t, 20, parseIntParam(req, "limit", 20))

	req = httptest.NewRequest(http.MethodGet, "/?limit=-5", nil)
	assert.Equal(t, 20, parseIntParam(req, "limit", 20))

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Equal(t, 20, parseIntParam(req, "limit", 20))
}

func TestParseMinConfidence(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?min_confidence=abc", nil)
	assert.Equal(t, 0.0, parseMinConfidence(req))

	req = httptest.NewRequest(http.MethodGet, "/?min_confidence=-1", nil)
	assert.Equal(t, 0.0, parseMinConfidence(req))

	req = httptest.NewRequest(http.MethodGet, "/?min_confidence=0.5", nil)
	assert.Equal(t, 0.5, parseMinConfidence(req))

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Equal(t, 0.0, parseMinConfidence(req))
}

func TestWriteError_UnknownError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, fmt.Errorf("something unexpected"))
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// --- Export memories tests ---

func TestHandler_ExportMemories(t *testing.T) {
	store := &mockStore{
		memories: []*memory.Memory{
			{ID: "1", Type: "preference", Content: "likes Go"},
			{ID: "2", Type: "fact", Content: "uses Linux"},
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/export?workspace=ws1&user_id=u1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, exportFilename, rr.Header().Get("Content-Disposition"))
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var resp MemoryListResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Memories, 2)
}

func TestHandler_ExportMemories_MissingWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/export", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Contains(t, resp.Error, "workspace")
}

func TestHandler_ExportMemories_StoreError(t *testing.T) {
	store := &mockStore{exportAllErr: fmt.Errorf("db error")}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/export?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// --- BatchDeleteMemories tests ---

func TestHandleBatchDeleteMemories_Success(t *testing.T) {
	store := &mockStore{batchDeleteN: 3}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories/batch?workspace=ws1&limit=3", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp BatchDeleteResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 3, resp.Deleted)
}

func TestHandleBatchDeleteMemories_ZeroRows(t *testing.T) {
	store := &mockStore{batchDeleteN: 0}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories/batch?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp BatchDeleteResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 0, resp.Deleted)
}

func TestHandleBatchDeleteMemories_MissingWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories/batch", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Contains(t, resp.Error, "workspace")
}

func TestHandleBatchDeleteMemories_StoreError(t *testing.T) {
	store := &mockStore{batchDeleteErr: fmt.Errorf("db error")}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories/batch?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleBatchDeleteMemories_LimitClamped(t *testing.T) {
	// limit exceeding maxBatchDeleteLimit is clamped
	store := &mockStore{batchDeleteN: 5}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/memories/batch?workspace=ws1&limit=%d", maxBatchDeleteLimit+999), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// fakeQuery implements the interface used by buildScope.
type fakeQuery map[string]string

func (f fakeQuery) Get(key string) string {
	return f[key]
}
