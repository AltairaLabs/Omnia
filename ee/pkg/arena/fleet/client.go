/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package fleet provides a WebSocket-based PromptKit provider that drives
// multi-turn conversations against a deployed agent for black-box evaluation
// in Arena fleet mode.
package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/altairalabs/omnia/internal/facade"
)

// defaultConnectTimeout is the maximum time to wait for the "connected" handshake.
const defaultConnectTimeout = 30 * time.Second

// Message represents a single message in the conversation transcript.
type Message struct {
	Role       string
	Content    string
	ToolCall   *facade.ToolCallInfo
	ToolResult *facade.ToolResultInfo
	Timestamp  time.Time
}

// Dialer abstracts WebSocket connection creation for testing.
type Dialer interface {
	DialContext(ctx context.Context, urlStr string, headers http.Header) (Conn, error)
}

// Conn abstracts a WebSocket connection for testing.
type Conn interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(messageType int, data []byte) error
	SetReadDeadline(t time.Time) error
	Close() error
}

// gorillaDialer wraps websocket.Dialer to implement our Dialer interface.
type gorillaDialer struct {
	dialer *websocket.Dialer
}

func (d *gorillaDialer) DialContext(ctx context.Context, urlStr string, headers http.Header) (Conn, error) {
	conn, _, err := d.dialer.DialContext(ctx, urlStr, headers)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// traceHeaders returns HTTP headers with W3C trace context injected from ctx.
func traceHeaders(ctx context.Context) http.Header {
	h := http.Header{}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(h))
	return h
}

// newDefaultDialer creates a gorilla WebSocket dialer with sensible defaults.
func newDefaultDialer() Dialer {
	return &gorillaDialer{
		dialer: &websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		},
	}
}

// waitForConnected reads messages until it receives a "connected" message,
// returning the session ID. It enforces the given timeout on the read.
func waitForConnected(conn Conn, timeout time.Duration) (string, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return "", fmt.Errorf("failed to set read deadline: %w", err)
	}

	_, data, err := conn.ReadMessage()
	if err != nil {
		return "", fmt.Errorf("read error: %w", err)
	}

	var msg facade.ServerMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return "", fmt.Errorf("failed to parse server message: %w", err)
	}

	if msg.Type != facade.MessageTypeConnected {
		return "", fmt.Errorf("expected connected message, got %q", msg.Type)
	}

	return msg.SessionID, nil
}

// sendMessage sends a user message over the WebSocket connection.
func sendMessage(conn Conn, sessionID, content string) error {
	msg := facade.ClientMessage{
		Type:      facade.MessageTypeMessage,
		SessionID: sessionID,
		Content:   content,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}

// TurnResult holds the outcome of a single turn, including TTFT measurement.
type TurnResult struct {
	Messages []Message
	TTFT     time.Duration // Time to first token (zero if no messages received)
}

// collectTurnResponse reads server messages until a "done" message is received,
// accumulating assistant text, tool calls, and tool results into the transcript.
// When a tool_call is received, it immediately sends a rejection since arena fleet
// clients don't execute client tools.
// It also measures TTFT (time to first token) from the call start to the first message.
func collectTurnResponse(ctx context.Context, conn Conn, sessionID string, turnStart time.Time) (*TurnResult, error) {
	result := &TurnResult{}
	var assistantText string
	firstMessage := true

	for {
		msg, err := readServerMessage(ctx, conn)
		if err != nil {
			return result, err
		}

		if firstMessage {
			result.TTFT = time.Since(turnStart)
			firstMessage = false
		}

		switch msg.Type {
		case facade.MessageTypeChunk:
			assistantText += msg.GetTextContent()

		case facade.MessageTypeDone:
			result.Messages = appendDoneMessage(result.Messages, &assistantText, msg)
			return result, nil

		case facade.MessageTypeToolCall:
			result.Messages = appendToolCallMessage(result.Messages, msg)
			if msg.ToolCall != nil {
				if err := rejectToolCall(conn, sessionID, msg.ToolCall.ID); err != nil {
					return result, err
				}
			}

		case facade.MessageTypeToolResult:
			result.Messages = append(result.Messages, Message{
				Role:       "tool_result",
				ToolResult: msg.ToolResult,
				Timestamp:  msg.Timestamp,
			})

		case facade.MessageTypeError:
			return result, agentError(msg)
		}
	}
}

// readServerMessage reads and parses the next server message, checking for context cancellation.
func readServerMessage(ctx context.Context, conn Conn) (*facade.ServerMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read error during turn: %w", err)
	}

	var msg facade.ServerMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse server message: %w", err)
	}
	return &msg, nil
}

// appendDoneMessage finalizes the assistant text and appends it to the transcript.
func appendDoneMessage(messages []Message, assistantText *string, msg *facade.ServerMessage) []Message {
	if doneText := msg.GetTextContent(); doneText != "" {
		*assistantText += doneText
	}
	if *assistantText != "" {
		messages = append(messages, Message{
			Role:      "assistant",
			Content:   *assistantText,
			Timestamp: msg.Timestamp,
		})
	}
	return messages
}

// appendToolCallMessage adds a tool_call entry to the transcript.
func appendToolCallMessage(messages []Message, msg *facade.ServerMessage) []Message {
	return append(messages, Message{
		Role:      "tool_call",
		Content:   msg.ToolCall.Name,
		ToolCall:  msg.ToolCall,
		Timestamp: msg.Timestamp,
	})
}

// agentError extracts an error from an error-type server message.
func agentError(msg *facade.ServerMessage) error {
	errMsg := "unknown error"
	if msg.Error != nil {
		errMsg = msg.Error.Message
	}
	return fmt.Errorf("agent error: %s", errMsg)
}

// rejectToolCall sends an immediate tool_result rejection over the WebSocket.
func rejectToolCall(conn Conn, sessionID, callID string) error {
	rejection := facade.ClientMessage{
		Type:      facade.MessageTypeToolResult,
		SessionID: sessionID,
		ToolResult: &facade.ClientToolResultInfo{
			CallID: callID,
			Error:  "tool execution not available in arena evaluation mode",
		},
	}
	data, err := json.Marshal(rejection)
	if err != nil {
		return fmt.Errorf("failed to marshal tool rejection: %w", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to reject tool call: %w", err)
	}
	return nil
}
