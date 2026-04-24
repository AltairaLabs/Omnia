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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// passthroughOptOut always returns true (no opt-out).
var passthroughOptOut OptOutChecker = func(_ context.Context, _, _, _ string, _ []string) bool { return true }

// noOpRedact returns content unchanged.
var noOpRedact ContentRedactor = func(_ context.Context, _, content, _ string) (string, error) {
	return content, nil
}

// optedOutChecker always returns false (user opted out).
var optedOutChecker OptOutChecker = func(_ context.Context, _, _, _ string, _ []string) bool { return false }

// panicOptOut panics if called — use to assert opt-out is never checked.
var panicOptOut OptOutChecker = func(_ context.Context, _, _, _ string, _ []string) bool {
	panic("OptOutChecker must not be called for this request")
}

// panicRedact panics if called — use to assert redaction is never invoked.
var panicRedact ContentRedactor = func(_ context.Context, _, _, _ string) (string, error) {
	panic("ContentRedactor must not be called for this request")
}

func newTestMiddleware(checkOptOut OptOutChecker, redact ContentRedactor) *MemoryPrivacyMiddleware {
	return NewMemoryPrivacyMiddleware(checkOptOut, redact, nil, logr.Discard())
}

func postBody(t *testing.T, content string) *bytes.Buffer {
	t.Helper()
	req := SaveMemoryRequest{
		Type:    "fact",
		Content: content,
		Scope:   map[string]string{"workspace": "ws-1"},
	}
	b, err := json.Marshal(req)
	require.NoError(t, err)
	return bytes.NewBuffer(b)
}

func makePostRequest(t *testing.T, content, workspace, userID string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/memories", postBody(t, content))
	q := r.URL.Query()
	q.Set("workspace", workspace)
	if userID != "" {
		q.Set("user_id", userID)
	}
	r.URL.RawQuery = q.Encode()
	r.Header.Set("Content-Type", "application/json")
	return r
}

// --- Tests ---

func TestMemoryPrivacyMiddleware_OptedOutUser_Returns204(t *testing.T) {
	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusCreated)
	})

	mw := newTestMiddleware(optedOutChecker, noOpRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, "some content", "ws-1", "user-abc")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNoContent, w.Code, "opted-out user should get 204")
	assert.False(t, handlerCalled, "next handler must not be called when opted out")
}

func TestMemoryPrivacyMiddleware_NoUserID_PassesThrough(t *testing.T) {
	// No user_id — opt-out check must be skipped entirely.
	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusCreated)
	})

	// Use panicOptOut: if the opt-out checker is called, the test panics.
	mw := newTestMiddleware(panicOptOut, noOpRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, "some content", "ws-1", "")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.True(t, handlerCalled)
}

func TestMemoryPrivacyMiddleware_UserNotOptedOut_PassesThrough(t *testing.T) {
	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusCreated)
	})

	mw := newTestMiddleware(passthroughOptOut, noOpRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, "my content", "ws-1", "user-xyz")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.True(t, handlerCalled)
}

func TestMemoryPrivacyMiddleware_PIIRedaction_ContentRedacted(t *testing.T) {
	const originalContent = "my email is test@example.com"
	const redactedContent = "my email is [EMAIL]"

	// Capture what the handler sees in the body.
	var receivedContent string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var req SaveMemoryRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		receivedContent = req.Content
	})

	redactor := ContentRedactor(func(_ context.Context, _, content, _ string) (string, error) {
		if content == originalContent {
			return redactedContent, nil
		}
		return content, nil
	})

	mw := newTestMiddleware(passthroughOptOut, redactor)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, originalContent, "ws-pii-test", "user-xyz")
	handler.ServeHTTP(w, r)

	assert.Equal(t, redactedContent, receivedContent, "handler should receive redacted content")
}

