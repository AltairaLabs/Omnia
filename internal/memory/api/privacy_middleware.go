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
	"strings"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/logging"
)

const consentGrantsHeader = "X-Consent-Grants"

// OptOutChecker returns false when the user has opted out of memory storage,
// indicating the write should be silently dropped.
// category is the consent category from the request body (may be empty string
// when no category was supplied; callers should apply a default in that case).
// consentOverride, when non-nil, provides per-request grant overrides from the
// X-Consent-Grants header, bypassing the database-backed consent source.
type OptOutChecker func(ctx context.Context, userID, workspace, category string, consentOverride []string) bool

// ContentRedactor redacts PII from memory content text.
// Returns the redacted string; if redaction is not configured it returns the
// original text unchanged.
//
// provenance is the PromptKit Provenance string carried on the memory's
// metadata (user_requested, agent_extracted, system_generated,
// operator_curated) — empty when the caller didn't set one. Implementations
// use it to decide whether to apply the full pattern set (agent-extracted /
// system-generated / unknown) or only the structural subset
// (user_requested / operator_curated) so intentionally-persisted personal
// details (e.g. "my work email is ...") survive while SSN / CC / IP remain
// scrubbed.
type ContentRedactor func(ctx context.Context, workspace, content, provenance string) (string, error)

// ContentClassifier infers a consent category from memory content.
// Returns a category string (e.g. "memory:identity") or empty string for default.
type ContentClassifier func(content string) string

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
	classifier  ContentClassifier // optional, nil when not configured
	log         logr.Logger
}

// NewMemoryPrivacyMiddleware creates a MemoryPrivacyMiddleware.
// checkOptOut and redact must not be nil. classifier may be nil.
func NewMemoryPrivacyMiddleware(
	checkOptOut OptOutChecker,
	redact ContentRedactor,
	classifier ContentClassifier,
	log logr.Logger,
) *MemoryPrivacyMiddleware {
	return &MemoryPrivacyMiddleware{
		checkOptOut: checkOptOut,
		redact:      redact,
		classifier:  classifier,
		log:         log.WithName("memory-privacy"),
	}
}

// provenanceMetaKey mirrors pkmemory.MetaKeyProvenance — duplicated here as
// a string literal to keep the middleware dependency-free from the memory
// package's PromptKit re-export.
const provenanceMetaKey = "provenance"

// provenanceFromMetadata extracts the provenance string from a memory's
// metadata map. Returns "" when unset or wrong type.
func provenanceFromMetadata(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	v, _ := meta[provenanceMetaKey].(string)
	return v
}

// readAndDecode reads the request body and attempts to decode it as a SaveMemoryRequest.
// Returns the raw bytes, the decoded request, and whether decoding succeeded.
func readAndDecode(r *http.Request) ([]byte, SaveMemoryRequest, bool, error) {
	var data []byte
	if r.Body != nil {
		var err error
		data, err = io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			return nil, SaveMemoryRequest{}, false, err
		}
	}
	var req SaveMemoryRequest
	decoded := len(data) > 0 && json.Unmarshal(data, &req) == nil
	return data, req, decoded, nil
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

		data, req, decoded, err := readAndDecode(r)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}

		// Classify content when no explicit category provided.
		if decoded && req.Category == "" && m.classifier != nil {
			req.Category = m.classifier(req.Content)
		}

		// Read per-request consent override from header.
		var consentOverride []string
		if h := r.Header.Get(consentGrantsHeader); h != "" {
			consentOverride = strings.Split(h, ",")
		}

		// Check opt-out with category (empty string if not decoded).
		if userID != "" && !m.checkOptOut(r.Context(), userID, workspace, req.Category, consentOverride) {
			m.log.V(1).Info("memory write suppressed", "reason", "user opt-out", "userHash", logging.HashID(userID), "workspace", workspace, "category", req.Category)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Apply PII redaction when body was successfully decoded.
		if decoded {
			provenance := provenanceFromMetadata(req.Metadata)
			redacted, redactErr := m.redact(r.Context(), workspace, req.Content, provenance)
			if redactErr != nil {
				m.log.Error(redactErr, "content redaction failed, blocking request", "workspace", workspace)
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
