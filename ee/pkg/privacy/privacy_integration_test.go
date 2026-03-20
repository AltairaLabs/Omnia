/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
)

// --- Test helpers ---

// Test constants.
const (
	// userIDHeader is the HTTP header that carries the user identity.
	userIDHeader = "X-Omnia-User-ID"
	// bodyHello is a simple JSON message body used across opt-out tests.
	bodyHello = `{"content":"Hello","role":"user"}`
	// bodyWithSSN is a JSON message body containing an SSN for redaction tests.
	bodyWithSSN = `{"content":"SSN is 123-45-6789","role":"user"}`
)

// policyKey identifies which policy to apply for a given session.
type policyKey struct {
	namespace string
	agent     string
}

// policyWatcher holds privacy policies in a sync.Map, keyed by scope.
// The global policy is stored under policyKey{}.
type policyWatcher struct {
	policies sync.Map // policyKey -> *omniav1alpha1.SessionPrivacyPolicySpec
}

func (pw *policyWatcher) store(key policyKey, spec *omniav1alpha1.SessionPrivacyPolicySpec) {
	pw.policies.Store(key, spec)
}

// resolve returns the most specific policy: agent > workspace > global.
func (pw *policyWatcher) resolve(namespace, agent string) *omniav1alpha1.SessionPrivacyPolicySpec {
	// Agent-level
	if v, ok := pw.policies.Load(policyKey{namespace: namespace, agent: agent}); ok {
		return v.(*omniav1alpha1.SessionPrivacyPolicySpec)
	}
	// Workspace (namespace) level
	if v, ok := pw.policies.Load(policyKey{namespace: namespace}); ok {
		return v.(*omniav1alpha1.SessionPrivacyPolicySpec)
	}
	// Global
	if v, ok := pw.policies.Load(policyKey{}); ok {
		return v.(*omniav1alpha1.SessionPrivacyPolicySpec)
	}
	return nil
}

// sessionMetadata provides the namespace/agent for a session ID lookup.
type sessionMetadata struct {
	namespace string
	agent     string
}

// mockSessionLookup maps session IDs to their metadata (namespace + agent).
type mockSessionLookup struct {
	sessions map[string]sessionMetadata
}

func (m *mockSessionLookup) lookup(sessionID string) (string, string) {
	if meta, ok := m.sessions[sessionID]; ok {
		return meta.namespace, meta.agent
	}
	return "", ""
}

// capturedRequest stores the body that the downstream handler received.
type capturedRequest struct {
	body    []byte
	called  bool
	headers http.Header
}

// privacyMiddleware is the integration-test middleware that applies the full
// privacy pipeline: opt-out check, then PII redaction on the request body.
// This mirrors what the production PrivacyMiddleware would do when wired.
func privacyMiddleware(
	watcher *policyWatcher,
	lookupFn func(string) (string, string),
	prefStore PreferencesStore,
	redactor redaction.Redactor,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionID := r.Header.Get("X-Session-ID")
			namespace, agent := lookupFn(sessionID)

			// Step 1: Opt-out check
			userID := r.Header.Get(userIDHeader)
			if userID != "" && prefStore != nil {
				if !ShouldRecord(r.Context(), prefStore, userID, namespace, agent) {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}

			// Step 2: Resolve policy and apply PII redaction to body
			policy := watcher.resolve(namespace, agent)
			if policy == nil || policy.Recording.PII == nil || !policy.Recording.PII.Redact {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read body", http.StatusBadRequest)
				return
			}
			_ = r.Body.Close()

			redacted, _, err := redactor.Redact(r.Context(), string(body), policy.Recording.PII)
			if err != nil {
				http.Error(w, "redaction error", http.StatusInternalServerError)
				return
			}

			r.Body = io.NopCloser(bytes.NewReader([]byte(redacted)))
			r.ContentLength = int64(len(redacted))
			next.ServeHTTP(w, r)
		})
	}
}

