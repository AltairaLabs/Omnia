/*
Copyright 2025.

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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/identity"
	"github.com/altairalabs/omnia/pkg/policy"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// testFunctionName is reused across handler tests. Extracted to satisfy
// goconst (string was duplicated ~14 times).
const testFunctionName = "echo"

// stubInvoker captures the runtime call inputs and returns a fixed
// response or error. The struct's exported fields are read-only after
// the handler returns so test assertions can inspect them.
type stubInvoker struct {
	resp *runtimev1.InvocationResponse
	err  error

	lastReq *runtimev1.InvocationRequest
}

func (s *stubInvoker) Invoke(_ context.Context, req *runtimev1.InvocationRequest, _ ...grpc.CallOption) (*runtimev1.InvocationResponse, error) {
	s.lastReq = req
	return s.resp, s.err
}

func newHandler(t *testing.T, specs map[string]*FunctionSpec, invoker InvocationInvoker) *FunctionsHandler {
	t.Helper()
	reg := NewMapFunctionRegistry()
	for _, s := range specs {
		reg.Register(s)
	}
	return NewFunctionsHandler(reg, invoker, logr.Discard())
}

func newJSONPost(t *testing.T, path, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// muxFor mounts the handler under POST /functions/{name} so PathValue
// resolution works exactly as it will in production.
func muxFor(h *FunctionsHandler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /functions/{name}", h)
	mux.Handle("/functions/{name}", h) // also serve non-POST to allow 405 testing
	return mux
}

func TestFunctionsHandler_HappyPath(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object","required":["q"]}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object","required":["a"]}`))
	require.NoError(t, err)

	// Set the runtime stub to echo a DIFFERENT invocation_id (or even an
	// empty one) so the test pins down that the response carries the
	// facade's authoritative id, not whatever the runtime sends back.
	invoker := &stubInvoker{
		resp: &runtimev1.InvocationResponse{
			OutputJson:   `{"a":"42"}`,
			DurationMs:   17,
			InvocationId: "runtime-tried-to-override",
			Usage:        &runtimev1.Usage{InputTokens: 12, OutputTokens: 3, CostUsd: 0.0004},
		},
	}
	h := newHandler(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{"q":"hi"}`))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, map[string]any{"a": "42"}, body["output"])
	assert.Equal(t, float64(17), body["duration_ms"])

	responseID, ok := body["invocation_id"].(string)
	require.True(t, ok, "invocation_id must be a string")
	assert.NotEmpty(t, responseID, "facade must generate an invocation_id")
	assert.NotEqual(t, "runtime-tried-to-override", responseID,
		"facade is authoritative on invocation_id; runtime's echo must not leak into the response")

	require.NotNil(t, invoker.lastReq)
	assert.Equal(t, `{"q":"hi"}`, invoker.lastReq.GetInputJson())
	// Round-trip invariant: the id the facade sent to the runtime must be
	// the same id it returned to the caller.
	assert.Equal(t, responseID, invoker.lastReq.GetInvocationId(),
		"facade must send the same invocation_id to the runtime that it returns to the caller")
}

func TestFunctionsHandler_AcceptsApplicationJSONWithCharsetSuffix(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	invoker := &stubInvoker{
		resp: &runtimev1.InvocationResponse{OutputJson: `{}`},
	}
	h := newHandler(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	req := httptest.NewRequest(http.MethodPost, "/functions/"+testFunctionName, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"Content-Type with a charset parameter must be accepted")
}

func TestFunctionsHandler_InputValidationFails(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object","required":["q"]}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	invoker := &stubInvoker{} // should not be called
	h := newHandler(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	// Missing the required "q" field.
	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{}`))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Nil(t, invoker.lastReq, "runtime must NOT be called when input is invalid")

	var resp errorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "input_invalid", resp.Error)
}

func TestFunctionsHandler_OutputValidationFails_502WithRawOutput(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object","required":["a"]}`))
	require.NoError(t, err)

	// Model returned valid JSON but missing the "a" field — schema fail.
	invoker := &stubInvoker{
		resp: &runtimev1.InvocationResponse{
			OutputJson: `{"wrong":"shape"}`,
		},
	}
	h := newHandler(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{}`))

	require.Equal(t, http.StatusBadGateway, rec.Code)
	var resp errorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "output_invalid", resp.Error)

	// The raw model output must be embedded so the function author can
	// debug what the pack actually emitted. Per #1103 Q2.
	require.NotNil(t, resp.RawOutput)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(resp.RawOutput, &raw))
	assert.Equal(t, map[string]any{"wrong": "shape"}, raw)
}

func TestFunctionsHandler_OutputValidationFails_NonJSONRawOutput(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	// Model returned a plain string, not JSON. Validator should reject;
	// raw output should still be visible (as a JSON string) in the body.
	invoker := &stubInvoker{
		resp: &runtimev1.InvocationResponse{
			OutputJson: `not actually json`,
		},
	}
	h := newHandler(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{}`))

	require.Equal(t, http.StatusBadGateway, rec.Code)
	var resp errorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "output_invalid", resp.Error)
	require.NotNil(t, resp.RawOutput)
	var asString string
	require.NoError(t, json.Unmarshal(resp.RawOutput, &asString))
	assert.Equal(t, "not actually json", asString)
}

func TestFunctionsHandler_RuntimeError(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	invoker := &stubInvoker{err: errors.New("simulated upstream failure")}
	h := newHandler(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{}`))

	require.Equal(t, http.StatusBadGateway, rec.Code)
	var resp errorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "runtime_error", resp.Error)
	assert.Equal(t, "runtime invocation failed", resp.Detail)
}

func TestFunctionsHandler_UnknownFunctionIs404(t *testing.T) {
	h := newHandler(t, map[string]*FunctionSpec{}, &stubInvoker{})

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/missing", `{}`))

	require.Equal(t, http.StatusNotFound, rec.Code)
	var resp errorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "function_not_found", resp.Error)
}

func TestFunctionsHandler_RejectsNonPOST(t *testing.T) {
	h := newHandler(t, map[string]*FunctionSpec{}, &stubInvoker{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/functions/echo", nil)
	muxFor(h).ServeHTTP(rec, req)

	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, http.MethodPost, rec.Header().Get("Allow"))
}

func TestFunctionsHandler_RejectsWrongContentType(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	h := newHandler(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, &stubInvoker{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/functions/echo", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	muxFor(h).ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestFunctionsHandler_BodyTooLarge(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	h := newHandler(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, &stubInvoker{})
	// Tighten the cap so we don't have to build a 1 MiB body.
	h.maxBodyBytes = 32

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName,
		`{"q":"`+strings.Repeat("x", 64)+`"}`))

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestFunctionsHandler_BlankPathValueIs400(t *testing.T) {
	// Defensive: production mounts the handler under "POST /functions/{name}"
	// so a blank PathValue is impossible in practice. This test pins the
	// handler's own guard against a misconfigured mux that hands it a
	// request with no {name} bound.
	h := newHandler(t, map[string]*FunctionSpec{}, &stubInvoker{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/functions/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMapFunctionRegistry_RegisterIgnoresInvalidSpecs(t *testing.T) {
	reg := NewMapFunctionRegistry()

	// nil spec is silently dropped.
	reg.Register(nil)
	// empty Name is silently dropped.
	reg.Register(&FunctionSpec{Name: ""})

	if _, ok := reg.GetFunction(""); ok {
		t.Errorf("registry must not store a spec with empty Name")
	}
	// A valid registration still works after the bad ones — verify the
	// silent-drop branches didn't poison the registry's internal map.
	reg.Register(&FunctionSpec{Name: testFunctionName})
	if _, ok := reg.GetFunction(testFunctionName); !ok {
		t.Errorf("registry must store valid specs after silent-dropped invalid ones")
	}
}

// stubSessionStore captures the calls the facade makes against the
// session store so tests can assert the session lifecycle. The
// methods the facade doesn't touch are left to delegate to the
// embedded Store; that field is nil so any unexpected call panics —
// which is what we want as a guardrail.
type stubSessionStore struct {
	session.Store

	mu        sync.Mutex
	creates   []session.SessionRecordOptions
	events    []session.RuntimeEvent
	statuses  []session.SessionStatusUpdate
	createErr error
	updateErr error
	eventErr  error
}

func (s *stubSessionStore) EnsureSessionRecord(_ context.Context, opts session.SessionRecordOptions) (*session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates = append(s.creates, opts)
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &session.Session{ID: opts.ID}, nil
}

func (s *stubSessionStore) UpdateSessionStatus(_ context.Context, _ string, update session.SessionStatusUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses = append(s.statuses, update)
	return s.updateErr
}

func (s *stubSessionStore) RecordRuntimeEvent(_ context.Context, _ string, evt session.RuntimeEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, evt)
	return s.eventErr
}

func newHandlerWithSession(
	t *testing.T,
	specs map[string]*FunctionSpec,
	invoker InvocationInvoker,
) (*FunctionsHandler, *stubSessionStore) {
	t.Helper()
	store := &stubSessionStore{}
	h := newHandler(t, specs, invoker).WithSessionStore(store, FunctionSessionMeta{
		Namespace:         "ns-test",
		AgentName:         "summarizer",
		WorkspaceName:     "ws-test",
		PromptPackName:    "summarizer-pack",
		PromptPackVersion: "1.0.0",
	})
	return h, store
}

func TestFunctionsHandler_Session_OpenedAndClosedOnHappyPath(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	invoker := &stubInvoker{resp: &runtimev1.InvocationResponse{OutputJson: `{}`, DurationMs: 5}}
	h, store := newHandlerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{}`))
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, store.creates, 1, "session must be opened exactly once")
	opts := store.creates[0]
	assert.Equal(t, "ns-test", opts.Namespace)
	assert.Equal(t, "summarizer", opts.AgentName)
	assert.Equal(t, "ws-test", opts.WorkspaceName)
	assert.Equal(t, "summarizer-pack", opts.PromptPackName)
	assert.Equal(t, "1.0.0", opts.PromptPackVersion)
	assert.Equal(t, []string{FunctionSessionTag}, opts.Tags)
	assert.NotEmpty(t, opts.ID, "session id must be populated up-front so input_invalid still has a correlation key")

	require.Len(t, store.statuses, 1, "session must be closed exactly once")
	assert.Equal(t, session.SessionStatusCompleted, store.statuses[0].SetStatus)
	assert.False(t, store.statuses[0].SetEndedAt.IsZero(), "ended_at must be populated on close")
	assert.Empty(t, store.events, "no failure events on the happy path")
}

func TestFunctionsHandler_Session_InputInvalidClosesAsError(t *testing.T) {
	// input_invalid is the critical case: the runtime never runs, but
	// the session row must still exist (with status=error + a runtime
	// event carrying the validator error) so operators can pivot from
	// the Loki log line back to a row in the dashboard.
	inputSchema, err := CompileSchema([]byte(`{"type":"object","required":["q"]}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	invoker := &stubInvoker{} // must not be called
	h, store := newHandlerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{}`))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Nil(t, invoker.lastReq, "runtime must NOT be called when input is invalid")

	require.Len(t, store.creates, 1)
	require.Len(t, store.statuses, 1)
	assert.Equal(t, session.SessionStatusError, store.statuses[0].SetStatus)

	require.Len(t, store.events, 1, "input_invalid must emit a failure runtime event")
	assert.Equal(t, "function.input_invalid", store.events[0].EventType)
	assert.NotEmpty(t, store.events[0].ErrorMessage)
}

func TestFunctionsHandler_Session_RuntimeErrorClosesAsError(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	invoker := &stubInvoker{err: errors.New("upstream down")}
	h, store := newHandlerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{}`))
	require.Equal(t, http.StatusBadGateway, rec.Code)

	require.Len(t, store.statuses, 1)
	assert.Equal(t, session.SessionStatusError, store.statuses[0].SetStatus)
	require.Len(t, store.events, 1)
	assert.Equal(t, "function.runtime_error", store.events[0].EventType)
	assert.Contains(t, store.events[0].ErrorMessage, "upstream down")
}

func TestFunctionsHandler_Session_OutputInvalidClosesAsError(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object","required":["a"]}`))
	require.NoError(t, err)

	invoker := &stubInvoker{resp: &runtimev1.InvocationResponse{OutputJson: `{"wrong":"shape"}`}}
	h, store := newHandlerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{}`))
	require.Equal(t, http.StatusBadGateway, rec.Code)

	require.Len(t, store.statuses, 1)
	assert.Equal(t, session.SessionStatusError, store.statuses[0].SetStatus)
	require.Len(t, store.events, 1)
	assert.Equal(t, "function.output_invalid", store.events[0].EventType)
}

func TestFunctionsHandler_Session_StoreFailuresAreNonFatal(t *testing.T) {
	// A store outage must not break the user-facing call — the runtime
	// still produces a result, the audit row just ends up orphaned.
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	invoker := &stubInvoker{resp: &runtimev1.InvocationResponse{OutputJson: `{}`}}
	h, store := newHandlerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)
	store.createErr = errors.New("session-api down")
	store.updateErr = errors.New("session-api still down")

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{}`))
	assert.Equal(t, http.StatusOK, rec.Code,
		"store failure must not fail the user-facing request")
}

func TestFunctionsHandler_Session_VirtualUserFromIdentity(t *testing.T) {
	// A request whose context carries an authenticated identity attributes the
	// function session to that identity's EndUser pseudonym. The function path
	// resolves the virtual user from the context identity inside
	// FunctionInvoker; the request-level on-behalf-of header precedence
	// (ResolveUserPseudonym) applies to the WS path, not the function path.
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	invoker := &stubInvoker{resp: &runtimev1.InvocationResponse{OutputJson: `{}`}}
	h, store := newHandlerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	req := newJSONPost(t, "/functions/"+testFunctionName, `{}`)
	id := &policy.AuthenticatedIdentity{Origin: policy.OriginManagementPlane, EndUser: "operator@example.com"}
	req = req.WithContext(policy.WithIdentity(req.Context(), id))

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, store.creates, 1)
	assert.Equal(t, identity.PseudonymizeID("operator@example.com"), store.creates[0].VirtualUserID,
		"function session must be attributed to the context identity's EndUser pseudonym")
}

func TestFunctionsHandler_Session_VirtualUserAnonymousFallback(t *testing.T) {
	// No identity in context, no device_id → each invocation is its own
	// virtual user, seeded by the invocation id. Must be non-empty so the
	// NOT-NULL create never 400s.
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	invoker := &stubInvoker{resp: &runtimev1.InvocationResponse{OutputJson: `{}`}}
	h, store := newHandlerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{}`))
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, store.creates, 1)
	opts := store.creates[0]
	assert.NotEmpty(t, opts.VirtualUserID, "anonymous invocations must still get a non-empty virtual user id")
	assert.Equal(t, identity.PseudonymizeID(opts.ID), opts.VirtualUserID,
		"anonymous fallback pseudonymizes the invocation id")
}

func TestFunctionsHandler_Session_TransientWhenStoreUnwired(t *testing.T) {
	// No WithSessionStore call → transient mode (used by tests today).
	// The handler must still serve the call cleanly, just without any
	// audit-row writes.
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	invoker := &stubInvoker{resp: &runtimev1.InvocationResponse{OutputJson: `{}`}}
	h := newHandler(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, invoker)

	rec := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rec, newJSONPost(t, "/functions/"+testFunctionName, `{}`))
	require.Equal(t, http.StatusOK, rec.Code)
}
