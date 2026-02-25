/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package fleet

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/types"
)

const (
	// fleetModel is the model name reported by the fleet provider.
	fleetModel = "fleet"
)

// Provider implements providers.Provider by wrapping the facade WebSocket protocol.
// It sends only the latest user message per turn (the agent maintains its own session state)
// and collects the response, making fleet mode compatible with the PromptKit engine pipeline.
type Provider struct {
	id        string
	wsURL     string
	dialer    Dialer
	conn      Conn
	sessionID string
	mu        sync.Mutex
}

// NewProvider creates a new fleet provider targeting the given WebSocket URL.
// If dialer is nil, a default gorilla WebSocket dialer is used.
func NewProvider(id, wsURL string, dialer Dialer) *Provider {
	if dialer == nil {
		dialer = newDefaultDialer()
	}
	return &Provider{
		id:     id,
		wsURL:  wsURL,
		dialer: dialer,
	}
}

// Connect dials the agent's WebSocket and waits for the connected message.
func (p *Provider) Connect(ctx context.Context) error {
	conn, err := p.dialer.DialContext(ctx, p.wsURL)
	if err != nil {
		return fmt.Errorf("failed to connect to agent: %w", err)
	}

	sessionID, err := waitForConnected(conn)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to receive connected message: %w", err)
	}

	p.conn = conn
	p.sessionID = sessionID
	return nil
}

// ID returns the provider identifier.
func (p *Provider) ID() string { return p.id }

// Model returns the model name (always "fleet").
func (p *Provider) Model() string { return fleetModel }

// Predict sends the latest user message to the agent via WebSocket and returns the response.
// Only the last user message from the request is sent — the agent maintains its own session state.
func (p *Provider) Predict(ctx context.Context, req providers.PredictionRequest) (providers.PredictionResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	start := time.Now()

	content := extractLastUserMessage(req.Messages)
	if content == "" {
		return providers.PredictionResponse{}, fmt.Errorf("no user message found in request")
	}

	if err := sendMessage(p.conn, p.sessionID, content); err != nil {
		return providers.PredictionResponse{}, fmt.Errorf("failed to send message: %w", err)
	}

	turnMsgs, err := collectTurnResponse(ctx, p.conn)
	if err != nil {
		return providers.PredictionResponse{}, fmt.Errorf("agent error during turn: %w", err)
	}

	resp := buildPredictionResponse(turnMsgs, time.Since(start))
	return resp, nil
}

// PredictStream sends the latest user message and streams the response as chunks.
func (p *Provider) PredictStream(
	ctx context.Context, req providers.PredictionRequest,
) (<-chan providers.StreamChunk, error) {
	p.mu.Lock()

	content := extractLastUserMessage(req.Messages)
	if content == "" {
		p.mu.Unlock()
		return nil, fmt.Errorf("no user message found in request")
	}

	if err := sendMessage(p.conn, p.sessionID, content); err != nil {
		p.mu.Unlock()
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	ch := make(chan providers.StreamChunk, 16)
	go p.streamResponse(ctx, ch)

	return ch, nil
}

// streamResponse reads WebSocket messages and sends them as stream chunks.
func (p *Provider) streamResponse(ctx context.Context, ch chan<- providers.StreamChunk) {
	defer p.mu.Unlock()
	defer close(ch)

	turnMsgs, err := collectTurnResponse(ctx, p.conn)
	if err != nil {
		finishReason := "error"
		ch <- providers.StreamChunk{
			Error:        err,
			FinishReason: &finishReason,
		}
		return
	}

	resp := buildPredictionResponse(turnMsgs, 0)
	finishReason := "stop"
	ch <- providers.StreamChunk{
		Content:      resp.Content,
		Delta:        resp.Content,
		ToolCalls:    resp.ToolCalls,
		FinishReason: &finishReason,
	}
}

// SupportsStreaming returns true.
func (p *Provider) SupportsStreaming() bool { return true }

// ShouldIncludeRawOutput returns false.
func (p *Provider) ShouldIncludeRawOutput() bool { return false }

// Close closes the WebSocket connection.
func (p *Provider) Close() error {
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// CalculateCost returns zero cost — the agent handles its own LLM costs internally.
func (p *Provider) CalculateCost(_, _, _ int) types.CostInfo {
	return types.CostInfo{}
}

// extractLastUserMessage returns the content of the last user message in the list.
func extractLastUserMessage(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

// buildPredictionResponse converts fleet Message slice into a PredictionResponse.
func buildPredictionResponse(msgs []Message, latency time.Duration) providers.PredictionResponse {
	var contentBuilder strings.Builder
	var toolCalls []types.MessageToolCall

	for _, msg := range msgs {
		switch msg.Role {
		case "assistant":
			contentBuilder.WriteString(msg.Content)
		case "tool_call":
			if msg.ToolCall != nil {
				toolCalls = append(toolCalls, types.MessageToolCall{
					ID:   msg.ToolCall.ID,
					Name: msg.ToolCall.Name,
				})
			}
		}
	}

	return providers.PredictionResponse{
		Content:   contentBuilder.String(),
		Latency:   latency,
		ToolCalls: toolCalls,
	}
}
