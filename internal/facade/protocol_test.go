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
	"encoding/json"
	"testing"
	"time"
)

const testSessionID = "session-1"

func TestMessageTypes(t *testing.T) {
	tests := []struct {
		msgType  MessageType
		expected string
	}{
		{MessageTypeMessage, "message"},
		{MessageTypeChunk, "chunk"},
		{MessageTypeDone, "done"},
		{MessageTypeToolCall, "tool_call"},
		{MessageTypeToolResult, "tool_result"},
		{MessageTypeError, "error"},
		{MessageTypeConnected, "connected"},
	}

	for _, tt := range tests {
		if string(tt.msgType) != tt.expected {
			t.Errorf("MessageType = %v, want %v", tt.msgType, tt.expected)
		}
	}
}

func TestClientMessageJSON(t *testing.T) {
	msg := ClientMessage{
		Type:      MessageTypeMessage,
		SessionID: "test-session",
		Content:   "Hello, world!",
		Metadata: map[string]string{
			"key": "value",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ClientMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("Type = %v, want %v", decoded.Type, msg.Type)
	}
	if decoded.SessionID != msg.SessionID {
		t.Errorf("SessionID = %v, want %v", decoded.SessionID, msg.SessionID)
	}
	if decoded.Content != msg.Content {
		t.Errorf("Content = %v, want %v", decoded.Content, msg.Content)
	}
	if decoded.Metadata["key"] != msg.Metadata["key"] {
		t.Errorf("Metadata[key] = %v, want %v", decoded.Metadata["key"], msg.Metadata["key"])
	}
}

func TestServerMessageJSON(t *testing.T) {
	msg := ServerMessage{
		Type:      MessageTypeChunk,
		SessionID: "test-session",
		Content:   "chunk content",
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ServerMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("Type = %v, want %v", decoded.Type, msg.Type)
	}
	if decoded.SessionID != msg.SessionID {
		t.Errorf("SessionID = %v, want %v", decoded.SessionID, msg.SessionID)
	}
	if decoded.Content != msg.Content {
		t.Errorf("Content = %v, want %v", decoded.Content, msg.Content)
	}
}

func TestNewChunkMessage(t *testing.T) {
	msg := NewChunkMessage(testSessionID, "test content")

	if msg.Type != MessageTypeChunk {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeChunk)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.Content != "test content" {
		t.Errorf("Content = %v, want 'test content'", msg.Content)
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestNewDoneMessage(t *testing.T) {
	msg := NewDoneMessage(testSessionID, "final content")

	if msg.Type != MessageTypeDone {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeDone)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.Content != "final content" {
		t.Errorf("Content = %v, want 'final content'", msg.Content)
	}
}

func TestNewToolCallMessage(t *testing.T) {
	toolCall := &ToolCallInfo{
		ID:   "tool-1",
		Name: "search",
		Arguments: map[string]interface{}{
			"query": "test query",
		},
	}

	msg := NewToolCallMessage(testSessionID, toolCall)

	if msg.Type != MessageTypeToolCall {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeToolCall)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.ToolCall == nil {
		t.Fatal("ToolCall should not be nil")
	}
	if msg.ToolCall.ID != toolCall.ID {
		t.Errorf("ToolCall.ID = %v, want %v", msg.ToolCall.ID, toolCall.ID)
	}
	if msg.ToolCall.Name != toolCall.Name {
		t.Errorf("ToolCall.Name = %v, want %v", msg.ToolCall.Name, toolCall.Name)
	}
}

func TestNewToolResultMessage(t *testing.T) {
	result := &ToolResultInfo{
		ID:     "tool-1",
		Result: "search results",
	}

	msg := NewToolResultMessage(testSessionID, result)

	if msg.Type != MessageTypeToolResult {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeToolResult)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.ToolResult == nil {
		t.Fatal("ToolResult should not be nil")
	}
	if msg.ToolResult.ID != result.ID {
		t.Errorf("ToolResult.ID = %v, want %v", msg.ToolResult.ID, result.ID)
	}
}

func TestNewToolResultMessageWithError(t *testing.T) {
	result := &ToolResultInfo{
		ID:    "tool-1",
		Error: "tool execution failed",
	}

	msg := NewToolResultMessage(testSessionID, result)

	if msg.ToolResult.Error != result.Error {
		t.Errorf("ToolResult.Error = %v, want %v", msg.ToolResult.Error, result.Error)
	}
}

func TestNewErrorMessage(t *testing.T) {
	msg := NewErrorMessage(testSessionID, ErrorCodeInvalidMessage, "invalid format")

	if msg.Type != MessageTypeError {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeError)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if msg.Error.Code != ErrorCodeInvalidMessage {
		t.Errorf("Error.Code = %v, want %v", msg.Error.Code, ErrorCodeInvalidMessage)
	}
	if msg.Error.Message != "invalid format" {
		t.Errorf("Error.Message = %v, want 'invalid format'", msg.Error.Message)
	}
}

func TestNewConnectedMessage(t *testing.T) {
	msg := NewConnectedMessage(testSessionID)

	if msg.Type != MessageTypeConnected {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeConnected)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestErrorCodes(t *testing.T) {
	codes := []string{
		ErrorCodeInvalidMessage,
		ErrorCodeSessionNotFound,
		ErrorCodeSessionExpired,
		ErrorCodeInternalError,
		ErrorCodeAgentUnavailable,
		ErrorCodeToolFailed,
	}

	for _, code := range codes {
		if code == "" {
			t.Error("Error code should not be empty")
		}
	}
}

func TestToolCallInfoJSON(t *testing.T) {
	info := ToolCallInfo{
		ID:   "call-123",
		Name: "calculate",
		Arguments: map[string]interface{}{
			"a": 10,
			"b": 20,
		},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ToolCallInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.ID != info.ID {
		t.Errorf("ID = %v, want %v", decoded.ID, info.ID)
	}
	if decoded.Name != info.Name {
		t.Errorf("Name = %v, want %v", decoded.Name, info.Name)
	}
}

func TestErrorInfoJSON(t *testing.T) {
	info := ErrorInfo{
		Code:    ErrorCodeInternalError,
		Message: "something went wrong",
		Details: map[string]interface{}{
			"trace_id": "abc123",
		},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ErrorInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Code != info.Code {
		t.Errorf("Code = %v, want %v", decoded.Code, info.Code)
	}
	if decoded.Message != info.Message {
		t.Errorf("Message = %v, want %v", decoded.Message, info.Message)
	}
}
