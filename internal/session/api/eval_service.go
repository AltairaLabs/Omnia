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

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/go-logr/logr"
)

// Sentinel errors for eval result operations.
var (
	ErrMissingEvalResults = errors.New("at least one eval result is required")
	ErrMissingEvalStore   = errors.New("eval store is not configured")
)

// EvalDefinition represents a single eval to run against session messages.
type EvalDefinition struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Trigger string         `json:"trigger"`
	Params  map[string]any `json:"params,omitempty"`
}

// EvaluateRequest is the JSON body for eval execution requests.
type EvaluateRequest struct {
	Evals []EvalDefinition `json:"evals"`
}

// EvaluateResultItem represents a single eval result.
type EvaluateResultItem struct {
	EvalID     string   `json:"evalId"`
	EvalType   string   `json:"evalType"`
	Trigger    string   `json:"trigger"`
	Passed     bool     `json:"passed"`
	Score      *float64 `json:"score,omitempty"`
	DurationMs int      `json:"durationMs"`
	Source     string   `json:"source"`
}

// EvaluateResponseSummary contains aggregate counts for an evaluation run.
type EvaluateResponseSummary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

// EvaluateResponse is the JSON response for eval execution.
type EvaluateResponse struct {
	Results []EvaluateResultItem    `json:"results"`
	Summary EvaluateResponseSummary `json:"summary"`
}

// EvalResultListResponse is the JSON response for eval result list endpoints.
type EvalResultListResponse struct {
	Results []*EvalResult `json:"results"`
	Total   int64         `json:"total"`
	HasMore bool          `json:"hasMore"`
}

// EvalResultSessionResponse is the JSON response for session eval results.
type EvalResultSessionResponse struct {
	Results []*EvalResult `json:"results"`
}

// EvalResultSummaryResponse is the JSON response for eval result summary.
type EvalResultSummaryResponse struct {
	Summaries []*EvalResultSummary `json:"summaries"`
}

// EvalService provides business logic for eval result CRUD operations.
type EvalService struct {
	store EvalStore
	log   logr.Logger
}

// NewEvalService creates a new EvalService with the given store.
func NewEvalService(store EvalStore, log logr.Logger) *EvalService {
	return &EvalService{
		store: store,
		log:   log.WithName("eval-service"),
	}
}

// CreateEvalResults persists one or more eval results.
func (s *EvalService) CreateEvalResults(ctx context.Context, results []*EvalResult) error {
	if len(results) == 0 {
		return ErrMissingEvalResults
	}
	if s.store == nil {
		return ErrMissingEvalStore
	}
	return s.store.InsertEvalResults(ctx, results)
}

// GetSessionEvalResults retrieves all eval results for a session.
func (s *EvalService) GetSessionEvalResults(ctx context.Context, sessionID string) ([]*EvalResult, error) {
	if sessionID == "" {
		return nil, ErrMissingSessionID
	}
	if s.store == nil {
		return nil, ErrMissingEvalStore
	}
	return s.store.GetSessionEvalResults(ctx, sessionID)
}

// ListEvalResults retrieves eval results matching the given filters.
func (s *EvalService) ListEvalResults(ctx context.Context, opts EvalResultListOpts) ([]*EvalResult, int64, error) {
	if s.store == nil {
		return nil, 0, ErrMissingEvalStore
	}
	return s.store.ListEvalResults(ctx, opts)
}

// GetEvalResultSummary returns aggregate statistics for eval results.
func (s *EvalService) GetEvalResultSummary(ctx context.Context, opts EvalResultSummaryOpts) ([]*EvalResultSummary, error) {
	if s.store == nil {
		return nil, ErrMissingEvalStore
	}
	return s.store.GetEvalResultSummary(ctx, opts)
}

// writeEvalError maps eval-specific errors to HTTP responses.
func writeEvalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrMissingEvalResults):
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
	case errors.Is(err, ErrMissingEvalStore):
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "eval store not configured"})
	default:
		writeError(w, err)
	}
}
