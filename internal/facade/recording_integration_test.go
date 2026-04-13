/*
Copyright 2025-2026.

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

package facade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/httpclient"
)

// fakePrivacyAPI returns an httptest server that responds to
// GET /api/v1/privacy-policy with the supplied body and status. It also
// counts how many times the endpoint was hit so tests can assert the cache
// was actually consulted.
func fakePrivacyAPI(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/privacy-policy" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		require.Equal(t, "ns-1", r.URL.Query().Get("namespace"))
		require.Equal(t, "agent-1", r.URL.Query().Get("agent"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// newWiredRecordingWriter assembles the production wiring path: a real
// httpclient.Store talking to the fake session-api, a real
// RecordingPolicyCache, a real recordingResponseWriter, with the policy set
// from the cache exactly the way session.go does it.
func newWiredRecordingWriter(t *testing.T, sessionAPIURL string) (
	*recordingResponseWriter, *mockResponseWriter, session.Store, string,
) {
	t.Helper()

	// Real httpclient.Store pointed at the fake session-api.
	store := httpclient.NewStore(sessionAPIURL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	// Real session store for AppendMessage assertions. The recording writer
	// records into this store; the policy fetch happens via the httpclient
	// against the fake session-api.
	recStore := session.NewMemoryStore()
	t.Cleanup(func() { _ = recStore.Close() })

	ctx := context.Background()
	sess, err := recStore.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "agent-1", Namespace: "ns-1",
	})
	require.NoError(t, err)

	// Real cache built from the real PolicyFetcher (the httpclient.Store).
	cache := NewRecordingPolicyCache(store, "ns-1", "agent-1", 60*time.Second, logr.Discard())

	// Real recording writer with mock inner (so we don't need a websocket).
	inner := &mockResponseWriter{}
	rw := newRecordingWriter(ctx, inner, recStore, sess.ID, logr.Discard(), nil)

	// Production path in session.go: fetch from cache, hand to writer.
	rw.setPolicy(cache.Get(ctx))

	_ = cache // returned policy already applied; cache lifetime tied to writer
	return rw, inner, recStore, sess.ID
}

// drainMessages returns the messages currently stored for the given session.
func drainMessages(t *testing.T, store session.Store, sessionID string) []session.Message {
	t.Helper()
	msgs, err := store.GetMessages(context.Background(), sessionID)
	require.NoError(t, err)
	return msgs
}

// TestRecordingIntegration_RecordingDisabled_GatesAll verifies that when the
// session-api reports Recording.Enabled=false, the full chain
// (httpclient -> RecordingPolicyCache -> recordingResponseWriter) suppresses
// AppendMessage calls. This catches regressions where one of the layers
// exists but is not wired (e.g., setPolicy never called, cache never queried).
func TestRecordingIntegration_RecordingDisabled_GatesAll(t *testing.T) {
	srv, hits := fakePrivacyAPI(t, http.StatusOK,
		`{"recording":{"enabled":false,"facadeData":false,"richData":false}}`)

	rw, inner, recStore, sessID := newWiredRecordingWriter(t, srv.URL)

	require.NoError(t, rw.WriteDone("hello"))
	require.NoError(t, rw.WriteToolCall(&ToolCallInfo{
		ID: "tc-1", Name: "search", Arguments: map[string]interface{}{"q": "x"},
	}))
	require.NoError(t, rw.WriteToolResult(&ToolResultInfo{ID: "tc-1", Result: "ok"}))
	require.NoError(t, rw.WriteError("ERR", "boom"))

	waitForAsyncWrites()

	// Inner writer (= the WebSocket) still saw everything — gating is on
	// recording only, not on user-visible delivery.
	assert.Equal(t, "hello", inner.doneContent)
	assert.Len(t, inner.toolCalls, 1)
	assert.Len(t, inner.toolResults, 1)
	assert.Len(t, inner.errors, 1)

	msgs := drainMessages(t, recStore, sessID)
	assert.Empty(t, msgs, "no messages should be appended when Recording.Enabled=false")

	assert.Equal(t, int32(1), hits.Load(), "policy should have been fetched exactly once")
}

// TestRecordingIntegration_RichDataDisabled_GatesContent verifies that when
// Recording.RichData=false, user/assistant content and tool calls are dropped
// but error messages still record (compliance/audit requirement).
func TestRecordingIntegration_RichDataDisabled_GatesContent(t *testing.T) {
	srv, _ := fakePrivacyAPI(t, http.StatusOK,
		`{"recording":{"enabled":true,"facadeData":true,"richData":false}}`)

	rw, _, recStore, sessID := newWiredRecordingWriter(t, srv.URL)

	require.NoError(t, rw.WriteDone("assistant content"))
	require.NoError(t, rw.WriteToolCall(&ToolCallInfo{
		ID: "tc-1", Name: "search", Arguments: map[string]interface{}{"q": "x"},
	}))
	require.NoError(t, rw.WriteToolResult(&ToolResultInfo{ID: "tc-1", Result: "ok"}))
	require.NoError(t, rw.WriteError("ERR", "boom"))

	waitForAsyncWrites()

	msgs := drainMessages(t, recStore, sessID)
	// Only the error message should have been recorded.
	require.Len(t, msgs, 1, "only error message should record when RichData=false")
	assert.Equal(t, "error", msgs[0].Metadata["type"])
}

// TestRecordingIntegration_FetchError_DefaultsToRecordingEnabled verifies the
// fail-open contract: when the session-api returns 500 for the policy fetch,
// the cache must yield a default-enabled policy and recording must continue
// as normal. This is the safety net that prevents transient session-api
// outages from silently dropping all session data.
func TestRecordingIntegration_FetchError_DefaultsToRecordingEnabled(t *testing.T) {
	srv, hits := fakePrivacyAPI(t, http.StatusInternalServerError, "")

	rw, _, recStore, sessID := newWiredRecordingWriter(t, srv.URL)

	require.NoError(t, rw.WriteDone("hello"))

	waitForAsyncWrites()

	msgs := drainMessages(t, recStore, sessID)
	require.Len(t, msgs, 1, "fetch error should fail open and record normally")
	assert.Equal(t, "hello", msgs[0].Content)
	assert.Equal(t, session.RoleAssistant, msgs[0].Role)

	// Hits should be > 0 (at least one fetch attempt was made — possibly more
	// due to httpclient retry policy on 5xx).
	assert.GreaterOrEqual(t, hits.Load(), int32(1),
		"policy fetch should have been attempted at least once")
}
