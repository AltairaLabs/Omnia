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
	"strings"

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/AltairaLabs/PromptKit/runtime/events"
	sdkmetrics "github.com/AltairaLabs/PromptKit/runtime/metrics"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/sdk"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// SDKRunner executes evals via the PromptKit SDK's Evaluate() function.
type SDKRunner struct {
	tracerProvider     trace.TracerProvider
	logger             *slog.Logger
	evalCollector      *sdkmetrics.Collector
	metrics            WorkerMetricsRecorder
	providerCallWriter ProviderCallWriter
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

// WithMetrics sets the WorkerMetricsRecorder for recording operational metrics
// (eval executions, sampling decisions) from within the SDK evaluation loop.
func WithMetrics(m WorkerMetricsRecorder) SDKRunnerOption {
	return func(r *SDKRunner) { r.metrics = m }
}

// WithProviderCallWriter wires a writer that persists the provider calls the
// eval pipeline emits (judge LLM calls, RAG-eval embeddings, …). When set, the
// runner attaches an event bus to sdk.Evaluate and forwards each
// ProviderCallCompleted/Failed to session-api. When nil (default), no bus is
// attached and the events are dropped — preserving the prior behavior.
func WithProviderCallWriter(w ProviderCallWriter) SDKRunnerOption {
	return func(r *SDKRunner) { r.providerCallWriter = w }
}

// EvalCollector returns the unified metrics Collector, if any.
func (s *SDKRunner) EvalCollector() *sdkmetrics.Collector {
	return s.evalCollector
}

// EvalLabels carries per-evaluation context for a single RunTurnEvals /
// RunSessionEvals call. Agent / Namespace / PromptPackName are bound to
// Prometheus metric labels. Groups is NOT a metric label — it's passed
// through to sdk.EvaluateOpts.EvalGroups to filter which evals this
// invocation executes. An empty or nil Groups leaves the SDK's default
// behavior (run all defs) in place; callers that want worker-scoped
// filtering should populate it.
type EvalLabels struct {
	Agent          string
	Namespace      string
	PromptPackName string
	Variant        string
	Groups         []string
}

// Prometheus instance-label keys for omnia_eval_<name> series.
const (
	labelKeyAgent          = "agent"
	labelKeyNamespace      = "namespace"
	labelKeyPromptPackName = "promptpack_name"
	labelKeyVariant        = "variant"
)

// evalInstanceLabels builds the Prometheus instance-label set attached to every
// omnia_eval_<name> series. variant ("stable"/"candidate") lets RolloutAnalysis
// gate eval quality on the rollout candidate; see
// docs/src/content/docs/explanation/rollout-strategies.md.
func evalInstanceLabels(labels EvalLabels) map[string]string {
	return map[string]string{
		labelKeyAgent:          labels.Agent,
		labelKeyNamespace:      labels.Namespace,
		labelKeyPromptPackName: labels.PromptPackName,
		labelKeyVariant:        labels.Variant,
	}
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
	// An answerless session has nothing to judge: when the provider stream fails
	// (a 429/5xx yields an empty answer) every assistant turn is empty. Running
	// quality evals on it would emit a score of 0 that an eval-gated rollout
	// reads as a quality regression — so an infra failure would masquerade as a
	// bad answer and could trip an auto-rollback. Skip these sessions entirely
	// (no judge call, no omnia_eval_<name> series emitted).
	if !hasEvaluableAnswer(messages) {
		if s.logger != nil {
			s.logger.Info("evals skipped",
				"reason", "noAssistantAnswer",
				"sessionID", sessionID,
				"trigger", string(trigger))
		}
		return nil
	}

	opts := sdk.EvaluateOpts{
		Messages:             ConvertToTypesMessages(messages),
		SessionID:            sessionID,
		TurnIndex:            turnIndex,
		Trigger:              trigger,
		TracerProvider:       s.tracerProvider,
		Logger:               s.logger,
		PackData:             packData,
		SkipSchemaValidation: true,
		EvalGroups:           labels.Groups,
	}

	if s.evalCollector != nil {
		opts.MetricsCollector = s.evalCollector
		opts.MetricsInstanceLabels = evalInstanceLabels(labels)
	}

	if len(providerSpecs) > 0 {
		opts.JudgeTargets = toAnyMap(providerSpecs)
	}

	// Capture the provider calls the eval pipeline makes (judge LLM calls,
	// RAG-eval embeddings, …) so their token usage is recorded. sdk.Evaluate
	// only emits these when an EventBus is attached.
	var collector *providerCallCollector
	var bus *events.EventBus
	if s.providerCallWriter != nil {
		bus = events.NewEventBus()
		collector = newProviderCallCollector(sessionID, labels.Namespace, labels.Agent)
		bus.Subscribe(events.EventProviderCallCompleted, collector.onCompleted)
		bus.Subscribe(events.EventProviderCallFailed, collector.onFailed)
		opts.EventBus = bus
	}

	results, err := sdk.Evaluate(ctx, opts)

	// Drain the bus (dispatch is async) before reading + forwarding the calls.
	if bus != nil {
		bus.Close()
		flushProviderCalls(ctx, s.providerCallWriter, s.logger, collector.collected())
	}

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

	s.recordEvalMetrics(results, trigger)

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
// Boolean evals (contains, regex) store pass/fail in Value; score evals pass if score > 0.
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

// recordEvalMetrics records operational metrics for all eval results,
// including both executed and skipped evals.
func (s *SDKRunner) recordEvalMetrics(results []runtimeevals.EvalResult, trigger runtimeevals.EvalTrigger) {
	if s.metrics == nil {
		return
	}
	triggerStr := string(trigger)
	for _, r := range results {
		if r.Skipped {
			s.metrics.RecordSamplingDecision(r.Type, MetricStatusSkipped)
			continue
		}
		s.metrics.RecordSamplingDecision(r.Type, MetricStatusSampled)
		status := MetricStatusSuccess
		if r.Error != "" {
			status = MetricStatusError
		}
		durationSec := float64(r.DurationMs) / 1000.0
		s.metrics.RecordEvalExecuted(r.Type, triggerStr, status, durationSec)
	}
}

// hasEvaluableAnswer reports whether the session contains at least one assistant
// message with non-empty text content. Sessions where every assistant turn is
// empty — e.g. the provider stream failed and returned no content — have nothing
// to evaluate; quality evals on them would emit a misleading score of 0.
func hasEvaluableAnswer(messages []session.Message) bool {
	for _, m := range messages {
		if m.Role == session.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return true
		}
	}
	return false
}

// toAnyMap converts a typed map to map[string]any for SDK compatibility.
func toAnyMap(specs map[string]providers.ProviderSpec) map[string]any {
	m := make(map[string]any, len(specs))
	for k, v := range specs {
		m[k] = v
	}
	return m
}
