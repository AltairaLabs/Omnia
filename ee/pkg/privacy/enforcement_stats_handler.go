/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"encoding/json"
	"net/http"

	"github.com/go-logr/logr"
)

// EnforcementStatsHandler exposes GET /api/v1/privacy/enforcement-stats — a
// workspace-scoped aggregate over audit_log enforcement events (PII redaction
// and opt-out write blocks) for the dashboard.
type EnforcementStatsHandler struct {
	store *PreferencesPostgresStore
	log   logr.Logger
}

// NewEnforcementStatsHandler creates an EnforcementStatsHandler.
func NewEnforcementStatsHandler(store *PreferencesPostgresStore, log logr.Logger) *EnforcementStatsHandler {
	return &EnforcementStatsHandler{store: store, log: log.WithName("enforcement-stats")}
}

// RegisterRoutes registers the enforcement stats route on the given mux.
func (h *EnforcementStatsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/privacy/enforcement-stats", h.handleEnforcementStats)
}

func (h *EnforcementStatsHandler) handleEnforcementStats(w http.ResponseWriter, r *http.Request) {
	workspace := r.URL.Query().Get("workspace")
	if workspace == "" {
		writeStatsErr(w, http.StatusBadRequest, "workspace parameter is required")
		return
	}
	stats, err := h.store.EnforcementStats(r.Context(), workspace)
	if err != nil {
		h.log.Error(err, "enforcement stats query failed")
		writeStatsErr(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		h.log.Error(err, "enforcement stats encode failed")
	}
}
