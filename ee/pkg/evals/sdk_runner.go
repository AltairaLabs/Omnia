/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"log/slog"

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/sdk"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// SDKRunner executes evals via the PromptKit SDK's Evaluate() function.
// When a TracerProvider is set, the SDK emits per-eval OTel spans automatically.
type SDKRunner struct {
	tracerProvider trace.TracerProvider
	logger         *slog.Logger
}

// NewSDKRunner creates an SDKRunner. Options can configure tracing and logging.
func NewSDKRunner(opts ...SDKRunnerOption) *SDKRunner {
	r := &SDKRunner{}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// SDKRunnerOption configures an SDKRunner.
type SDKRunnerOption func(*SDKRunner)

// WithTracerProvider sets the OTel tracer provider for per-eval span emission.
func WithTracerProvider(tp trace.TracerProvider) SDKRunnerOption {
	return func(r *SDKRunner) { r.tracerProvider = tp }
}

// WithLogger sets the logger for the SDK runner.
func WithLogger(l *slog.Logger) SDKRunnerOption {
	return func(r *SDKRunner) { r.logger = l }
}

// RunTurnEvals executes per-turn evals via sdk.Evaluate().
func (s *SDKRunner) RunTurnEvals(
	ctx context.Context,
	defs []EvalDef,
	messages []session.Message,
	sessionID string,
	turnIndex int,
	providerSpecs map[string]providers.ProviderSpec,
) []api.EvaluateResultItem {
	return s.evaluate(ctx, defs, messages, sessionID, turnIndex, providerSpecs, runtimeevals.TriggerEveryTurn)
}

// RunSessionEvals executes session-complete evals via sdk.Evaluate().
func (s *SDKRunner) RunSessionEvals(
	ctx context.Context,
	defs []EvalDef,
	messages []session.Message,
	sessionID string,
	turnIndex int,
	providerSpecs map[string]providers.ProviderSpec,
) []api.EvaluateResultItem {
	return s.evaluate(ctx, defs, messages, sessionID, turnIndex, providerSpecs, runtimeevals.TriggerOnSessionComplete)
}

// evaluate calls sdk.Evaluate() with the appropriate trigger and converts results.
func (s *SDKRunner) evaluate(
	ctx context.Context,
	defs []EvalDef,
	messages []session.Message,
	sessionID string,
	turnIndex int,
	providerSpecs map[string]providers.ProviderSpec,
	trigger runtimeevals.EvalTrigger,
) []api.EvaluateResultItem {
	sdkDefs := convertToSDKDefs(defs)
	typesMessages := ConvertToTypesMessages(messages)

	opts := sdk.EvaluateOpts{
		EvalDefs:       sdkDefs,
		Messages:       typesMessages,
		SessionID:      sessionID,
		TurnIndex:      turnIndex,
		Trigger:        trigger,
		TracerProvider: s.tracerProvider,
		Logger:         s.logger,
	}

	if len(providerSpecs) > 0 {
		opts.JudgeTargets = toAnyMap(providerSpecs)
	}

	results, err := sdk.Evaluate(ctx, opts)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("sdk.Evaluate failed",
				"sessionID", sessionID,
				"trigger", string(trigger),
				"error", err,
			)
		}
		return nil
	}

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

// toAnyMap converts a typed map to map[string]any for SDK compatibility.
func toAnyMap(specs map[string]providers.ProviderSpec) map[string]any {
	m := make(map[string]any, len(specs))
	for k, v := range specs {
		m[k] = v
	}
	return m
}
