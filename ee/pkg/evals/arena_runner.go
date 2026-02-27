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
	_ "github.com/AltairaLabs/PromptKit/runtime/evals/handlers" // registers default eval handlers
	"github.com/AltairaLabs/PromptKit/runtime/providers"
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

// RunArenaAssertionWithProviders executes a PromptKit arena assertion with
// provider specs available for LLM judge evals. Provider specs are injected
// into EvalContext.Metadata["judge_targets"] where PromptKit's llm_judge
// handler can find them.
func RunArenaAssertionWithProviders(
	def api.EvalDefinition,
	messages []session.Message,
	providerSpecs map[string]providers.ProviderSpec,
) (api.EvaluateResultItem, error) {
	return runArenaAssertionInternal(def, messages, providerSpecs)
}

// RunArenaAssertion executes a PromptKit arena assertion against session
// messages using the unified eval pipeline. It matches the EvalRunner function
// signature so it can be composed with RunRuleEval via a dispatcher.
func RunArenaAssertion(def api.EvalDefinition, messages []session.Message) (api.EvaluateResultItem, error) {
	return runArenaAssertionInternal(def, messages, nil)
}

// runArenaAssertionInternal is the shared implementation for arena assertion execution.
// When providerSpecs is non-nil, they are injected into EvalContext.Metadata["judge_targets"]
// so PromptKit's llm_judge handler can create provider instances on demand.
func runArenaAssertionInternal(
	def api.EvalDefinition,
	messages []session.Message,
	providerSpecs map[string]providers.ProviderSpec,
) (api.EvaluateResultItem, error) {
	start := time.Now()

	assertionType, err := extractAssertionType(def.Params)
	if err != nil {
		return api.EvaluateResultItem{}, fmt.Errorf("eval %q: %w", def.ID, err)
	}

	assertionParams := extractAssertionParams(def.Params)

	// Convert assertion to an EvalDef via the unified eval pipeline
	assertionCfg := assertions.AssertionConfig{
		Type:   assertionType,
		Params: assertionParams,
	}
	evalDef := assertionCfg.ToConversationEvalDef(0)
	evalDef.ID = def.ID

	// Build EvalContext from messages
	typesMessages := ConvertToTypesMessages(messages)
	evalCtx := buildEvalContext(typesMessages)

	// Inject provider specs for LLM judge evals
	if len(providerSpecs) > 0 {
		if evalCtx.Metadata == nil {
			evalCtx.Metadata = make(map[string]any)
		}
		evalCtx.Metadata["judge_targets"] = providerSpecs
	}

	// Create registry with built-in handlers and run the eval
	registry := runtimeevals.NewEvalTypeRegistry()
	runner := runtimeevals.NewEvalRunner(registry)
	results := runner.RunConversationEvals(context.Background(), []runtimeevals.EvalDef{evalDef}, evalCtx)

	durationMs := int(time.Since(start).Milliseconds())

	// No result means the eval was skipped or filtered
	if len(results) == 0 {
		score := scoreFromPassed(false)
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

	result := results[0]
	passed := result.Passed && result.Error == ""
	score := scoreFromPassed(passed)

	return api.EvaluateResultItem{
		EvalID:     def.ID,
		EvalType:   EvalTypeArenaAssertion,
		Trigger:    def.Trigger,
		Passed:     passed,
		Score:      &score,
		DurationMs: durationMs,
		Source:     "manual",
	}, nil
}

// buildEvalContext constructs the EvalContext needed by the unified eval pipeline
// from a slice of types.Message.
func buildEvalContext(messages []types.Message) *runtimeevals.EvalContext {
	evalCtx := &runtimeevals.EvalContext{
		Messages: messages,
	}

	for i, msg := range messages {
		for _, tc := range msg.ToolCalls {
			record := runtimeevals.ToolCallRecord{
				TurnIndex: i,
				ToolName:  tc.Name,
				Arguments: parseArgsToMap(tc.Args),
			}
			evalCtx.ToolCalls = append(evalCtx.ToolCalls, record)
		}
	}

	return evalCtx
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
