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
	"strings"

	"github.com/go-logr/logr"
)

// DeletionHandler provides HTTP endpoints for GDPR/CCPA deletion requests.
type DeletionHandler struct {
	service *DeletionService
	log     logr.Logger
}

// NewDeletionHandler creates a new DeletionHandler.
func NewDeletionHandler(service *DeletionService, log logr.Logger) *DeletionHandler {
	return &DeletionHandler{
		service: service,
		log:     log.WithName("deletion-handler"),
	}
}

// RegisterRoutes registers deletion API routes on the given mux.
func (h *DeletionHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/privacy/deletion-request", h.handleCreate)
	mux.HandleFunc("GET /api/v1/privacy/deletion-request/{id}", h.handleGet)
	mux.HandleFunc("GET /api/v1/privacy/deletion-requests", h.handleList)
}

// handleCreate processes a POST request to create a new deletion request.
func (h *DeletionHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateDeletionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req, err := h.service.CreateRequest(r.Context(), &input)
	if err != nil {
		statusCode := mapErrorToStatus(err)
		writeJSONError(w, statusCode, err.Error())
		return
	}

	// Process the deletion asynchronously in a goroutine.
	go func() {
		if processErr := h.service.ProcessRequest(r.Context(), req.ID); processErr != nil {
			h.log.Error(processErr, "deletion processing failed", "requestID", req.ID)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(req)
}

// handleGet processes a GET request to retrieve a deletion request status.
func (h *DeletionHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing request ID")
		return
	}

	req, err := h.service.GetRequest(r.Context(), id)
	if err != nil {
		statusCode := mapErrorToStatus(err)
		writeJSONError(w, statusCode, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req)
}

// handleList processes a GET request to list deletion requests by user.
func (h *DeletionHandler) handleList(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing user_id query parameter")
		return
	}

	requests, err := h.service.ListRequestsByUser(r.Context(), userID)
	if err != nil {
		h.log.Error(err, "listing deletion requests failed", "userID", userID)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if requests == nil {
		requests = []*DeletionRequest{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(requests)
}

// mapErrorToStatus converts a service error to an HTTP status code.
func mapErrorToStatus(err error) int {
	msg := err.Error()
	if strings.Contains(msg, "not found") {
		return http.StatusNotFound
	}
	if strings.Contains(msg, "required") || strings.Contains(msg, "must be") {
		return http.StatusBadRequest
	}
	if strings.Contains(msg, "already being processed") {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

// errorResponse is the JSON response for errors.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSONError writes a JSON error response.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg})
}
