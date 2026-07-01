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
	"time"

	"github.com/go-logr/logr"
)

// sessionEraseRoute is the path served by SessionEraseHandler.
const sessionEraseRoute = "POST /api/v1/privacy/sessions/delete-by-user"

// sessionEraseRequest is the JSON body for the delete-by-user endpoint.
type sessionEraseRequest struct {
	VirtualUserID string     `json:"virtual_user_id"`
	Workspace     string     `json:"workspace"`
	DateFrom      *time.Time `json:"date_from"`
	DateTo        *time.Time `json:"date_to"`
}

// SessionEraseHandler exposes session-tier DSAR erasure over HTTP so privacy-api
// can orchestrate it across service-groups. Mount behind ServiceAccount auth.
type SessionEraseHandler struct {
	eraser *SessionEraser
	log    logr.Logger
}

// NewSessionEraseHandler builds a SessionEraseHandler.
func NewSessionEraseHandler(eraser *SessionEraser, log logr.Logger) *SessionEraseHandler {
	return &SessionEraseHandler{eraser: eraser, log: log}
}

// RegisterRoutes mounts the delete-by-user route on mux.
func (h *SessionEraseHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc(sessionEraseRoute, h.handleDelete)
}

func (h *SessionEraseHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	var body sessionEraseRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.eraser.Erase(r.Context(), EraseScope(body))
	if errors.Is(err, ErrMissingVirtualUserID) {
		writeJSONError(w, http.StatusBadRequest, "virtual_user_id is required")
		return
	}
	if err != nil {
		h.log.Error(err, "session erase failed", "virtualUserID", body.VirtualUserID)
		writeJSONError(w, http.StatusInternalServerError, "erase failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}
