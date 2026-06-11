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
	"context"
	"encoding/json"
	"net/http"

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/altairalabs/omnia/pkg/logging"
)

// DimensionConsentRecorder records one-shot consent to change the memory
// embedding vector dimension. It is injected from cmd/memory-api as a closure
// over the Postgres pool; nil when the binary wasn't wired with one, in which
// case the admin endpoint responds 503.
type DimensionConsentRecorder func(ctx context.Context, targetDim int, createdBy string) error

// maxIndexableEmbeddingDim is pgvector's HNSW/IVFFlat dimension cap; a larger
// vector cannot be indexed, so we reject it at the door rather than let the
// reconciler fail the index build at startup.
const maxIndexableEmbeddingDim = 2000

type embeddingDimensionChangeRequest struct {
	TargetDim int `json:"target_dim"`
}

// handleEmbeddingDimensionChange records one-shot consent to change the memory
// embedding dimension to target_dim. The startup reconciler consumes the marker
// the next time it performs a (destructive) reshape to that dimension. See
// #1309. This only records intent; it does not itself alter the schema.
func (h *Handler) handleEmbeddingDimensionChange(w http.ResponseWriter, r *http.Request) {
	if h.recordDimConsent == nil {
		writeError(w, httpError{status: http.StatusServiceUnavailable, msg: "embedding dimension change consent is unavailable"})
		return
	}

	var req embeddingDimensionChangeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, h.maxBodySize)).Decode(&req); err != nil {
		writeError(w, ErrMissingBody)
		return
	}
	if req.TargetDim <= 0 || req.TargetDim > maxIndexableEmbeddingDim {
		writeError(w, httpError{status: http.StatusBadRequest, msg: "target_dim must be between 1 and 2000"})
		return
	}

	createdBy := hashedCaller(r)
	if err := h.recordDimConsent(r.Context(), req.TargetDim, createdBy); err != nil {
		h.log.Error(err, "record embedding dimension change consent failed", "targetDim", req.TargetDim)
		writeError(w, httpError{status: http.StatusInternalServerError, msg: "failed to record consent"})
		return
	}

	h.log.Info("embedding dimension change consent recorded",
		"targetDim", req.TargetDim, "createdBy", createdBy)
	_ = httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "consent recorded",
		"target_dim": req.TargetDim,
	})
}

// hashedCaller returns a hashed identifier for the operator recording consent,
// for provenance only. Raw IDs are PII, so hash before storage.
func hashedCaller(r *http.Request) string {
	if id := r.Header.Get("x-omnia-user-id"); id != "" {
		return logging.HashID(id)
	}
	return "operator"
}
