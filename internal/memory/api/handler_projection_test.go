/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/altairalabs/omnia/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const projTierUser = "user"

func denseProjInputs(n int) []memory.ProjectionInput {
	out := make([]memory.ProjectionInput, n)
	for i := 0; i < n; i++ {
		out[i] = memory.ProjectionInput{
			EntityID:   fmt.Sprintf("e%04d", i),
			Content:    "some memory content here",
			Embedding:  []float32{float32(i % 5), float32((i * 3) % 7), 1, 0, float32(i % 2)},
			Tier:       projTierUser,
			User:       "u1",
			Confidence: 0.5,
			ObservedAt: time.Unix(int64(i), 0).UTC(),
		}
	}
	return out
}

func TestHandleProjection_MissingWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/projection", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleProjection_EmptyScope(t *testing.T) {
	store := &mockStore{projFingerprint: "", projInputs: nil}
	h := newTestHandler(store)
	mux := setupMux(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/projection?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp ProjectionResult
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 0, resp.Total)
	assert.Empty(t, resp.Points)
}

func TestHandleProjection_HappyPathDense(t *testing.T) {
	store := &mockStore{projFingerprint: "fp1", projInputs: denseProjInputs(40)}
	h := newTestHandler(store)
	mux := setupMux(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/projection?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp ProjectionResult
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "dense", resp.Basis)
	assert.Equal(t, 40, resp.Total)
	assert.Len(t, resp.Points, 40)
	for _, p := range resp.Points {
		assert.GreaterOrEqual(t, p.X, -1.0001)
		assert.LessOrEqual(t, p.X, 1.0001)
		assert.GreaterOrEqual(t, p.Y, -1.0001)
		assert.LessOrEqual(t, p.Y, 1.0001)
	}
	// Computed path persisted the layout.
	assert.Len(t, store.projSavedPoints, 40)
}
