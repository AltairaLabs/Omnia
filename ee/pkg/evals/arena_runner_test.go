/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package evals

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/altairalabs/omnia/internal/session"
	api "github.com/altairalabs/omnia/internal/session/api"
)

func TestRunArenaAssertion_ToolsCalled_Pass(t *testing.T) {
	messages := []session.Message{
		{
			ID: "m1", Role: session.RoleUser,
			Content: "Check the weather", Timestamp: time.Now(),
		},
		{
			ID: "m2", Role: session.RoleAssistant,
			Content:    `{"name":"get_weather","arguments":"{\"city\":\"NYC\"}"}`,
			ToolCallID: "tc-1", Timestamp: time.Now(),
			Metadata: map[string]string{"type": "tool_call"},
		},
		{
			ID: "m3", Role: session.RoleSystem,
			Content:    `{"temp":72}`,
			ToolCallID: "tc-1", Timestamp: time.Now(),
			Metadata: map[string]string{"type": "tool_result"},
		},
		{
			ID: "m4", Role: session.RoleAssistant,
			Content: "It's 72F in NYC.", Timestamp: time.Now(),
		},
	}

	def := api.EvalDefinition{
		ID:      "eval-1",
		Type:    EvalTypeArenaAssertion,
		Trigger: "session_end",
		Params: map[string]any{
			"assertion_type": "tools_called",
			"assertion_params": map[string]any{
				"tool_names": []any{"get_weather"},
			},
		},
	}

	result, err := RunArenaAssertion(def, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected assertion to pass")
	}
	if result.Score == nil || *result.Score != 1.0 {
		t.Errorf("expected score 1.0, got %v", result.Score)
	}
	if result.EvalID != "eval-1" {
		t.Errorf("expected evalID eval-1, got %s", result.EvalID)
	}
	if result.EvalType != EvalTypeArenaAssertion {
		t.Errorf("expected evalType %s, got %s", EvalTypeArenaAssertion, result.EvalType)
	}
	if result.Trigger != "session_end" {
		t.Errorf("expected trigger session_end, got %s", result.Trigger)
	}
}

func TestRunArenaAssertion_ToolsCalled_Fail(t *testing.T) {
	messages := []session.Message{
		{
			ID: "m1", Role: session.RoleUser,
			Content: "Hello", Timestamp: time.Now(),
		},
		{
			ID: "m2", Role: session.RoleAssistant,
			Content: "Hi there!", Timestamp: time.Now(),
		},
	}

	def := api.EvalDefinition{
		ID:   "eval-2",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "tools_called",
			"assertion_params": map[string]any{
				"tool_names": []any{"get_weather"},
			},
		},
	}

	result, err := RunArenaAssertion(def, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected assertion to fail")
	}
	if result.Score == nil || *result.Score != 0.0 {
		t.Errorf("expected score 0.0, got %v", result.Score)
	}
}

func TestRunArenaAssertion_UnknownAssertionType(t *testing.T) {
	def := api.EvalDefinition{
		ID:   "eval-3",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "nonexistent_validator",
		},
	}

	result, err := RunArenaAssertion(def, []session.Message{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected assertion to fail for unknown type")
	}
	if result.Score == nil || *result.Score != 0.0 {
		t.Errorf("expected score 0.0, got %v", result.Score)
	}
}

func TestRunArenaAssertion_MissingAssertionType(t *testing.T) {
	def := api.EvalDefinition{
		ID:     "eval-4",
		Type:   EvalTypeArenaAssertion,
		Params: map[string]any{},
	}

	_, err := RunArenaAssertion(def, []session.Message{})
	if err == nil {
		t.Fatal("expected error for missing assertion_type")
	}
}

func TestRunArenaAssertion_EmptyAssertionType(t *testing.T) {
	def := api.EvalDefinition{
		ID:   "eval-5",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "",
		},
	}

	_, err := RunArenaAssertion(def, []session.Message{})
	if err == nil {
		t.Fatal("expected error for empty assertion_type")
	}
}

func TestRunArenaAssertion_EmptyMessages(t *testing.T) {
	def := api.EvalDefinition{
		ID:   "eval-6",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "tools_called",
			"assertion_params": map[string]any{
				"tool_names": []any{"some_tool"},
			},
		},
	}

	result, err := RunArenaAssertion(def, []session.Message{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected assertion to fail with empty messages")
	}
}

