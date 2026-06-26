/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/session"
)

// captureSink records Info/Error log lines (message + flattened key/values) so
// tests can assert on the drop warning and its dedup.
type captureSink struct {
	mu    sync.Mutex
	lines []string
}

func (s *captureSink) Init(logr.RuntimeInfo)          {}
func (s *captureSink) Enabled(int) bool               { return true }
func (s *captureSink) WithName(string) logr.LogSink   { return s }
func (s *captureSink) WithValues(...any) logr.LogSink { return s }

func (s *captureSink) record(msg string, kv ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, msg+" "+fmt.Sprint(kv...))
}

func (s *captureSink) Info(_ int, msg string, kv ...any)    { s.record(msg, kv...) }
func (s *captureSink) Error(_ error, msg string, kv ...any) { s.record(msg, kv...) }

func (s *captureSink) count(substr string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, l := range s.lines {
		if strings.Contains(l, substr) {
			n++
		}
	}
	return n
}

// systemNamespace is where the global-default SessionPrivacyPolicy lives.
const systemNamespace = "omnia-system"

func dropTestMiddleware(
	t *testing.T, rec omniav1alpha1.RecordingConfig, agent string, sink logr.LogSink,
) *PrivacyMiddleware {
	t.Helper()
	lookup := &mockSessionLookup{ns: "ns-drop", agent: agent}
	cache := NewSessionMetadataCache(lookup, 100)
	watcher := &PolicyWatcher{}
	watcher.policies.Store(systemNamespace+"/"+testNamespace, &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: testNamespace, Namespace: systemNamespace},
		Spec:       omniav1alpha1.SessionPrivacyPolicySpec{Recording: rec},
	})
	return NewPrivacyMiddleware(watcher, cache, nil, nil, logr.New(sink))
}

func postDrop(t *testing.T, mw *PrivacyMiddleware, path string) {
	t.Helper()
	called := false
	h := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"role":"assistant","content":"x"}`))
	rr := httptest.NewRecorder()
	mw.Wrap(h).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code, "a policy-dropped write returns 204")
	assert.False(t, called, "the next handler must not run on a dropped write")
}

// An assistant message under runtimeData:false is dropped: the metric counts
// every drop, but the warning is emitted once per agent+reason (not per write).
func TestPrivacyMiddleware_RuntimeContentDrop_CountsEveryDrop_WarnsOnce(t *testing.T) {
	sink := &captureSink{}
	mw := dropTestMiddleware(t, omniav1alpha1.RecordingConfig{Enabled: true, RuntimeData: false}, "agent-rt-drop", sink)

	before := testutil.ToFloat64(writesDropped.WithLabelValues(dropReasonRuntimeData))
	for i := 0; i < 3; i++ {
		postDrop(t, mw, "/api/v1/sessions/abc-123/messages")
	}
	after := testutil.ToFloat64(writesDropped.WithLabelValues(dropReasonRuntimeData))

	assert.Equal(t, float64(3), after-before, "metric increments on every dropped write")
	assert.Equal(t, 1, sink.count("session write dropped by privacy policy"),
		"warning deduped to once per agent+reason")
	assert.Equal(t, 1, sink.count("runtime-data-disabled"), "warning carries the reason")
	assert.Equal(t, 1, sink.count("agent-rt-drop"), "warning carries the agent")
}

// X-Omnia-Source gates message content by emitter: facade-sourced content (user
// turns) is always recorded; runtime-sourced content (assistant turns) requires
// runtimeData.
func TestPrivacyMiddleware_SourceHeaderGatesMessageContent(t *testing.T) {
	mw := dropTestMiddleware(t,
		omniav1alpha1.RecordingConfig{Enabled: true, RuntimeData: false}, "agent-src", &captureSink{})

	cases := []struct {
		name    string
		source  string
		allowed bool
	}{
		{"facade content always allowed", session.SourceFacade, true},
		{"runtime content gated off", session.SourceRuntime, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusCreated)
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/abc-123/messages",
				strings.NewReader(`{"role":"assistant","content":"x"}`))
			req.Header.Set(session.SourceHeader, tc.source)
			rr := httptest.NewRecorder()
			mw.Wrap(h).ServeHTTP(rr, req)
			assert.Equal(t, tc.allowed, called)
		})
	}
}

// Eval results post to /api/v1/eval-results (not session-scoped), so the privacy
// middleware never intercepts them — they flow regardless of policy.
func TestPrivacyMiddleware_EvalResultsNotGated(t *testing.T) {
	mw := dropTestMiddleware(t,
		omniav1alpha1.RecordingConfig{Enabled: true, RuntimeData: false}, "agent-eval", &captureSink{})
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval-results", strings.NewReader(`{"evalId":"e1"}`))
	rr := httptest.NewRecorder()
	mw.Wrap(h).ServeHTTP(rr, req)
	assert.True(t, called, "eval-results must flow regardless of privacy policy")
}

// Metering and structured records are NEVER gated by runtimeData — only message
// content is. Provider calls (cost/tokens), tool calls and events must flow
// through even when runtimeData:false.
func TestPrivacyMiddleware_MeteringNotGatedByRuntimeData(t *testing.T) {
	sink := &captureSink{}
	mw := dropTestMiddleware(t, omniav1alpha1.RecordingConfig{Enabled: true, RuntimeData: false}, "agent-metering", sink)

	for _, path := range []string{
		"/api/v1/sessions/abc-123/provider-calls",
		"/api/v1/sessions/abc-123/tool-calls",
		"/api/v1/sessions/abc-123/events",
	} {
		called := false
		h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusCreated)
		})
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"id":"x"}`))
		rr := httptest.NewRecorder()
		mw.Wrap(h).ServeHTTP(rr, req)
		assert.True(t, called, "metering/structured write must reach the handler: "+path)
		assert.NotEqual(t, http.StatusNoContent, rr.Code, "must not be dropped: "+path)
	}
	assert.Equal(t, 0, sink.count("session write dropped"), "no drops for metering/structured records")
}

// Recording disabled drops everything (including user messages) and warns.
func TestPrivacyMiddleware_RecordingDisabledDrop_Warns(t *testing.T) {
	sink := &captureSink{}
	mw := dropTestMiddleware(t, omniav1alpha1.RecordingConfig{Enabled: false}, "agent-rec-off", sink)

	before := testutil.ToFloat64(writesDropped.WithLabelValues(dropReasonRecordingDisabled))
	postDrop(t, mw, "/api/v1/sessions/abc-123/messages")
	after := testutil.ToFloat64(writesDropped.WithLabelValues(dropReasonRecordingDisabled))

	assert.Equal(t, float64(1), after-before)
	assert.Equal(t, 1, sink.count("recording-disabled"))
	assert.Equal(t, 1, sink.count("session write dropped by privacy policy"))
}
