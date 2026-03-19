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

	// metadataKeyConversationID is the PredictionRequest.Metadata key that the
	// arena engine uses to pass the per-run conversation ID through the pipeline.
	metadataKeyConversationID = "conversation_id"
)

// connEntry holds a WebSocket connection and its facade-assigned session ID.
// Each arena run (conversation) gets its own entry, enabling parallel execution.
type connEntry struct {
	conn      Conn
	sessionID string
	mu        sync.Mutex
}

// Provider implements providers.Provider by wrapping the facade WebSocket protocol.
// It maintains a pool of WebSocket connections keyed by conversation ID so that
// each arena run gets its own facade session and runs can execute in parallel.
// For non-arena usage (no conversation_id in metadata), a default connection
// established via Connect is used.
type Provider struct {
	id     string
	wsURL  string
	dialer Dialer

	mu       sync.Mutex
	conns    map[string]*connEntry // conversation_id → connection
	fallback *connEntry            // default connection from Connect()
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
		conns:  make(map[string]*connEntry),
	}
}

// Connect dials the agent's WebSocket and waits for the connected message.
// This establishes the default (fallback) connection used when no conversation_id
// is present in PredictionRequest metadata.
func (p *Provider) Connect(ctx context.Context) error {
	entry, err := p.dial(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.fallback = entry
	p.mu.Unlock()
	return nil
}

// dial opens a new WebSocket connection and returns a connEntry.
func (p *Provider) dial(ctx context.Context) (*connEntry, error) {
	headers := traceHeaders(ctx)
	conn, err := p.dialer.DialContext(ctx, p.wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to agent: %w", err)
	}

	sessionID, err := waitForConnected(conn, defaultConnectTimeout)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to receive connected message: %w", err)
	}

	_ = conn.SetReadDeadline(time.Time{})

	return &connEntry{conn: conn, sessionID: sessionID}, nil
}

// getOrCreateConn returns the connection entry for a request.
// If the request has a conversation_id, a per-conversation connection is used
// (created on first access). Otherwise the fallback connection is returned.
func (p *Provider) getOrCreateConn(ctx context.Context, req providers.PredictionRequest) (*connEntry, error) {
	cid := conversationID(req)
	if cid == "" {
		p.mu.Lock()
		fb := p.fallback
		p.mu.Unlock()
		if fb == nil {
			return nil, fmt.Errorf("no connection established — call Connect first")
		}
		return fb, nil
	}

	p.mu.Lock()
	entry, ok := p.conns[cid]
	if ok {
		p.mu.Unlock()
		return entry, nil
	}
	p.mu.Unlock()

	// Dial outside the lock to avoid holding it during network I/O.
	newEntry, err := p.dial(ctx)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	// Check again — another goroutine may have created it while we were dialing.
	if existing, ok := p.conns[cid]; ok {
		p.mu.Unlock()
		_ = newEntry.conn.Close()
		return existing, nil
	}
	p.conns[cid] = newEntry
	p.mu.Unlock()
	return newEntry, nil
}

// conversationID extracts the conversation_id from request metadata.
func conversationID(req providers.PredictionRequest) string {
	if cid, ok := req.Metadata[metadataKeyConversationID]; ok {
		if s, ok := cid.(string); ok {
			return s
		}
	}
	return ""
}

// ID returns the provider identifier.
func (p *Provider) ID() string { return p.id }

// Model returns the model name (always "fleet").
func (p *Provider) Model() string { return fleetModel }

// Predict sends the latest user message to the agent via WebSocket and returns the response.
func (p *Provider) Predict(ctx context.Context, req providers.PredictionRequest) (providers.PredictionResponse, error) {
	entry, err := p.getOrCreateConn(ctx, req)
	if err != nil {
		return providers.PredictionResponse{}, err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	start := time.Now()

	content := extractLastUserMessage(req.Messages)
	if content == "" {
		return providers.PredictionResponse{}, fmt.Errorf("no user message found in request")
	}

	if err := sendMessage(entry.conn, entry.sessionID, content); err != nil {
		return providers.PredictionResponse{}, fmt.Errorf("failed to send message: %w", err)
	}

	turnMsgs, err := collectTurnResponse(ctx, entry.conn, entry.sessionID)
	if err != nil {
		return providers.PredictionResponse{}, fmt.Errorf("agent error during turn: %w", err)
	}

	return buildPredictionResponse(turnMsgs, time.Since(start)), nil
}

// PredictStream sends the latest user message and streams the response as chunks.
func (p *Provider) PredictStream(
	ctx context.Context, req providers.PredictionRequest,
) (<-chan providers.StreamChunk, error) {
	entry, err := p.getOrCreateConn(ctx, req)
	if err != nil {
		return nil, err
	}

	entry.mu.Lock()

	content := extractLastUserMessage(req.Messages)
	if content == "" {
		entry.mu.Unlock()
		return nil, fmt.Errorf("no user message found in request")
	}

	if err := sendMessage(entry.conn, entry.sessionID, content); err != nil {
		entry.mu.Unlock()
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	ch := make(chan providers.StreamChunk, 16)
	go streamResponse(ctx, entry, ch)

	return ch, nil
}

// streamResponse reads WebSocket messages and sends them as stream chunks.
func streamResponse(ctx context.Context, entry *connEntry, ch chan<- providers.StreamChunk) {
	defer entry.mu.Unlock()
	defer close(ch)

	turnMsgs, err := collectTurnResponse(ctx, entry.conn, entry.sessionID)
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

// CloseConversation closes and removes the connection for a specific conversation.
func (p *Provider) CloseConversation(conversationID string) {
	p.mu.Lock()
	entry, ok := p.conns[conversationID]
	if ok {
		delete(p.conns, conversationID)
	}
	p.mu.Unlock()
	if entry != nil {
		_ = entry.conn.Close()
	}
}

// SupportsStreaming returns true.
func (p *Provider) SupportsStreaming() bool { return true }

// ShouldIncludeRawOutput returns false.
func (p *Provider) ShouldIncludeRawOutput() bool { return false }

// SessionID returns the session ID of the fallback connection.
func (p *Provider) SessionID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fallback != nil {
		return p.fallback.sessionID
	}
	return ""
}

// Close closes all connections (pooled and fallback).
func (p *Provider) Close() error {
	p.mu.Lock()
	entries := make([]*connEntry, 0, len(p.conns)+1)
	for _, e := range p.conns {
		entries = append(entries, e)
	}
	p.conns = make(map[string]*connEntry)
	if p.fallback != nil {
		entries = append(entries, p.fallback)
		p.fallback = nil
	}
	p.mu.Unlock()

	var firstErr error
	for _, e := range entries {
		if err := e.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
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
