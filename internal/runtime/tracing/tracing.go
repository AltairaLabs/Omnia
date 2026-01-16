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

// Package tracing provides OpenTelemetry tracing for the runtime.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TracerName is the name of the tracer used for runtime spans.
	TracerName = "omnia-runtime"

	// SpanKindConversation indicates a conversation span.
	SpanKindConversation = "conversation"
	// SpanKindLLM indicates an LLM call span.
	SpanKindLLM = "llm"
	// SpanKindTool indicates a tool execution span.
	SpanKindTool = "tool"

	// attrSpanKind is the attribute key for omnia span kind.
	attrSpanKind = "omnia.span_kind"
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
}

// NewProvider creates a new tracing provider with the given configuration.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		// Return a no-op provider
		return &Provider{
			tracer: otel.Tracer(TracerName),
		}, nil
	}

	// Set defaults
	if cfg.ServiceName == "" {
		cfg.ServiceName = "omnia-runtime"
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 1.0
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

	// Create sampler
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// Create TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set as global provider
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{
		tp:     tp,
		tracer: tp.Tracer(TracerName),
	}, nil
}

// Tracer returns the tracer for creating spans.
func (p *Provider) Tracer() trace.Tracer {
	return p.tracer
}

// Shutdown shuts down the tracer provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.tp != nil {
		return p.tp.Shutdown(ctx)
	}
	return nil
}

// StartConversationSpan starts a new span for a conversation turn.
func (p *Provider) StartConversationSpan(ctx context.Context, sessionID string) (context.Context, trace.Span) {
	ctx, span := p.tracer.Start(ctx, "conversation.turn",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String(attrSpanKind, SpanKindConversation),
			attribute.String("omnia.session_id", sessionID),
		),
	)
	return ctx, span
}

// StartLLMSpan starts a new span for an LLM call.
func (p *Provider) StartLLMSpan(ctx context.Context, model string) (context.Context, trace.Span) {
	ctx, span := p.tracer.Start(ctx, "llm.call",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String(attrSpanKind, SpanKindLLM),
			attribute.String("llm.model", model),
		),
	)
	return ctx, span
}

// StartToolSpan starts a new span for a tool execution.
func (p *Provider) StartToolSpan(ctx context.Context, toolName string) (context.Context, trace.Span) {
	ctx, span := p.tracer.Start(ctx, fmt.Sprintf("tool.%s", toolName),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String(attrSpanKind, SpanKindTool),
			attribute.String("tool.name", toolName),
		),
	)
	return ctx, span
}

// RecordError records an error on the span.
func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetSuccess marks the span as successful.
func SetSuccess(span trace.Span) {
	span.SetStatus(codes.Ok, "success")
}

// AddLLMMetrics adds LLM-specific metrics to a span.
func AddLLMMetrics(span trace.Span, inputTokens, outputTokens int, costUSD float64) {
	span.SetAttributes(
		attribute.Int("llm.input_tokens", inputTokens),
		attribute.Int("llm.output_tokens", outputTokens),
		attribute.Int("llm.total_tokens", inputTokens+outputTokens),
		attribute.Float64("llm.cost_usd", costUSD),
	)
}

// AddToolResult adds tool execution result info to a span.
func AddToolResult(span trace.Span, isError bool, resultSize int) {
	span.SetAttributes(
		attribute.Bool("tool.is_error", isError),
		attribute.Int("tool.result_size", resultSize),
	)
}

// AddConversationMetrics adds conversation metrics to a span.
func AddConversationMetrics(span trace.Span, messageLength int, responseLength int) {
	span.SetAttributes(
		attribute.Int("conversation.message_length", messageLength),
		attribute.Int("conversation.response_length", responseLength),
	)
}
