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
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func (s *stubInvoker) Invoke(_ context.Context, req *runtimev1.InvocationRequest) (*runtimev1.InvocationResponse, error) {
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

	invoker := &stubInvoker{
		resp: &runtimev1.InvocationResponse{
			OutputJson:   `{"a":"42"}`,
			DurationMs:   17,
			InvocationId: "ignored-server-side",
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
	assert.NotEmpty(t, body["invocation_id"], "facade should generate a fresh invocation_id")

	require.NotNil(t, invoker.lastReq)
	assert.Equal(t, `{"q":"hi"}`, invoker.lastReq.GetInputJson())
	assert.NotEmpty(t, invoker.lastReq.GetInvocationId(),
		"facade must send an invocation_id to the runtime")
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
	assert.Contains(t, resp.Detail, "simulated upstream failure")
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

func TestFunctionsHandler_BlankFunctionNameIs400(t *testing.T) {
	h := newHandler(t, map[string]*FunctionSpec{}, &stubInvoker{})

	// http.ServeMux will 404 a path that doesn't have {name} bound, so
	// drive the handler directly with no PathValue.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/functions/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
