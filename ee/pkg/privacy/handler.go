/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-logr/logr"
)

// OptOutHandler provides HTTP endpoints for user privacy opt-out management.
type OptOutHandler struct {
	store PreferencesStore
	log   logr.Logger
}

// NewOptOutHandler creates a new OptOutHandler.
func NewOptOutHandler(store PreferencesStore, log logr.Logger) *OptOutHandler {
	return &OptOutHandler{
		store: store,
		log:   log.WithName("optout-handler"),
	}
}

// RegisterRoutes registers the opt-out API routes on the given mux.
func (h *OptOutHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/privacy/opt-out", h.handleSetOptOut)
	mux.HandleFunc("DELETE /api/v1/privacy/opt-out", h.handleRemoveOptOut)
	mux.HandleFunc("GET /api/v1/privacy/preferences/{userID}", h.handleGetPreferences)
}

// OptOutRequest is the JSON body for opt-out operations.
type OptOutRequest struct {
	UserID string `json:"userId"`
	Scope  string `json:"scope"`
	Target string `json:"target,omitempty"`
}

// handleSetOptOut sets an opt-out preference for a user.
func (h *OptOutHandler) handleSetOptOut(w http.ResponseWriter, r *http.Request) {
	var req OptOutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateOptOutRequest(req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.SetOptOut(r.Context(), req.UserID, req.Scope, req.Target); err != nil {
		h.log.Error(err, "SetOptOut failed", "userID", req.UserID)
		writeErr(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveOptOut removes an opt-out preference for a user.
func (h *OptOutHandler) handleRemoveOptOut(w http.ResponseWriter, r *http.Request) {
	var req OptOutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateOptOutRequest(req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.RemoveOptOut(r.Context(), req.UserID, req.Scope, req.Target); err != nil {
		if errors.Is(err, ErrPreferencesNotFound) {
			writeErr(w, http.StatusNotFound, "user preferences not found")
			return
		}
		h.log.Error(err, "RemoveOptOut failed", "userID", req.UserID)
		writeErr(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleGetPreferences returns a user's privacy preferences.
func (h *OptOutHandler) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if userID == "" {
		writeErr(w, http.StatusBadRequest, "user ID is required")
		return
	}

	prefs, err := h.store.GetPreferences(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrPreferencesNotFound) {
			writeErr(w, http.StatusNotFound, "user preferences not found")
			return
		}
		h.log.Error(err, "GetPreferences failed", "userID", userID)
		writeErr(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(prefs)
}

// errResponse is the JSON error response body.
type errResponse struct {
	Error string `json:"error"`
}

// writeErr writes a JSON error response.
func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errResponse{Error: msg})
}

// validateOptOutRequest validates the opt-out request fields.
func validateOptOutRequest(req OptOutRequest) error {
	if req.UserID == "" {
		return errors.New("userId is required")
	}
	switch req.Scope {
	case ScopeAll:
		return nil
	case ScopeWorkspace, ScopeAgent:
		if req.Target == "" {
			return errors.New("target is required for workspace and agent scopes")
		}
		return nil
	default:
		return errors.New("scope must be one of: all, workspace, agent")
	}
}
