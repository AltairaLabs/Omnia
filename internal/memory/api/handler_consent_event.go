/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"encoding/json"
	"net/http"

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/altairalabs/omnia/internal/memory"
)

// ConsentEventRequest is the JSON body for POST /api/v1/memories/consent-events.
type ConsentEventRequest struct {
	UserID   string `json:"userId"`
	Category string `json:"category"`
}

// ConsentEventResponse is the JSON body returned from
// POST /api/v1/memories/consent-events.
type ConsentEventResponse struct {
	Deleted int64 `json:"deleted"`
}

// handleConsentEvent processes an inbound consent-revocation event from
// privacy-api. It prunes the caller-supplied user's memories in the
// given consent category according to the workspace MemoryPolicy.
//
// Route: POST /api/v1/memories/consent-events (enterprise-gated)
// Query: ?workspace=<workspaceID>
// Body:  { "userId": "...", "category": "..." }
func (h *Handler) handleConsentEvent(w http.ResponseWriter, r *http.Request) {
	scope, err := h.parseWorkspaceScope(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var req ConsentEventRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		writeError(w, httpError{status: http.StatusBadRequest, msg: "invalid JSON body"})
		return
	}
	if req.UserID == "" {
		writeError(w, httpError{status: http.StatusBadRequest, msg: "userId is required"})
		return
	}
	if req.Category == "" {
		writeError(w, httpError{status: http.StatusBadRequest, msg: "category is required"})
		return
	}

	workspaceID := scope[memory.ScopeWorkspaceID]
	n, err := h.service.PruneUserConsentCategory(r.Context(), workspaceID, req.UserID, req.Category)
	if err != nil {
		h.log.Error(err, "PruneUserConsentCategory failed",
			"workspace", workspaceID,
			"category", req.Category,
		)
		writeError(w, err)
		return
	}

	h.log.V(1).Info("consent event processed",
		"workspace", workspaceID,
		"category", req.Category,
		"deleted", n,
	)
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(ConsentEventResponse{Deleted: n})
}
