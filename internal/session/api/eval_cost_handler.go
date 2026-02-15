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
	"net/http"

	"github.com/go-logr/logr"
)

// EvalCostSummary holds aggregate cost data for a specific agent and eval.
type EvalCostSummary struct {
	AgentName    string  `json:"agentName"`
	EvalID       string  `json:"evalId"`
	TotalCostUSD float64 `json:"totalCostUsd"`
	TotalTokens  int64   `json:"totalTokens"`
	EvalCount    int64   `json:"evalCount"`
}

// EvalCostSummaryResponse is the JSON response for cost summary queries.
type EvalCostSummaryResponse struct {
	Namespace    string             `json:"namespace"`
	Summaries    []*EvalCostSummary `json:"summaries"`
	TotalCostUSD float64            `json:"totalCostUsd"`
	TotalTokens  int64              `json:"totalTokens"`
}

// EvalCostStore defines the persistence interface for eval cost queries.
type EvalCostStore interface {
	// GetEvalCostSummary returns aggregate cost data grouped by agent and eval.
	GetEvalCostSummary(ctx context.Context, namespace string) ([]*EvalCostSummary, error)
}

// EvalCostHandler provides the HTTP endpoint for eval cost summaries.
type EvalCostHandler struct {
	store EvalCostStore
	log   logr.Logger
}

// NewEvalCostHandler creates a new eval cost handler.
func NewEvalCostHandler(store EvalCostStore, log logr.Logger) *EvalCostHandler {
	return &EvalCostHandler{
		store: store,
		log:   log.WithName("eval-cost-handler"),
	}
}

// RegisterRoutes registers eval cost routes on the given mux.
func (h *EvalCostHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/eval-costs/summary", h.handleGetEvalCostSummary)
}

// handleGetEvalCostSummary handles GET /api/v1/eval-costs/summary?namespace=X.
func (h *EvalCostHandler) handleGetEvalCostSummary(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		writeError(w, ErrMissingNamespace)
		return
	}

	if h.store == nil {
		writeEvalError(w, ErrMissingEvalStore)
		return
	}

	summaries, err := h.store.GetEvalCostSummary(r.Context(), namespace)
	if err != nil {
		h.log.Error(err, "GetEvalCostSummary failed", "namespace", namespace)
		writeError(w, err)
		return
	}

	resp := buildCostSummaryResponse(namespace, summaries)
	writeJSON(w, resp)
}

// buildCostSummaryResponse aggregates totals from individual summaries.
func buildCostSummaryResponse(
	namespace string, summaries []*EvalCostSummary,
) *EvalCostSummaryResponse {
	var totalCost float64
	var totalTokens int64
	for _, s := range summaries {
		totalCost += s.TotalCostUSD
		totalTokens += s.TotalTokens
	}
	return &EvalCostSummaryResponse{
		Namespace:    namespace,
		Summaries:    summaries,
		TotalCostUSD: totalCost,
		TotalTokens:  totalTokens,
	}
}
