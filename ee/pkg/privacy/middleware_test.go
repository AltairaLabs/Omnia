/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
)

// mockSessionLookup always returns a fixed namespace/agent.
type mockSessionLookup struct {
	ns    string
	agent string
	err   error
}

func (m *mockSessionLookup) LookupSession(_ context.Context, _ string) (*SessionMetadata, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &SessionMetadata{Namespace: m.ns, AgentName: m.agent}, nil
}

// optOutPrefStore returns opted-out preferences for testing.
func optOutPrefStore() *mockPreferencesStore {
	return &mockPreferencesStore{prefs: &Preferences{OptOutAll: true}}
}

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/sessions/abc-123/messages", "abc-123"},
		{"/api/v1/sessions/abc-123/tool-calls", "abc-123"},
		{"/api/v1/sessions/abc-123", "abc-123"},
		{"/api/v1/sessions", ""},
		{"/healthz", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, extractSessionID(tt.path))
		})
	}
}

func TestPrivacyMiddleware_PassthroughGET(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	lookup := &mockSessionLookup{ns: "default", agent: "test-agent"}
	cache := NewSessionMetadataCache(lookup, 100)
	watcher := &PolicyWatcher{}
	mw := NewPrivacyMiddleware(watcher, cache, nil, nil, logr.Discard())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/abc-123/messages", nil)
	rr := httptest.NewRecorder()
	mw.Wrap(handler).ServeHTTP(rr, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestPrivacyMiddleware_NoSessionID(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	watcher := &PolicyWatcher{}
	mw := NewPrivacyMiddleware(watcher, nil, nil, nil, logr.Discard())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval-results", strings.NewReader("{}"))
	rr := httptest.NewRecorder()
	mw.Wrap(handler).ServeHTTP(rr, req)

	assert.True(t, called)
}

func TestPrivacyMiddleware_OptOutReturns204(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	lookup := &mockSessionLookup{ns: "default", agent: "test-agent"}
	cache := NewSessionMetadataCache(lookup, 100)

	// Set up watcher with a policy that enables opt-out
	watcher := &PolicyWatcher{}
	watcher.policies.Store("test", &omniav1alpha1.SessionPrivacyPolicy{
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level: omniav1alpha1.PolicyLevelGlobal,
			Recording: omniav1alpha1.RecordingConfig{
				Enabled: true,
			},
			UserOptOut: &omniav1alpha1.UserOptOutConfig{Enabled: true},
		},
	})

	prefStore := optOutPrefStore()
	mw := NewPrivacyMiddleware(watcher, cache, nil, prefStore, logr.Discard())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/abc-123/messages",
		strings.NewReader(`{"content":"hello"}`))
	req.Header.Set(UserIDHeader, "user-1")
	rr := httptest.NewRecorder()
	mw.Wrap(handler).ServeHTTP(rr, req)

	assert.False(t, called, "handler should not be called when user opted out")
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestPrivacyMiddleware_NoPolicyPassthrough(t *testing.T) {
	var bodyContent string
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodyContent = string(b)
	})

	lookup := &mockSessionLookup{ns: "default", agent: "test-agent"}
	cache := NewSessionMetadataCache(lookup, 100)
	watcher := &PolicyWatcher{} // empty — no policies

	mw := NewPrivacyMiddleware(watcher, cache, nil, nil, logr.Discard())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/abc-123/messages",
		strings.NewReader(`{"content":"hello"}`))
	rr := httptest.NewRecorder()
	mw.Wrap(handler).ServeHTTP(rr, req)

	assert.Equal(t, `{"content":"hello"}`, bodyContent)
}

func TestSessionMetadataCache_Resolve(t *testing.T) {
	lookup := &mockSessionLookup{ns: "ns1", agent: "agent1"}
	cache := NewSessionMetadataCache(lookup, 100)

	ns, agent, err := cache.Resolve(context.Background(), "session-1")
	assert.NoError(t, err)
	assert.Equal(t, "ns1", ns)
	assert.Equal(t, "agent1", agent)

	// Second call should hit the cache (same result)
	ns2, agent2, err2 := cache.Resolve(context.Background(), "session-1")
	assert.NoError(t, err2)
	assert.Equal(t, ns, ns2)
	assert.Equal(t, agent, agent2)
}