func TestMemoryPrivacyMiddleware_ForwardsProvenance(t *testing.T) {
	var seenProvenance string
	redactor := ContentRedactor(func(_ context.Context, _, content, provenance string) (string, error) {
		seenProvenance = provenance
		return content, nil
	})

	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	mw := newTestMiddleware(passthroughOptOut, redactor)
	handler := mw.Wrap(next)

	// Build a request whose metadata carries provenance=user_requested.
	body := SaveMemoryRequest{
		Type:    "fact",
		Content: "my work email is alice@example.com",
		Scope: map[string]string{
			"workspace_id": "ws-1",
			"user_id":      "user-1",
		},
		Metadata: map[string]any{"provenance": "user_requested"},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/memories?workspace=ws-1&user_id=user-1", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), r)

	assert.Equal(t, "user_requested", seenProvenance,
		"redactor must receive the provenance from the request metadata")
}

func TestMemoryPrivacyMiddleware_EmptyProvenanceWhenMetadataMissing(t *testing.T) {
	var seenProvenance string
	redactor := ContentRedactor(func(_ context.Context, _, content, provenance string) (string, error) {
		seenProvenance = provenance
		return content, nil
	})

	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	mw := newTestMiddleware(passthroughOptOut, redactor)
	handler := mw.Wrap(next)

	r := makePostRequest(t, "hello", "ws-1", "user-1")
	handler.ServeHTTP(httptest.NewRecorder(), r)

	assert.Empty(t, seenProvenance,
		"redactor must receive empty provenance when metadata is absent")
}

func TestMemoryPrivacyMiddleware_RedactionError_Returns500(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	errRedact := ContentRedactor(func(_ context.Context, _, _, _ string) (string, error) {
		return "", errors.New("regex engine failure")
	})

	mw := newTestMiddleware(passthroughOptOut, errRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, "some content", "ws-1", "user-xyz")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestMemoryPrivacyMiddleware_GETRequest_PassesThroughWithoutChecks(t *testing.T) {
	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Both panicking functions — neither should be called for a GET.
	mw := newTestMiddleware(panicOptOut, panicRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories?workspace=ws-1&user_id=user-abc", nil)
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, handlerCalled)
}

func TestMemoryPrivacyMiddleware_DELETERequest_PassesThroughWithoutChecks(t *testing.T) {
	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	mw := newTestMiddleware(panicOptOut, panicRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/memories/mem-1?workspace=ws-1", nil)
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, handlerCalled)
}

func TestMemoryPrivacyMiddleware_NoRedactionWhenContentUnchanged(t *testing.T) {
	const originalContent = "clean content"

	// Track that redactor was called but content unchanged.
	redactorCallCount := 0
	redactor := ContentRedactor(func(_ context.Context, _, content, _ string) (string, error) {
		redactorCallCount++
		return content, nil // no change
	})

	var receivedBody []byte
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
	})

	mw := newTestMiddleware(passthroughOptOut, redactor)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, originalContent, "ws-1", "user-xyz")

	// Capture original body bytes for comparison.
	originalBody, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(originalBody))

	handler.ServeHTTP(w, r)

	assert.Equal(t, 1, redactorCallCount, "redactor should be called once")
	assert.Equal(t, originalBody, receivedBody, "body bytes should be unchanged when content not modified")
}

func TestMemoryPrivacyMiddleware_EmptyBody_PassesThrough(t *testing.T) {
	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusBadRequest) // handler would return 400 for empty body
	})

	mw := newTestMiddleware(passthroughOptOut, noOpRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/memories?workspace=ws-1", nil)
	handler.ServeHTTP(w, r)

	// Handler should be called even with empty body — middleware doesn't block it.
	assert.True(t, handlerCalled)
}

func TestMemoryPrivacyMiddleware_InvalidJSON_PassesThrough(t *testing.T) {
	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusBadRequest)
	})

	// noOpRedact — invalid JSON should not trigger redaction error, just pass through.
	mw := newTestMiddleware(passthroughOptOut, noOpRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/memories?workspace=ws-1",
		bytes.NewBufferString(`{not valid json`))
	handler.ServeHTTP(w, r)

	// Handler should be called; it returns 400 for invalid JSON.
	assert.True(t, handlerCalled)
}

