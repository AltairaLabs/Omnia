/*
Copyright 2026.

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

package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/facade"
)

// Invoker is the interface FunctionToolAdapter calls to run a function
// invocation. Production passes *facade.FunctionInvoker; tests pass a
// fake.
type Invoker interface {
	Invoke(ctx context.Context, name string, input json.RawMessage) (*facade.InvocationResult, error)
}

// FunctionToolAdapterConfig assembles a FunctionToolAdapter.
type FunctionToolAdapterConfig struct {
	// Invoker runs the function. Required.
	Invoker Invoker

	// Tool describes the single MCP tool this adapter advertises.
	// Function-mode pods expose exactly one tool — their own function.
	Tool Tool

	// Log receives diagnostic events. logr.Discard() is fine.
	Log logr.Logger
}

// FunctionToolAdapter exposes one Omnia function as one MCP tool. One
// pod, one function, one tool — matching the function-mode deploy model.
//
// Implements the transport's ToolAdapter interface: ListTools returns
// the configured Tool; CallTool routes to the Invoker and maps the
// InvocationResult onto a CallToolResult.
type FunctionToolAdapter struct {
	cfg FunctionToolAdapterConfig
}

// NewFunctionToolAdapter constructs an adapter. Returns the value (not
// a pointer) only conceptually; the struct holds a config pointer so
// concurrent use is safe.
func NewFunctionToolAdapter(cfg FunctionToolAdapterConfig) *FunctionToolAdapter {
	if cfg.Log.GetSink() == nil {
		cfg.Log = logr.Discard()
	}
	return &FunctionToolAdapter{cfg: cfg}
}

// ListTools returns the single configured tool descriptor.
func (a *FunctionToolAdapter) ListTools() []Tool {
	return []Tool{a.cfg.Tool}
}

// CallTool runs the function and maps the outcome to a CallToolResult.
// Unknown tool names → IsError=true with a tool_not_found text part.
// All other failures (input/output/runtime/payload-too-large) become
// IsError=true with a descriptive text part. Success returns the
// runtime's output JSON as a text content part.
//
// System errors from the invoker (transport failures, session-store
// crash) become IsError=true with an internal_error message — MCP
// callers see a tool-level failure rather than a protocol-level one.
func (a *FunctionToolAdapter) CallTool(ctx context.Context, name string, args json.RawMessage) CallToolResult {
	if name != a.cfg.Tool.Name {
		return errorResult(fmt.Sprintf("tool_not_found: %q", name))
	}
	result, err := a.cfg.Invoker.Invoke(ctx, name, args)
	if err != nil {
		a.cfg.Log.Error(err, "invoker returned system error", "tool", name)
		return errorResult("internal_error: " + err.Error())
	}
	return mapInvocationResult(result)
}

// mapInvocationResult converts a *facade.InvocationResult to a
// CallToolResult. Each outcome maps to a specific text-part shape so
// MCP clients (and prompt-pack authors debugging) can grep the
// failure reason.
func mapInvocationResult(r *facade.InvocationResult) CallToolResult {
	switch r.Outcome {
	case facade.OutcomeOK:
		return CallToolResult{
			Content: []ContentPart{{Type: ContentTypeText, Text: string(r.OutputJSON)}},
		}
	case facade.OutcomeFunctionNotFound:
		return errorResult("function_not_found: " + r.ErrorDetail)
	case facade.OutcomeInputInvalid:
		return errorResult("input_invalid: " + r.ErrorDetail)
	case facade.OutcomeRuntimeError:
		return errorResult("runtime_error: " + r.ErrorDetail)
	case facade.OutcomeOutputInvalid:
		// Surface the raw runtime output too so pack authors can debug
		// schema mismatches — matches the HTTP route's 502 behaviour.
		return CallToolResult{
			IsError: true,
			Content: []ContentPart{
				{Type: ContentTypeText, Text: "output_invalid: " + r.ErrorDetail},
				{Type: ContentTypeText, Text: "raw_output: " + string(r.RawOutput)},
			},
		}
	case facade.OutcomePayloadTooLarge:
		return errorResult("payload_too_large: " + r.ErrorDetail)
	default:
		return errorResult("unknown_outcome: " + string(r.Outcome))
	}
}

func errorResult(text string) CallToolResult {
	return CallToolResult{
		IsError: true,
		Content: []ContentPart{{Type: ContentTypeText, Text: text}},
	}
}
