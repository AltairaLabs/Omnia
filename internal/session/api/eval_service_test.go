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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

// mockEvalStore is a test double for EvalStore.
type mockEvalStore struct {
	insertErr      error
	getResults     []*EvalResult
	getErr         error
	listResults    []*EvalResult
	listTotal      int64
	listErr        error
	summaryResults []*EvalResultSummary
	summaryErr     error
}

func (m *mockEvalStore) InsertEvalResults(_ context.Context, _ []*EvalResult) error {
	return m.insertErr
}

func (m *mockEvalStore) GetSessionEvalResults(_ context.Context, _ string) ([]*EvalResult, error) {
	return m.getResults, m.getErr
}

func (m *mockEvalStore) ListEvalResults(_ context.Context, _ EvalResultListOpts) ([]*EvalResult, int64, error) {
	return m.listResults, m.listTotal, m.listErr
}

func (m *mockEvalStore) GetEvalResultSummary(_ context.Context, _ EvalResultSummaryOpts) ([]*EvalResultSummary, error) {
	return m.summaryResults, m.summaryErr
}

func TestNewEvalService(t *testing.T) {
	store := &mockEvalStore{}
	svc := NewEvalService(store, logr.Discard())
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestCreateEvalResults_Success(t *testing.T) {
	store := &mockEvalStore{}
	svc := NewEvalService(store, logr.Discard())
	err := svc.CreateEvalResults(context.Background(), []*EvalResult{{ID: "r1"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateEvalResults_Empty(t *testing.T) {
	svc := NewEvalService(&mockEvalStore{}, logr.Discard())
	err := svc.CreateEvalResults(context.Background(), nil)
	if !errors.Is(err, ErrMissingEvalResults) {
		t.Fatalf("expected ErrMissingEvalResults, got %v", err)
	}
}

func TestCreateEvalResults_NilStore(t *testing.T) {
	svc := NewEvalService(nil, logr.Discard())
	err := svc.CreateEvalResults(context.Background(), []*EvalResult{{ID: "r1"}})
	if !errors.Is(err, ErrMissingEvalStore) {
		t.Fatalf("expected ErrMissingEvalStore, got %v", err)
	}
}

func TestGetSessionEvalResults_Success(t *testing.T) {
	store := &mockEvalStore{getResults: []*EvalResult{{ID: "r1"}}}
	svc := NewEvalService(store, logr.Discard())
	results, err := svc.GetSessionEvalResults(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestGetSessionEvalResults_EmptySessionID(t *testing.T) {
	svc := NewEvalService(&mockEvalStore{}, logr.Discard())
	_, err := svc.GetSessionEvalResults(context.Background(), "")
	if !errors.Is(err, ErrMissingSessionID) {
		t.Fatalf("expected ErrMissingSessionID, got %v", err)
	}
}

func TestGetSessionEvalResults_NilStore(t *testing.T) {
	svc := NewEvalService(nil, logr.Discard())
	_, err := svc.GetSessionEvalResults(context.Background(), "sess-1")
	if !errors.Is(err, ErrMissingEvalStore) {
		t.Fatalf("expected ErrMissingEvalStore, got %v", err)
	}
}

func TestListEvalResults_Success(t *testing.T) {
	store := &mockEvalStore{listResults: []*EvalResult{{ID: "r1"}}, listTotal: 1}
	svc := NewEvalService(store, logr.Discard())
	results, total, err := svc.ListEvalResults(context.Background(), EvalResultListOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || total != 1 {
		t.Fatalf("expected 1 result with total 1, got %d/%d", len(results), total)
	}
}

func TestListEvalResults_NilStore(t *testing.T) {
	svc := NewEvalService(nil, logr.Discard())
	_, _, err := svc.ListEvalResults(context.Background(), EvalResultListOpts{})
	if !errors.Is(err, ErrMissingEvalStore) {
		t.Fatalf("expected ErrMissingEvalStore, got %v", err)
	}
}

func TestGetEvalResultSummary_Success(t *testing.T) {
	store := &mockEvalStore{summaryResults: []*EvalResultSummary{{EvalID: "e1"}}}
	svc := NewEvalService(store, logr.Discard())
	results, err := svc.GetEvalResultSummary(context.Background(), EvalResultSummaryOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(results))
	}
}

func TestGetEvalResultSummary_NilStore(t *testing.T) {
	svc := NewEvalService(nil, logr.Discard())
	_, err := svc.GetEvalResultSummary(context.Background(), EvalResultSummaryOpts{})
	if !errors.Is(err, ErrMissingEvalStore) {
		t.Fatalf("expected ErrMissingEvalStore, got %v", err)
	}
}

func TestWriteEvalError_MissingResults(t *testing.T) {
	w := httptest.NewRecorder()
	writeEvalError(w, ErrMissingEvalResults)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWriteEvalError_MissingStore(t *testing.T) {
	w := httptest.NewRecorder()
	writeEvalError(w, ErrMissingEvalStore)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestWriteEvalError_UnknownError(t *testing.T) {
	w := httptest.NewRecorder()
	writeEvalError(w, errors.New("some error"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
