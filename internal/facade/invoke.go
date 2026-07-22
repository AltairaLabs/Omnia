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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/logctx"
	"github.com/altairalabs/omnia/pkg/policy"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// InvocationOutcome enumerates terminal states for one function call.
type InvocationOutcome string

const (
	OutcomeOK               InvocationOutcome = "ok"
	OutcomeFunctionNotFound InvocationOutcome = "function_not_found"
	OutcomeInputInvalid     InvocationOutcome = "input_invalid"
	OutcomeRuntimeError     InvocationOutcome = "runtime_error"
	OutcomeOutputInvalid    InvocationOutcome = "output_invalid"
	OutcomePayloadTooLarge  InvocationOutcome = "payload_too_large"
)

// InvocationResult carries the full outcome of one invocation. The
// transport-specific caller (ServeHTTP for HTTP, a future MCP tool adapter)
// maps this onto its wire format.
type InvocationResult struct {
	Outcome      InvocationOutcome
	OutputJSON   json.RawMessage // populated when Outcome == OutcomeOK
	RawOutput    json.RawMessage // populated when Outcome == OutcomeOutputInvalid (for debugging)
	InvocationID string
	DurationMs   int64
	ErrorDetail  string           // populated on non-OK outcomes
	Usage        *runtimev1.Usage // nil when not reported by the runtime
}

// FunctionInvokerConfig collects FunctionInvoker dependencies.
type FunctionInvokerConfig struct {
	Registry     FunctionRegistry
	Invoker      InvocationInvoker
	SessionStore session.Recorder    // optional; if nil, no session rows are written
	SessionMeta  FunctionSessionMeta // ignored when SessionStore is nil
	// MaxBodyBytes caps input size. 0 means no guard (useful when the caller
	// already enforces the limit, e.g. via http.MaxBytesReader). The HTTP
	// handler sets MaxBodyBytes = 0 and relies on its own MaxBytesReader so
	// that http.MaxBytesError is preserved for the 413 response shape.
	MaxBodyBytes int64
	Log          logr.Logger
}

// FunctionInvoker runs one function invocation end-to-end: lookup →
// validate input → openSession → runtime call → validate output →
// closeSession. Safe for concurrent use; all mutable state is
// per-invocation.
type FunctionInvoker struct {
	cfg FunctionInvokerConfig
}

// NewFunctionInvoker builds an invoker.
func NewFunctionInvoker(cfg FunctionInvokerConfig) *FunctionInvoker {
	if cfg.Log.GetSink() == nil {
		cfg.Log = logr.Discard()
	}
	return &FunctionInvoker{cfg: cfg}
}

