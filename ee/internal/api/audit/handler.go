/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/ee/pkg/audit"
)

// auditQuerier abstracts the query method of audit.Logger for testability.
type auditQuerier interface {
	Query(ctx context.Context, opts audit.QueryOpts) (*audit.QueryResult, error)
}

// Handler provides HTTP endpoints for querying audit logs.
type Handler struct {
	logger auditQuerier
	log    logr.Logger
}

// NewHandler creates a new audit query handler.
func NewHandler(logger *audit.Logger, log logr.Logger) *Handler {
	return &Handler{
		logger: logger,
		log:    log.WithName("audit-handler"),
	}
}

// RegisterRoutes registers the audit API routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/audit/sessions", h.handleQuery)
}

// handleQuery returns paginated audit log entries matching the query filters.
func (h *Handler) handleQuery(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := audit.QueryOpts{
		SessionID: q.Get("sessionId"),
		UserID:    q.Get("userId"),
		Workspace: q.Get("workspace"),
		Limit:     parseIntParam(r, "limit", 50),
		Offset:    parseIntParam(r, "offset", 0),
	}

	if eventTypes := q.Get("eventTypes"); eventTypes != "" {
		opts.EventTypes = strings.Split(eventTypes, ",")
	}

	if from := q.Get("from"); from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid 'from' time format, expected RFC3339")
			return
		}
		opts.From = t
	}

	if to := q.Get("to"); to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid 'to' time format, expected RFC3339")
			return
		}
		opts.To = t
	}

	result, err := h.logger.Query(r.Context(), opts)
	if err != nil {
		h.log.Error(err, "audit query failed")
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// parseIntParam returns an integer query parameter or the default value.
func parseIntParam(r *http.Request, name string, defaultVal int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}

// errorResponse is the JSON response for errors.
type errorResponse struct {
	Error string `json:"error"`
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg})
}
