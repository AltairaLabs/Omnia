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
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/internal/session"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// FunctionSessionMeta is the per-pod identity the facade stamps onto
// each invocation's session row. One AgentRuntime per function-mode
// pod, so these are stable for the lifetime of the handler.
type FunctionSessionMeta struct {
	Namespace         string
	AgentName         string
	WorkspaceName     string
	PromptPackName    string
	PromptPackVersion string
}

// FunctionSessionTag marks a sessions row as belonging to a
// function-mode invocation. Dashboards filter on this to surface
// function history without crossing into agent-mode sessions.
const FunctionSessionTag = "function"

// FunctionSpec is the per-Function metadata the facade needs at request
// time. The operator loads spec.inputSchema / spec.outputSchema from the
// AgentRuntime CRD and CompileSchema's them once at startup.
type FunctionSpec struct {
	Name         string
	InputSchema  *jsonschema.Schema
	OutputSchema *jsonschema.Schema
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
// The opts variadic allows callers to control per-call gRPC options such as
// compression (e.g. grpc.UseCompressor(gzip.Name) for text/function paths).
type InvocationInvoker interface {
	Invoke(ctx context.Context, req *runtimev1.InvocationRequest, opts ...grpc.CallOption) (*runtimev1.InvocationResponse, error)
}

// FunctionsHandler serves POST /functions/{name} for function-mode
// AgentRuntimes. The handler is the single source of truth for input
// and output JSON-Schema validation; the runtime is schema-agnostic.
//
// Persistence: every invocation creates one `sessions` row (tagged
// "function"). PromptKit's OmniaEventStore fills in the downstream
// audit tables (messages / tool_calls / provider_calls / eval_results
// / runtime_events) keyed off that session_id, identically to agent
// mode. The handler closes the session at the end of the call with
// the final status (completed | error).
type FunctionsHandler struct {
	registry     FunctionRegistry
	invoker      InvocationInvoker
	sessionStore session.Recorder
	sessionMeta  FunctionSessionMeta
	funcInvoker  *FunctionInvoker
	log          logr.Logger

	// maxBodyBytes caps the incoming request body. Defaults to 1 MiB,
	// matching the WS path's reasonable upper bound.
	maxBodyBytes int64
}

// NewFunctionsHandler constructs a FunctionsHandler. log may be a
// zero-value logr.Logger; the handler tolerates a nil sink.
//
// Session persistence is opt-in via WithSessionStore. When unset, the
// handler runs in transient mode — useful for tests that don't care
// about audit rows; production wires the store in cmd/agent.
func NewFunctionsHandler(registry FunctionRegistry, invoker InvocationInvoker, log logr.Logger) *FunctionsHandler {
	if log.GetSink() == nil {
		log = logr.Discard()
	}
	h := &FunctionsHandler{
		registry:     registry,
		invoker:      invoker,
		log:          log.WithName("functions-handler"),
		maxBodyBytes: 1 << 20, // 1 MiB
	}
	h.funcInvoker = NewFunctionInvoker(FunctionInvokerConfig{
		Registry:     h.registry,
		Invoker:      h.invoker,
		SessionStore: h.sessionStore,
		SessionMeta:  h.sessionMeta,
		MaxBodyBytes: 0,
		Log:          h.log,
	})
	return h
}

// WithSessionStore wires session persistence onto the handler. Each
// invocation creates one `sessions` row at request start and closes
// it with the terminal status when the response is written.
func (h *FunctionsHandler) WithSessionStore(store session.Recorder, meta FunctionSessionMeta) *FunctionsHandler {
	h.sessionStore = store
	h.sessionMeta = meta
	h.funcInvoker = NewFunctionInvoker(FunctionInvokerConfig{
		Registry:     h.registry,
		Invoker:      h.invoker,
		SessionStore: h.sessionStore,
		SessionMeta:  h.sessionMeta,
		MaxBodyBytes: 0,
		Log:          h.log,
	})
	return h
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
		writeError(w, http.StatusBadRequest, "read_body_failed", "failed to read request body")
		return
	}

	result, err := h.funcInvoker.Invoke(r.Context(), name, body)
	if err != nil {
		h.log.Error(err, "function invocation failed")
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if result == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "invocation result is nil")
		return
	}

	switch result.Outcome {
	case OutcomeOK:
		if err := writeSuccess(w, result); err != nil {
			h.log.Error(err, "failed to write success response", "function", name,
				"invocationID", result.InvocationID)
		}
	case OutcomeFunctionNotFound:
		writeError(w, http.StatusNotFound, "function_not_found", result.ErrorDetail)
	case OutcomeInputInvalid:
		writeError(w, http.StatusBadRequest, "input_invalid", result.ErrorDetail)
	case OutcomeRuntimeError:
		writeError(w, http.StatusBadGateway, "runtime_error", "runtime invocation failed")
	case OutcomeOutputInvalid:
		writeOutputValidationError(w, errors.New(result.ErrorDetail), result.RawOutput)
	case OutcomePayloadTooLarge:
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", result.ErrorDetail)
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "unknown invocation outcome")
	}
}

// newFailureEvent shapes a RuntimeEvent for a facade-side outcome
// the runtime never saw (input_invalid, body-read errors,
// output-validation rejection, response-write failure). PromptKit's
// own loop fires its own events for inner failures, so this is
// scoped strictly to the boundary.
func newFailureEvent(invocationID, eventType, errMessage string) *session.RuntimeEvent {
	return &session.RuntimeEvent{
		ID:           uuid.NewString(),
		SessionID:    invocationID,
		EventType:    eventType,
		ErrorMessage: errMessage,
		Timestamp:    time.Now().UTC(),
	}
}

// writeSuccess emits the function-mode 200 response envelope using the
// canonical result produced by FunctionInvoker.
func writeSuccess(w http.ResponseWriter, result *InvocationResult) error {
	envelope := map[string]any{
		"output":        result.OutputJSON,
		"invocation_id": result.InvocationID,
		"duration_ms":   result.DurationMs,
	}
	if u := result.Usage; u != nil {
		envelope["usage"] = map[string]any{
			"input_tokens":  u.InputTokens,
			"output_tokens": u.OutputTokens,
			"cost_usd":      u.CostUsd,
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

// MapFunctionRegistry is a small map-backed FunctionRegistry. PR 4
// uses it directly in `cmd/agent` — a function-mode pod serves a
// single Function (the one defined by its own AgentRuntime CRD) so
// there's no need for a live CRD watch; a snapshot built at startup
// is correct by construction (any CRD change triggers a Deployment
// rollout that restarts the pod).
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
