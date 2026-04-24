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

// ConsentStatsHandler exposes GET /api/v1/privacy/consent/stats — a
// workspace-scoped aggregate over user_privacy_preferences. The
// workspace param is reserved for future per-workspace scoping; today
// it's required for shape parity with other endpoints but ignored
// (preferences table is user-keyed, not workspace-keyed).
type ConsentStatsHandler struct {
	store *PreferencesPostgresStore
	log   logr.Logger
}

// NewConsentStatsHandler creates a ConsentStatsHandler.
func NewConsentStatsHandler(store *PreferencesPostgresStore, log logr.Logger) *ConsentStatsHandler {
	return &ConsentStatsHandler{store: store, log: log.WithName("consent-stats")}
}

// RegisterRoutes registers the consent stats route on the given mux.
func (h *ConsentStatsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/privacy/consent/stats", h.handleConsentStats)
}

func (h *ConsentStatsHandler) handleConsentStats(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("workspace") == "" {
		writeStatsErr(w, http.StatusBadRequest, "workspace parameter is required")
		return
	}
	stats, err := h.store.Stats(r.Context())
	if err != nil {
		h.log.Error(err, "consent stats query failed")
		writeStatsErr(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		h.log.Error(err, "consent stats encode failed")
	}
}

func writeStatsErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
