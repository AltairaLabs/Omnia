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
	"log/slog"

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	sdkmetrics "github.com/AltairaLabs/PromptKit/runtime/metrics"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/sdk"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// SDKRunner executes evals via the PromptKit SDK's Evaluate() function.
type SDKRunner struct {
	tracerProvider trace.TracerProvider
	logger         *slog.Logger
	evalCollector  *sdkmetrics.Collector
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

// WithEvalCollector sets the unified PromptKit metrics Collector so sdk.Evaluate()
// records per-eval Prometheus metrics (e.g., omnia_eval_helpfulness).
func WithEvalCollector(c *sdkmetrics.Collector) SDKRunnerOption {
	return func(r *SDKRunner) { r.evalCollector = c }
}

// EvalCollector returns the unified metrics Collector, if any.
func (s *SDKRunner) EvalCollector() *sdkmetrics.Collector {
	return s.evalCollector
}

// EvalLabels carries per-evaluation instance label values for metrics binding.
type EvalLabels struct {
	Agent          string
	Namespace      string
	PromptPackName string
}

// RunTurnEvals executes per-turn evals via sdk.Evaluate().
func (s *SDKRunner) RunTurnEvals(
	ctx context.Context,
	packData []byte,
	messages []session.Message,
	sessionID string,
	turnIndex int,
	providerSpecs map[string]providers.ProviderSpec,
	labels EvalLabels,
) []api.EvaluateResultItem {
	return s.evaluate(ctx, packData, messages, sessionID, turnIndex, providerSpecs, runtimeevals.TriggerEveryTurn, labels)
}

// RunSessionEvals executes session-complete evals via sdk.Evaluate().
func (s *SDKRunner) RunSessionEvals(
	ctx context.Context,
	packData []byte,
	messages []session.Message,
	sessionID string,
	turnIndex int,
	providerSpecs map[string]providers.ProviderSpec,
	labels EvalLabels,
) []api.EvaluateResultItem {
	return s.evaluate(ctx, packData, messages, sessionID, turnIndex,
		providerSpecs, runtimeevals.TriggerOnSessionComplete, labels)
}

// evaluate calls sdk.Evaluate() with PackData and converts results.
func (s *SDKRunner) evaluate(
	ctx context.Context,
	packData []byte,
	messages []session.Message,
	sessionID string,
	turnIndex int,
	providerSpecs map[string]providers.ProviderSpec,
	trigger runtimeevals.EvalTrigger,
	labels EvalLabels,
) []api.EvaluateResultItem {
	opts := sdk.EvaluateOpts{
		Messages:             ConvertToTypesMessages(messages),
		SessionID:            sessionID,
		TurnIndex:            turnIndex,
		Trigger:              trigger,
		TracerProvider:       s.tracerProvider,
		Logger:               s.logger,
		PackData:             packData,
		SkipSchemaValidation: true,
	}

	if s.evalCollector != nil {
		opts.MetricsCollector = s.evalCollector
		opts.MetricsInstanceLabels = map[string]string{
			"agent":           labels.Agent,
			"namespace":       labels.Namespace,
			"promptpack_name": labels.PromptPackName,
		}
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

	return convertSDKResults(results, trigger)
}

// convertSDKResults converts PromptKit EvalResult to Omnia EvaluateResultItem.
func convertSDKResults(results []runtimeevals.EvalResult, trigger runtimeevals.EvalTrigger) []api.EvaluateResultItem {
	items := make([]api.EvaluateResultItem, 0, len(results))
	for _, r := range results {
		if r.Skipped {
			continue
		}
		item := api.EvaluateResultItem{
			EvalID:     r.EvalID,
			EvalType:   r.Type,
			Trigger:    string(trigger),
			Passed:     derivePassedFromResult(r),
			DurationMs: int(r.DurationMs),
			Source:     evalSource,
		}
		if r.Score != nil {
			item.Score = r.Score
		}
		item.Details = buildDetailsJSON(r)
		items = append(items, item)
	}
	return items
}

// derivePassedFromResult determines pass/fail from an EvalResult.
// Boolean evals store pass/fail in Value; score evals pass if score > 0.
func derivePassedFromResult(r runtimeevals.EvalResult) bool {
	if r.Error != "" {
		return false
	}
	if b, ok := r.Value.(bool); ok {
		return b
	}
	if r.Score != nil {
		return *r.Score > 0
	}
	return false
}

// buildDetailsJSON assembles a details JSON blob from the SDK result's
// diagnostic fields. Returns nil if no diagnostic fields are populated.
func buildDetailsJSON(r runtimeevals.EvalResult) json.RawMessage {
	details := make(map[string]any)
	if r.Explanation != "" {
		details["explanation"] = r.Explanation
	}
	if r.Error != "" {
		details["error"] = r.Error
	}
	if r.Message != "" {
		details["message"] = r.Message
	}
	if len(r.Details) > 0 {
		details["details"] = r.Details
	}
	if len(r.Violations) > 0 {
		details["violations"] = r.Violations
	}
	if len(details) == 0 {
		return nil
	}
	data, err := json.Marshal(details)
	if err != nil {
		return nil
	}
	return data
}

// toAnyMap converts a typed map to map[string]any for SDK compatibility.
func toAnyMap(specs map[string]providers.ProviderSpec) map[string]any {
	m := make(map[string]any, len(specs))
	for k, v := range specs {
		m[k] = v
	}
	return m
}
