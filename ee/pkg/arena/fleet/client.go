/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package fleet provides a WebSocket client that drives multi-turn conversations
// against a deployed agent for black-box evaluation in Arena fleet mode.
package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"

	"github.com/altairalabs/omnia/internal/facade"
)

// ConversationResult contains the full transcript and metrics from a fleet conversation.
type ConversationResult struct {
	SessionID string
	Messages  []Message
	Duration  time.Duration
	Error     string
}

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

// Client drives multi-turn WebSocket conversations against a deployed agent.
type Client struct {
	wsURL  string
	dialer Dialer
}

// NewClient creates a new fleet client targeting the given WebSocket URL.
func NewClient(wsURL string) *Client {
	return &Client{
		wsURL: wsURL,
		dialer: &gorillaDialer{
			dialer: &websocket.Dialer{
				HandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

// RunConversation connects to the agent via WebSocket and drives a multi-turn
// conversation using the provided scenario turns. It collects the full transcript
// including assistant responses, tool calls, and tool results.
func (c *Client) RunConversation(ctx context.Context, turns []ScenarioTurn) (*ConversationResult, error) {
	start := time.Now()

	conn, err := c.dialer.DialContext(ctx, c.wsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to agent: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Wait for connected message with session ID
	sessionID, err := waitForConnected(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to receive connected message: %w", err)
	}

	result := &ConversationResult{
		SessionID: sessionID,
		Messages:  make([]Message, 0, len(turns)*2),
	}

	for _, turn := range turns {
		// Record the user message
		result.Messages = append(result.Messages, Message{
			Role:      "user",
			Content:   turn.Content,
			Timestamp: time.Now(),
		})

		// Send the user message
		if err := sendMessage(conn, sessionID, turn.Content); err != nil {
			result.Error = fmt.Sprintf("failed to send message: %v", err)
			result.Duration = time.Since(start)
			return result, nil
		}

		// Collect response messages until done
		turnMsgs, turnErr := collectTurnResponse(ctx, conn)
		result.Messages = append(result.Messages, turnMsgs...)
		if turnErr != nil {
			result.Error = turnErr.Error()
			result.Duration = time.Since(start)
			return result, nil
		}
	}

	result.Duration = time.Since(start)
	return result, nil
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
