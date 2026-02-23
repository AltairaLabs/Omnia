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

func TestConvertMessages(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    []session.Message
		expected []PromptKitMessage
	}{
		{
			name:     "empty slice",
			input:    []session.Message{},
			expected: []PromptKitMessage{},
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: []PromptKitMessage{},
		},
		{
			name: "user message without metadata",
			input: []session.Message{
				{
					ID:        "msg-1",
					Role:      session.RoleUser,
					Content:   "Hello, world",
					Timestamp: now,
				},
			},
			expected: []PromptKitMessage{
				{
					Role:    "user",
					Content: "Hello, world",
				},
			},
		},
		{
			name: "assistant message with tokens and latency",
			input: []session.Message{
				{
					ID:           "msg-2",
					Role:         session.RoleAssistant,
					Content:      "Here is the answer",
					Timestamp:    now,
					InputTokens:  100,
					OutputTokens: 50,
					Metadata: map[string]string{
						"latency_ms": "250",
						"cost_usd":   "0.0015",
					},
				},
			},
			expected: []PromptKitMessage{
				{
					Role:    "assistant",
					Content: "Here is the answer",
					Metadata: map[string]any{
						"inputTokens":  int32(100),
						"outputTokens": int32(50),
						"latency_ms":   "250",
						"cost_usd":     "0.0015",
					},
				},
			},
		},
		{
			name: "system message",
			input: []session.Message{
				{
					ID:        "msg-3",
					Role:      session.RoleSystem,
					Content:   "You are a helpful assistant",
					Timestamp: now,
				},
			},
			expected: []PromptKitMessage{
				{
					Role:    "system",
					Content: "You are a helpful assistant",
				},
			},
		},
		{
			name: "tool call message",
			input: []session.Message{
				{
					ID:         "msg-4",
					Role:       session.RoleAssistant,
					Content:    `{"name":"get_weather","arguments":"{\"city\":\"London\"}"}`,
					ToolCallID: "tc-1",
					Timestamp:  now,
					Metadata: map[string]string{
						"type": "tool_call",
					},
				},
			},
			expected: []PromptKitMessage{
				{
					Role:       "assistant",
					Content:    `{"name":"get_weather","arguments":"{\"city\":\"London\"}"}`,
					ToolCallID: "tc-1",
					ToolCalls: []ToolCall{
						{
							ID:        "tc-1",
							Name:      "get_weather",
							Arguments: `{"city":"London"}`,
						},
					},
				},
			},
		},
		{
			name: "tool call with object arguments",
			input: []session.Message{
				{
					ID:         "msg-5",
					Role:       session.RoleAssistant,
					Content:    `{"name":"search","arguments":{"query":"test","limit":10}}`,
					ToolCallID: "tc-2",
					Timestamp:  now,
					Metadata: map[string]string{
						"type": "tool_call",
					},
				},
			},
			expected: []PromptKitMessage{
				{
					Role:       "assistant",
					Content:    `{"name":"search","arguments":{"query":"test","limit":10}}`,
					ToolCallID: "tc-2",
					ToolCalls: []ToolCall{
						{
							ID:        "tc-2",
							Name:      "search",
							Arguments: `{"limit":10,"query":"test"}`,
						},
					},
				},
			},
		},
		{
			name: "tool result message",
			input: []session.Message{
				{
					ID:         "msg-6",
					Role:       session.RoleSystem,
					Content:    `{"temperature":20,"unit":"celsius"}`,
					ToolCallID: "tc-1",
					Timestamp:  now,
					Metadata: map[string]string{
						"type": "tool_result",
					},
				},
			},
			expected: []PromptKitMessage{
				{
					Role:       "tool",
					Content:    `{"temperature":20,"unit":"celsius"}`,
					ToolCallID: "tc-1",
				},
			},
		},
		{
			name: "tool result with error",
			input: []session.Message{
				{
					ID:         "msg-7",
					Role:       session.RoleSystem,
					Content:    "connection refused",
					ToolCallID: "tc-3",
					Timestamp:  now,
					Metadata: map[string]string{
						"type":     "tool_result",
						"is_error": "true",
					},
				},
			},
			expected: []PromptKitMessage{
				{
					Role:       "tool",
					Content:    "connection refused",
					ToolCallID: "tc-3",
					Metadata: map[string]any{
						"is_error": "true",
					},
				},
			},
		},
		{
			name: "malformed tool call content is skipped",
			input: []session.Message{
				{
					ID:         "msg-8",
					Role:       session.RoleAssistant,
					Content:    "not valid json",
					ToolCallID: "tc-4",
					Timestamp:  now,
					Metadata: map[string]string{
						"type": "tool_call",
					},
				},
			},
			expected: []PromptKitMessage{
				{
					Role:       "assistant",
					Content:    "not valid json",
					ToolCallID: "tc-4",
				},
			},
		},
		{
			name: "tool call with missing name is skipped",
			input: []session.Message{
				{
					ID:         "msg-9",
					Role:       session.RoleAssistant,
					Content:    `{"arguments":"test"}`,
					ToolCallID: "tc-5",
					Timestamp:  now,
					Metadata: map[string]string{
						"type": "tool_call",
					},
				},
			},
			expected: []PromptKitMessage{
				{
					Role:       "assistant",
					Content:    `{"arguments":"test"}`,
					ToolCallID: "tc-5",
				},
			},
		},
		{
			name: "tool call with empty content",
			input: []session.Message{
				{
					ID:         "msg-10",
					Role:       session.RoleAssistant,
					Content:    "",
					ToolCallID: "tc-6",
					Timestamp:  now,
					Metadata: map[string]string{
						"type": "tool_call",
					},
				},
			},
			expected: []PromptKitMessage{
				{
					Role:       "assistant",
					Content:    "",
					ToolCallID: "tc-6",
				},
			},
		},
		{
			name: "message with empty metadata map",
			input: []session.Message{
				{
					ID:        "msg-11",
					Role:      session.RoleUser,
					Content:   "test",
					Timestamp: now,
					Metadata:  map[string]string{},
				},
			},
			expected: []PromptKitMessage{
				{
					Role:    "user",
					Content: "test",
				},
			},
		},
		{
			name: "full conversation flow",
			input: []session.Message{
				{ID: "m1", Role: session.RoleSystem, Content: "You are helpful", Timestamp: now},
				{ID: "m2", Role: session.RoleUser, Content: "What is the weather?", Timestamp: now},
				{
					ID: "m3", Role: session.RoleAssistant,
					Content:    `{"name":"get_weather","arguments":"{\"city\":\"NYC\"}"}`,
					ToolCallID: "tc-10",
					Timestamp:  now,
					Metadata:   map[string]string{"type": "tool_call"},
				},
				{
					ID: "m4", Role: session.RoleSystem,
					Content:    `{"temp":72}`,
					ToolCallID: "tc-10",
					Timestamp:  now,
					Metadata:   map[string]string{"type": "tool_result"},
				},
				{
					ID: "m5", Role: session.RoleAssistant,
					Content:      "It is 72 degrees in NYC.",
					Timestamp:    now,
					InputTokens:  200,
					OutputTokens: 30,
					Metadata:     map[string]string{"latency_ms": "500", "cost_usd": "0.003"},
				},
			},
			expected: []PromptKitMessage{
				{Role: "system", Content: "You are helpful"},
				{Role: "user", Content: "What is the weather?"},
				{
					Role:       "assistant",
					Content:    `{"name":"get_weather","arguments":"{\"city\":\"NYC\"}"}`,
					ToolCallID: "tc-10",
					ToolCalls: []ToolCall{
						{ID: "tc-10", Name: "get_weather", Arguments: `{"city":"NYC"}`},
					},
				},
				{Role: "tool", Content: `{"temp":72}`, ToolCallID: "tc-10"},
				{
					Role: "assistant", Content: "It is 72 degrees in NYC.",
					Metadata: map[string]any{
						"inputTokens":  int32(200),
						"outputTokens": int32(30),
						"latency_ms":   "500",
						"cost_usd":     "0.003",
					},
				},
			},
		},
		{
			name: "unknown role is passed through",
			input: []session.Message{
				{
					ID:        "msg-12",
					Role:      session.MessageRole("custom"),
					Content:   "custom role message",
					Timestamp: now,
				},
			},
			expected: []PromptKitMessage{
				{
					Role:    "custom",
					Content: "custom role message",
				},
			},
		},
		{
			name: "only input tokens set",
			input: []session.Message{
				{
					ID:          "msg-13",
					Role:        session.RoleAssistant,
					Content:     "response",
					Timestamp:   now,
					InputTokens: 50,
				},
			},
			expected: []PromptKitMessage{
				{
					Role:    "assistant",
					Content: "response",
					Metadata: map[string]any{
						"inputTokens": int32(50),
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ConvertMessages(tc.input)
			assertMessagesEqual(t, tc.expected, result)
		})
	}
}

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

// assertMessagesEqual compares two slices of PromptKitMessage.
func assertMessagesEqual(t *testing.T, expected, actual []PromptKitMessage) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Fatalf("message count mismatch: want %d, got %d", len(expected), len(actual))
	}
	for i := range expected {
		assertMessageEqual(t, i, expected[i], actual[i])
	}
}