// Invoke runs one function call. A nil error means the invocation
// completed (success or any typed failure captured in the result).
// A non-nil error is reserved for unrecoverable failures the caller
// should map to 500.
func (i *FunctionInvoker) Invoke(ctx context.Context, name string, input json.RawMessage) (*InvocationResult, error) {
	started := time.Now()

	invocationID := uuid.NewString()
	ctx = logctx.WithInvocationID(ctx, invocationID)
	log := i.cfg.Log.WithValues("function", name, "invocationID", invocationID)

	// Size check — only active when MaxBodyBytes > 0. The HTTP handler sets
	// this to 0 and enforces size via http.MaxBytesReader instead, so the
	// 413 response carries a proper http.MaxBytesError.
	if i.cfg.MaxBodyBytes > 0 && int64(len(input)) > i.cfg.MaxBodyBytes {
		return &InvocationResult{
			Outcome:      OutcomePayloadTooLarge,
			InvocationID: invocationID,
			DurationMs:   time.Since(started).Milliseconds(),
			ErrorDetail:  fmt.Sprintf("input exceeds %d bytes", i.cfg.MaxBodyBytes),
		}, nil
	}

	spec, ok := i.cfg.Registry.GetFunction(name)
	if !ok {
		log.V(1).Info("function not registered", "name", name)
		return &InvocationResult{
			Outcome:      OutcomeFunctionNotFound,
			InvocationID: invocationID,
			DurationMs:   time.Since(started).Milliseconds(),
			ErrorDetail:  fmt.Sprintf("no function named %q is registered on this facade", name),
		}, nil
	}

	// Attribute the session to a virtual user. This path (MCP adapter) has
	// only a ctx — no *http.Request — so we resolve from the context identity
	// injected by auth.Middleware rather than ResolveUserPseudonym. With no
	// resolvable identity we pseudonymize the invocation id so the NOT-NULL
	// virtual_user_id create never rejects an anonymous invocation. See #1285.
	virtualUserID := pseudonymFromIdentity(policy.IdentityFromContext(ctx), invocationID)

	i.openSession(ctx, invocationID, virtualUserID, log)
	finalStatus := session.SessionStatusCompleted
	var failureEvt *session.RuntimeEvent
	defer func() {
		i.closeSession(ctx, invocationID, finalStatus, failureEvt, log)
	}()

	if err := ValidateJSON(spec.InputSchema, input); err != nil {
		finalStatus = session.SessionStatusError
		failureEvt = newFailureEvent(invocationID, "function.input_invalid", err.Error())
		return &InvocationResult{
			Outcome:      OutcomeInputInvalid,
			InvocationID: invocationID,
			DurationMs:   time.Since(started).Milliseconds(),
			ErrorDetail:  err.Error(),
		}, nil
	}

	resp, err := i.cfg.Invoker.Invoke(ctx, &runtimev1.InvocationRequest{
		InputJson:    string(input),
		InvocationId: invocationID,
	}, grpc.UseCompressor(gzip.Name))
	if err != nil {
		log.Error(err, "runtime invoke failed")
		finalStatus = session.SessionStatusError
		failureEvt = newFailureEvent(invocationID, "function.runtime_error", err.Error())
		return &InvocationResult{
			Outcome:      OutcomeRuntimeError,
			InvocationID: invocationID,
			DurationMs:   time.Since(started).Milliseconds(),
			ErrorDetail:  err.Error(),
		}, nil
	}

	rawOutput := []byte(resp.GetOutputJson())
	if err := ValidateJSON(spec.OutputSchema, rawOutput); err != nil {
		log.Error(err, "function output failed schema validation", "outputBytes", len(rawOutput))
		finalStatus = session.SessionStatusError
		failureEvt = newFailureEvent(invocationID, "function.output_invalid", err.Error())
		return &InvocationResult{
			Outcome:      OutcomeOutputInvalid,
			InvocationID: invocationID,
			DurationMs:   time.Since(started).Milliseconds(),
			ErrorDetail:  err.Error(),
			RawOutput:    json.RawMessage(rawOutput),
		}, nil
	}

	log.V(1).Info("function invocation complete",
		"durationMs", resp.GetDurationMs(),
		"outputBytes", len(rawOutput))

	return &InvocationResult{
		Outcome:      OutcomeOK,
		OutputJSON:   json.RawMessage(rawOutput),
		InvocationID: invocationID,
		DurationMs:   int64(resp.GetDurationMs()),
		Usage:        resp.GetUsage(),
	}, nil
}

// openSession creates the session row for an invocation. Failure is
// best-effort: a session-api outage logs but does not fail the
// user-facing request.
func (i *FunctionInvoker) openSession(ctx context.Context, invocationID, virtualUserID string, log logr.Logger) {
	if i.cfg.SessionStore == nil {
		return
	}
	if _, err := i.cfg.SessionStore.EnsureSessionRecord(ctx, session.SessionRecordOptions{
		ID:                invocationID,
		AgentName:         i.cfg.SessionMeta.AgentName,
		Namespace:         i.cfg.SessionMeta.Namespace,
		WorkspaceName:     i.cfg.SessionMeta.WorkspaceName,
		PromptPackName:    i.cfg.SessionMeta.PromptPackName,
		PromptPackVersion: i.cfg.SessionMeta.PromptPackVersion,
		VirtualUserID:     virtualUserID,
		Tags:              []string{FunctionSessionTag},
	}); err != nil {
		log.Error(err, "failed to create session row for function invocation (non-fatal)")
	}
}

// closeSession writes the terminal status and any pre-runtime failure event
// before returning. Errors are logged and not propagated.
func (i *FunctionInvoker) closeSession(
	ctx context.Context,
	invocationID string,
	status session.SessionStatus,
	failure *session.RuntimeEvent,
	log logr.Logger,
) {
	if i.cfg.SessionStore == nil {
		return
	}
	if failure != nil {
		if err := i.cfg.SessionStore.RecordRuntimeEvent(ctx, invocationID, *failure); err != nil {
			log.Error(err, "failed to record function failure event (non-fatal)",
				"eventType", failure.EventType)
		}
	}
	if err := i.cfg.SessionStore.UpdateSessionStatus(ctx, invocationID, session.SessionStatusUpdate{
		SetStatus:  status,
		SetEndedAt: time.Now().UTC(),
	}); err != nil {
		log.Error(err, "failed to close session for function invocation (non-fatal)",
			"status", status)
	}
}
