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

// Package facade provides the WebSocket facade for agent communication.
package facade

import "time"

// MessageType represents the type of WebSocket message.
type MessageType string

const (
	// Client to Server message types
	MessageTypeMessage MessageType = "message"

	// Server to Client message types
	MessageTypeChunk      MessageType = "chunk"
	MessageTypeDone       MessageType = "done"
	MessageTypeToolCall   MessageType = "tool_call"
	MessageTypeToolResult MessageType = "tool_result"
	MessageTypeError      MessageType = "error"
	MessageTypeConnected  MessageType = "connected"
)

// ClientMessage represents a message sent from client to server.
type ClientMessage struct {
	// Type is the message type (always "message" for client messages).
	Type MessageType `json:"type"`
	// SessionID is the optional session ID for resuming a session.
	SessionID string `json:"session_id,omitempty"`
	// Content is the message content.
	Content string `json:"content"`
	// Metadata contains optional additional data.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ServerMessage represents a message sent from server to client.
type ServerMessage struct {
	// Type is the message type.
	Type MessageType `json:"type"`
	// SessionID is the session identifier.
	SessionID string `json:"session_id,omitempty"`
	// Content is the message content (for chunk, done, error types).
	Content string `json:"content,omitempty"`
	// ToolCall contains tool call details (for tool_call type).
	ToolCall *ToolCallInfo `json:"tool_call,omitempty"`
	// ToolResult contains tool result details (for tool_result type).
	ToolResult *ToolResultInfo `json:"tool_result,omitempty"`
	// Error contains error details (for error type).
	Error *ErrorInfo `json:"error,omitempty"`
	// Timestamp is when the message was created.
	Timestamp time.Time `json:"timestamp"`
}

// ToolCallInfo contains information about a tool call.
type ToolCallInfo struct {
	// ID is the unique identifier for this tool call.
	ID string `json:"id"`
	// Name is the name of the tool being called.
	Name string `json:"name"`
	// Arguments are the arguments passed to the tool.
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ToolResultInfo contains information about a tool result.
type ToolResultInfo struct {
	// ID is the tool call ID this result is for.
	ID string `json:"id"`
	// Result is the tool execution result.
	Result interface{} `json:"result,omitempty"`
	// Error is the error message if the tool failed.
	Error string `json:"error,omitempty"`
}

// ErrorInfo contains error details.
type ErrorInfo struct {
	// Code is the error code.
	Code string `json:"code"`
	// Message is the error message.
	Message string `json:"message"`
	// Details contains additional error details.
	Details map[string]interface{} `json:"details,omitempty"`
}

// Error codes.
const (
	ErrorCodeInvalidMessage   = "INVALID_MESSAGE"
	ErrorCodeSessionNotFound  = "SESSION_NOT_FOUND"
	ErrorCodeSessionExpired   = "SESSION_EXPIRED"
	ErrorCodeInternalError    = "INTERNAL_ERROR"
	ErrorCodeAgentUnavailable = "AGENT_UNAVAILABLE"
	ErrorCodeToolFailed       = "TOOL_FAILED"
)

// NewChunkMessage creates a new chunk message.
func NewChunkMessage(sessionID, content string) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeChunk,
		SessionID: sessionID,
		Content:   content,
		Timestamp: time.Now(),
	}
}

// NewDoneMessage creates a new done message.
func NewDoneMessage(sessionID, content string) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeDone,
		SessionID: sessionID,
		Content:   content,
		Timestamp: time.Now(),
	}
}

// NewToolCallMessage creates a new tool call message.
func NewToolCallMessage(sessionID string, toolCall *ToolCallInfo) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeToolCall,
		SessionID: sessionID,
		ToolCall:  toolCall,
		Timestamp: time.Now(),
	}
}

// NewToolResultMessage creates a new tool result message.
func NewToolResultMessage(sessionID string, result *ToolResultInfo) *ServerMessage {
	return &ServerMessage{
		Type:       MessageTypeToolResult,
		SessionID:  sessionID,
		ToolResult: result,
		Timestamp:  time.Now(),
	}
}

// NewErrorMessage creates a new error message.
func NewErrorMessage(sessionID, code, message string) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeError,
		SessionID: sessionID,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
		Timestamp: time.Now(),
	}
}

// NewConnectedMessage creates a new connected message.
func NewConnectedMessage(sessionID string) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeConnected,
		SessionID: sessionID,
		Timestamp: time.Now(),
	}
}
