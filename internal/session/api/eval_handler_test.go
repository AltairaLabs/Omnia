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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// --- Mock eval store ---

type mockEvalStore struct {
	results   []*EvalResult
	summaries []*EvalResultSummary
	total     int64
	insertErr error
	listErr   error
	getErr    error
	summErr   error
	inserted  []*EvalResult
}

func (m *mockEvalStore) InsertEvalResults(_ context.Context, results []*EvalResult) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.inserted = append(m.inserted, results...)
	return nil
}

func (m *mockEvalStore) GetSessionEvalResults(_ context.Context, _ string) ([]*EvalResult, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.results, nil
}

func (m *mockEvalStore) ListEvalResults(_ context.Context, _ EvalResultListOpts) ([]*EvalResult, int64, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	return m.results, m.total, nil
}

func (m *mockEvalStore) GetEvalResultSummary(_ context.Context, _ EvalResultSummaryOpts) ([]*EvalResultSummary, error) {
	if m.summErr != nil {
		return nil, m.summErr
	}
	return m.summaries, nil
}

// --- Helpers ---

func testEvalResult() *EvalResult {
	score := 0.95
	dur := 150
	return &EvalResult{
		ID:             "er-1",
		SessionID:      "s1",
		AgentName:      "test-agent",
		Namespace:      "default",
		PromptPackName: "pack-1",
		EvalID:         "eval-1",
		EvalType:       "llm-judge",
		Trigger:        "on-message",
		Passed:         true,
		Score:          &score,
		DurationMs:     &dur,
		Source:         "worker",
		CreatedAt:      time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	}
}

func setupEvalHandler(store EvalStore) *EvalHandler {
	svc := NewEvalService(store, logr.Discard())
	return NewEvalHandler(svc, logr.Discard())
}

// --- Service tests ---

func TestEvalService_CreateEvalResults_Empty(t *testing.T) {
	svc := NewEvalService(&mockEvalStore{}, logr.Discard())
	err := svc.CreateEvalResults(context.Background(), []*EvalResult{})
	if !errors.Is(err, ErrMissingEvalResults) {
		t.Fatalf("expected ErrMissingEvalResults, got %v", err)
	}
}

func TestEvalService_CreateEvalResults_NilStore(t *testing.T) {
	svc := NewEvalService(nil, logr.Discard())
	err := svc.CreateEvalResults(context.Background(), []*EvalResult{testEvalResult()})
	if !errors.Is(err, ErrMissingEvalStore) {
		t.Fatalf("expected ErrMissingEvalStore, got %v", err)
	}
}

