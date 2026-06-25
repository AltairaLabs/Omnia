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

// A rich write under richData:false is dropped: the metric counts every drop,
// but the warning is emitted once per agent+reason (not per write).
func TestPrivacyMiddleware_RichDataDrop_CountsEveryDrop_WarnsOnce(t *testing.T) {
	sink := &captureSink{}
	mw := dropTestMiddleware(t, omniav1alpha1.RecordingConfig{Enabled: true, RichData: false}, "agent-rich-drop", sink)

	before := testutil.ToFloat64(writesDropped.WithLabelValues(dropReasonRichDataDisabled))
	for i := 0; i < 3; i++ {
		postDrop(t, mw, "/api/v1/sessions/abc-123/provider-calls")
	}
	after := testutil.ToFloat64(writesDropped.WithLabelValues(dropReasonRichDataDisabled))

	assert.Equal(t, float64(3), after-before, "metric increments on every dropped write")
	assert.Equal(t, 1, sink.count("session write dropped by privacy policy"),
		"warning deduped to once per agent+reason")
	assert.Equal(t, 1, sink.count("rich-data-disabled"), "warning carries the reason")
	assert.Equal(t, 1, sink.count("agent-rich-drop"), "warning carries the agent")
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
