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
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/altairalabs/omnia/pkg/logctx"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// FunctionSpec is the per-Function metadata the facade needs at request
// time. The operator loads spec.inputSchema / spec.outputSchema from the
// AgentRuntime CRD and CompileSchema's them once at startup (PR 4 wiring;
// for PR 3, the registry is populated by tests and out-of-band callers).
//
// RecordsInvocations mirrors AgentRuntime.spec.invocationRecording.state
// == "enabled"; PR 5 (session-api persistence) is the first consumer.
type FunctionSpec struct {
	Name               string
	InputSchema        *jsonschema.Schema
	OutputSchema       *jsonschema.Schema
	RecordsInvocations bool
}

// FunctionRegistry returns the FunctionSpec for a given function-mode
// AgentRuntime by name. Returns (nil, false) when no such function
// exists or when the AgentRuntime is mode=agent.
type FunctionRegistry interface {
	GetFunction(name string) (*FunctionSpec, bool)
}

// InvocationInvoker is the subset of the runtime gRPC client used by
// FunctionsHandler. Pulled out as an interface so tests can substitute a
// mock without standing up a real gRPC server.
type InvocationInvoker interface {
	Invoke(ctx context.Context, req *runtimev1.InvocationRequest) (*runtimev1.InvocationResponse, error)
}

// FunctionsHandler serves POST /functions/{name} for function-mode
// AgentRuntimes. The handler is the single source of truth for input
// and output JSON-Schema validation (the runtime is intentionally
// schema-agnostic; see PR 2 / #1105).
type FunctionsHandler struct {
	registry FunctionRegistry
	invoker  InvocationInvoker
	log      logr.Logger

	// maxBodyBytes caps the incoming request body. Defaults to 1 MiB,
	// matching the WS path's reasonable upper bound. Function payloads
	// are JSON; if someone needs more they can lift this via PR 4.
	maxBodyBytes int64
}

// NewFunctionsHandler constructs a FunctionsHandler. log may be a
// zero-value logr.Logger; the handler tolerates a nil sink.
func NewFunctionsHandler(registry FunctionRegistry, invoker InvocationInvoker, log logr.Logger) *FunctionsHandler {
	if log.GetSink() == nil {
		log = logr.Discard()
	}
	return &FunctionsHandler{
		registry:     registry,
		invoker:      invoker,
		log:          log.WithName("functions-handler"),
		maxBodyBytes: 1 << 20, // 1 MiB
	}
}

// errorResponse is the JSON envelope returned on 4xx errors. The 502
// output-validation case is special and includes the raw model output
// in raw_output so authors can debug (per #1103 Q2 lock).
type errorResponse struct {
	Error     string          `json:"error"`
	Detail    string          `json:"detail,omitempty"`
	RawOutput json.RawMessage `json:"raw_output,omitempty"`
}

