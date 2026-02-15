/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package evals provides utilities for the PromptKit evaluation pipeline.
package evals

import (
	"encoding/json"

	"github.com/altairalabs/omnia/internal/session"
)

// PromptKitMessage represents a message in PromptKit's expected format.
// This is our intermediate representation -- the actual PromptKit types will
// be wired when the SDK is available.
type PromptKitMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []ToolCall     `json:"toolCalls,omitempty"`
	ToolCallID string         `json:"toolCallId,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ToolCall represents a single tool invocation within an assistant message.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Metadata keys used by the recording writer.
const (
	metaKeyType      = "type"
	metaKeyLatencyMs = "latency_ms"
	metaKeyCostUSD   = "cost_usd"
	metaKeyIsError   = "is_error"

	metaTypeToolCall   = "tool_call"
	metaTypeToolResult = "tool_result"
)

// ConvertMessages converts Omnia session messages to PromptKit format.
// Messages with malformed tool call data are converted without tool call
// information rather than returning an error.
func ConvertMessages(messages []session.Message) []PromptKitMessage {
	result := make([]PromptKitMessage, 0, len(messages))
	for i := range messages {
		result = append(result, convertOne(&messages[i]))
	}
	return result
}

// convertOne converts a single session message to PromptKit format.
func convertOne(msg *session.Message) PromptKitMessage {
	pm := PromptKitMessage{
		Role:    mapRole(msg.Role),
		Content: msg.Content,
	}

	meta := buildMetadata(msg)
	if len(meta) > 0 {
		pm.Metadata = meta
	}

	msgType := msg.Metadata[metaKeyType]

	if msgType == metaTypeToolCall {
		pm.ToolCallID = msg.ToolCallID
		tc := extractToolCall(msg)
		if tc != nil {
			pm.ToolCalls = []ToolCall{*tc}
		}
		return pm
	}

	if msgType == metaTypeToolResult {
		pm.Role = "tool"
		pm.ToolCallID = msg.ToolCallID
		return pm
	}

	return pm
}

// mapRole converts a session MessageRole to a PromptKit role string.
func mapRole(role session.MessageRole) string {
	switch role {
	case session.RoleUser:
		return "user"
	case session.RoleAssistant:
		return "assistant"
	case session.RoleSystem:
		return "system"
	default:
		return string(role)
	}
}

// extractToolCall attempts to parse tool call information from an assistant
// message's content. Returns nil if parsing fails or data is incomplete.
func extractToolCall(msg *session.Message) *ToolCall {
	if msg.Content == "" {
		return nil
	}

	var parsed struct {
		Name      string `json:"name"`
		Arguments any    `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(msg.Content), &parsed); err != nil {
		return nil
	}
	if parsed.Name == "" {
		return nil
	}

	args := marshalArguments(parsed.Arguments)

	return &ToolCall{
		ID:        msg.ToolCallID,
		Name:      parsed.Name,
		Arguments: args,
	}
}

// marshalArguments converts tool call arguments to a JSON string.
func marshalArguments(args any) string {
	if args == nil {
		return ""
	}
	if s, ok := args.(string); ok {
		return s
	}
	data, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	return string(data)
}

// buildMetadata extracts preserved metadata (tokens, cost, latency) from
// the session message, omitting internal type markers.
func buildMetadata(msg *session.Message) map[string]any {
	meta := make(map[string]any)

	if msg.InputTokens > 0 {
		meta["inputTokens"] = msg.InputTokens
	}
	if msg.OutputTokens > 0 {
		meta["outputTokens"] = msg.OutputTokens
	}

	addIfPresent(meta, msg.Metadata, metaKeyLatencyMs, metaKeyLatencyMs)
	addIfPresent(meta, msg.Metadata, metaKeyCostUSD, metaKeyCostUSD)
	addIfPresent(meta, msg.Metadata, metaKeyIsError, metaKeyIsError)

	return meta
}

// addIfPresent copies a value from src to dst if it exists.
func addIfPresent(dst map[string]any, src map[string]string, srcKey, dstKey string) {
	if src == nil {
		return
	}
	if v, ok := src[srcKey]; ok && v != "" {
		dst[dstKey] = v
	}
}