// newCapturingHandler returns an HTTP handler that captures the request body
// and a pointer to the captured data.
func newCapturingHandler() (http.Handler, *capturedRequest) {
	cap := &capturedRequest{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.called = true
		cap.headers = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		cap.body = body
		w.WriteHeader(http.StatusCreated)
	})
	return handler, cap
}

// --- Helpers for building policies ---

func globalPIIPolicy(patterns []string) *omniav1alpha1.SessionPrivacyPolicySpec {
	return &omniav1alpha1.SessionPrivacyPolicySpec{
		Level: omniav1alpha1.PolicyLevelGlobal,
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:  true,
			RichData: true,
			PII: &omniav1alpha1.PIIConfig{
				Redact:   true,
				Patterns: patterns,
			},
		},
	}
}

func workspacePIIPolicy(patterns []string) *omniav1alpha1.SessionPrivacyPolicySpec {
	return &omniav1alpha1.SessionPrivacyPolicySpec{
		Level: omniav1alpha1.PolicyLevelWorkspace,
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:  true,
			RichData: true,
			PII: &omniav1alpha1.PIIConfig{
				Redact:   true,
				Patterns: patterns,
			},
		},
	}
}

func optOutPolicy() *omniav1alpha1.SessionPrivacyPolicySpec {
	return &omniav1alpha1.SessionPrivacyPolicySpec{
		Level: omniav1alpha1.PolicyLevelGlobal,
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:  true,
			RichData: true,
		},
		UserOptOut: &omniav1alpha1.UserOptOutConfig{
			Enabled: true,
		},
	}
}

// postJSON creates a test POST request with JSON body and session/user headers.
func postJSON(t *testing.T, url, body, sessionID, userID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("X-Session-ID", sessionID)
	}
	if userID != "" {
		req.Header.Set(userIDHeader, userID)
	}
	return req
}

// ============================================================================
// Test Suite 1: PII Redaction End-to-End
// ============================================================================

func TestPrivacyIntegration_MessageRedaction(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, globalPIIPolicy([]string{"ssn", "email"}))

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := `{"content":"My SSN is 123-45-6789 and email is user@test.com","role":"user"}`
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called, "downstream handler should be called")
	assert.Equal(t, http.StatusCreated, rec.Code)

	capturedBody := string(cap.body)
	assert.NotContains(t, capturedBody, "123-45-6789")
	assert.NotContains(t, capturedBody, "user@test.com")
	assert.Contains(t, capturedBody, "[REDACTED_SSN]")
	assert.Contains(t, capturedBody, "[REDACTED_EMAIL]")
}

func TestPrivacyIntegration_ToolCallRedaction(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, globalPIIPolicy([]string{"ssn", "email"}))

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := `{"name":"lookup","arguments":"SSN is 123-45-6789","result":"found user@test.com"}`
	req := postJSON(t, "/api/v1/sessions/sess-1/tool-calls", body, "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called)
	capturedBody := string(cap.body)
	assert.NotContains(t, capturedBody, "123-45-6789")
	assert.NotContains(t, capturedBody, "user@test.com")
	assert.Contains(t, capturedBody, "[REDACTED_SSN]")
	assert.Contains(t, capturedBody, "[REDACTED_EMAIL]")
}

func TestPrivacyIntegration_ProviderCallRedaction(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, globalPIIPolicy([]string{"ssn", "email"}))

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := `{"provider":"anthropic","model":"claude","request":"SSN 123-45-6789","response":"email user@test.com"}`
	req := postJSON(t, "/api/v1/sessions/sess-1/provider-calls", body, "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called)
	capturedBody := string(cap.body)
	assert.NotContains(t, capturedBody, "123-45-6789")
	assert.NotContains(t, capturedBody, "user@test.com")
	assert.Contains(t, capturedBody, "[REDACTED_SSN]")
	assert.Contains(t, capturedBody, "[REDACTED_EMAIL]")
}