func TestRunArenaAssertion_ContentIncludesAny_Pass(t *testing.T) {
	messages := []session.Message{
		{
			ID: "m1", Role: session.RoleUser,
			Content: "Tell me about Go", Timestamp: time.Now(),
		},
		{
			ID: "m2", Role: session.RoleAssistant,
			Content: "Go is a statically typed programming language.", Timestamp: time.Now(),
		},
	}

	def := api.EvalDefinition{
		ID:   "eval-7",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "contains_any",
			"assertion_params": map[string]any{
				"patterns": []any{"programming", "language"},
			},
		},
	}

	result, err := RunArenaAssertion(def, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected content_includes_any to pass")
	}
}

func TestRunArenaAssertion_ContentNotIncludes_Pass(t *testing.T) {
	messages := []session.Message{
		{
			ID: "m1", Role: session.RoleUser,
			Content: "Tell me about Go", Timestamp: time.Now(),
		},
		{
			ID: "m2", Role: session.RoleAssistant,
			Content: "Go is a great language.", Timestamp: time.Now(),
		},
	}

	def := api.EvalDefinition{
		ID:   "eval-8",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "content_excludes",
			"assertion_params": map[string]any{
				"patterns": []any{"Python", "Java"},
			},
		},
	}

	result, err := RunArenaAssertion(def, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected content_not_includes to pass")
	}
}

func TestRunArenaAssertion_ContentNotIncludes_Fail(t *testing.T) {
	messages := []session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content: "Python is also great.", Timestamp: time.Now(),
		},
	}

	def := api.EvalDefinition{
		ID:   "eval-9",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "content_excludes",
			"assertion_params": map[string]any{
				"patterns": []any{"Python"},
			},
		},
	}

	result, err := RunArenaAssertion(def, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected content_not_includes to fail when content contains forbidden word")
	}
}

func TestRunArenaAssertion_NilAssertionParams(t *testing.T) {
	def := api.EvalDefinition{
		ID:   "eval-10",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "tools_called",
		},
	}

	result, err := RunArenaAssertion(def, []session.Message{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With no tool_names specified, the handler fails (missing required param).
	if result.Passed {
		t.Error("expected tools_called with nil params to fail (missing tool_names)")
	}
}

func TestRunArenaAssertion_ToolCallsWithArgs(t *testing.T) {
	messages := []session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content:    `{"name":"search","arguments":"{\"query\":\"test\",\"limit\":10}"}`,
			ToolCallID: "tc-1", Timestamp: time.Now(),
			Metadata: map[string]string{"type": "tool_call"},
		},
		{
			ID: "m2", Role: session.RoleSystem,
			Content:    `{"results":[]}`,
			ToolCallID: "tc-1", Timestamp: time.Now(),
			Metadata: map[string]string{"type": "tool_result"},
		},
	}

	def := api.EvalDefinition{
		ID:   "eval-11",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "tool_calls_with_args",
			"assertion_params": map[string]any{
				"tool_name":     "search",
				"required_args": map[string]any{"query": "test"},
			},
		},
	}

	result, err := RunArenaAssertion(def, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected tool_calls_with_args to pass")
	}
}

func TestRunArenaAssertion_MultipleToolCalls(t *testing.T) {
	messages := []session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content:    `{"name":"tool_a","arguments":"{}"}`,
			ToolCallID: "tc-1", Timestamp: time.Now(),
			Metadata: map[string]string{"type": "tool_call"},
		},
		{
			ID: "m2", Role: session.RoleSystem,
			Content:    `"ok"`,
			ToolCallID: "tc-1", Timestamp: time.Now(),
			Metadata: map[string]string{"type": "tool_result"},
		},
		{
			ID: "m3", Role: session.RoleAssistant,
			Content:    `{"name":"tool_b","arguments":"{}"}`,
			ToolCallID: "tc-2", Timestamp: time.Now(),
			Metadata: map[string]string{"type": "tool_call"},
		},
		{
			ID: "m4", Role: session.RoleSystem,
			Content:    `"ok"`,
			ToolCallID: "tc-2", Timestamp: time.Now(),
			Metadata: map[string]string{"type": "tool_result"},
		},
	}

	def := api.EvalDefinition{
		ID:   "eval-12",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "tools_called",
			"assertion_params": map[string]any{
				"tool_names": []any{"tool_a", "tool_b"},
			},
		},
	}

	result, err := RunArenaAssertion(def, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected tools_called to pass with both tools present")
	}
}

func TestRunArenaAssertion_ToolsNotCalled_Pass(t *testing.T) {
	messages := []session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content: "I can help you directly.", Timestamp: time.Now(),
		},
	}

	def := api.EvalDefinition{
		ID:   "eval-13",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "tools_not_called",
			"assertion_params": map[string]any{
				"tool_names": []any{"dangerous_tool"},
			},
		},
	}

	result, err := RunArenaAssertion(def, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected tools_not_called to pass")
	}
}

