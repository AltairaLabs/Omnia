/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import "net/http"

// handleProjection serves GET /api/v1/memories/projection — a 2D Memory Galaxy
// layout for the workspace scope. res.Points is always non-nil ([] for empty).
func (h *Handler) handleProjection(w http.ResponseWriter, r *http.Request) {
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
