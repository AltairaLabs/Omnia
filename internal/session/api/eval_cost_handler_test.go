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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

// --- Mock cost store ---

type mockEvalCostStore struct {
	summaries []*EvalCostSummary
	err       error
}

func (m *mockEvalCostStore) GetEvalCostSummary(
	_ context.Context, _ string,
) ([]*EvalCostSummary, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.summaries, nil
}

// --- Helpers ---

func setupCostHandler(store EvalCostStore) *EvalCostHandler {
	return NewEvalCostHandler(store, logr.Discard())
}

func testCostSummaries() []*EvalCostSummary {
	return []*EvalCostSummary{
		{
			AgentName:    "agent-a",
			EvalID:       "eval-1",
			TotalCostUSD: 1.50,
			TotalTokens:  5000,
			EvalCount:    10,
		},
		{
			AgentName:    "agent-a",
			EvalID:       "eval-2",
			TotalCostUSD: 0.75,
			TotalTokens:  2500,
			EvalCount:    5,
		},
		{
			AgentName:    "agent-b",
			EvalID:       "eval-1",
			TotalCostUSD: 3.00,
			TotalTokens:  10000,
			EvalCount:    20,
		},
	}
}

// --- Handler tests ---

func TestHandleGetEvalCostSummary_OK(t *testing.T) {
	store := &mockEvalCostStore{summaries: testCostSummaries()}
	h := setupCostHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(
		http.MethodGet, "/api/v1/eval-costs/summary?namespace=prod", nil,
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp EvalCostSummaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Namespace != "prod" {
		t.Fatalf("expected namespace=prod, got %s", resp.Namespace)
	}
	if len(resp.Summaries) != 3 {
		t.Fatalf("expected 3 summaries, got %d", len(resp.Summaries))
	}
	if resp.TotalCostUSD != 5.25 {
		t.Fatalf("expected total cost=5.25, got %v", resp.TotalCostUSD)
	}
	if resp.TotalTokens != 17500 {
		t.Fatalf("expected total tokens=17500, got %d", resp.TotalTokens)
	}
}

func TestHandleGetEvalCostSummary_MissingNamespace(t *testing.T) {
	h := setupCostHandler(&mockEvalCostStore{})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(
		http.MethodGet, "/api/v1/eval-costs/summary", nil,
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleGetEvalCostSummary_NilStore(t *testing.T) {
	h := setupCostHandler(nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(
		http.MethodGet, "/api/v1/eval-costs/summary?namespace=prod", nil,
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleGetEvalCostSummary_StoreError(t *testing.T) {
	store := &mockEvalCostStore{err: errors.New("db error")}
	h := setupCostHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(
		http.MethodGet, "/api/v1/eval-costs/summary?namespace=prod", nil,
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleGetEvalCostSummary_EmptySummaries(t *testing.T) {
	store := &mockEvalCostStore{summaries: []*EvalCostSummary{}}
	h := setupCostHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(
		http.MethodGet, "/api/v1/eval-costs/summary?namespace=empty-ns", nil,
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp EvalCostSummaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Namespace != "empty-ns" {
		t.Fatalf("expected namespace=empty-ns, got %s", resp.Namespace)
	}
	if len(resp.Summaries) != 0 {
		t.Fatalf("expected 0 summaries, got %d", len(resp.Summaries))
	}
	if resp.TotalCostUSD != 0 {
		t.Fatalf("expected total cost=0, got %v", resp.TotalCostUSD)
	}
	if resp.TotalTokens != 0 {
		t.Fatalf("expected total tokens=0, got %d", resp.TotalTokens)
	}
}

func TestHandleGetEvalCostSummary_RegisterRoutes(t *testing.T) {
	store := &mockEvalCostStore{summaries: []*EvalCostSummary{}}
	h := setupCostHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Verify the route is registered by hitting it.
	req := httptest.NewRequest(
		http.MethodGet, "/api/v1/eval-costs/summary?namespace=test", nil,
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestBuildCostSummaryResponse(t *testing.T) {
	summaries := testCostSummaries()
	resp := buildCostSummaryResponse("test-ns", summaries)

	if resp.Namespace != "test-ns" {
		t.Fatalf("expected namespace=test-ns, got %s", resp.Namespace)
	}
	if resp.TotalCostUSD != 5.25 {
		t.Fatalf("expected total cost=5.25, got %v", resp.TotalCostUSD)
	}
	if resp.TotalTokens != 17500 {
		t.Fatalf("expected total tokens=17500, got %d", resp.TotalTokens)
	}
	if len(resp.Summaries) != 3 {
		t.Fatalf("expected 3 summaries, got %d", len(resp.Summaries))
	}
}

func TestBuildCostSummaryResponse_Nil(t *testing.T) {
	resp := buildCostSummaryResponse("ns", nil)

	if resp.TotalCostUSD != 0 {
		t.Fatalf("expected 0, got %v", resp.TotalCostUSD)
	}
	if resp.TotalTokens != 0 {
		t.Fatalf("expected 0, got %d", resp.TotalTokens)
	}
}

func TestWriteError_MissingNamespace(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, ErrMissingNamespace)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