func TestPrivacyIntegration_EventMetadataRedaction(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, globalPIIPolicy([]string{"email"}))

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	// Event with PII in metadata values (keys should be preserved).
	eventBody := map[string]any{
		"type": "custom.event",
		"metadata": map[string]string{
			"sender": "user@test.com",
			"action": "submitted",
		},
	}
	bodyBytes, _ := json.Marshal(eventBody)
	req := postJSON(t, "/api/v1/sessions/sess-1/events", string(bodyBytes), "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called)
	capturedBody := string(cap.body)
	assert.NotContains(t, capturedBody, "user@test.com")
	assert.Contains(t, capturedBody, "[REDACTED_EMAIL]")
	// Keys should be preserved
	assert.Contains(t, capturedBody, "sender")
	assert.Contains(t, capturedBody, "action")
}

func TestPrivacyIntegration_EvalResultRedaction(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, globalPIIPolicy([]string{"ssn", "email"}))

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	evalBody := map[string]any{
		"input":    "User SSN is 123-45-6789",
		"output":   "Contact user@test.com",
		"expected": "SSN 987-65-4321 and admin@test.com",
	}
	bodyBytes, _ := json.Marshal(evalBody)
	req := postJSON(t, "/api/v1/eval-results", string(bodyBytes), "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called)
	capturedBody := string(cap.body)
	assert.NotContains(t, capturedBody, "123-45-6789")
	assert.NotContains(t, capturedBody, "987-65-4321")
	assert.NotContains(t, capturedBody, "user@test.com")
	assert.NotContains(t, capturedBody, "admin@test.com")
	assert.Contains(t, capturedBody, "[REDACTED_SSN]")
	assert.Contains(t, capturedBody, "[REDACTED_EMAIL]")
}

// ============================================================================
// Test Suite 2: User Opt-Out End-to-End
// ============================================================================

func TestPrivacyIntegration_OptOutDropsWrite(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, optOutPolicy())

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	prefStore := &mockPreferencesStore{
		prefs: &Preferences{
			OptOutAll:        true,
			OptOutWorkspaces: []string{},
			OptOutAgents:     []string{},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, prefStore, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := bodyHello
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "user-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.False(t, cap.called, "downstream handler should NOT be called for opted-out user")
}

func TestPrivacyIntegration_OptOutNotTriggeredWithoutHeader(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, optOutPolicy())

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	prefStore := &mockPreferencesStore{
		prefs: &Preferences{
			OptOutAll:        true,
			OptOutWorkspaces: []string{},
			OptOutAgents:     []string{},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, prefStore, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	// No X-Omnia-User-ID header — opt-out should not trigger.
	body := bodyHello
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.True(t, cap.called, "downstream handler should be called when no user ID header")
}

func TestPrivacyIntegration_NonOptedOutUserPassesThrough(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, globalPIIPolicy([]string{"ssn"}))

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	// user-2 has NOT opted out.
	prefStore := &mockPreferencesStore{
		prefs: &Preferences{
			OptOutAll:        false,
			OptOutWorkspaces: []string{},
			OptOutAgents:     []string{},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, prefStore, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := bodyWithSSN
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "user-2")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	require.True(t, cap.called, "downstream handler should be called for non-opted-out user")

	// PII should still be redacted because a PII policy is active.
	capturedBody := string(cap.body)
	assert.NotContains(t, capturedBody, "123-45-6789")
	assert.Contains(t, capturedBody, "[REDACTED_SSN]")
}

// ============================================================================
// Test Suite 3: Policy Inheritance
// ============================================================================

func TestPrivacyIntegration_WorkspacePolicyOverridesGlobal(t *testing.T) {
	watcher := &policyWatcher{}

	// Global: no redaction.
	globalSpec := &omniav1alpha1.SessionPrivacyPolicySpec{
		Level: omniav1alpha1.PolicyLevelGlobal,
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:  true,
			RichData: true,
			PII: &omniav1alpha1.PIIConfig{
				Redact: false,
			},
		},
	}
	watcher.store(policyKey{}, globalSpec)

	// Workspace "my-ns": redaction enabled with SSN pattern.
	watcher.store(policyKey{namespace: "my-ns"}, workspacePIIPolicy([]string{"ssn"}))

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "my-ns", agent: "my-agent"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := bodyWithSSN
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called)
	capturedBody := string(cap.body)
	assert.NotContains(t, capturedBody, "123-45-6789", "workspace policy should override global and redact SSN")
	assert.Contains(t, capturedBody, "[REDACTED_SSN]")
}

