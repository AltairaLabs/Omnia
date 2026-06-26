/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"fmt"
	"net/http"
)

// handleProjection serves GET /api/v1/memories/projection — a 2D Memory Galaxy
// layout for the workspace scope. res.Points is always non-nil ([] for empty).
func (h *Handler) handleProjection(w http.ResponseWriter, r *http.Request) {
	// Defence in depth behind the PCA clamp (#1588): a numeric edge case in the
	// projection compute must surface as a clean 500 JSON, not a panic that
	// resets the socket (which the dashboard proxy reports as a 502).
	defer h.recoverProjection(w)

	scope, err := parseWorkspaceScope(r)
	if err != nil {
		writeError(w, err)
		return
	}
	res, err := h.service.Project(r.Context(), scope)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, res)
}

// recoverProjection turns a panic during projection compute into a 500 JSON
// response. The panic happens before any body is written (during Project), so
// the status can still be set. Logged loudly so the underlying bug is visible.
func (h *Handler) recoverProjection(w http.ResponseWriter) {
	if rec := recover(); rec != nil {
		h.log.Error(fmt.Errorf("panic: %v", rec), "projection handler panicked")
		writeError(w, fmt.Errorf("projection failed"))
	}
}