func assertMessageEqual(t *testing.T, idx int, expected, actual PromptKitMessage) {
	t.Helper()
	prefix := func(field string) string {
		return "message[" + string(rune('0'+idx)) + "]." + field
	}

	assertEqual(t, prefix("Role"), expected.Role, actual.Role)
	assertEqual(t, prefix("Content"), expected.Content, actual.Content)
	assertEqual(t, prefix("ToolCallID"), expected.ToolCallID, actual.ToolCallID)
	assertToolCallsEqual(t, idx, expected.ToolCalls, actual.ToolCalls)
	assertMetadataEqual(t, idx, expected.Metadata, actual.Metadata)
}

func assertEqual(t *testing.T, field, expected, actual string) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s: want %q, got %q", field, expected, actual)
	}
}

func assertToolCallsEqual(t *testing.T, msgIdx int, expected, actual []ToolCall) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Fatalf("message[%d].ToolCalls length: want %d, got %d", msgIdx, len(expected), len(actual))
	}
	for i := range expected {
		if expected[i] != actual[i] {
			t.Errorf("message[%d].ToolCalls[%d]: want %+v, got %+v", msgIdx, i, expected[i], actual[i])
		}
	}
}

func assertMetadataEqual(t *testing.T, msgIdx int, expected, actual map[string]any) {
	t.Helper()
	if len(expected) == 0 && len(actual) == 0 {
		return
	}
	if len(expected) != len(actual) {
		t.Fatalf("message[%d].Metadata length: want %d, got %d\n  expected: %v\n  actual:   %v",
			msgIdx, len(expected), len(actual), expected, actual)
	}
	for k, ev := range expected {
		av, ok := actual[k]
		if !ok {
			t.Errorf("message[%d].Metadata missing key %q", msgIdx, k)
			continue
		}
		if ev != av {
			t.Errorf("message[%d].Metadata[%q]: want %v (%T), got %v (%T)", msgIdx, k, ev, ev, av, av)
		}
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
	if msg.ToolResult.Content != `{"temp":72}` {
		t.Errorf("expected tool result content, got %q", msg.ToolResult.Content)
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
