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
	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

// IngestRequest is the body for POST /api/v1/institutional/ingest. The caller
// (a source adapter) sends already-extracted text; the strategy decides how it
// becomes index items.
type IngestRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Site        string `json:"site"`
	Text        string `json:"text"`
}

// handleIngest runs the configured ingestion strategy and persists items.
// Returns 202 — embedding happens asynchronously via the ReembedWorker.
func (h *Handler) handleIngest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}
	if req.WorkspaceID == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}
	err := h.service.IngestDocument(r.Context(), req.WorkspaceID, ingestion.SourceDoc{
		Title: req.Title, URL: req.URL, Site: req.Site, Text: req.Text,
	})
	if err != nil {
		h.log.Error(err, "IngestDocument failed", "workspace", req.WorkspaceID, "url", req.URL)
		writeError(w, err)
		return
	}
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusAccepted)
}
