// Package fixtures provides shared test data builders for use across all
// service test suites. Using these builders instead of inline struct literals
// ensures consistent fixture shapes and makes data model changes a single-
// point update.
//
// Usage:
//
//	s := fixtures.Session()                          // sensible defaults
//	s := fixtures.Session(fixtures.WithAgent("bot")) // override agent name
//	msg := fixtures.UserMessage("hello")
//	tc := fixtures.ClientToolCall("get_location")
package fixtures

import (
	"fmt"
	"time"

	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/session"
)

// Default values used across all fixtures.
var (
	DefaultAgent     = "test-agent"
	DefaultNamespace = "default"
	DefaultWorkspace = "test-workspace"
	DefaultSessionID = "session-001"
	// BaseTime is a stable timestamp for deterministic tests.
	BaseTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

// ──────────────────────────────────────────────────────────────────────────────
// Session builders
// ──────────────────────────────────────────────────────────────────────────────

// SessionOption configures a Session fixture.
type SessionOption func(*session.Session)

// Session returns a session with sensible defaults. Override with options.
func Session(opts ...SessionOption) *session.Session {
	s := &session.Session{
		ID:            DefaultSessionID,
		AgentName:     DefaultAgent,
		Namespace:     DefaultNamespace,
		WorkspaceName: DefaultWorkspace,
		Status:        session.SessionStatusActive,
		CreatedAt:     BaseTime,
		UpdatedAt:     BaseTime,
		Messages:      []session.Message{},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// WithSessionID sets the session ID.
func WithSessionID(id string) SessionOption {
	return func(s *session.Session) { s.ID = id }
}

// WithAgent sets the agent name.
func WithAgent(name string) SessionOption {
	return func(s *session.Session) { s.AgentName = name }
}

// WithNamespace sets the namespace.
func WithNamespace(ns string) SessionOption {
	return func(s *session.Session) { s.Namespace = ns }
}

// WithWorkspace sets the workspace name.
func WithWorkspace(ws string) SessionOption {
	return func(s *session.Session) { s.WorkspaceName = ws }
}

// WithStatus sets the session status.
func WithStatus(status session.SessionStatus) SessionOption {
	return func(s *session.Session) { s.Status = status }
}

// WithMessages sets the session messages.
func WithMessages(msgs ...session.Message) SessionOption {
	return func(s *session.Session) {
		s.Messages = msgs
		s.MessageCount = int32(len(msgs))
	}
}

// WithTimestamp sets both CreatedAt and UpdatedAt.
func WithTimestamp(t time.Time) SessionOption {
	return func(s *session.Session) {
		s.CreatedAt = t
		s.UpdatedAt = t
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Message builders
// ──────────────────────────────────────────────────────────────────────────────

var msgSeq int32

// resetSeq resets the message sequence counter (for tests that need determinism).
func resetSeq() { msgSeq = 0 }

func nextSeq() int32 {
	msgSeq++
	return msgSeq
}

// UserMessage creates a user message with default metadata.
func UserMessage(content string) session.Message {
	return session.Message{
		ID:          fmt.Sprintf("msg-user-%d", nextSeq()),
		Role:        session.RoleUser,
		Content:     content,
		Timestamp:   BaseTime,
		SequenceNum: msgSeq,
	}
}

// AssistantMessage creates an assistant message.
func AssistantMessage(content string) session.Message {
	return session.Message{
		ID:          fmt.Sprintf("msg-asst-%d", nextSeq()),
		Role:        session.RoleAssistant,
		Content:     content,
		Timestamp:   BaseTime,
		SequenceNum: msgSeq,
	}
}

// SystemMessage creates a system message.
func SystemMessage(content string) session.Message {
	return session.Message{
		ID:          fmt.Sprintf("msg-sys-%d", nextSeq()),
		Role:        session.RoleSystem,
		Content:     content,
		Timestamp:   BaseTime,
		SequenceNum: msgSeq,
	}
}

// ToolCallMessage creates a message representing a tool call.
func ToolCallMessage(toolName, callID string) session.Message {
	return session.Message{
		ID:          fmt.Sprintf("msg-tool-%d", nextSeq()),
		Role:        session.RoleAssistant,
		Content:     fmt.Sprintf("Calling tool: %s", toolName),
		Timestamp:   BaseTime,
		SequenceNum: msgSeq,
		ToolCallID:  callID,
		Metadata:    map[string]string{"type": "tool_call", "tool_name": toolName},
	}
}

// Conversation returns a typical 3-message exchange.
func Conversation() []session.Message {
	resetSeq()
	return []session.Message{
		UserMessage("Hello"),
		AssistantMessage("Hi! How can I help you?"),
		UserMessage("What's the weather?"),
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// WebSocket protocol builders
// ──────────────────────────────────────────────────────────────────────────────

// ClientMsg creates a ClientMessage with the given content.
func ClientMsg(content string) *facade.ClientMessage {
	return &facade.ClientMessage{
		Type:      facade.MessageTypeMessage,
		SessionID: DefaultSessionID,
		Content:   content,
	}
}

// ClientMsgWithParts creates a multimodal ClientMessage.
func ClientMsgWithParts(parts ...facade.ContentPart) *facade.ClientMessage {
	return &facade.ClientMessage{
		Type:      facade.MessageTypeMessage,
		SessionID: DefaultSessionID,
		Parts:     parts,
	}
}

// ClientToolResult creates a tool result ClientMessage.
func ClientToolResult(callID string, result interface{}) *facade.ClientMessage {
	return &facade.ClientMessage{
		Type:      facade.MessageTypeToolResult,
		SessionID: DefaultSessionID,
		ToolResult: &facade.ClientToolResultInfo{
			CallID: callID,
			Result: result,
		},
	}
}

// ClientToolRejection creates a rejected tool result ClientMessage.
func ClientToolRejection(callID, reason string) *facade.ClientMessage {
	return &facade.ClientMessage{
		Type:      facade.MessageTypeToolResult,
		SessionID: DefaultSessionID,
		ToolResult: &facade.ClientToolResultInfo{
			CallID: callID,
			Error:  reason,
		},
	}
}

// ClientToolCall creates a ToolCallInfo for a client-side tool.
func ClientToolCall(name string) *facade.ToolCallInfo {
	return &facade.ToolCallInfo{
		ID:   fmt.Sprintf("tc-%s", name),
		Name: name,
	}
}

// ClientToolCallWithConsent creates a ToolCallInfo with consent fields.
func ClientToolCallWithConsent(name, consentMsg string, categories ...string) *facade.ToolCallInfo {
	return &facade.ToolCallInfo{
		ID:             fmt.Sprintf("tc-%s", name),
		Name:           name,
		ConsentMessage: consentMsg,
		Categories:     categories,
	}
}

// ToolResult creates a ToolResultInfo.
func ToolResult(id string, result interface{}) *facade.ToolResultInfo {
	return &facade.ToolResultInfo{
		ID:     id,
		Result: result,
	}
}

// ToolError creates a ToolResultInfo with an error.
func ToolError(id, errMsg string) *facade.ToolResultInfo {
	return &facade.ToolResultInfo{
		ID:    id,
		Error: errMsg,
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// ContentPart builders
// ──────────────────────────────────────────────────────────────────────────────

// TextPart creates a text ContentPart.
func TextPart(text string) facade.ContentPart {
	return facade.NewTextPart(text)
}

// ImagePart creates an image ContentPart with a URL.
func ImagePart(url, mimeType string) facade.ContentPart {
	return facade.ContentPart{
		Type: facade.ContentPartTypeImage,
		Media: &facade.MediaContent{
			URL:      url,
			MimeType: mimeType,
		},
	}
}

// ImagePartBase64 creates an image ContentPart with base64 data.
func ImagePartBase64(data, mimeType string) facade.ContentPart {
	return facade.ContentPart{
		Type: facade.ContentPartTypeImage,
		Media: &facade.MediaContent{
			Data:     data,
			MimeType: mimeType,
		},
	}
}

// AudioPart creates an audio ContentPart.
func AudioPart(url, mimeType string, durationMs int64) facade.ContentPart {
	return facade.ContentPart{
		Type: facade.ContentPartTypeAudio,
		Media: &facade.MediaContent{
			URL:        url,
			MimeType:   mimeType,
			DurationMs: durationMs,
		},
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// CreateSessionOptions builder
// ──────────────────────────────────────────────────────────────────────────────

// SessionOpts returns CreateSessionOptions with defaults.
func SessionOpts() session.CreateSessionOptions {
	return session.CreateSessionOptions{
		AgentName:     DefaultAgent,
		Namespace:     DefaultNamespace,
		WorkspaceName: DefaultWorkspace,
		TTL:           1 * time.Hour,
	}
}
