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
	"strconv"

	"github.com/go-logr/logr"
)

// ConsentUsersHandler exposes GET /api/v1/privacy/consent/users.
// It returns the set of user IDs (pseudonyms) who have a preferences row
// and whose consent for the given category matches the requested state.
//
// granted=true  → users whose consent_grants contains the category.
// granted=false → users who have a row but have NOT granted the category.
//
// Users with no preferences row at all are excluded from granted=false results
// (opt-in-by-default semantics for silent users).
type ConsentUsersHandler struct {
	store *PreferencesPostgresStore
	log   logr.Logger
}

// NewConsentUsersHandler creates a ConsentUsersHandler.
func NewConsentUsersHandler(store *PreferencesPostgresStore, log logr.Logger) *ConsentUsersHandler {
	return &ConsentUsersHandler{store: store, log: log.WithName("consent-users")}
}

// RegisterRoutes registers the consent users route on the given mux.
func (h *ConsentUsersHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/privacy/consent/users", h.handleConsentUsers)
}

// consentUsersResponse is the JSON body returned by GET /api/v1/privacy/consent/users.
type consentUsersResponse struct {
	Category string   `json:"category"`
	Granted  bool     `json:"granted"`
	UserIDs  []string `json:"userIds"`
}

func (h *ConsentUsersHandler) handleConsentUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	catStr := q.Get("category")
	if catStr == "" {
		writeStatsErr(w, http.StatusBadRequest, "category parameter is required")
		return
	}
	category := ConsentCategory(catStr)
	if _, valid := CategoryInfo(category); !valid {
		writeStatsErr(w, http.StatusBadRequest, "unknown consent category")
		return
	}

	grantedStr := q.Get("granted")
	if grantedStr == "" {
		writeStatsErr(w, http.StatusBadRequest, "granted parameter is required")
		return
	}
	granted, err := strconv.ParseBool(grantedStr)
	if err != nil {
		writeStatsErr(w, http.StatusBadRequest, "granted must be a boolean")
		return
	}

	userIDs, err := h.store.ListUsersByConsent(r.Context(), category, granted)
	if err != nil {
		h.log.Error(err, "consent users query failed")
		writeStatsErr(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(consentUsersResponse{
		Category: catStr,
		Granted:  granted,
		UserIDs:  userIDs,
	}); err != nil {
		h.log.Error(err, "consent users encode failed")
	}
}