func TestPrivacyIntegration_NoPolicyMeansNoRedaction(t *testing.T) {
	watcher := &policyWatcher{} // empty — no policies loaded

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := bodyWithSSN
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called)
	capturedBody := string(cap.body)
	assert.Contains(t, capturedBody, "123-45-6789", "without a policy, PII should pass through unchanged")
}

func TestPrivacyIntegration_OptOutWorkspaceScope(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, optOutPolicy())

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "workspace-a", agent: "my-agent"},
		},
	}

	// User opted out of workspace-a only.
	prefStore := &mockPreferencesStore{
		prefs: &Preferences{
			OptOutAll:        false,
			OptOutWorkspaces: []string{"workspace-a"},
			OptOutAgents:     []string{},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, prefStore, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := bodyHello
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "user-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.False(t, cap.called, "user opted out of this workspace")
}

func TestPrivacyIntegration_OptOutAgentScope(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, optOutPolicy())

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "agent-x"},
		},
	}

	// User opted out of agent-x only.
	prefStore := &mockPreferencesStore{
		prefs: &Preferences{
			OptOutAll:        false,
			OptOutWorkspaces: []string{},
			OptOutAgents:     []string{"agent-x"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, prefStore, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := bodyHello
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "user-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.False(t, cap.called, "user opted out of this agent")
}

func TestPrivacyIntegration_PreferencesNotFoundAllowsRecording(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, globalPIIPolicy([]string{"ssn"}))

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	// Store returns ErrPreferencesNotFound — user has no preferences set.
	prefStore := &mockPreferencesStore{err: ErrPreferencesNotFound}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, prefStore, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := bodyWithSSN
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "user-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	require.True(t, cap.called, "missing preferences should default to allowing recording")
	assert.NotContains(t, string(cap.body), "123-45-6789")
}

func TestPrivacyIntegration_MultiplePatterns(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, globalPIIPolicy([]string{"ssn", "email", "credit_card", "ip_address"}))

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := `{"content":"SSN 123-45-6789, card 4111-1111-1111-1111, email admin@corp.com, server 10.0.0.1"}`
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called)
	capturedBody := string(cap.body)
	assert.NotContains(t, capturedBody, "123-45-6789")
	assert.NotContains(t, capturedBody, "4111-1111-1111-1111")
	assert.NotContains(t, capturedBody, "admin@corp.com")
	assert.NotContains(t, capturedBody, "10.0.0.1")
	assert.Contains(t, capturedBody, "[REDACTED_SSN]")
	assert.Contains(t, capturedBody, "[REDACTED_CC]")
	assert.Contains(t, capturedBody, "[REDACTED_EMAIL]")
	assert.Contains(t, capturedBody, "[REDACTED_IP]")
}

func TestPrivacyIntegration_RedactionPreservesJSONStructure(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, globalPIIPolicy([]string{"ssn"}))

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	input := map[string]string{
		"content": "My SSN is 123-45-6789",
		"role":    "user",
	}
	bodyBytes, _ := json.Marshal(input)
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", string(bodyBytes), "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called)

	// The redacted body should still be valid JSON
	var parsed map[string]string
	err := json.Unmarshal(cap.body, &parsed)
	require.NoError(t, err, "redacted body should remain valid JSON")
	assert.Equal(t, "user", parsed["role"])
	assert.Contains(t, parsed["content"], "[REDACTED_SSN]")
	assert.NotContains(t, parsed["content"], "123-45-6789")
}