func TestRunArenaAssertion_ToolsNotCalled_Fail(t *testing.T) {
	messages := []session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content:    `{"name":"dangerous_tool","arguments":"{}"}`,
			ToolCallID: "tc-1", Timestamp: time.Now(),
			Metadata: map[string]string{"type": "tool_call"},
		},
	}

	def := api.EvalDefinition{
		ID:   "eval-14",
		Type: EvalTypeArenaAssertion,
		Params: map[string]any{
			"assertion_type": "tools_not_called",
			"assertion_params": map[string]any{
				"tool_names": []any{"dangerous_tool"},
			},
		},
	}

	result, err := RunArenaAssertion(def, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected tools_not_called to fail")
	}
}

func TestExtractToolCallRecords(t *testing.T) {
	messages := ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleUser,
			Content: "Use tools", Timestamp: time.Now(),
		},
		{
			ID: "m2", Role: session.RoleAssistant,
			Content:    `{"name":"search","arguments":"{\"q\":\"test\"}"}`,
			ToolCallID: "tc-1", Timestamp: time.Now(),
			Metadata: map[string]string{"type": "tool_call"},
		},
		{
			ID: "m3", Role: session.RoleSystem,
			Content:    `{"results":[]}`,
			ToolCallID: "tc-1", Timestamp: time.Now(),
			Metadata: map[string]string{"type": "tool_result"},
		},
	})

	records := extractToolCallRecords(messages)

	if len(records) != 1 {
		t.Fatalf("expected 1 tool call record, got %d", len(records))
	}

	tc := records[0]
	if tc.ToolName != "search" {
		t.Errorf("expected tool name 'search', got %q", tc.ToolName)
	}
	if tc.TurnIndex != 1 {
		t.Errorf("expected turn index 1, got %d", tc.TurnIndex)
	}
	if tc.Arguments["q"] != "test" {
		t.Errorf("expected argument q=test, got %v", tc.Arguments)
	}
}

func TestExtractToolCallRecords_NoToolCalls(t *testing.T) {
	messages := ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleUser,
			Content: "Hello", Timestamp: time.Now(),
		},
		{
			ID: "m2", Role: session.RoleAssistant,
			Content: "Hi!", Timestamp: time.Now(),
		},
	})

	records := extractToolCallRecords(messages)

	if len(records) != 0 {
		t.Errorf("expected 0 tool call records, got %d", len(records))
	}
}

func TestParseArgsToMap(t *testing.T) {
	tests := []struct {
		name     string
		input    json.RawMessage
		wantNil  bool
		wantKeys []string
	}{
		{"nil input", nil, true, nil},
		{"empty input", json.RawMessage(""), true, nil},
		{"valid object", json.RawMessage(`{"key":"val"}`), false, []string{"key"}},
		{"invalid json", json.RawMessage("not json"), true, nil},
		{"array input", json.RawMessage(`[1,2,3]`), true, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseArgsToMap(tc.input)
			if tc.wantNil && result != nil {
				t.Errorf("expected nil, got %v", result)
			}
			if !tc.wantNil && result == nil {
				t.Error("expected non-nil result")
			}
			for _, k := range tc.wantKeys {
				if _, ok := result[k]; !ok {
					t.Errorf("missing expected key %q", k)
				}
			}
		})
	}
}

func TestScoreFromPassed(t *testing.T) {
	if s := scoreFromPassed(true); s != 1.0 {
		t.Errorf("expected 1.0, got %f", s)
	}
	if s := scoreFromPassed(false); s != 0.0 {
		t.Errorf("expected 0.0, got %f", s)
	}
}

func TestExtractAssertionType(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]any
		want    string
		wantErr bool
	}{
		{"valid", map[string]any{"assertion_type": "tools_called"}, "tools_called", false},
		{"missing", map[string]any{}, "", true},
		{"empty string", map[string]any{"assertion_type": ""}, "", true},
		{"wrong type", map[string]any{"assertion_type": 42}, "", true},
		{"nil params", nil, "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractAssertionType(tc.params)
			if (err != nil) != tc.wantErr {
				t.Errorf("extractAssertionType() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("extractAssertionType() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractAssertionParams(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		want   int // expected length, -1 for nil
	}{
		{"valid map", map[string]any{"assertion_params": map[string]any{"k": "v"}}, 1},
		{"missing key", map[string]any{}, -1},
		{"wrong type", map[string]any{"assertion_params": "not a map"}, -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractAssertionParams(tc.params)
			if tc.want == -1 && got != nil {
				t.Errorf("expected nil, got %v", got)
			}
			if tc.want >= 0 && len(got) != tc.want {
				t.Errorf("expected len %d, got %d", tc.want, len(got))
			}
		})
	}
}
