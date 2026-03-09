/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package tracing provides OpenTelemetry tracing for Omnia components.
package tracing

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TracerName is the name of the tracer used for runtime spans.
	TracerName = "omnia-runtime"
)

// GenAI semantic convention attribute keys.
// See: https://opentelemetry.io/docs/specs/semconv/gen-ai/
const (
	AttrGenAISystem            = "gen_ai.system"
	AttrGenAIOperationName     = "gen_ai.operation.name"
	AttrGenAIRequestModel      = "gen_ai.request.model"
	AttrGenAIResponseModel     = "gen_ai.response.model"
	AttrGenAIResponseFinish    = "gen_ai.response.finish_reasons"
	AttrGenAIUsageInputTokens  = "gen_ai.usage.input_tokens"
	AttrGenAIUsageOutputTokens = "gen_ai.usage.output_tokens"
	AttrGenAIUsageCost         = "gen_ai.usage.cost"
	AttrGenAIPromptLength      = "gen_ai.prompt.length"
	AttrGenAIResponseLength    = "gen_ai.response.length"
)

// Config holds tracing configuration.
type Config struct {
	// Enabled enables tracing.
	Enabled bool

	// Endpoint is the OTLP collector endpoint (e.g., "localhost:4317").
	Endpoint string

	// ServiceName is the service name for traces.
	ServiceName string

	// ServiceVersion is the service version.
	ServiceVersion string

	// Environment is the deployment environment (e.g., "production", "staging").
	Environment string

	// SampleRate is the sampling rate (0.0 to 1.0). Default 1.0 (all traces).
	SampleRate float64

	// Insecure disables TLS for the OTLP connection.
	Insecure bool
}

// Provider wraps the OpenTelemetry TracerProvider.
type Provider struct {
	tp     *sdktrace.TracerProvider
	tracer trace.Tracer
	log    logr.Logger
}

// WithLogger returns a copy of the Provider with the given logger attached.
func (p *Provider) WithLogger(log logr.Logger) *Provider {
	cp := *p
	cp.log = log.WithName("tracing")
	return &cp
}

// NewProvider creates a new tracing provider with the given configuration.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		// Return a no-op provider that uses the global tracer from otel package.
		// Note: This still benefits from the global text map propagator set in main.go.
		return &Provider{
			tracer: otel.Tracer(TracerName),
		}, nil
	}

	// Set defaults
	if cfg.ServiceName == "" {
		cfg.ServiceName = "omnia-runtime"
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 0.1
	}

	// Create OTLP exporter
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	client := otlptracegrpc.NewClient(opts...)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource with service info
	// Note: We avoid resource.Merge with resource.Default() because different OTel
	// package versions (e.g., PromptKit vs Omnia) may use different schema URLs,
	// causing "conflicting Schema URL" errors. Instead, we create a standalone resource.
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironment(cfg.Environment),
	)

	// Create sampler — wrapped in ParentBased so that remote parents with
	// the Sampled flag (e.g. arena worker deterministic traces) are always
	// recorded, while root spans use the configured rate.
	var rootSampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		rootSampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0 {
		rootSampler = sdktrace.NeverSample()
	} else {
		rootSampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}
	sampler := sdktrace.ParentBased(rootSampler)

	// Create TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	p := &Provider{
		tp:     tp,
		tracer: tp.Tracer(TracerName),
	}
	p.log.V(1).Info("tracing provider created",
		"endpoint", cfg.Endpoint,
		"serviceName", cfg.ServiceName,
		"sampleRate", cfg.SampleRate,
		"insecure", cfg.Insecure)
	return p, nil
}

// NewTestProvider creates a Provider from a pre-configured TracerProvider.
// This is intended for tests that supply an in-memory exporter.
func NewTestProvider(tp *sdktrace.TracerProvider) *Provider {
	return &Provider{
		tp:     tp,
		tracer: tp.Tracer(TracerName),
	}
}

// Tracer returns the tracer for creating spans.
func (p *Provider) Tracer() trace.Tracer {
	return p.tracer
}

// TracerProvider returns the underlying TracerProvider for SDK integration.
// Returns the configured provider if tracing is enabled, or the global provider otherwise.
func (p *Provider) TracerProvider() trace.TracerProvider {
	if p.tp != nil {
		return p.tp
	}
	return otel.GetTracerProvider()
}

// Shutdown shuts down the tracer provider, flushing any pending spans.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.tp != nil {
		p.log.V(1).Info("shutting down tracing provider")
		if err := p.tp.Shutdown(ctx); err != nil {
			p.log.Error(err, "tracing provider shutdown failed")
			return err
		}
		p.log.V(1).Info("tracing provider shutdown complete")
	}
	return nil
}

