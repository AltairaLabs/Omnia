/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/PromptKit/tools/arena/assertions"

	"github.com/altairalabs/omnia/internal/session"
	api "github.com/altairalabs/omnia/internal/session/api"
)

// EvalTypeArenaAssertion is the eval type for PromptKit arena assertions.
const EvalTypeArenaAssertion = "arena_assertion"

// Param keys for arena assertion eval definitions.
const (
	paramAssertionType   = "assertion_type"
	paramAssertionParams = "assertion_params"
)

// RunArenaAssertion executes a PromptKit arena conversation assertion against
// session messages. It matches the EvalRunner function signature so it can be
// composed with RunRuleEval via a dispatcher.
func RunArenaAssertion(def api.EvalDefinition, messages []session.Message) (api.EvaluateResultItem, error) {
	start := time.Now()

	assertionType, err := extractAssertionType(def.Params)
	if err != nil {
		return api.EvaluateResultItem{}, fmt.Errorf("eval %q: %w", def.ID, err)
	}

	assertionParams := extractAssertionParams(def.Params)

	typesMessages := ConvertToTypesMessages(messages)
	convCtx := buildConversationContext(typesMessages)

	registry := assertions.NewConversationAssertionRegistry()
	assertion := assertions.ConversationAssertion{
		Type:   assertionType,
		Params: assertionParams,
	}

	result := registry.ValidateConversation(context.Background(), assertion, convCtx)

	durationMs := int(time.Since(start).Milliseconds())
	score := scoreFromPassed(result.Passed)

	return api.EvaluateResultItem{
		EvalID:     def.ID,
		EvalType:   EvalTypeArenaAssertion,
		Trigger:    def.Trigger,
		Passed:     result.Passed,
		Score:      &score,
		DurationMs: durationMs,
		Source:     "manual",
	}, nil
}

// buildConversationContext constructs the ConversationContext needed by arena
// assertion validators from a slice of types.Message.
func buildConversationContext(messages []types.Message) *assertions.ConversationContext {
	convCtx := &assertions.ConversationContext{
		AllTurns: messages,
	}

	for i, msg := range messages {
		for _, tc := range msg.ToolCalls {
			record := assertions.ToolCallRecord{
				TurnIndex: i,
				ToolName:  tc.Name,
				Arguments: parseArgsToMap(tc.Args),
			}
			convCtx.ToolCalls = append(convCtx.ToolCalls, record)
		}
	}

	return convCtx
}

// parseArgsToMap converts JSON-encoded arguments to a map.
func parseArgsToMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// extractAssertionType gets the required assertion_type from params.
func extractAssertionType(params map[string]any) (string, error) {
	v, ok := params[paramAssertionType]
	if !ok {
		return "", fmt.Errorf("missing required param %q", paramAssertionType)
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("param %q must be a non-empty string", paramAssertionType)
	}
	return s, nil
}

// extractAssertionParams gets the optional assertion_params map from params.
func extractAssertionParams(params map[string]any) map[string]any {
	v, ok := params[paramAssertionParams]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return m
}

// scoreFromPassed returns 1.0 for passed, 0.0 for failed.
func scoreFromPassed(passed bool) float64 {
	if passed {
		return 1.0
	}
	return 0.0
}