func TestMemoryPrivacyMiddleware_OptedOutUserWithNoRedaction_Returns204(t *testing.T) {
	// Verify opt-out check happens before redaction (redactor must NOT be called).
	mw := newTestMiddleware(optedOutChecker, panicRedact)
	handler := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	w := httptest.NewRecorder()
	r := makePostRequest(t, "some content", "ws-1", "user-abc")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func makePostRequestWithCategory(t *testing.T, content, workspace, userID, category string) *http.Request {
	t.Helper()
	req := SaveMemoryRequest{
		Type:     "fact",
		Content:  content,
		Scope:    map[string]string{"workspace": workspace},
		Category: category,
	}
	b, err := json.Marshal(req)
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewBuffer(b))
	q := r.URL.Query()
	q.Set("workspace", workspace)
	if userID != "" {
		q.Set("user_id", userID)
	}
	r.URL.RawQuery = q.Encode()
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestMemoryPrivacyMiddleware_NoCategoryInBody_OptOutCheckerReceivesEmpty(t *testing.T) {
	// POST without a category field — opt-out checker must receive an empty string.
	var receivedCategory string
	checker := OptOutChecker(func(_ context.Context, _, _, category string, _ []string) bool {
		receivedCategory = category
		return true // allow
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	mw := newTestMiddleware(checker, noOpRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, "some content", "ws-1", "user-abc")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "", receivedCategory, "category should be empty string when not in body")
}

func TestMemoryPrivacyMiddleware_WithCategory_OptOutCheckerReceivesCategory(t *testing.T) {
	// POST with category: "memory:identity" — opt-out checker must receive it.
	var receivedCategory string
	checker := OptOutChecker(func(_ context.Context, _, _, category string, _ []string) bool {
		receivedCategory = category
		return true // allow
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	mw := newTestMiddleware(checker, noOpRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequestWithCategory(t, "my name is Alice", "ws-1", "user-abc", "memory:identity")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "memory:identity", receivedCategory, "checker should receive the category from the body")
}

func TestMemoryPrivacyMiddleware_CategoryOptedOut_Returns204(t *testing.T) {
	// Opt-out checker that only rejects "memory:identity".
	checker := OptOutChecker(func(_ context.Context, _, _, category string, _ []string) bool {
		return category != "memory:identity"
	})

	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusCreated)
	})

	mw := newTestMiddleware(checker, noOpRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequestWithCategory(t, "my name is Alice", "ws-1", "user-abc", "memory:identity")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.False(t, handlerCalled)
}

func TestMemoryPrivacyMiddleware_ValidatorFillsCategoryWhenEmpty(t *testing.T) {
	// POST with no category + validator returns "memory:identity" → opt-out checker receives "memory:identity".
	var receivedCategory string
	checker := OptOutChecker(func(_ context.Context, _, _, category string, _ []string) bool {
		receivedCategory = category
		return true // allow
	})

	validator := CategoryValidator(func(_ context.Context, _, _ string) ValidatorResult {
		return ValidatorResult{Category: "memory:identity"}
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	mw := NewMemoryPrivacyMiddleware(checker, noOpRedact, validator, logr.Discard())
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, "my name is Alice", "ws-1", "user-abc")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "memory:identity", receivedCategory, "validator result should reach opt-out checker")
}

func TestMemoryPrivacyMiddleware_ValidatorRunsEvenWithCallerCategory(t *testing.T) {
	// POST with explicit category → validator is still called (it owns the
	// upgrade decision), but a pass-through validator leaves the claim alone.
	var observed string
	validator := CategoryValidator(func(_ context.Context, claimed, _ string) ValidatorResult {
		observed = claimed
		return ValidatorResult{Category: claimed}
	})

	var receivedCategory string
	checker := OptOutChecker(func(_ context.Context, _, _, category string, _ []string) bool {
		receivedCategory = category
		return true
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	mw := NewMemoryPrivacyMiddleware(checker, noOpRedact, validator, logr.Discard())
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequestWithCategory(t, "my preferences", "ws-1", "user-abc", "memory:preferences")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "memory:preferences", observed, "validator receives the caller's claim")
	assert.Equal(t, "memory:preferences", receivedCategory, "explicit category should pass through unchanged")
}

func TestMemoryPrivacyMiddleware_NilValidator_CategoryStaysEmpty(t *testing.T) {
	// POST with no category + nil validator → opt-out checker receives empty string.
	var receivedCategory string
	checker := OptOutChecker(func(_ context.Context, _, _, category string, _ []string) bool {
		receivedCategory = category
		return true
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	mw := NewMemoryPrivacyMiddleware(checker, noOpRedact, nil, logr.Discard())
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, "some content", "ws-1", "user-abc")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "", receivedCategory, "category should remain empty when validator is nil")
}

func TestMemoryPrivacyMiddleware_ValidatorRunsBeforeOptOut(t *testing.T) {
	// POST + validator returns "memory:health" + opt-out rejects "memory:health" → 204.
	validator := CategoryValidator(func(_ context.Context, _, _ string) ValidatorResult {
		return ValidatorResult{Category: "memory:health"}
	})

	checker := OptOutChecker(func(_ context.Context, _, _, category string, _ []string) bool {
		return category != "memory:health"
	})

	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusCreated)
	})

	mw := NewMemoryPrivacyMiddleware(checker, panicRedact, validator, logr.Discard())
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, "I have diabetes", "ws-1", "user-abc")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.False(t, handlerCalled)
}

