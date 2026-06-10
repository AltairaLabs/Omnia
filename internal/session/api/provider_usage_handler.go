/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/altairalabs/omnia/internal/httputil"
)

// handleRecordProviderUsage persists one or more workspace-scoped provider
// usage rows (embeddings, judge tokens, …). POST /api/v1/provider-usage
// Body: a JSON array of ProviderUsage objects. Returns 201 on success.
func (h *Handler) handleRecordProviderUsage(w http.ResponseWriter, r *http.Request) {
	if h.providerUsageService == nil {
		writeProviderUsageError(w, ErrMissingProviderUsageStore)
		return
	}

	h.limitBody(w, r)
	var rows []*ProviderUsage
	if err := json.NewDecoder(r.Body).Decode(&rows); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	if err := h.providerUsageService.RecordProviderUsage(r.Context(), rows); err != nil {
		writeProviderUsageError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// writeProviderUsageError maps provider-usage service errors to HTTP statuses.
func writeProviderUsageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrMissingProviderUsageStore):
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "provider usage store not configured"})
	case errors.Is(err, ErrInvalidProviderUsage):
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
	default:
		writeError(w, err)
	}
}
