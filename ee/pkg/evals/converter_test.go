/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package evals

import (
	"testing"
	"time"

	"github.com/altairalabs/omnia/internal/session"
)

func TestMapRole(t *testing.T) {
	tests := []struct {
		input    session.MessageRole
		expected string
	}{
		{session.RoleUser, "user"},
		{session.RoleAssistant, "assistant"},
		{session.RoleSystem, "system"},
		{session.MessageRole("unknown"), "unknown"},
		{session.MessageRole(""), ""},
	}

	for _, tc := range tests {
		t.Run(string(tc.input), func(t *testing.T) {
			if got := mapRole(tc.input); got != tc.expected {
				t.Errorf("mapRole(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestMarshalArguments(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"object", map[string]any{"key": "val"}, `{"key":"val"}`},
		{"number", float64(42), "42"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := marshalArguments(tc.input); got != tc.expected {
				t.Errorf("marshalArguments(%v) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

const testRoleAssistant = "assistant"

func TestConvertToTypesMessages_EmptyAndNil(t *testing.T) {
	result := ConvertToTypesMessages([]session.Message{})
	if len(result) != 0 {
		t.Fatalf("expected 0 messages for empty slice, got %d", len(result))
	}

	result = ConvertToTypesMessages(nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 messages for nil slice, got %d", len(result))
	}
}

func TestConvertToTypesMessages_RoleMapping(t *testing.T) {
	now := time.Now()
	result := ConvertToTypesMessages([]session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "hello", Timestamp: now},
		{ID: "m2", Role: session.RoleAssistant, Content: "hi", Timestamp: now},
		{ID: "m3", Role: session.RoleSystem, Content: "system", Timestamp: now},
	})

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected role 'user', got %q", result[0].Role)
	}
	if result[1].Role != testRoleAssistant {
		t.Errorf("expected role 'assistant', got %q", result[1].Role)
	}
	if result[2].Role != "system" {
		t.Errorf("expected role 'system', got %q", result[2].Role)
	}
}

func TestConvertToTypesMessages_ToolCall(t *testing.T) {
	now := time.Now()
	result := ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content:    `{"name":"get_weather","arguments":"{\"city\":\"NYC\"}"}`,
			ToolCallID: "tc-1", Timestamp: now,
			Metadata: map[string]string{"type": "tool_call"},
		},
	})

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	msg := result[0]
	if msg.Role != testRoleAssistant {
		t.Errorf("expected role 'assistant', got %q", msg.Role)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].ID != "tc-1" {
		t.Errorf("expected tool call ID 'tc-1', got %q", msg.ToolCalls[0].ID)
	}
	if msg.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got %q", msg.ToolCalls[0].Name)
	}
}

func TestConvertToTypesMessages_ToolResult(t *testing.T) {
	now := time.Now()
	result := ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleSystem,
			Content:    `{"temp":72}`,
			ToolCallID: "tc-1", Timestamp: now,
			Metadata: map[string]string{"type": "tool_result"},
		},
	})

	msg := result[0]
	if msg.Role != "tool" {
		t.Errorf("expected role 'tool', got %q", msg.Role)
	}
	if msg.ToolResult == nil {
		t.Fatal("expected ToolResult to be set")
	}
	if msg.ToolResult.ID != "tc-1" {
		t.Errorf("expected tool result ID 'tc-1', got %q", msg.ToolResult.ID)
	}
	if got := msg.ToolResult.GetTextContent(); got != `{"temp":72}` {
		t.Errorf("expected tool result content, got %q", got)
	}
}

func TestConvertToTypesMessages_ToolResultError(t *testing.T) {
	now := time.Now()
	result := ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleSystem,
			Content:    "connection refused",
			ToolCallID: "tc-1", Timestamp: now,
			Metadata: map[string]string{"type": "tool_result", "is_error": "true"},
		},
	})

	if result[0].ToolResult == nil {
		t.Fatal("expected ToolResult to be set")
	}
	if result[0].ToolResult.Error != "connection refused" {
		t.Errorf("expected error 'connection refused', got %q", result[0].ToolResult.Error)
	}
}

