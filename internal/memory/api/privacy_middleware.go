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
const consentLayerHeader = "X-Consent-Layer"
const consentDecisionHeader = "X-Consent-Decision"

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

// CategoryValidator infers and validates the consent category for a memory
// write. claimedCategory is whatever the caller put in req.Category (may
// be empty). Implementations return the final category to use plus enough
// detail for callers to record overrides. Implementations must not return
// an error on internal classifier failures — they should fall through to
// a sensible default and let the caller observe degradation via metrics.
type CategoryValidator func(ctx context.Context, claimedCategory, content string) ValidatorResult

// ValidatorResult is the structured outcome reported by a CategoryValidator.
// Mirrors classify.Result but keeps this package free of an EE import.
type ValidatorResult struct {
	Category   string
	Overridden bool
	From       string
	Source     string // "regex" | "embedding" | ""
}

// MemoryPrivacyMiddleware intercepts POST requests to the memory API and:
//  1. Returns 204 No Content when the user has opted out of memory storage.
//  2. Redacts PII from the request body's "content" field before forwarding.
//  3. Optionally upgrades / fills req.Category via a CategoryValidator.
//
// Only POST requests are intercepted. GET, DELETE, and other methods pass
// through without modification. This type is only constructed in enterprise
// mode; non-enterprise builds receive a nil pointer and the wrapper function
// is a no-op.
type MemoryPrivacyMiddleware struct {
	checkOptOut OptOutChecker
	redact      ContentRedactor
	validator   CategoryValidator   // optional, nil when not configured
	metrics     *SuppressionMetrics // optional, nil when not configured
	log         logr.Logger
}

// NewMemoryPrivacyMiddleware creates a MemoryPrivacyMiddleware.
// checkOptOut and redact must not be nil. validator may be nil — when nil
// the middleware uses the caller-supplied req.Category verbatim and no
// classification or override happens.
func NewMemoryPrivacyMiddleware(
	checkOptOut OptOutChecker,
	redact ContentRedactor,
	validator CategoryValidator,
	log logr.Logger,
) *MemoryPrivacyMiddleware {
	return &MemoryPrivacyMiddleware{
		checkOptOut: checkOptOut,
		redact:      redact,
		validator:   validator,
		log:         log.WithName("memory-privacy"),
	}
}

// NewMemoryPrivacyMiddlewareWithMetrics is like NewMemoryPrivacyMiddleware
// but additionally wires a SuppressionMetrics collector for observability
// on dropped writes. metrics may be nil; nil disables metric recording.
func NewMemoryPrivacyMiddlewareWithMetrics(
	checkOptOut OptOutChecker,
	redact ContentRedactor,
	validator CategoryValidator,
	metrics *SuppressionMetrics,
	log logr.Logger,
) *MemoryPrivacyMiddleware {
	mw := NewMemoryPrivacyMiddleware(checkOptOut, redact, validator, log)
	mw.metrics = metrics
	return mw
}

// formatConsentDecision builds the value for the X-Consent-Decision response
// header. Format: "deny; category=<category>; layer=<layer>". Empty fields
// become "unknown" to keep the header well-formed.
func formatConsentDecision(category, layer string) string {
	if category == "" {
		category = "unknown"
	}
	if layer == "" {
		layer = "unknown"
	}
	return "deny; category=" + category + "; layer=" + layer
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

		// Run the validator to fill / upgrade req.Category. The validator
		// owns the full decision (caller's claim + classifier results).
		// Track whether the body needs re-encoding before forwarding.
		bodyDirty := false
		if decoded && m.validator != nil {
			res := m.validator(r.Context(), req.Category, req.Content)
			if res.Overridden {
				m.log.V(1).Info("consent category upgraded",
					"from", res.From,
					"to", res.Category,
					"source", res.Source,
					"workspace", workspace,
				)
			}
			if res.Category != req.Category {
				req.Category = res.Category
				bodyDirty = true
			}
		}

		// Read per-request consent override from header.
		var consentOverride []string
		if h := r.Header.Get(consentGrantsHeader); h != "" {
			consentOverride = strings.Split(h, ",")
		}

		// Check opt-out with category (empty string if not decoded).
		if userID != "" && !m.checkOptOut(r.Context(), userID, workspace, req.Category, consentOverride) {
			layer := r.Header.Get(consentLayerHeader)
			if layer == "" {
				layer = "persistent"
			}
			m.log.Info("memory write suppressed",
				"reason", "opt-out",
				"category", req.Category,
				"layer", layer,
				"grants", consentOverride,
				"userHash", logging.HashID(userID),
				"workspace", workspace,
			)
			if m.metrics != nil {
				m.metrics.RecordSuppression(layer, req.Category, "opt-out")
			}
			w.Header().Set(consentDecisionHeader, formatConsentDecision(req.Category, layer))
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
				bodyDirty = true
			}
		}

		if bodyDirty {
			encoded, _ := json.Marshal(req)
			data = encoded
		}

		r.Body = io.NopCloser(bytes.NewReader(data))
		r.ContentLength = int64(len(data))
		next.ServeHTTP(w, r)
	})
}
