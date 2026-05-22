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
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"

	"github.com/altairalabs/omnia/internal/facade"
)

// Test-scoped constants extracted to satisfy goconst.
const (
	testToolName       = "echo"
	testEchoOutputJSON = `{"echo":"hi"}`
)

// stubInvoker is a deterministic Invoker for adapter tests.
type stubInvoker struct {
	result *facade.InvocationResult
	err    error

	lastName  string
	lastInput json.RawMessage
}

func (s *stubInvoker) Invoke(_ context.Context, name string, input json.RawMessage) (*facade.InvocationResult, error) {
	s.lastName = name
	s.lastInput = input
	return s.result, s.err
}

func newTestAdapter(t *testing.T, inv Invoker) *FunctionToolAdapter {
	t.Helper()
	return NewFunctionToolAdapter(FunctionToolAdapterConfig{
		Invoker: inv,
		Tool: Tool{
			Name:        testToolName,
			Description: "test tool",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		Log: testr.New(t),
	})
}

func TestAdapter_ListTools_ReturnsConfiguredTool(t *testing.T) {
	a := newTestAdapter(t, &stubInvoker{})
	tools := a.ListTools()
	if len(tools) != 1 || tools[0].Name != testToolName {
		t.Errorf("ListTools: %+v", tools)
	}
}

func TestAdapter_CallTool_OKMapsToTextContent(t *testing.T) {
	inv := &stubInvoker{
		result: &facade.InvocationResult{
			Outcome:    facade.OutcomeOK,
			OutputJSON: json.RawMessage(testEchoOutputJSON),
		},
	}
	a := newTestAdapter(t, inv)
	result := a.CallTool(context.Background(), "echo", json.RawMessage(`{"message":"hi"}`))
	if result.IsError {
		t.Fatalf("unexpected IsError; content=%+v", result.Content)
	}
	if len(result.Content) != 1 || result.Content[0].Text != testEchoOutputJSON {
		t.Errorf("Content: %+v", result.Content)
	}
	if inv.lastName != testToolName {
		t.Errorf("Invoker.Invoke called with name=%q want %s", inv.lastName, testToolName)
	}
}

func TestAdapter_CallTool_UnknownToolIsError(t *testing.T) {
	inv := &stubInvoker{}
	a := newTestAdapter(t, inv)
	result := a.CallTool(context.Background(), "other", json.RawMessage(`{}`))
	if !result.IsError {
		t.Error("expected IsError=true for unknown tool name")
	}
	if inv.lastName != "" {
		t.Error("Invoker should not be called for unknown tool name")
	}
	if !strings.Contains(result.Content[0].Text, "tool_not_found") {
		t.Errorf("Content: %+v", result.Content)
	}
}

func TestAdapter_CallTool_InputInvalidIsError(t *testing.T) {
	inv := &stubInvoker{
		result: &facade.InvocationResult{
			Outcome:     facade.OutcomeInputInvalid,
			ErrorDetail: "missing required field: message",
		},
	}
	a := newTestAdapter(t, inv)
	result := a.CallTool(context.Background(), "echo", json.RawMessage(`{}`))
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if !strings.Contains(result.Content[0].Text, "input_invalid") {
		t.Errorf("Content: %+v", result.Content)
	}
}

func TestAdapter_CallTool_RuntimeErrorIsError(t *testing.T) {
	inv := &stubInvoker{
		result: &facade.InvocationResult{
			Outcome:     facade.OutcomeRuntimeError,
			ErrorDetail: "deadline exceeded",
		},
	}
	a := newTestAdapter(t, inv)
	result := a.CallTool(context.Background(), "echo", json.RawMessage(`{}`))
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if !strings.Contains(result.Content[0].Text, "runtime_error") {
		t.Errorf("Content: %+v", result.Content)
	}
}

func TestAdapter_CallTool_OutputInvalidIncludesRawOutput(t *testing.T) {
	inv := &stubInvoker{
		result: &facade.InvocationResult{
			Outcome:     facade.OutcomeOutputInvalid,
			RawOutput:   json.RawMessage(`{"wrong":"shape"}`),
			ErrorDetail: "missing required field: echo",
		},
	}
	a := newTestAdapter(t, inv)
	result := a.CallTool(context.Background(), "echo", json.RawMessage(`{}`))
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if len(result.Content) < 2 {
		t.Fatalf("Content len=%d want 2 (error + raw output)", len(result.Content))
	}
	if !strings.Contains(result.Content[0].Text, "output_invalid") {
		t.Errorf("Content[0]: %+v", result.Content[0])
	}
	if !strings.Contains(result.Content[1].Text, `{"wrong":"shape"}`) {
		t.Errorf("Content[1]: %+v", result.Content[1])
	}
}

func TestAdapter_CallTool_FunctionNotFoundFromInvoker(t *testing.T) {
	// The adapter checks the tool name first, but the Invoker may
	// also return OutcomeFunctionNotFound (e.g., racy registry update).
	inv := &stubInvoker{
		result: &facade.InvocationResult{
			Outcome:     facade.OutcomeFunctionNotFound,
			ErrorDetail: "no function named echo",
		},
	}
	a := newTestAdapter(t, inv)
	result := a.CallTool(context.Background(), "echo", json.RawMessage(`{}`))
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if !strings.Contains(result.Content[0].Text, "function_not_found") {
		t.Errorf("Content: %+v", result.Content)
	}
}

func TestAdapter_CallTool_PayloadTooLargeIsError(t *testing.T) {
	inv := &stubInvoker{
		result: &facade.InvocationResult{
			Outcome:     facade.OutcomePayloadTooLarge,
			ErrorDetail: "input exceeds maximum size",
		},
	}
	a := newTestAdapter(t, inv)
	result := a.CallTool(context.Background(), "echo", json.RawMessage(`{}`))
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if !strings.Contains(result.Content[0].Text, "payload_too_large") {
		t.Errorf("Content: %+v", result.Content)
	}
}

func TestAdapter_CallTool_SystemErrorIsInternalError(t *testing.T) {
	inv := &stubInvoker{err: errors.New("session-store crash")}
	a := newTestAdapter(t, inv)
	result := a.CallTool(context.Background(), "echo", json.RawMessage(`{}`))
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if !strings.Contains(result.Content[0].Text, "internal_error") {
		t.Errorf("Content: %+v", result.Content)
	}
}

func TestAdapter_NoLogger_FallsBackToDiscard(t *testing.T) {
	// Cover the cfg.Log fallback when caller passes a zero logr.Logger.
	a := NewFunctionToolAdapter(FunctionToolAdapterConfig{
		Invoker: &stubInvoker{result: &facade.InvocationResult{Outcome: facade.OutcomeOK, OutputJSON: json.RawMessage(`{}`)}},
		Tool:    Tool{Name: testToolName, InputSchema: json.RawMessage(`{}`)},
	})
	result := a.CallTool(context.Background(), testToolName, json.RawMessage(`{}`))
	if result.IsError {
		t.Errorf("unexpected IsError=true")
	}
}