func TestConvertToTypesMessages_Metadata(t *testing.T) {
	now := time.Now()
	result := ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content: "response", Timestamp: now,
			InputTokens: 100, OutputTokens: 50,
			Metadata: map[string]string{
				"latency_ms": "250",
				"cost_usd":   "0.0015",
			},
		},
	})

	msg := result[0]
	if msg.CostInfo == nil {
		t.Fatal("expected CostInfo to be set")
	}
	if msg.CostInfo.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", msg.CostInfo.InputTokens)
	}
	if msg.CostInfo.OutputTokens != 50 {
		t.Errorf("expected 50 output tokens, got %d", msg.CostInfo.OutputTokens)
	}
	if msg.CostInfo.TotalCost != 0.0015 {
		t.Errorf("expected total cost 0.0015, got %f", msg.CostInfo.TotalCost)
	}
	if msg.LatencyMs != 250 {
		t.Errorf("expected latency 250, got %d", msg.LatencyMs)
	}
}

func TestConvertToTypesMessages_NoCostInfo(t *testing.T) {
	now := time.Now()
	result := ConvertToTypesMessages([]session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "hello", Timestamp: now},
	})
	if result[0].CostInfo != nil {
		t.Error("expected CostInfo to be nil for user message")
	}
}

func TestConvertToTypesMessages_MalformedToolCall(t *testing.T) {
	now := time.Now()
	result := ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content:    "not valid json",
			ToolCallID: "tc-1", Timestamp: now,
			Metadata: map[string]string{"type": "tool_call"},
		},
	})
	if len(result[0].ToolCalls) != 0 {
		t.Error("expected no tool calls for malformed content")
	}
}

func TestConvertToTypesMessages_EmptyContentToolCall(t *testing.T) {
	now := time.Now()
	result := ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content:    "",
			ToolCallID: "tc-1", Timestamp: now,
			Metadata: map[string]string{"type": "tool_call"},
		},
	})
	if len(result[0].ToolCalls) != 0 {
		t.Error("expected no tool calls for empty content")
	}
}

func TestConvertToTypesMessages_MissingNameToolCall(t *testing.T) {
	now := time.Now()
	result := ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content:    `{"arguments":"test"}`,
			ToolCallID: "tc-1", Timestamp: now,
			Metadata: map[string]string{"type": "tool_call"},
		},
	})
	if len(result[0].ToolCalls) != 0 {
		t.Error("expected no tool calls when name is missing")
	}
}

func TestConvertToTypesMessages_NilArguments(t *testing.T) {
	now := time.Now()
	result := ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content:    `{"name":"my_tool"}`,
			ToolCallID: "tc-1", Timestamp: now,
			Metadata: map[string]string{"type": "tool_call"},
		},
	})
	if len(result[0].ToolCalls) != 1 {
		t.Fatal("expected 1 tool call")
	}
	if string(result[0].ToolCalls[0].Args) != "{}" {
		t.Errorf("expected args '{}' for nil arguments, got %q", string(result[0].ToolCalls[0].Args))
	}
}

func TestConvertToTypesMessages_InvalidLatencyAndCost(t *testing.T) {
	now := time.Now()

	result := ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content: "test", Timestamp: now,
			Metadata: map[string]string{"latency_ms": "not-a-number"},
		},
	})
	if result[0].LatencyMs != 0 {
		t.Errorf("expected latency 0 for invalid value, got %d", result[0].LatencyMs)
	}

	result = ConvertToTypesMessages([]session.Message{
		{
			ID: "m1", Role: session.RoleAssistant,
			Content: "test", Timestamp: now,
			InputTokens: 10,
			Metadata:    map[string]string{"cost_usd": "invalid"},
		},
	})
	if result[0].CostInfo == nil {
		t.Fatal("expected CostInfo to be set (has tokens)")
	}
	if result[0].CostInfo.TotalCost != 0 {
		t.Errorf("expected total cost 0 for invalid value, got %f", result[0].CostInfo.TotalCost)
	}
}