// StartConversationSpan starts a new span for a conversation turn.
func (p *Provider) StartConversationSpan(ctx context.Context, sessionID, promptPackName, promptPackVersion string, turnIndex int) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("session.id", sessionID),
		attribute.Int("omnia.turn.index", turnIndex),
	}
	if promptPackName != "" {
		attrs = append(attrs, attribute.String("omnia.promptpack.name", promptPackName))
	}
	if promptPackVersion != "" {
		attrs = append(attrs, attribute.String("omnia.promptpack.version", promptPackVersion))
	}

	ctx, span := p.tracer.Start(ctx, "omnia.runtime.conversation.turn",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
	p.log.V(1).Info("span started",
		"spanName", "omnia.runtime.conversation.turn",
		"sessionID", sessionID,
		"turnIndex", turnIndex,
		"promptPackName", promptPackName,
		"promptPackVersion", promptPackVersion,
		"traceID", span.SpanContext().TraceID())
	return ctx, span
}

// StartLLMSpan starts a new span for an LLM call following GenAI semantic conventions.
func (p *Provider) StartLLMSpan(ctx context.Context, model string, system string) (context.Context, trace.Span) {
	ctx, span := p.tracer.Start(ctx, "genai.chat",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String(AttrGenAISystem, system),
			attribute.String(AttrGenAIOperationName, "chat"),
			attribute.String(AttrGenAIRequestModel, model),
		),
	)
	p.log.V(1).Info("span started",
		"spanName", "genai.chat",
		"model", model,
		"system", system,
		"traceID", span.SpanContext().TraceID())
	return ctx, span
}

// ToolSpanMeta holds optional registry/handler metadata for tool spans.
type ToolSpanMeta struct {
	RegistryName      string
	RegistryNamespace string
	HandlerName       string
	HandlerType       string
}

// StartToolSpan starts a new span for a tool execution.
func (p *Provider) StartToolSpan(ctx context.Context, toolName string, meta ToolSpanMeta) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("tool.name", toolName),
	}
	if meta.RegistryName != "" {
		attrs = append(attrs,
			attribute.String("tool.registry.name", meta.RegistryName),
			attribute.String("tool.registry.namespace", meta.RegistryNamespace),
			attribute.String("tool.handler.name", meta.HandlerName),
			attribute.String("tool.handler.type", meta.HandlerType),
		)
	}

	ctx, span := p.tracer.Start(ctx, "omnia.tool.call",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
	p.log.V(1).Info("span started",
		"spanName", "omnia.tool.call",
		"toolName", toolName,
		"traceID", span.SpanContext().TraceID())
	return ctx, span
}

// RecordError records an error on the span with exception event and stack trace.
// This adds standard OTel exception.type, exception.message, and exception.stacktrace
// attributes that many UIs surface better than plain status.
func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err, trace.WithStackTrace(true))
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetSuccess marks the span as successful.
func SetSuccess(span trace.Span) {
	span.SetStatus(codes.Ok, "success")
}

// AddLLMMetrics adds GenAI usage metrics to a span.
func AddLLMMetrics(span trace.Span, inputTokens, outputTokens int, costUSD float64) {
	span.SetAttributes(
		attribute.Int(AttrGenAIUsageInputTokens, inputTokens),
		attribute.Int(AttrGenAIUsageOutputTokens, outputTokens),
		attribute.Float64(AttrGenAIUsageCost, costUSD),
	)
}

// AddResponseModel sets the response model on a span (may differ from request model).
func AddResponseModel(span trace.Span, model string) {
	span.SetAttributes(
		attribute.String(AttrGenAIResponseModel, model),
	)
}

// AddFinishReason sets the finish reason on a span.
func AddFinishReason(span trace.Span, reason string) {
	span.SetAttributes(
		attribute.StringSlice(AttrGenAIResponseFinish, []string{reason}),
	)
}

// AddToolResult adds tool execution result info to a span.
func AddToolResult(span trace.Span, isError bool, durationMs int) {
	if isError {
		span.SetStatus(codes.Error, "tool execution failed")
	}
	span.SetAttributes(
		attribute.Int("tool.duration_ms", durationMs),
	)
}

// AddConversationMetrics adds conversation metrics to a span.
func AddConversationMetrics(span trace.Span, messageLength int, responseLength int) {
	span.SetAttributes(
		attribute.Int(AttrGenAIPromptLength, messageLength),
		attribute.Int(AttrGenAIResponseLength, responseLength),
		attribute.Int("omnia.input.bytes", messageLength),
		attribute.Int("omnia.output.bytes", responseLength),
	)
}

// AddToolMetrics adds tool execution metrics to a span.
func AddToolMetrics(span trace.Span, requestBytes, responseBytes int) {
	span.SetAttributes(
		attribute.Int("tool.request.bytes", requestBytes),
		attribute.Int("tool.response.bytes", responseBytes),
	)
}
