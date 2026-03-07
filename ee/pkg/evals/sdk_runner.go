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

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	_ "github.com/AltairaLabs/PromptKit/runtime/evals/handlers" // registers default eval handlers
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/types"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// SDKRunner wraps the PromptKit EvalRunner to execute evals via the SDK's
// public API (RunTurnEvals / RunSessionEvals) instead of calling internals.
type SDKRunner struct {
	runner *runtimeevals.EvalRunner
}

// NewSDKRunner creates an SDKRunner backed by the full PromptKit eval registry.
func NewSDKRunner() *SDKRunner {
	registry := runtimeevals.NewEvalTypeRegistry()
	runner := runtimeevals.NewEvalRunner(registry)
	return &SDKRunner{runner: runner}
}

// RunTurnEvals executes per-turn evals via the SDK pipeline.
func (s *SDKRunner) RunTurnEvals(
	ctx context.Context,
	defs []EvalDef,
	messages []session.Message,
	sessionID string,
	turnIndex int,
	providerSpecs map[string]providers.ProviderSpec,
) []api.EvaluateResultItem {
	sdkDefs := convertToSDKDefs(defs)
	evalCtx := buildSDKEvalContext(messages, sessionID, turnIndex, providerSpecs)
	results := s.runner.RunTurnEvals(ctx, sdkDefs, evalCtx)
	return convertSDKResults(results)
}

// RunSessionEvals executes session-complete evals via the SDK pipeline.
func (s *SDKRunner) RunSessionEvals(
	ctx context.Context,
	defs []EvalDef,
	messages []session.Message,
	sessionID string,
	turnIndex int,
	providerSpecs map[string]providers.ProviderSpec,
) []api.EvaluateResultItem {
	sdkDefs := convertToSDKDefs(defs)
	evalCtx := buildSDKEvalContext(messages, sessionID, turnIndex, providerSpecs)
	results := s.runner.RunSessionEvals(ctx, sdkDefs, evalCtx)
	return convertSDKResults(results)
}

// convertToSDKDefs converts Omnia EvalDef to PromptKit SDK EvalDef.
func convertToSDKDefs(defs []EvalDef) []runtimeevals.EvalDef {
	sdkDefs := make([]runtimeevals.EvalDef, len(defs))
	for i, d := range defs {
		sdkDefs[i] = runtimeevals.EvalDef{
			ID:      d.ID,
			Type:    d.Type,
			Trigger: mapTrigger(d.Trigger),
			Params:  d.Params,
		}
	}
	return sdkDefs
}

// mapTrigger converts Omnia trigger strings to SDK EvalTrigger values.
func mapTrigger(trigger string) runtimeevals.EvalTrigger {
	switch trigger {
	case triggerPerTurn:
		return runtimeevals.TriggerEveryTurn
	case triggerOnComplete:
		return runtimeevals.TriggerOnSessionComplete
	default:
		return runtimeevals.EvalTrigger(trigger)
	}
}

// buildSDKEvalContext constructs the PromptKit EvalContext from session messages.
func buildSDKEvalContext(
	messages []session.Message,
	sessionID string,
	turnIndex int,
	providerSpecs map[string]providers.ProviderSpec,
) *runtimeevals.EvalContext {
	typesMessages := ConvertToTypesMessages(messages)

	evalCtx := &runtimeevals.EvalContext{
		Messages:      typesMessages,
		SessionID:     sessionID,
		TurnIndex:     turnIndex,
		CurrentOutput: lastAssistantContent(typesMessages),
	}

	// Build tool call records from messages.
	for i, msg := range typesMessages {
		for _, tc := range msg.ToolCalls {
			evalCtx.ToolCalls = append(evalCtx.ToolCalls, runtimeevals.ToolCallRecord{
				TurnIndex: i,
				ToolName:  tc.Name,
				Arguments: parseArgsToMap(tc.Args),
			})
		}
	}

	// Inject provider specs for LLM judge evals.
	if len(providerSpecs) > 0 {
		evalCtx.Metadata = map[string]any{
			"judge_targets": providerSpecs,
		}
	}

	return evalCtx
}

// lastAssistantContent returns the content of the last assistant message,
// which SDK eval handlers use as CurrentOutput.
func lastAssistantContent(msgs []types.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			return msgs[i].Content
		}
	}
	return ""
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

// convertSDKResults converts PromptKit EvalResult to Omnia EvaluateResultItem.
func convertSDKResults(results []runtimeevals.EvalResult) []api.EvaluateResultItem {
	items := make([]api.EvaluateResultItem, 0, len(results))
	for _, r := range results {
		if r.Skipped {
			continue
		}
		item := api.EvaluateResultItem{
			EvalID:     r.EvalID,
			EvalType:   r.Type,
			Passed:     r.Passed,
			DurationMs: int(r.DurationMs),
			Source:     evalSource,
		}
		if r.Score != nil {
			item.Score = r.Score
		}
		if r.Error != "" {
			item.Passed = false
		}
		items = append(items, item)
	}
	return items
}
