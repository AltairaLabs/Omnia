/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/ee/pkg/audit"
)

// AuditIngester is the subset of AuditStore the ingest handler depends on, so
// the handler can be unit-tested without a database.
type AuditIngester interface {
	InsertEvents(ctx context.Context, sourceService string, events []*audit.Entry) (int, error)
}

// AuditIngestHandler accepts audit events forwarded by memory-api / session-api
// and persists them in privacy-api's central audit_log (#1673).
type AuditIngestHandler struct {
	store AuditIngester
	log   logr.Logger
}

// NewAuditIngestHandler creates an AuditIngestHandler.
func NewAuditIngestHandler(store AuditIngester, log logr.Logger) *AuditIngestHandler {
	return &AuditIngestHandler{store: store, log: log.WithName("audit-ingest")}
}

// RegisterRoutes registers the audit-ingest route on the given mux.
func (h *AuditIngestHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/privacy/audit-events", h.handleIngest)
}

// AuditIngestRequest is the JSON body for POST /api/v1/privacy/audit-events.
// A batch carries events from a single source service.
type AuditIngestRequest struct {
	SourceService string         `json:"sourceService"`
	Events        []*audit.Entry `json:"events"`
}

// AuditIngestResponse reports how many events were newly stored versus skipped
// as already-seen duplicates (at-least-once delivery).
type AuditIngestResponse struct {
	Ingested   int `json:"ingested"`
	Duplicates int `json:"duplicates"`
}

func (h *AuditIngestHandler) handleIngest(w http.ResponseWriter, r *http.Request) {
	var req AuditIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStatsErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.SourceService == "" {
		writeStatsErr(w, http.StatusBadRequest, "sourceService is required")
		return
	}

	ingested, err := h.store.InsertEvents(r.Context(), req.SourceService, req.Events)
	if err != nil {
		h.log.Error(err, "audit ingest failed", "sourceService", req.SourceService, "eventCount", len(req.Events))
		writeStatsErr(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	resp := AuditIngestResponse{Ingested: ingested, Duplicates: len(req.Events) - ingested}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error(err, "audit ingest encode failed")
	}
}
