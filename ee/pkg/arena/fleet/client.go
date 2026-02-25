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
	"time"

	"github.com/gorilla/websocket"

	"github.com/altairalabs/omnia/internal/facade"
)

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
	DialContext(ctx context.Context, urlStr string) (Conn, error)
}

// Conn abstracts a WebSocket connection for testing.
type Conn interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// gorillaDialer wraps websocket.Dialer to implement our Dialer interface.
type gorillaDialer struct {
	dialer *websocket.Dialer
}

func (d *gorillaDialer) DialContext(ctx context.Context, urlStr string) (Conn, error) {
	conn, _, err := d.dialer.DialContext(ctx, urlStr, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
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
// returning the session ID.
func waitForConnected(conn Conn) (string, error) {
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

// collectTurnResponse reads server messages until a "done" message is received,
// accumulating assistant text, tool calls, and tool results into the transcript.
func collectTurnResponse(ctx context.Context, conn Conn) ([]Message, error) {
	var messages []Message
	var assistantText string

	for {
		select {
		case <-ctx.Done():
			return messages, ctx.Err()
		default:
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			return messages, fmt.Errorf("read error during turn: %w", err)
		}

		var msg facade.ServerMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return messages, fmt.Errorf("failed to parse server message: %w", err)
		}

		switch msg.Type {
		case facade.MessageTypeChunk:
			assistantText += msg.GetTextContent()

		case facade.MessageTypeDone:
			// Append any final content from the done message
			if doneText := msg.GetTextContent(); doneText != "" {
				assistantText += doneText
			}
			if assistantText != "" {
				messages = append(messages, Message{
					Role:      "assistant",
					Content:   assistantText,
					Timestamp: msg.Timestamp,
				})
			}
			return messages, nil

		case facade.MessageTypeToolCall:
			messages = append(messages, Message{
				Role:      "tool_call",
				Content:   msg.ToolCall.Name,
				ToolCall:  msg.ToolCall,
				Timestamp: msg.Timestamp,
			})

		case facade.MessageTypeToolResult:
			messages = append(messages, Message{
				Role:       "tool_result",
				ToolResult: msg.ToolResult,
				Timestamp:  msg.Timestamp,
			})

		case facade.MessageTypeError:
			errMsg := "unknown error"
			if msg.Error != nil {
				errMsg = msg.Error.Message
			}
			return messages, fmt.Errorf("agent error: %s", errMsg)
		}
	}
}
