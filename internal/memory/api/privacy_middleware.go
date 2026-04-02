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
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/logging"
)

// OptOutChecker returns false when the user has opted out of memory storage,
// indicating the write should be silently dropped.
// category is the consent category from the request body (may be empty string
// when no category was supplied; callers should apply a default in that case).
type OptOutChecker func(ctx context.Context, userID, workspace, category string) bool

// ContentRedactor redacts PII from memory content text.
// Returns the redacted string; if redaction is not configured it returns the
// original text unchanged.
type ContentRedactor func(ctx context.Context, workspace, content string) (string, error)

// MemoryPrivacyMiddleware intercepts POST requests to the memory API and:
//  1. Returns 204 No Content when the user has opted out of memory storage.
//  2. Redacts PII from the request body's "content" field before forwarding.
//
// Only POST requests are intercepted. GET, DELETE, and other methods pass
// through without modification. This type is only constructed in enterprise
// mode; non-enterprise builds receive a nil pointer and the wrapper function
// is a no-op.
type MemoryPrivacyMiddleware struct {
	checkOptOut OptOutChecker
	redact      ContentRedactor
	log         logr.Logger
}

// NewMemoryPrivacyMiddleware creates a MemoryPrivacyMiddleware.
// checkOptOut and redact must not be nil.
func NewMemoryPrivacyMiddleware(
	checkOptOut OptOutChecker,
	redact ContentRedactor,
	log logr.Logger,
) *MemoryPrivacyMiddleware {
	return &MemoryPrivacyMiddleware{
		checkOptOut: checkOptOut,
		redact:      redact,
		log:         log.WithName("memory-privacy"),
	}
}

// Wrap returns an http.Handler that enforces privacy policy before delegating
// to next. Only POST requests are intercepted.
func (m *MemoryPrivacyMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		workspace := r.URL.Query().Get("workspace")
		userID := r.URL.Query().Get("user_id")

		// Read body once so we can inspect category for opt-out and content for redaction.
		var data []byte
		if r.Body != nil {
			var err error
			data, err = io.ReadAll(r.Body)
			_ = r.Body.Close()
			if err != nil {
				http.Error(w, "failed to read body", http.StatusInternalServerError)
				return
			}
		}

		// Try to decode to get category and content for downstream use.
		var req SaveMemoryRequest
		decoded := len(data) > 0 && json.Unmarshal(data, &req) == nil

		// Check opt-out with category (empty string if not decoded).
		if userID != "" && !m.checkOptOut(r.Context(), userID, workspace, req.Category) {
			m.log.V(1).Info("memory write suppressed", "reason", "user opt-out", "userHash", logging.HashID(userID), "workspace", workspace, "category", req.Category)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Apply PII redaction when body was successfully decoded.
		if decoded {
			redacted, err := m.redact(r.Context(), workspace, req.Content)
			if err != nil {
				m.log.Error(err, "content redaction failed, blocking request", "workspace", workspace)
				http.Error(w, "redaction failed", http.StatusInternalServerError)
				return
			}
			if redacted != req.Content {
				req.Content = redacted
				encoded, _ := json.Marshal(req)
				data = encoded
			}
		}

		r.Body = io.NopCloser(bytes.NewReader(data))
		r.ContentLength = int64(len(data))
		next.ServeHTTP(w, r)
	})
}
