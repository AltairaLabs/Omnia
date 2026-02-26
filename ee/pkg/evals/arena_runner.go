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

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	_ "github.com/AltairaLabs/PromptKit/runtime/evals/handlers" // Register built-in eval handlers.
	"github.com/AltairaLabs/PromptKit/runtime/types"

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
	toolCalls := extractToolCallRecords(typesMessages)

	registry := runtimeevals.NewEvalTypeRegistry()
	runner := runtimeevals.NewEvalRunner(registry)

	evalDef := runtimeevals.EvalDef{
		ID:      def.ID,
		Type:    assertionType,
		Trigger: runtimeevals.TriggerOnConversationComplete,
		Params:  assertionParams,
	}

	evalCtx := &runtimeevals.EvalContext{
		Messages:  typesMessages,
		ToolCalls: toolCalls,
		SessionID: def.ID,
	}

	results := runner.RunConversationEvals(context.Background(), []runtimeevals.EvalDef{evalDef}, evalCtx)

	durationMs := int(time.Since(start).Milliseconds())

	if len(results) == 0 {
		// Handler not found or eval was skipped
		score := 0.0
		return api.EvaluateResultItem{
			EvalID:     def.ID,
			EvalType:   EvalTypeArenaAssertion,
			Trigger:    def.Trigger,
			Passed:     false,
			Score:      &score,
			DurationMs: durationMs,
			Source:     "manual",
		}, nil
	}

	r := results[0]
	score := scoreFromPassed(r.Passed)

	return api.EvaluateResultItem{
		EvalID:     def.ID,
		EvalType:   EvalTypeArenaAssertion,
		Trigger:    def.Trigger,
		Passed:     r.Passed,
		Score:      &score,
		DurationMs: durationMs,
		Source:     "manual",
	}, nil
}

// extractToolCallRecords builds ToolCallRecord entries from messages for the EvalContext.
func extractToolCallRecords(messages []types.Message) []runtimeevals.ToolCallRecord {
	var records []runtimeevals.ToolCallRecord
	for i, msg := range messages {
		for _, tc := range msg.ToolCalls {
			records = append(records, runtimeevals.ToolCallRecord{
				TurnIndex: i,
				ToolName:  tc.Name,
				Arguments: parseArgsToMap(tc.Args),
			})
		}
	}
	return records
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