func TestPrivacyIntegration_AgentPolicyOverridesWorkspace(t *testing.T) {
	watcher := &policyWatcher{}

	// Workspace: no redaction
	wsSpec := &omniav1alpha1.SessionPrivacyPolicySpec{
		Level: omniav1alpha1.PolicyLevelWorkspace,
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:  true,
			RichData: true,
			PII: &omniav1alpha1.PIIConfig{
				Redact: false,
			},
		},
	}
	watcher.store(policyKey{namespace: "ns-1"}, wsSpec)

	// Agent: redaction enabled
	agentSpec := &omniav1alpha1.SessionPrivacyPolicySpec{
		Level: omniav1alpha1.PolicyLevelAgent,
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:  true,
			RichData: true,
			PII: &omniav1alpha1.PIIConfig{
				Redact:   true,
				Patterns: []string{"email"},
			},
		},
	}
	watcher.store(policyKey{namespace: "ns-1", agent: "agent-1"}, agentSpec)

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "ns-1", agent: "agent-1"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := `{"content":"Contact admin@corp.com"}`
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called)
	capturedBody := string(cap.body)
	assert.NotContains(t, capturedBody, "admin@corp.com")
	assert.Contains(t, capturedBody, "[REDACTED_EMAIL]")
}

func TestPrivacyIntegration_OptOutCombinedWithRedaction(t *testing.T) {
	watcher := &policyWatcher{}
	// Policy has both PII redaction and opt-out enabled.
	spec := &omniav1alpha1.SessionPrivacyPolicySpec{
		Level: omniav1alpha1.PolicyLevelGlobal,
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:  true,
			RichData: true,
			PII: &omniav1alpha1.PIIConfig{
				Redact:   true,
				Patterns: []string{"ssn"},
			},
		},
		UserOptOut: &omniav1alpha1.UserOptOutConfig{
			Enabled: true,
		},
	}
	watcher.store(policyKey{}, spec)

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	// User is opted out — should get 204 regardless of redaction.
	prefStore := &mockPreferencesStore{
		prefs: &Preferences{
			OptOutAll:        true,
			OptOutWorkspaces: []string{},
			OptOutAgents:     []string{},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, prefStore, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := bodyWithSSN
	req := postJSON(t, "/api/v1/sessions/sess-1/messages", body, "sess-1", "user-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.False(t, cap.called, "opted-out user should not reach downstream even with redaction active")
}

func TestPrivacyIntegration_EmptyBodyPassesThrough(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, globalPIIPolicy([]string{"ssn"}))

	lookup := &mockSessionLookup{
		sessions: map[string]sessionMetadata{
			"sess-1": {namespace: "default", agent: "my-agent"},
		},
	}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	req := postJSON(t, "/api/v1/sessions/sess-1/messages", "", "sess-1", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called)
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Empty(t, cap.body)
}

func TestPrivacyIntegration_UnknownSessionPassesThrough(t *testing.T) {
	watcher := &policyWatcher{}
	watcher.store(policyKey{}, globalPIIPolicy([]string{"ssn"}))

	// No sessions registered in lookup — will return empty namespace/agent.
	// Global policy should still apply.
	lookup := &mockSessionLookup{sessions: map[string]sessionMetadata{}}

	redactor := redaction.NewRedactor()
	mw := privacyMiddleware(watcher, lookup.lookup, nil, redactor)
	downstream, cap := newCapturingHandler()
	handler := mw(downstream)

	body := `{"content":"SSN is 123-45-6789"}`
	req := postJSON(t, "/api/v1/sessions/unknown-sess/messages", body, "unknown-sess", "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, cap.called)
	// Global policy should still apply even for unknown sessions.
	capturedBody := string(cap.body)
	assert.NotContains(t, capturedBody, "123-45-6789")
	assert.Contains(t, capturedBody, "[REDACTED_SSN]")
}
