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
	"errors"
	"sync"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/identity"
	"github.com/altairalabs/omnia/pkg/policy"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// wrongShapeJSON is a JSON object that intentionally fails the output schema
// (which requires "a") so tests can exercise the output-invalid path.
const wrongShapeJSON = `{"wrong":"shape"}`

// invokerStub is a minimal InvocationInvoker for invoker unit tests.
type invokerStub struct {
	resp    *runtimev1.InvocationResponse
	err     error
	lastReq *runtimev1.InvocationRequest
}

func (s *invokerStub) Invoke(_ context.Context, req *runtimev1.InvocationRequest, _ ...grpc.CallOption) (*runtimev1.InvocationResponse, error) {
	s.lastReq = req
	return s.resp, s.err
}

// invokerSessionStore records session lifecycle calls for assertion.
type invokerSessionStore struct {
	session.Store
	mu        sync.Mutex
	creates   []session.SessionRecordOptions
	events    []session.RuntimeEvent
	statuses  []session.SessionStatusUpdate
	createErr error
	updateErr error
	eventErr  error
}

func (s *invokerSessionStore) EnsureSessionRecord(_ context.Context, opts session.SessionRecordOptions) (*session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates = append(s.creates, opts)
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &session.Session{ID: opts.ID}, nil
}

func (s *invokerSessionStore) UpdateSessionStatus(_ context.Context, _ string, update session.SessionStatusUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses = append(s.statuses, update)
	return s.updateErr
}

func (s *invokerSessionStore) RecordRuntimeEvent(_ context.Context, _ string, evt session.RuntimeEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, evt)
	return s.eventErr
}

// newTestInvoker builds a FunctionInvoker using the given specs and runtime stub.
func newTestInvoker(t *testing.T, specs map[string]*FunctionSpec, rts *invokerStub) *FunctionInvoker {
	t.Helper()
	reg := NewMapFunctionRegistry()
	for _, s := range specs {
		reg.Register(s)
	}
	return NewFunctionInvoker(FunctionInvokerConfig{
		Registry: reg,
		Invoker:  rts,
		Log:      testr.New(t),
	})
}

// newTestInvokerWithSession builds a FunctionInvoker with a session store attached.
func newTestInvokerWithSession(
	t *testing.T,
	specs map[string]*FunctionSpec,
	rts *invokerStub,
	store *invokerSessionStore,
) *FunctionInvoker {
	t.Helper()
	reg := NewMapFunctionRegistry()
	for _, s := range specs {
		reg.Register(s)
	}
	return NewFunctionInvoker(FunctionInvokerConfig{
		Registry:     reg,
		Invoker:      rts,
		SessionStore: store,
		SessionMeta: FunctionSessionMeta{
			Namespace:         "ns-test",
			AgentName:         "echo-fn",
			WorkspaceName:     "ws-test",
			PromptPackName:    "echo-pack",
			PromptPackVersion: "1.0.0",
		},
		Log: testr.New(t),
	})
}

func TestInvoker_HappyPath(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object","required":["q"]}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object","required":["a"]}`))
	require.NoError(t, err)

	rts := &invokerStub{
		resp: &runtimev1.InvocationResponse{
			OutputJson: `{"a":"hello"}`,
			DurationMs: 42,
		},
	}
	inv := newTestInvoker(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, rts)

	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{"q":"world"}`))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, OutcomeOK, result.Outcome)
	assert.NotEmpty(t, result.InvocationID)
	assert.Equal(t, int64(42), result.DurationMs)
	assert.Empty(t, result.ErrorDetail)

	// Output JSON must be the raw model output.
	assert.JSONEq(t, `{"a":"hello"}`, string(result.OutputJSON))

	// The request sent to the runtime must carry the facade-generated ID.
	require.NotNil(t, rts.lastReq)
	assert.Equal(t, result.InvocationID, rts.lastReq.GetInvocationId())
	assert.Equal(t, `{"q":"world"}`, rts.lastReq.GetInputJson())
}

