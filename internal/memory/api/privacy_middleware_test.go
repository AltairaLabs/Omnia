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
var passthroughOptOut OptOutChecker = func(_ context.Context, _, _ string) bool { return true }

// noOpRedact returns content unchanged.
var noOpRedact ContentRedactor = func(_ context.Context, _, content string) (string, error) {
	return content, nil
}

// optedOutChecker always returns false (user opted out).
var optedOutChecker OptOutChecker = func(_ context.Context, _, _ string) bool { return false }

// panicOptOut panics if called — use to assert opt-out is never checked.
var panicOptOut OptOutChecker = func(_ context.Context, _, _ string) bool {
	panic("OptOutChecker must not be called for this request")
}

// panicRedact panics if called — use to assert redaction is never invoked.
var panicRedact ContentRedactor = func(_ context.Context, _, _ string) (string, error) {
	panic("ContentRedactor must not be called for this request")
}

func newTestMiddleware(checkOptOut OptOutChecker, redact ContentRedactor) *MemoryPrivacyMiddleware {
	return NewMemoryPrivacyMiddleware(checkOptOut, redact, logr.Discard())
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

	redactor := ContentRedactor(func(_ context.Context, _, content string) (string, error) {
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

func TestMemoryPrivacyMiddleware_RedactionError_Returns500(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	errRedact := ContentRedactor(func(_ context.Context, _, _ string) (string, error) {
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
	redactor := ContentRedactor(func(_ context.Context, _, content string) (string, error) {
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