func TestPrivacyMiddleware_RedactsMessageContent(t *testing.T) {
	var bodyContent string
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodyContent = string(b)
	})

	lookup := &mockSessionLookup{ns: "default", agent: "test-agent"}
	cache := NewSessionMetadataCache(lookup, 100)

	watcher := &PolicyWatcher{}
	watcher.policies.Store("test", &omniav1alpha1.SessionPrivacyPolicy{
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level: omniav1alpha1.PolicyLevelGlobal,
			Recording: omniav1alpha1.RecordingConfig{
				Enabled: true,
				PII: &omniav1alpha1.PIIConfig{
					Redact:   true,
					Patterns: []string{"ssn"},
					Strategy: omniav1alpha1.RedactionStrategyReplace,
				},
			},
		},
	})

	r := redaction.NewRedactor()
	mw := NewPrivacyMiddleware(watcher, cache, r, nil, logr.Discard())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/abc-123/messages",
		strings.NewReader(`{"content":"My SSN is 123-45-6789"}`))
	rr := httptest.NewRecorder()
	mw.Wrap(handler).ServeHTTP(rr, req)

	assert.NotContains(t, bodyContent, "123-45-6789")
	assert.Contains(t, bodyContent, "REDACTED")
}

func TestPrivacyMiddleware_SessionLookupError(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	lookup := &mockSessionLookup{err: io.ErrUnexpectedEOF}
	cache := NewSessionMetadataCache(lookup, 100)
	watcher := &PolicyWatcher{}
	mw := NewPrivacyMiddleware(watcher, cache, nil, nil, logr.Discard())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/abc-123/messages",
		strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	mw.Wrap(handler).ServeHTTP(rr, req)

	assert.True(t, called, "should pass through on lookup error")
}

func TestPrivacyMiddleware_OptOutNoUserID(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	lookup := &mockSessionLookup{ns: "default", agent: "test-agent"}
	cache := NewSessionMetadataCache(lookup, 100)

	watcher := &PolicyWatcher{}
	watcher.policies.Store("test", &omniav1alpha1.SessionPrivacyPolicy{
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level:      omniav1alpha1.PolicyLevelGlobal,
			Recording:  omniav1alpha1.RecordingConfig{Enabled: true},
			UserOptOut: &omniav1alpha1.UserOptOutConfig{Enabled: true},
		},
	})

	prefStore := optOutPrefStore()
	mw := NewPrivacyMiddleware(watcher, cache, nil, prefStore, logr.Discard())

	// No X-Omnia-User-ID header — should pass through
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/abc-123/messages",
		strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	mw.Wrap(handler).ServeHTTP(rr, req)

	assert.True(t, called, "should pass through when no user ID header")
}

func TestPrivacyMiddleware_RedactionFailureReturns500(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	lookup := &mockSessionLookup{ns: "default", agent: "test-agent"}
	cache := NewSessionMetadataCache(lookup, 100)

	watcher := &PolicyWatcher{}
	watcher.policies.Store("test", &omniav1alpha1.SessionPrivacyPolicy{
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level: omniav1alpha1.PolicyLevelGlobal,
			Recording: omniav1alpha1.RecordingConfig{
				Enabled: true,
				PII: &omniav1alpha1.PIIConfig{
					Redact:   true,
					Patterns: []string{"ssn"},
					Strategy: omniav1alpha1.RedactionStrategyReplace,
				},
			},
		},
	})

	r := redaction.NewRedactor()
	mw := NewPrivacyMiddleware(watcher, cache, r, nil, logr.Discard())

	// Send invalid JSON to a /messages endpoint — redaction will fail on unmarshal.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/abc-123/messages",
		strings.NewReader(`not valid json`))
	rr := httptest.NewRecorder()
	mw.Wrap(handler).ServeHTTP(rr, req)

	assert.False(t, called, "handler should not be called when redaction fails")
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestSessionMetadataCache_Eviction(t *testing.T) {
	lookup := &mockSessionLookup{ns: "ns1", agent: "agent1"}
	cache := NewSessionMetadataCache(lookup, 2)

	// Fill to capacity
	_, _, _ = cache.Resolve(context.Background(), "s1")
	_, _, _ = cache.Resolve(context.Background(), "s2")

	// This should evict s1
	_, _, _ = cache.Resolve(context.Background(), "s3")

	// s1 should no longer be in cache, but still resolvable via lookup
	ns, _, err := cache.Resolve(context.Background(), "s1")
	assert.NoError(t, err)
	assert.Equal(t, "ns1", ns)
}