func TestInvoker_FunctionNotFound(t *testing.T) {
	rts := &invokerStub{} // must not be called
	inv := newTestInvoker(t, map[string]*FunctionSpec{}, rts)

	result, err := inv.Invoke(context.Background(), "missing", []byte(`{}`))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, OutcomeFunctionNotFound, result.Outcome)
	assert.NotEmpty(t, result.InvocationID)
	assert.NotEmpty(t, result.ErrorDetail)
	assert.Nil(t, rts.lastReq, "runtime must NOT be called when function is unknown")
}

func TestInvoker_InputInvalid(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object","required":["q"]}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	rts := &invokerStub{} // must not be called
	inv := newTestInvoker(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, rts)

	// Missing required "q" field.
	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{}`))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, OutcomeInputInvalid, result.Outcome)
	assert.NotEmpty(t, result.ErrorDetail)
	assert.Nil(t, rts.lastReq, "runtime must NOT be called when input is invalid")
}

func TestInvoker_RuntimeError(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	rts := &invokerStub{err: errors.New("upstream failure")}
	inv := newTestInvoker(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, rts)

	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{}`))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, OutcomeRuntimeError, result.Outcome)
	assert.Contains(t, result.ErrorDetail, "upstream failure")
	assert.Nil(t, result.OutputJSON)
}

func TestInvoker_OutputInvalid(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object","required":["a"]}`))
	require.NoError(t, err)

	// Runtime returns valid JSON but wrong shape.
	rts := &invokerStub{
		resp: &runtimev1.InvocationResponse{OutputJson: wrongShapeJSON},
	}
	inv := newTestInvoker(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, rts)

	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{}`))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, OutcomeOutputInvalid, result.Outcome)
	assert.NotEmpty(t, result.ErrorDetail)
	// RawOutput must carry the bytes the runtime returned so the author can debug.
	require.NotNil(t, result.RawOutput)
	assert.JSONEq(t, wrongShapeJSON, string(result.RawOutput))
	assert.Nil(t, result.OutputJSON)
}

func TestInvoker_PayloadTooLarge(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	rts := &invokerStub{} // must not be called
	reg := NewMapFunctionRegistry()
	reg.Register(&FunctionSpec{Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema})
	inv := NewFunctionInvoker(FunctionInvokerConfig{
		Registry:     reg,
		Invoker:      rts,
		MaxBodyBytes: 10,
		Log:          testr.New(t),
	})

	// Input exceeds the 10-byte limit.
	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{"q":"this is too long"}`))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, OutcomePayloadTooLarge, result.Outcome)
	assert.NotEmpty(t, result.ErrorDetail)
	assert.Nil(t, rts.lastReq, "runtime must NOT be called when payload is too large")
}

func TestInvoker_SessionLifecycle_HappyPath(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	rts := &invokerStub{
		resp: &runtimev1.InvocationResponse{OutputJson: `{}`, DurationMs: 5},
	}
	store := &invokerSessionStore{}
	inv := newTestInvokerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, rts, store)

	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, OutcomeOK, result.Outcome)

	require.Len(t, store.creates, 1, "session must be opened exactly once")
	opts := store.creates[0]
	assert.Equal(t, result.InvocationID, opts.ID)
	assert.Equal(t, "ns-test", opts.Namespace)
	assert.Equal(t, "echo-fn", opts.AgentName)
	assert.Equal(t, "ws-test", opts.WorkspaceName)
	assert.Equal(t, "echo-pack", opts.PromptPackName)
	assert.Equal(t, "1.0.0", opts.PromptPackVersion)
	assert.Equal(t, []string{FunctionSessionTag}, opts.Tags)

	require.Len(t, store.statuses, 1, "session must be closed exactly once")
	assert.Equal(t, session.SessionStatusCompleted, store.statuses[0].SetStatus)
	assert.False(t, store.statuses[0].SetEndedAt.IsZero(), "ended_at must be set on close")
	assert.Empty(t, store.events, "no failure events on happy path")
}

func TestInvoker_SessionLifecycle_InputInvalid(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object","required":["q"]}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	store := &invokerSessionStore{}
	inv := newTestInvokerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, &invokerStub{}, store)

	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, OutcomeInputInvalid, result.Outcome)

	require.Len(t, store.creates, 1)
	require.Len(t, store.statuses, 1)
	assert.Equal(t, session.SessionStatusError, store.statuses[0].SetStatus)
	require.Len(t, store.events, 1, "input_invalid must emit a failure runtime event")
	assert.Equal(t, "function.input_invalid", store.events[0].EventType)
	assert.NotEmpty(t, store.events[0].ErrorMessage)
}

