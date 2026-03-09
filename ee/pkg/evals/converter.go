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
	"strconv"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/altairalabs/omnia/internal/session"
)

// Metadata keys used by the recording writer.
const (
	metaKeyType      = "type"
	metaKeyLatencyMs = "latency_ms"
	metaKeyCostUSD   = "cost_usd"
	metaKeyIsError   = "is_error"

	metaTypeToolCall   = "tool_call"
	metaTypeToolResult = "tool_result"
)

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

// ConvertToTypesMessages converts Omnia session messages to PromptKit's
// native types.Message format for use with sdk.Evaluate().
func ConvertToTypesMessages(messages []session.Message) []types.Message {
	result := make([]types.Message, 0, len(messages))
	for i := range messages {
		result = append(result, convertToTypesMessage(&messages[i]))
	}
	return result
}

// convertToTypesMessage converts a single session message to types.Message.
func convertToTypesMessage(msg *session.Message) types.Message {
	tm := types.Message{
		Role:    mapRole(msg.Role),
		Content: msg.Content,
	}

	populateCostInfo(&tm, msg)
	populateLatency(&tm, msg)

	msgType := msg.Metadata[metaKeyType]

	if msgType == metaTypeToolCall {
		tc := extractTypesToolCall(msg)
		if tc != nil {
			tm.ToolCalls = []types.MessageToolCall{*tc}
		}
		return tm
	}

	if msgType == metaTypeToolResult {
		tm.Role = "tool"
		tm.ToolResult = extractToolResult(msg)
		return tm
	}

	return tm
}

// extractTypesToolCall parses tool call info into a types.MessageToolCall.
func extractTypesToolCall(msg *session.Message) *types.MessageToolCall {
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

	args := json.RawMessage(marshalArguments(parsed.Arguments))
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}

	return &types.MessageToolCall{
		ID:   msg.ToolCallID,
		Name: parsed.Name,
		Args: args,
	}
}

// extractToolResult builds a MessageToolResult from a tool_result message.
func extractToolResult(msg *session.Message) *types.MessageToolResult {
	tr := &types.MessageToolResult{
		ID: msg.ToolCallID,
	}
	if msg.Content != "" {
		tr.Parts = []types.ContentPart{types.NewTextPart(msg.Content)}
	}
	if msg.Metadata[metaKeyIsError] == "true" {
		tr.Error = msg.Content
	}
	return tr
}

// populateCostInfo sets CostInfo on the types.Message if token data exists.
func populateCostInfo(tm *types.Message, msg *session.Message) {
	if msg.InputTokens == 0 && msg.OutputTokens == 0 {
		return
	}
	tm.CostInfo = &types.CostInfo{
		InputTokens:  int(msg.InputTokens),
		OutputTokens: int(msg.OutputTokens),
	}
	if costStr, ok := msg.Metadata[metaKeyCostUSD]; ok && costStr != "" {
		if cost, err := strconv.ParseFloat(costStr, 64); err == nil {
			tm.CostInfo.TotalCost = cost
		}
	}
}

// populateLatency sets LatencyMs on the types.Message if available.
func populateLatency(tm *types.Message, msg *session.Message) {
	if latStr, ok := msg.Metadata[metaKeyLatencyMs]; ok && latStr != "" {
		if lat, err := strconv.ParseInt(latStr, 10, 64); err == nil {
			tm.LatencyMs = lat
		}
	}
}
