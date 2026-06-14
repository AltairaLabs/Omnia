/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"encoding/json"
	"errors"
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
			Kind:       "profile",
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
	assert.Equal(t, "embedding", resp.ProjectionInput)
	assert.Equal(t, 40, resp.Total)
	assert.Len(t, resp.Points, 40)
	assert.Equal(t, "profile", resp.Points[0].Type)
	assert.Equal(t, "u1", resp.Points[0].UserRef)
	for _, p := range resp.Points {
		assert.GreaterOrEqual(t, p.X, -1.0001)
		assert.LessOrEqual(t, p.X, 1.0001)
		assert.GreaterOrEqual(t, p.Y, -1.0001)
		assert.LessOrEqual(t, p.Y, 1.0001)
	}
	// Computed path persisted the layout.
	assert.Len(t, store.projSavedPoints, 40)
	assert.Equal(t, "ready", resp.Status)
}

func TestHandleProjection_LargeScopePending(t *testing.T) {
	// fingerprint count > onDemandProjectionThreshold, no stored layout.
	store := &mockStore{projFingerprint: "5000:123", projInputs: denseProjInputs(40)}
	h := newTestHandler(store)
	mux := setupMux(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/projection?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp ProjectionResult
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "pending", resp.Status)
	assert.Equal(t, 5000, resp.Total)
	assert.Empty(t, resp.Points)
	assert.Nil(t, store.projSavedPoints) // pending must NOT compute/persist
}

func TestHandleProjection_SmallScopeReady(t *testing.T) {
	store := &mockStore{projFingerprint: "40:123", projInputs: denseProjInputs(40)}
	h := newTestHandler(store)
	mux := setupMux(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/projection?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var resp ProjectionResult
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "ready", resp.Status)
	assert.Equal(t, 40, resp.Total)
	assert.Len(t, store.projSavedPoints, 40) // small scope computed+persisted
}

// TestHandleProjection_StoreError proves a store failure surfaces as a 5xx
// (covers Project's fingerprint-error branch).
func TestHandleProjection_StoreError(t *testing.T) {
	store := &mockStore{projFingerprint: "40:1", projErr: errors.New("db down")}
	h := newTestHandler(store)
	mux := setupMux(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/projection?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.GreaterOrEqual(t, rr.Code, 500)
}

// TestHandleProjection_ServesFreshStored is the cache-hit path: when a stored
// layout's fingerprint matches the live fingerprint, the endpoint serves the
// stored coordinates (refreshed metadata) WITHOUT recomputing or re-persisting.
// This is the feature's primary path — every request after the worker's render.
func TestHandleProjection_ServesFreshStored(t *testing.T) {
	inputs := denseProjInputs(40)
	layout := make(map[string][2]float64, len(inputs))
	for i, in := range inputs {
		layout[in.EntityID] = [2]float64{float64(i) * 0.01, float64(i) * -0.01}
	}
	computedAt := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	store := &mockStore{
		projFingerprint: "40:123",
		projInputs:      inputs,
		projStored: &memory.StoredProjection{
			Fingerprint: "40:123",
			Layout:      layout,
			Model:       "tsne",
			Basis:       "dense",
			ComputedAt:  computedAt,
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/projection?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp ProjectionResult
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "ready", resp.Status)
	assert.Equal(t, "dense", resp.Basis)
	assert.Len(t, resp.Points, 40)
	assert.WithinDuration(t, computedAt, resp.ComputedAt, 0) // stored timestamp, not now
	assert.Nil(t, store.projSavedPoints)                     // served path must NOT re-persist
}