func TestInvoker_SessionLifecycle_RuntimeError(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	store := &invokerSessionStore{}
	rts := &invokerStub{err: errors.New("upstream down")}
	inv := newTestInvokerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, rts, store)

	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, OutcomeRuntimeError, result.Outcome)

	require.Len(t, store.statuses, 1)
	assert.Equal(t, session.SessionStatusError, store.statuses[0].SetStatus)
	require.Len(t, store.events, 1)
	assert.Equal(t, "function.runtime_error", store.events[0].EventType)
	assert.Contains(t, store.events[0].ErrorMessage, "upstream down")
}

func TestInvoker_SessionLifecycle_OutputInvalid(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object","required":["a"]}`))
	require.NoError(t, err)

	store := &invokerSessionStore{}
	rts := &invokerStub{resp: &runtimev1.InvocationResponse{OutputJson: wrongShapeJSON}}
	inv := newTestInvokerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, rts, store)

	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, OutcomeOutputInvalid, result.Outcome)

	require.Len(t, store.statuses, 1)
	assert.Equal(t, session.SessionStatusError, store.statuses[0].SetStatus)
	require.Len(t, store.events, 1)
	assert.Equal(t, "function.output_invalid", store.events[0].EventType)
}

func TestInvoker_SessionLifecycle_StoreFailuresAreNonFatal(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	store := &invokerSessionStore{
		createErr: errors.New("session-api down"),
		updateErr: errors.New("session-api still down"),
	}
	rts := &invokerStub{resp: &runtimev1.InvocationResponse{OutputJson: `{}`}}
	inv := newTestInvokerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, rts, store)

	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, OutcomeOK, result.Outcome, "store failures must not fail the invocation")
}

func TestInvoker_SessionLifecycle_VirtualUserFromIdentity(t *testing.T) {
	// When the context carries an authenticated identity, the session is
	// attributed to that end user's pseudonym.
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	store := &invokerSessionStore{}
	rts := &invokerStub{resp: &runtimev1.InvocationResponse{OutputJson: `{}`}}
	inv := newTestInvokerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, rts, store)

	id := &policy.AuthenticatedIdentity{EndUser: "mcp-user@example.com"}
	ctx := policy.WithIdentity(context.Background(), id)
	result, err := inv.Invoke(ctx, testFunctionName, []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, OutcomeOK, result.Outcome)

	require.Len(t, store.creates, 1)
	assert.Equal(t, identity.PseudonymizeID("mcp-user@example.com"), store.creates[0].VirtualUserID,
		"session must be attributed to the context identity's end user")
}

func TestInvoker_SessionLifecycle_VirtualUserAnonymousFallback(t *testing.T) {
	// No identity in context → fall back to pseudonymizing the invocation id
	// so the NOT-NULL virtual_user_id is always populated.
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	store := &invokerSessionStore{}
	rts := &invokerStub{resp: &runtimev1.InvocationResponse{OutputJson: `{}`}}
	inv := newTestInvokerWithSession(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, rts, store)

	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, OutcomeOK, result.Outcome)

	require.Len(t, store.creates, 1)
	opts := store.creates[0]
	assert.NotEmpty(t, opts.VirtualUserID, "anonymous invocations must still get a non-empty virtual user id")
	assert.Equal(t, identity.PseudonymizeID(opts.ID), opts.VirtualUserID)
}

func TestInvoker_NoSessionStore_TransientMode(t *testing.T) {
	inputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	outputSchema, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	rts := &invokerStub{resp: &runtimev1.InvocationResponse{OutputJson: `{}`}}
	// No session store — pure transient mode.
	inv := newTestInvoker(t, map[string]*FunctionSpec{
		testFunctionName: {Name: testFunctionName, InputSchema: inputSchema, OutputSchema: outputSchema},
	}, rts)

	result, err := inv.Invoke(context.Background(), testFunctionName, []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, OutcomeOK, result.Outcome)
}