// ServeHTTP routes POST /functions/{name}. Non-POST is 405, unknown name
// (or agent-mode runtime) is 404. Per the WS endpoint convention, the
// agent name comes from the path; the namespace + workspace are bound
// to the facade pod's identity.
func (h *FunctionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing_function_name",
			"function name is required in the URL path")
		return
	}

	spec, ok := h.registry.GetFunction(name)
	if !ok {
		// Distinguish "not configured" from "wrong mode" in logs but use
		// 404 for both — leaking the existence of agent-mode runtimes
		// at a function-mode endpoint isn't useful and authoring tools
		// shouldn't depend on that distinction.
		h.log.V(1).Info("function not registered", "name", name)
		writeError(w, http.StatusNotFound, "function_not_found",
			fmt.Sprintf("no function named %q is registered on this facade", name))
		return
	}

	// Parse the media type so application/json; charset=utf-8 (default for
	// many HTTP clients) is accepted. Only the bare media type is checked;
	// parameters like charset are ignored.
	if rawCT := r.Header.Get("Content-Type"); rawCT != "" {
		mediaType, _, err := mime.ParseMediaType(rawCT)
		if err != nil || mediaType != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type",
				"Content-Type must be application/json")
			return
		}
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxBodyBytes))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large",
				fmt.Sprintf("request body exceeds %d bytes", h.maxBodyBytes))
			return
		}
		writeError(w, http.StatusBadRequest, "read_body_failed", err.Error())
		return
	}

	if err := ValidateJSON(spec.InputSchema, body); err != nil {
		writeError(w, http.StatusBadRequest, "input_invalid", err.Error())
		return
	}

	invocationID := uuid.NewString()
	ctx := logctx.WithInvocationID(r.Context(), invocationID)
	log := h.log.WithValues("function", name, "invocationID", invocationID)

	resp, err := h.invoker.Invoke(ctx, &runtimev1.InvocationRequest{
		InputJson:    string(body),
		InvocationId: invocationID,
	})
	if err != nil {
		log.Error(err, "runtime invoke failed")
		writeError(w, http.StatusBadGateway, "runtime_error", err.Error())
		return
	}

	rawOutput := []byte(resp.GetOutputJson())
	if err := ValidateJSON(spec.OutputSchema, rawOutput); err != nil {
		// Per #1103 Q2: 502 with raw output so the author can debug the
		// pack. No in-runtime retry.
		log.Error(err, "function output failed schema validation",
			"outputBytes", len(rawOutput))
		writeOutputValidationError(w, err, rawOutput)
		return
	}

	// Echo the response shape: { output: <model output>, usage, duration_ms, invocation_id }.
	// output is forwarded as raw JSON (already validated above). The
	// invocationID returned is always the facade-generated one — the
	// runtime echoes it but the facade is the source of truth, so a
	// runtime that returned a different (or empty) value would still
	// see this id in the response and in correlation traces.
	if err := writeSuccess(w, rawOutput, invocationID, resp); err != nil {
		log.Error(err, "failed to write success response")
		return
	}
	log.V(1).Info("function invocation complete",
		"durationMs", resp.GetDurationMs(),
		"outputBytes", len(rawOutput))
}

// writeSuccess emits the function-mode 200 response envelope. invocationID
// is the facade-authoritative UUID generated when the request arrived;
// it is the value returned to the caller regardless of what the runtime
// echoed back in resp.GetInvocationId().
func writeSuccess(w http.ResponseWriter, rawOutput []byte, invocationID string, resp *runtimev1.InvocationResponse) error {
	envelope := map[string]any{
		"output":        json.RawMessage(rawOutput),
		"invocation_id": invocationID,
		"duration_ms":   resp.GetDurationMs(),
	}
	if u := resp.GetUsage(); u != nil {
		envelope["usage"] = map[string]any{
			"input_tokens":  u.GetInputTokens(),
			"output_tokens": u.GetOutputTokens(),
			"cost_usd":      u.GetCostUsd(),
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(envelope)
}

// writeError writes a uniform JSON error envelope.
func writeError(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Error:  code,
		Detail: detail,
	})
}

// writeOutputValidationError writes a 502 with the raw model output
// embedded so the function author can diagnose schema drift. If the
// raw output isn't valid JSON we still include it as a JSON string for
// debugging.
func writeOutputValidationError(w http.ResponseWriter, validationErr error, rawOutput []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	body := errorResponse{
		Error:  "output_invalid",
		Detail: validationErr.Error(),
	}
	// Pass the raw output through as JSON if possible; otherwise as a
	// JSON string. Either way the author can read it from the response.
	if json.Valid(rawOutput) {
		body.RawOutput = json.RawMessage(rawOutput)
	} else {
		s, _ := json.Marshal(string(rawOutput))
		body.RawOutput = s
	}
	_ = json.NewEncoder(w).Encode(body)
}

// MapFunctionRegistry is a trivial map-backed FunctionRegistry useful
// for tests and bootstrap wiring. The operator (PR 4) will replace this
// with a live, CRD-backed implementation.
type MapFunctionRegistry struct {
	specs map[string]*FunctionSpec
}

// NewMapFunctionRegistry returns an empty MapFunctionRegistry.
func NewMapFunctionRegistry() *MapFunctionRegistry {
	return &MapFunctionRegistry{specs: make(map[string]*FunctionSpec)}
}

// Register adds or replaces a FunctionSpec under its Name.
func (r *MapFunctionRegistry) Register(spec *FunctionSpec) {
	if spec == nil || spec.Name == "" {
		return
	}
	r.specs[spec.Name] = spec
}

// GetFunction implements FunctionRegistry.
func (r *MapFunctionRegistry) GetFunction(name string) (*FunctionSpec, bool) {
	spec, ok := r.specs[name]
	return spec, ok
}
