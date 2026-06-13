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

	"github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/pkg/logging"
)

// ConsentHandler provides HTTP endpoints for managing consent grants.
type ConsentHandler struct {
	store *PreferencesPostgresStore
	audit api.AuditLogger
	log   logr.Logger
}

// NewConsentHandler creates a new ConsentHandler.
func NewConsentHandler(store *PreferencesPostgresStore, audit api.AuditLogger, log logr.Logger) *ConsentHandler {
	return &ConsentHandler{
		store: store,
		audit: audit,
		log:   log.WithName("consent-handler"),
	}
}

// RegisterRoutes registers consent API routes on the given mux.
func (h *ConsentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("PUT /api/v1/privacy/preferences/{userID}/consent", h.handleSetConsent)
	mux.HandleFunc("GET /api/v1/privacy/preferences/{userID}/consent", h.handleGetConsent)
}

// ConsentRequest is the JSON body for consent mutation operations.
type ConsentRequest struct {
	Grants      []ConsentCategory `json:"grants"`
	Revocations []ConsentCategory `json:"revocations"`
}

// ConsentResponse is the JSON response body for consent state.
type ConsentResponse struct {
	Grants   []ConsentCategory `json:"grants"`
	Defaults []ConsentCategory `json:"defaults"`
	Denied   []ConsentCategory `json:"denied"`
}

// handleSetConsent applies consent grants and/or revocations for a user.
func (h *ConsentHandler) handleSetConsent(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if userID == "" {
		writeErr(w, http.StatusBadRequest, "user ID is required")
		return
	}

	var req ConsentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateCategories(req.Grants, req.Revocations); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.applyGrants(r, userID, req.Grants); err != nil {
		h.log.Error(err, "apply grants failed", "userHash", logging.HashID(userID))
		writeErr(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if err := h.applyRevocations(r, userID, req.Revocations); err != nil {
		h.log.Error(err, "apply revocations failed", "userHash", logging.HashID(userID))
		writeErr(w, http.StatusInternalServerError, "internal server error")
		return
	}

	resp, err := h.buildConsentResponse(r, userID)
	if err != nil {
		h.log.Error(err, "build consent response failed", "userHash", logging.HashID(userID))
		writeErr(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleGetConsent returns the current consent state for a user.
func (h *ConsentHandler) handleGetConsent(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if userID == "" {
		writeErr(w, http.StatusBadRequest, "user ID is required")
		return
	}

	resp, err := h.buildConsentResponse(r, userID)
	if err != nil {
		h.log.Error(err, "get consent failed", "userHash", logging.HashID(userID))
		writeErr(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// validateCategories checks that all provided categories are known platform categories.
func validateCategories(grants, revocations []ConsentCategory) error {
	for _, cat := range grants {
		if _, valid := CategoryInfo(cat); !valid {
			return errors.New("unknown consent category: " + string(cat))
		}
	}
	for _, cat := range revocations {
		if _, valid := CategoryInfo(cat); !valid {
			return errors.New("unknown consent category: " + string(cat))
		}
	}
	return nil
}

// applyGrants calls SetConsentGrant for each category and emits audit events.
func (h *ConsentHandler) applyGrants(r *http.Request, userID string, grants []ConsentCategory) error {
	for _, cat := range grants {
		if err := h.store.SetConsentGrant(r.Context(), userID, cat); err != nil {
			return err
		}
		h.emitAudit(r, "consent_granted", userID, cat)
	}
	return nil
}

// applyRevocations calls RemoveConsentGrant for each category, ignoring ErrPreferencesNotFound.
func (h *ConsentHandler) applyRevocations(r *http.Request, userID string, revocations []ConsentCategory) error {
	for _, cat := range revocations {
		err := h.store.RemoveConsentGrant(r.Context(), userID, cat)
		if err != nil && !errors.Is(err, ErrPreferencesNotFound) {
			return err
		}
		h.emitAudit(r, "consent_revoked", userID, cat)
	}
	return nil
}

// buildConsentResponse fetches current grants and computes defaults/denied.
func (h *ConsentHandler) buildConsentResponse(r *http.Request, userID string) (*ConsentResponse, error) {
	grants, err := h.store.GetConsentGrants(r.Context(), userID)
	if err != nil {
		return nil, err
	}

	grantSet := make(map[ConsentCategory]bool, len(grants))
	for _, g := range grants {
		grantSet[g] = true
	}

	var defaults, denied []ConsentCategory
	for _, cat := range ValidCategories() {
		requiresGrant, _ := CategoryInfo(cat)
		if !requiresGrant {
			defaults = append(defaults, cat)
		} else if !grantSet[cat] {
			denied = append(denied, cat)
		}
	}

	if grants == nil {
		grants = []ConsentCategory{}
	}
	if defaults == nil {
		defaults = []ConsentCategory{}
	}
	if denied == nil {
		denied = []ConsentCategory{}
	}

	return &ConsentResponse{
		Grants:   grants,
		Defaults: defaults,
		Denied:   denied,
	}, nil
}

// emitAudit emits an audit event if an audit logger is configured.
func (h *ConsentHandler) emitAudit(r *http.Request, eventType, userID string, cat ConsentCategory) {
	if h.audit == nil {
		return
	}
	h.audit.LogEvent(r.Context(), &api.AuditEntry{
		EventType: eventType,
		Metadata: map[string]string{
			"user_id":  userID,
			"category": string(cat),
		},
	})
}
