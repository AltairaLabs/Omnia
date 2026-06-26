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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altairalabs/omnia/internal/memory"
	"github.com/stretchr/testify/assert"
)

// errBoom is a generic non-sentinel store error used to drive the
// 500 / writeError fall-through branches of the moved handlers.
var errBoom = errors.New("boom")

// relTypeAbout is the relation type used by the link-handler tests.
const relTypeAbout = "ABOUT"

// TestHandleListConflicts_StoreError exercises the FindConflicts error
// branch — a store failure surfaces as 500, not a partial result.
func TestHandleListConflicts_StoreError(t *testing.T) {
	h := newTestHandler(&mockStore{conflictsErr: errBoom})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/conflicts?workspace="+testWS, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestHandleSupersedeMemories_BadJSON proves a malformed body is a 400.
func TestHandleSupersedeMemories_BadJSON(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/supersede",
		strings.NewReader("not-json"))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleSupersedeMemories_StoreError proves a store failure surfaces
// as 500 from the supersede path.
func TestHandleSupersedeMemories_StoreError(t *testing.T) {
	h := newTestHandler(&mockStore{supersedeErr: errBoom})
	mux := setupMux(h)

	body := SupersedeRequest{
		SourceIDs: []string{"a"},
		Content:   "x",
		Scope:     map[string]string{memory.ScopeWorkspaceID: testWS, memory.ScopeUserID: testUser},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/supersede",
		bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestHandleLinkMemories_BadJSON proves a malformed body is a 400.
func TestHandleLinkMemories_BadJSON(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/relations",
		strings.NewReader("not-json"))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleLinkMemories_RequiresWorkspace proves the workspace guard
// fires when scope omits the workspace ID.
func TestHandleLinkMemories_RequiresWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	body := LinkRequest{
		SourceID:     "a",
		TargetID:     "b",
		RelationType: relTypeAbout,
		Scope:        map[string]string{},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/relations",
		bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleLinkMemories_NotFound proves a missing source/target entity
// surfaces as a 404, not a 500.
func TestHandleLinkMemories_NotFound(t *testing.T) {
	h := newTestHandler(&mockStore{linkErr: memory.ErrNotFound})
	mux := setupMux(h)

	body := LinkRequest{
		SourceID:     "a",
		TargetID:     "b",
		RelationType: relTypeAbout,
		Scope:        map[string]string{memory.ScopeWorkspaceID: testWS, memory.ScopeUserID: testUser},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/relations",
		bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// TestHandleLinkMemories_StoreError proves a generic store failure is a 500.
func TestHandleLinkMemories_StoreError(t *testing.T) {
	h := newTestHandler(&mockStore{linkErr: errBoom})
	mux := setupMux(h)

	body := LinkRequest{
		SourceID:     "a",
		TargetID:     "b",
		RelationType: relTypeAbout,
		Scope:        map[string]string{memory.ScopeWorkspaceID: testWS, memory.ScopeUserID: testUser},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/relations",
		bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestHandleUpdateMemory_BadJSON proves a malformed body is a 400.
func TestHandleUpdateMemory_BadJSON(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/memories/x",
		strings.NewReader("not-json"))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleUpdateMemory_MissingID proves an empty path id is a 400.
func TestHandleUpdateMemory_MissingID(t *testing.T) {
	h := newTestHandler(&mockStore{})

	// Call directly: the mux can't route an empty {id} segment.
	body := UpdateMemoryRequest{
		Content: "x",
		Scope:   map[string]string{memory.ScopeWorkspaceID: testWS, memory.ScopeUserID: testUser},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/memories/", bytes.NewReader(b))
	rr := httptest.NewRecorder()
	h.handleUpdateMemory(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleUpdateMemory_RequiresUser proves the user-id guard fires.
func TestHandleUpdateMemory_RequiresUser(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	body := UpdateMemoryRequest{
		Content: "x",
		Scope:   map[string]string{memory.ScopeWorkspaceID: testWS},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/memories/ent-1",
		bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleUpdateMemory_NotFound proves a missing entity is a 404.
func TestHandleUpdateMemory_NotFound(t *testing.T) {
	h := newTestHandler(&mockStore{appendErr: memory.ErrNotFound})
	mux := setupMux(h)

	body := UpdateMemoryRequest{
		Content: "x",
		Scope:   map[string]string{memory.ScopeWorkspaceID: testWS, memory.ScopeUserID: testUser},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/memories/ent-1",
		bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// TestHandleUpdateMemory_StoreError proves a generic store failure is a 500.
func TestHandleUpdateMemory_StoreError(t *testing.T) {
	h := newTestHandler(&mockStore{appendErr: errBoom})
	mux := setupMux(h)

	body := UpdateMemoryRequest{
		Content: "x",
		Scope:   map[string]string{memory.ScopeWorkspaceID: testWS, memory.ScopeUserID: testUser},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/memories/ent-1",
		bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestHandleOpenMemory_MissingWorkspace proves the workspace guard fires
// before the store is consulted.
func TestHandleOpenMemory_MissingWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/ent-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleOpenMemory_StoreError proves a generic store failure is a 500.
func TestHandleOpenMemory_StoreError(t *testing.T) {
	h := newTestHandler(&mockStore{getMemErr: errBoom})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/ent-1?workspace="+testWS+"&user_id="+testUser, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestHandleOpenMemory_MissingID proves an empty path id is a 400.
func TestHandleOpenMemory_MissingID(t *testing.T) {
	h := newTestHandler(&mockStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/", nil)
	rr := httptest.NewRecorder()
	h.handleOpenMemory(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleDeleteMemory_MissingID proves an empty path id is a 400.
func TestHandleDeleteMemory_MissingID(t *testing.T) {
	h := newTestHandler(&mockStore{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories/", nil)
	rr := httptest.NewRecorder()
	h.handleDeleteMemory(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleDeleteMemory_MissingWorkspace proves the workspace guard fires.
func TestHandleDeleteMemory_MissingWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories/ent-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
