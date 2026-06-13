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

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/identity"
	"github.com/altairalabs/omnia/pkg/logctx"
	"github.com/altairalabs/omnia/pkg/policy"
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
type InvocationInvoker interface {
	Invoke(ctx context.Context, req *runtimev1.InvocationRequest) (*runtimev1.InvocationResponse, error)
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
	sessionStore session.Store
	sessionMeta  FunctionSessionMeta
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
	return &FunctionsHandler{
		registry:     registry,
		invoker:      invoker,
		log:          log.WithName("functions-handler"),
		maxBodyBytes: 1 << 20, // 1 MiB
	}
}

// WithSessionStore wires session persistence onto the handler. Each
// invocation creates one `sessions` row at request start and closes
// it with the terminal status when the response is written.
func (h *FunctionsHandler) WithSessionStore(store session.Store, meta FunctionSessionMeta) *FunctionsHandler {
	h.sessionStore = store
	h.sessionMeta = meta
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

	// Generate the invocation ID up-front so input_invalid still has a
	// session_id for Loki + dashboard correlation. ctx carries it via
	// logctx so every log line under this scope can be joined back.
	invocationID := uuid.NewString()
	ctx := logctx.WithInvocationID(r.Context(), invocationID)
	log := h.log.WithValues("function", name, "invocationID", invocationID)

	// Resolve the virtual user the session is attributed to. The request is
	// already past auth (cmd/agent wraps this route with auth.Middleware,
	// which injects the identity into the context), so we read that identity
	// here. ResolveUserPseudonym honors the mgmt-plane on-behalf-of header,
	// device_id, and validated EndUser. With no resolvable identity we fall
	// back to pseudonymizing the invocation id so the NOT-NULL
	// virtual_user_id create never rejects an anonymous invocation. See #1285.
	authIdentity := policy.IdentityFromContext(r.Context())
	virtualUserID := ResolveUserPseudonym(r, authIdentity)
	if virtualUserID == "" {
		virtualUserID = identity.PseudonymizeID(invocationID)
	}

	// Open the session row at the very start. The runtime's
	// OmniaEventStore writes downstream audit rows (messages,
	// tool_calls, provider_calls, eval_results, runtime_events)
	// against this session_id. closeSession patches the terminal
	// status before we return.
	h.openSession(ctx, invocationID, virtualUserID, log)
	finalStatus := session.SessionStatusCompleted
	var failureEvt *session.RuntimeEvent
	defer func() {
		h.closeSession(ctx, invocationID, finalStatus, failureEvt, log)
	}()

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxBodyBytes))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			finalStatus = session.SessionStatusError
			failureEvt = newFailureEvent(invocationID, "function.payload_too_large", err.Error())
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large",
				fmt.Sprintf("request body exceeds %d bytes", h.maxBodyBytes))
			return
		}
		finalStatus = session.SessionStatusError
		failureEvt = newFailureEvent(invocationID, "function.read_body_failed", err.Error())
		writeError(w, http.StatusBadRequest, "read_body_failed", err.Error())
		return
	}

	if err := ValidateJSON(spec.InputSchema, body); err != nil {
		finalStatus = session.SessionStatusError
		failureEvt = newFailureEvent(invocationID, "function.input_invalid", err.Error())
		writeError(w, http.StatusBadRequest, "input_invalid", err.Error())
		return
	}

	resp, err := h.invoker.Invoke(ctx, &runtimev1.InvocationRequest{
		InputJson:    string(body),
		InvocationId: invocationID,
	})
	if err != nil {
		log.Error(err, "runtime invoke failed")
		finalStatus = session.SessionStatusError
		failureEvt = newFailureEvent(invocationID, "function.runtime_error", err.Error())
		writeError(w, http.StatusBadGateway, "runtime_error", err.Error())
		return
	}

	rawOutput := []byte(resp.GetOutputJson())
	if err := ValidateJSON(spec.OutputSchema, rawOutput); err != nil {
		// 502 with raw output so the author can debug the pack.
		log.Error(err, "function output failed schema validation",
			"outputBytes", len(rawOutput))
		finalStatus = session.SessionStatusError
		failureEvt = newFailureEvent(invocationID, "function.output_invalid", err.Error())
		writeOutputValidationError(w, err, rawOutput)
		return
	}

	if err := writeSuccess(w, rawOutput, invocationID, resp); err != nil {
		log.Error(err, "failed to write success response")
		finalStatus = session.SessionStatusError
		failureEvt = newFailureEvent(invocationID, "function.response_write_failed", err.Error())
		return
	}
	log.V(1).Info("function invocation complete",
		"durationMs", resp.GetDurationMs(),
		"outputBytes", len(rawOutput))
}

// openSession creates the session row for an invocation. Failure is
// best-effort: a session-api outage logs but does not fail the
// user-facing request — the runtime can still produce its result and
// the downstream audit rows simply land orphaned (parent missing).
func (h *FunctionsHandler) openSession(ctx context.Context, invocationID, virtualUserID string, log logr.Logger) {
	if h.sessionStore == nil {
		return
	}
	if _, err := h.sessionStore.CreateSession(ctx, session.CreateSessionOptions{
		ID:                invocationID,
		AgentName:         h.sessionMeta.AgentName,
		Namespace:         h.sessionMeta.Namespace,
		WorkspaceName:     h.sessionMeta.WorkspaceName,
		PromptPackName:    h.sessionMeta.PromptPackName,
		PromptPackVersion: h.sessionMeta.PromptPackVersion,
		VirtualUserID:     virtualUserID,
		Tags:              []string{FunctionSessionTag},
	}); err != nil {
		log.Error(err, "failed to create session row for function invocation (non-fatal)")
	}
}

// closeSession writes the terminal status (and any pre-runtime
// failure event the facade itself observed) before returning. Errors
// are logged and not propagated.
func (h *FunctionsHandler) closeSession(
	ctx context.Context,
	invocationID string,
	status session.SessionStatus,
	failure *session.RuntimeEvent,
	log logr.Logger,
) {
	if h.sessionStore == nil {
		return
	}
	if failure != nil {
		if err := h.sessionStore.RecordRuntimeEvent(ctx, invocationID, *failure); err != nil {
			log.Error(err, "failed to record function failure event (non-fatal)",
				"eventType", failure.EventType)
		}
	}
	if err := h.sessionStore.UpdateSessionStatus(ctx, invocationID, session.SessionStatusUpdate{
		SetStatus:  status,
		SetEndedAt: time.Now().UTC(),
	}); err != nil {
		log.Error(err, "failed to close session for function invocation (non-fatal)",
			"status", status)
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