func TestEvalService_CreateEvalResults_OK(t *testing.T) {
	store := &mockEvalStore{}
	svc := NewEvalService(store, logr.Discard())
	err := svc.CreateEvalResults(context.Background(), []*EvalResult{testEvalResult()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 inserted, got %d", len(store.inserted))
	}
}

func TestEvalService_GetSessionEvalResults_EmptyID(t *testing.T) {
	svc := NewEvalService(&mockEvalStore{}, logr.Discard())
	_, err := svc.GetSessionEvalResults(context.Background(), "")
	if !errors.Is(err, ErrMissingSessionID) {
		t.Fatalf("expected ErrMissingSessionID, got %v", err)
	}
}

func TestEvalService_GetSessionEvalResults_NilStore(t *testing.T) {
	svc := NewEvalService(nil, logr.Discard())
	_, err := svc.GetSessionEvalResults(context.Background(), "s1")
	if !errors.Is(err, ErrMissingEvalStore) {
		t.Fatalf("expected ErrMissingEvalStore, got %v", err)
	}
}

func TestEvalService_GetSessionEvalResults_OK(t *testing.T) {
	store := &mockEvalStore{results: []*EvalResult{testEvalResult()}}
	svc := NewEvalService(store, logr.Discard())
	results, err := svc.GetSessionEvalResults(context.Background(), "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestEvalService_ListEvalResults_NilStore(t *testing.T) {
	svc := NewEvalService(nil, logr.Discard())
	_, _, err := svc.ListEvalResults(context.Background(), EvalResultListOpts{})
	if !errors.Is(err, ErrMissingEvalStore) {
		t.Fatalf("expected ErrMissingEvalStore, got %v", err)
	}
}

func TestEvalService_ListEvalResults_OK(t *testing.T) {
	store := &mockEvalStore{results: []*EvalResult{testEvalResult()}, total: 1}
	svc := NewEvalService(store, logr.Discard())
	results, total, err := svc.ListEvalResults(context.Background(), EvalResultListOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestEvalService_GetEvalResultSummary_NilStore(t *testing.T) {
	svc := NewEvalService(nil, logr.Discard())
	_, err := svc.GetEvalResultSummary(context.Background(), EvalResultSummaryOpts{})
	if !errors.Is(err, ErrMissingEvalStore) {
		t.Fatalf("expected ErrMissingEvalStore, got %v", err)
	}
}

func TestEvalService_GetEvalResultSummary_OK(t *testing.T) {
	store := &mockEvalStore{summaries: []*EvalResultSummary{
		{EvalID: "eval-1", EvalType: "llm-judge", Total: 10, Passed: 8, Failed: 2, PassRate: 0.8},
	}}
	svc := NewEvalService(store, logr.Discard())
	summaries, err := svc.GetEvalResultSummary(context.Background(), EvalResultSummaryOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].PassRate != 0.8 {
		t.Fatalf("expected pass rate 0.8, got %f", summaries[0].PassRate)
	}
}

// --- Handler tests ---

func TestHandleCreateEvalResults_OK(t *testing.T) {
	store := &mockEvalStore{}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal([]*EvalResult{testEvalResult()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval-results", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 inserted, got %d", len(store.inserted))
	}
}

func TestHandleCreateEvalResults_NoBody(t *testing.T) {
	h := setupEvalHandler(&mockEvalStore{})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval-results", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleCreateEvalResults_EmptyArray(t *testing.T) {
	h := setupEvalHandler(&mockEvalStore{})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval-results", bytes.NewBufferString("[]"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleCreateEvalResults_StoreError(t *testing.T) {
	store := &mockEvalStore{insertErr: errors.New("db error")}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal([]*EvalResult{testEvalResult()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval-results", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleGetSessionEvalResults_OK(t *testing.T) {
	store := &mockEvalStore{results: []*EvalResult{testEvalResult()}}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/s1/eval-results", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp EvalResultSessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
}

func TestHandleGetSessionEvalResults_StoreError(t *testing.T) {
	store := &mockEvalStore{getErr: errors.New("db error")}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/s1/eval-results", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleListEvalResults_OK(t *testing.T) {
	store := &mockEvalStore{
		results: []*EvalResult{testEvalResult()},
		total:   1,
	}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results?agent_name=test-agent&limit=10", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp EvalResultListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("expected total 1, got %d", resp.Total)
	}
	if resp.HasMore {
		t.Fatal("expected hasMore=false")
	}
}

func TestHandleListEvalResults_HasMore(t *testing.T) {
	store := &mockEvalStore{
		results: []*EvalResult{testEvalResult()},
		total:   50,
	}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp EvalResultListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.HasMore {
		t.Fatal("expected hasMore=true")
	}
}

func TestHandleListEvalResults_WithAllFilters(t *testing.T) {
	store := &mockEvalStore{results: []*EvalResult{}, total: 0}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	url := "/api/v1/eval-results?agent_name=a&namespace=ns&eval_id=e1&passed=true" +
		"&created_after=2026-01-01T00:00:00Z&created_before=2026-02-01T00:00:00Z&limit=5&offset=10"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleListEvalResults_PassedFalse(t *testing.T) {
	store := &mockEvalStore{results: []*EvalResult{}, total: 0}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results?passed=false", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleListEvalResults_InvalidTime(t *testing.T) {
	h := setupEvalHandler(&mockEvalStore{})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results?created_after=bad-time", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleListEvalResults_InvalidCreatedBefore(t *testing.T) {
	h := setupEvalHandler(&mockEvalStore{})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results?created_before=bad-time", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleListEvalResults_StoreError(t *testing.T) {
	store := &mockEvalStore{listErr: errors.New("db error")}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleGetEvalResultSummary_OK(t *testing.T) {
	store := &mockEvalStore{summaries: []*EvalResultSummary{
		{EvalID: "eval-1", EvalType: "llm-judge", Total: 10, Passed: 8, Failed: 2, PassRate: 0.8},
	}}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results/summary?agent_name=a&namespace=ns", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp EvalResultSummaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(resp.Summaries))
	}
}

func TestHandleGetEvalResultSummary_WithTimeFilters(t *testing.T) {
	store := &mockEvalStore{summaries: []*EvalResultSummary{}}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	url := "/api/v1/eval-results/summary?created_after=2026-01-01T00:00:00Z&created_before=2026-02-01T00:00:00Z"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleGetEvalResultSummary_InvalidCreatedAfter(t *testing.T) {
	h := setupEvalHandler(&mockEvalStore{})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results/summary?created_after=bad", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleGetEvalResultSummary_InvalidCreatedBefore(t *testing.T) {
	h := setupEvalHandler(&mockEvalStore{})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results/summary?created_before=bad", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleGetEvalResultSummary_StoreError(t *testing.T) {
	store := &mockEvalStore{summErr: errors.New("db error")}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results/summary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestWriteEvalError_MissingEvalResults(t *testing.T) {
	rec := httptest.NewRecorder()
	writeEvalError(rec, ErrMissingEvalResults)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestWriteEvalError_MissingEvalStore(t *testing.T) {
	rec := httptest.NewRecorder()
	writeEvalError(rec, ErrMissingEvalStore)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestWriteEvalError_UnknownError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeEvalError(rec, errors.New("unknown"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleCreateEvalResults_NilStore(t *testing.T) {
	svc := NewEvalService(nil, logr.Discard())
	h := NewEvalHandler(svc, logr.Discard())

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal([]*EvalResult{testEvalResult()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval-results", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

// TestParseEvalListParams tests the parsing of eval list query parameters.
func TestParseEvalListParams(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name: "all params",
			url:  "/api/v1/eval-results?agent_name=a&namespace=ns&eval_id=e1&passed=true&created_after=2026-01-01T00:00:00Z&created_before=2026-02-01T00:00:00Z&limit=10&offset=5",
		},
		{
			name: "defaults only",
			url:  "/api/v1/eval-results",
		},
		{
			name:    "invalid created_after",
			url:     "/api/v1/eval-results?created_after=bad",
			wantErr: true,
		},
		{
			name:    "invalid created_before",
			url:     "/api/v1/eval-results?created_before=bad",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			_, err := parseEvalListParams(req)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseEvalListParams_PassedValues(t *testing.T) {
	// Test passed=true
	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results?passed=true", nil)
	opts, err := parseEvalListParams(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Passed == nil || *opts.Passed != true {
		t.Fatal("expected passed=true")
	}

	// Test passed=false
	req = httptest.NewRequest(http.MethodGet, "/api/v1/eval-results?passed=false", nil)
	opts, err = parseEvalListParams(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Passed == nil || *opts.Passed != false {
		t.Fatal("expected passed=false")
	}

	// Test passed not set
	req = httptest.NewRequest(http.MethodGet, "/api/v1/eval-results", nil)
	opts, err = parseEvalListParams(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Passed != nil {
		t.Fatal("expected passed=nil when not set")
	}
}

func TestParseEvalSummaryParams(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name: "all params",
			url:  "/api/v1/eval-results/summary?agent_name=a&namespace=ns&created_after=2026-01-01T00:00:00Z&created_before=2026-02-01T00:00:00Z",
		},
		{
			name: "defaults only",
			url:  "/api/v1/eval-results/summary",
		},
		{
			name:    "invalid created_after",
			url:     "/api/v1/eval-results/summary?created_after=bad",
			wantErr: true,
		},
		{
			name:    "invalid created_before",
			url:     "/api/v1/eval-results/summary?created_before=bad",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			_, err := parseEvalSummaryParams(req)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEvalHandler_RegisterRoutes(t *testing.T) {
	store := &mockEvalStore{
		results:   []*EvalResult{testEvalResult()},
		total:     1,
		summaries: []*EvalResultSummary{},
	}
	h := setupEvalHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodGet, "/api/v1/eval-results", "", http.StatusOK},
		{http.MethodGet, "/api/v1/eval-results/summary", "", http.StatusOK},
		{http.MethodGet, "/api/v1/sessions/s1/eval-results", "", http.StatusOK},
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			var req *http.Request
			if rt.body != "" {
				req = httptest.NewRequest(rt.method, rt.path, bytes.NewBufferString(rt.body))
			} else {
				req = httptest.NewRequest(rt.method, rt.path, nil)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != rt.want {
				t.Fatalf("expected %d, got %d", rt.want, rec.Code)
			}
		})
	}
}

func TestEvalService_CreateEvalResults_StoreError(t *testing.T) {
	store := &mockEvalStore{insertErr: errors.New("db down")}
	svc := NewEvalService(store, logr.Discard())
	err := svc.CreateEvalResults(context.Background(), []*EvalResult{testEvalResult()})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEvalService_GetSessionEvalResults_StoreError(t *testing.T) {
	store := &mockEvalStore{getErr: errors.New("db down")}
	svc := NewEvalService(store, logr.Discard())
	_, err := svc.GetSessionEvalResults(context.Background(), "s1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEvalService_ListEvalResults_StoreError(t *testing.T) {
	store := &mockEvalStore{listErr: errors.New("db down")}
	svc := NewEvalService(store, logr.Discard())
	_, _, err := svc.ListEvalResults(context.Background(), EvalResultListOpts{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEvalService_GetEvalResultSummary_StoreError(t *testing.T) {
	store := &mockEvalStore{summErr: errors.New("db down")}
	svc := NewEvalService(store, logr.Discard())
	_, err := svc.GetEvalResultSummary(context.Background(), EvalResultSummaryOpts{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Mock message fetcher ---

type mockMessageFetcher struct {
	messages []*session.Message
	err      error
}

func (m *mockMessageFetcher) GetMessages(_ context.Context, _ string, _ providers.MessageQueryOpts) ([]*session.Message, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.messages, nil
}

func testEvalMessages() []*session.Message {
	return []*session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "hello"},
		{ID: "m2", Role: session.RoleAssistant, Content: "Hello! How can I help?"},
		{ID: "m3", Role: session.RoleUser, Content: "tell me a joke"},
		{ID: "m4", Role: session.RoleAssistant, Content: "Why did the chicken cross the road?"},
	}
}

func setupEvalHandlerWithFetcher(store EvalStore, fetcher MessageFetcher) *EvalHandler {
	svc := NewEvalService(store, logr.Discard())
	svc.SetMessageFetcher(fetcher)
	return NewEvalHandler(svc, logr.Discard())
}

// --- Evaluate session handler tests ---

func TestHandleEvaluateSession_OK(t *testing.T) {
	store := &mockEvalStore{}
	fetcher := &mockMessageFetcher{messages: testEvalMessages()}
	h := setupEvalHandlerWithFetcher(store, fetcher)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	reqBody := EvaluateRequest{
		Evals: []EvalDefinition{
			{ID: "check-greeting", Type: "contains", Trigger: "per_turn", Params: map[string]any{"value": "Hello"}},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/s1/evaluate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp EvaluateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].EvalID != "check-greeting" {
		t.Fatalf("expected evalId=check-greeting, got %s", resp.Results[0].EvalID)
	}
	if resp.Results[0].Source != "manual" {
		t.Fatalf("expected source=manual, got %s", resp.Results[0].Source)
	}
	if resp.Summary.Total != 1 {
		t.Fatalf("expected total=1, got %d", resp.Summary.Total)
	}
	// Verify results were stored
	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 stored result, got %d", len(store.inserted))
	}
}

func TestHandleEvaluateSession_MultipleEvals(t *testing.T) {
	store := &mockEvalStore{}
	fetcher := &mockMessageFetcher{messages: testEvalMessages()}
	h := setupEvalHandlerWithFetcher(store, fetcher)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	reqBody := EvaluateRequest{
		Evals: []EvalDefinition{
			{ID: "check-greeting", Type: "contains", Params: map[string]any{"value": "Hello"}},
			{ID: "no-errors", Type: "not_contains", Params: map[string]any{"value": "ERROR"}},
			{ID: "max-len", Type: "max_length", Params: map[string]any{"maxLength": float64(100)}},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/s1/evaluate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp EvaluateResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Summary.Total != 3 {
		t.Fatalf("expected total=3, got %d", resp.Summary.Total)
	}
}

func TestHandleEvaluateSession_NoBody(t *testing.T) {
	h := setupEvalHandlerWithFetcher(&mockEvalStore{}, &mockMessageFetcher{})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/s1/evaluate", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleEvaluateSession_EmptyEvals(t *testing.T) {
	fetcher := &mockMessageFetcher{messages: testEvalMessages()}
	h := setupEvalHandlerWithFetcher(&mockEvalStore{}, fetcher)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	reqBody := EvaluateRequest{Evals: []EvalDefinition{}}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/s1/evaluate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleEvaluateSession_SessionNotFound(t *testing.T) {
	fetcher := &mockMessageFetcher{err: session.ErrSessionNotFound}
	h := setupEvalHandlerWithFetcher(&mockEvalStore{}, fetcher)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	reqBody := EvaluateRequest{
		Evals: []EvalDefinition{
			{ID: "e1", Type: "contains", Params: map[string]any{"value": "hi"}},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/s1/evaluate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleEvaluateSession_EmptyMessages(t *testing.T) {
	fetcher := &mockMessageFetcher{messages: []*session.Message{}}
	h := setupEvalHandlerWithFetcher(&mockEvalStore{}, fetcher)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	reqBody := EvaluateRequest{
		Evals: []EvalDefinition{
			{ID: "e1", Type: "contains", Params: map[string]any{"value": "hi"}},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/s1/evaluate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleEvaluateSession_InvalidEvalType(t *testing.T) {
	fetcher := &mockMessageFetcher{messages: testEvalMessages()}
	h := setupEvalHandlerWithFetcher(&mockEvalStore{}, fetcher)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	reqBody := EvaluateRequest{
		Evals: []EvalDefinition{
			{ID: "e1", Type: "unknown_type", Params: map[string]any{}},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/s1/evaluate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// Invalid eval types are skipped (logged) rather than failing the whole request.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp EvaluateResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Summary.Total != 0 {
		t.Fatalf("expected total=0 (invalid skipped), got %d", resp.Summary.Total)
	}
}

func TestHandleEvaluateSession_StoreInsertError(t *testing.T) {
	// Store error on insert should not fail the response -- results are stored best-effort.
	store := &mockEvalStore{insertErr: errors.New("db error")}
	fetcher := &mockMessageFetcher{messages: testEvalMessages()}
	h := setupEvalHandlerWithFetcher(store, fetcher)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	reqBody := EvaluateRequest{
		Evals: []EvalDefinition{
			{ID: "e1", Type: "contains", Params: map[string]any{"value": "Hello"}},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/s1/evaluate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even with store error, got %d", rec.Code)
	}
}

// --- EvaluateSession service tests ---

func TestEvalService_EvaluateSession_EmptySessionID(t *testing.T) {
	svc := NewEvalService(&mockEvalStore{}, logr.Discard())
	svc.SetMessageFetcher(&mockMessageFetcher{messages: testEvalMessages()})
	_, err := svc.EvaluateSession(context.Background(), "", []EvalDefinition{{ID: "e1", Type: "contains"}})
	if !errors.Is(err, ErrMissingSessionID) {
		t.Fatalf("expected ErrMissingSessionID, got %v", err)
	}
}

func TestEvalService_EvaluateSession_EmptyEvals(t *testing.T) {
	svc := NewEvalService(&mockEvalStore{}, logr.Discard())
	svc.SetMessageFetcher(&mockMessageFetcher{messages: testEvalMessages()})
	_, err := svc.EvaluateSession(context.Background(), "s1", nil)
	if !errors.Is(err, ErrMissingEvalDefinition) {
		t.Fatalf("expected ErrMissingEvalDefinition, got %v", err)
	}
}

func TestEvalService_EvaluateSession_NoMessageFetcher(t *testing.T) {
	svc := NewEvalService(&mockEvalStore{}, logr.Discard())
	_, err := svc.EvaluateSession(context.Background(), "s1", []EvalDefinition{{ID: "e1", Type: "contains"}})
	if err == nil {
		t.Fatal("expected error when no message fetcher configured")
	}
}

func TestEvalService_EvaluateSession_FetchError(t *testing.T) {
	svc := NewEvalService(&mockEvalStore{}, logr.Discard())
	svc.SetMessageFetcher(&mockMessageFetcher{err: errors.New("db error")})
	_, err := svc.EvaluateSession(context.Background(), "s1", []EvalDefinition{{ID: "e1", Type: "contains"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEvalService_EvaluateSession_NoMessages(t *testing.T) {
	svc := NewEvalService(&mockEvalStore{}, logr.Discard())
	svc.SetMessageFetcher(&mockMessageFetcher{messages: []*session.Message{}})
	_, err := svc.EvaluateSession(context.Background(), "s1", []EvalDefinition{{ID: "e1", Type: "contains"}})
	if !errors.Is(err, ErrNoMessages) {
		t.Fatalf("expected ErrNoMessages, got %v", err)
	}
}

func TestEvalService_EvaluateSession_NilStore(t *testing.T) {
	// With nil store, evals should still run -- results just won't be stored.
	svc := NewEvalService(nil, logr.Discard())
	svc.SetMessageFetcher(&mockMessageFetcher{messages: testEvalMessages()})
	resp, err := svc.EvaluateSession(context.Background(), "s1", []EvalDefinition{
		{ID: "e1", Type: "contains", Params: map[string]any{"value": "Hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Summary.Total != 1 {
		t.Fatalf("expected total=1, got %d", resp.Summary.Total)
	}
}

func TestWriteEvalError_MissingEvalDefinition(t *testing.T) {
	rec := httptest.NewRecorder()
	writeEvalError(rec, ErrMissingEvalDefinition)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestWriteEvalError_NoMessages(t *testing.T) {
	rec := httptest.NewRecorder()
	writeEvalError(rec, ErrNoMessages)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
}