func TestMemoryPrivacyMiddleware_OverrideAppliedToOutboundBody(t *testing.T) {
	// Validator upgrades the category — the outbound body forwarded to the
	// next handler must carry the upgraded value (not the caller's claim).
	validator := CategoryValidator(func(_ context.Context, _, _ string) ValidatorResult {
		return ValidatorResult{
			Category:   "memory:health",
			Overridden: true,
			From:       "memory:preferences",
			Source:     "regex",
		}
	})

	var bodyCategory string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body SaveMemoryRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		bodyCategory = body.Category
		w.WriteHeader(http.StatusCreated)
	})

	mw := NewMemoryPrivacyMiddleware(passthroughOptOut, noOpRedact, validator, logr.Discard())
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequestWithCategory(t, "I get migraines", "ws-1", "user-abc", "memory:preferences")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "memory:health", bodyCategory, "upgraded category must reach the next handler")
}

func TestMemoryPrivacyMiddleware_ConsentOverrideHeader_PassedToChecker(t *testing.T) {
	// POST with X-Consent-Grants header → checker receives the parsed grants slice.
	var receivedOverride []string
	checker := OptOutChecker(func(_ context.Context, _, _, _ string, consentOverride []string) bool {
		receivedOverride = consentOverride
		return true
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	mw := newTestMiddleware(checker, noOpRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, "some content", "ws-1", "user-abc")
	r.Header.Set(consentGrantsHeader, "memory:identity,memory:preferences")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, []string{"memory:identity", "memory:preferences"}, receivedOverride)
}

func TestMemoryPrivacyMiddleware_NoConsentHeader_NilOverride(t *testing.T) {
	// POST without X-Consent-Grants header → checker receives nil override.
	var receivedOverride []string
	checker := OptOutChecker(func(_ context.Context, _, _, _ string, consentOverride []string) bool {
		receivedOverride = consentOverride
		return true
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	mw := newTestMiddleware(checker, noOpRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, "some content", "ws-1", "user-abc")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Nil(t, receivedOverride, "no header should yield nil override")
}

func TestMemoryPrivacyMiddleware_ConsentOverride_OverridesDB(t *testing.T) {
	// Checker that allows when override contains "memory:identity", else denies.
	checker := OptOutChecker(func(_ context.Context, _, _, _ string, consentOverride []string) bool {
		for _, g := range consentOverride {
			if g == "memory:identity" {
				return true
			}
		}
		return false
	})

	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusCreated)
	})

	mw := newTestMiddleware(checker, noOpRedact)
	handler := mw.Wrap(next)

	w := httptest.NewRecorder()
	r := makePostRequest(t, "my name is Alice", "ws-1", "user-abc")
	r.Header.Set(consentGrantsHeader, "memory:identity")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code, "header override should allow the write")
	assert.True(t, handlerCalled)
}
