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
	"fmt"
	"io"
	"net/http"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/logging"
)

// OptOutChecker returns false when the user has opted out of memory storage,
// indicating the write should be silently dropped.
type OptOutChecker func(ctx context.Context, userID, workspace string) bool

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

		// Check opt-out: if the user has opted out, silently drop the write.
		if userID != "" && !m.checkOptOut(r.Context(), userID, workspace) {
			m.log.V(1).Info("memory write suppressed", "reason", "user opt-out", "userHash", logging.HashID(userID), "workspace", workspace)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Apply PII redaction to the request body's content field.
		if err := m.applyRedaction(r, workspace); err != nil {
			m.log.Error(err, "content redaction failed, blocking request", "workspace", workspace)
			http.Error(w, "redaction failed", http.StatusInternalServerError)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// applyRedaction reads the request body, redacts the content field, and
// replaces r.Body with the redacted payload.
func (m *MemoryPrivacyMiddleware) applyRedaction(r *http.Request, workspace string) error {
	if r.Body == nil {
		return nil
	}

	data, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		return fmt.Errorf("reading request body: %w", err)
	}

	if len(data) == 0 {
		r.Body = io.NopCloser(bytes.NewReader(data))
		return nil
	}

	var req SaveMemoryRequest
	if err := json.Unmarshal(data, &req); err != nil {
		// Not valid JSON — restore the body and let the handler return the
		// appropriate 400 error.
		r.Body = io.NopCloser(bytes.NewReader(data))
		return nil
	}

	redacted, err := m.redact(r.Context(), workspace, req.Content)
	if err != nil {
		return fmt.Errorf("redacting content: %w", err)
	}

	if redacted == req.Content {
		// Nothing changed — avoid re-encoding unnecessarily.
		r.Body = io.NopCloser(bytes.NewReader(data))
		return nil
	}

	req.Content = redacted
	encoded, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("re-encoding request body: %w", err)
	}

	r.Body = io.NopCloser(bytes.NewReader(encoded))
	r.ContentLength = int64(len(encoded))
	return nil
}
