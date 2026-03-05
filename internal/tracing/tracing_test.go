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

package tracing

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// newTestProvider creates a Provider backed by an in-memory span exporter so
// that tests can inspect the attributes that are actually recorded on spans.
func newTestProvider(t *testing.T) (*Provider, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	return &Provider{
		tp:     tp,
		tracer: tp.Tracer(TracerName),
	}, exporter
}

// findAttr looks up an attribute by key in a span's attribute set.
func findAttr(span tracetest.SpanStub, key string) (attribute.Value, bool) {
	for _, a := range span.Attributes {
		if string(a.Key) == key {
			return a.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestNewProvider_Disabled(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, err := NewProvider(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider == nil {
		t.Fatal("expected non-nil provider")
	}

	// Tracer should still work (no-op)
	tracer := provider.Tracer()
	if tracer == nil {
		t.Fatal("expected non-nil tracer")
	}
}

func TestNewProvider_Defaults(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, err := NewProvider(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test that shutdown works for disabled provider
	err = provider.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on shutdown: %v", err)
	}
}

func TestProvider_StartConversationSpan(t *testing.T) {
	provider, exporter := newTestProvider(t)

	_, span := provider.StartConversationSpan(context.Background(), "test-session", "test-pack", "v1.0", 0)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if s.Name != "omnia.runtime.conversation.turn" {
		t.Errorf("expected span name 'omnia.runtime.conversation.turn', got %q", s.Name)
	}
	if s.SpanKind != trace.SpanKindInternal {
		t.Errorf("expected SpanKindServer, got %v", s.SpanKind)
	}

	val, ok := findAttr(s, "session.id")
	if !ok {
		t.Fatal("missing attribute 'session.id'")
	}
	if val.AsString() != "test-session" {
		t.Errorf("expected session.id='test-session', got %q", val.AsString())
	}
}

func TestProvider_StartLLMSpan(t *testing.T) {
	provider, exporter := newTestProvider(t)

	_, span := provider.StartLLMSpan(context.Background(), "claude-3-opus", "anthropic")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if s.Name != "genai.chat" {
		t.Errorf("expected span name 'genai.chat', got %q", s.Name)
	}
	if s.SpanKind != trace.SpanKindClient {
		t.Errorf("expected SpanKindClient, got %v", s.SpanKind)
	}

	val, ok := findAttr(s, "gen_ai.request.model")
	if !ok {
		t.Fatal("missing attribute 'gen_ai.request.model'")
	}
	if val.AsString() != "claude-3-opus" {
		t.Errorf("expected gen_ai.request.model='claude-3-opus', got %q", val.AsString())
	}
}

func TestProvider_StartToolSpan(t *testing.T) {
	provider, exporter := newTestProvider(t)

	_, span := provider.StartToolSpan(context.Background(), "get_weather", ToolSpanMeta{})
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if s.Name != "omnia.tool.call" {
		t.Errorf("expected span name 'omnia.tool.call', got %q", s.Name)
	}
	if s.SpanKind != trace.SpanKindClient {
		t.Errorf("expected SpanKindClient, got %v", s.SpanKind)
	}

	val, ok := findAttr(s, "tool.name")
	if !ok {
		t.Fatal("missing attribute 'tool.name'")
	}
	if val.AsString() != "get_weather" {
		t.Errorf("expected tool.name='get_weather', got %q", val.AsString())
	}
}

func TestProvider_StartToolSpan_WithMeta(t *testing.T) {
	provider, exporter := newTestProvider(t)

	meta := ToolSpanMeta{
		RegistryName:      "my-registry",
		RegistryNamespace: "my-ns",
		HandlerName:       "http-handler",
		HandlerType:       "http",
	}
	_, span := provider.StartToolSpan(context.Background(), "get_weather", meta)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]

	// Verify tool.name
	val, ok := findAttr(s, "tool.name")
	if !ok {
		t.Fatal("missing attribute 'tool.name'")
	}
	if val.AsString() != "get_weather" {
		t.Errorf("expected tool.name='get_weather', got %q", val.AsString())
	}

	// Verify registry attributes
	val, ok = findAttr(s, "tool.registry.name")
	if !ok {
		t.Fatal("missing attribute 'tool.registry.name'")
	}
	if val.AsString() != "my-registry" {
		t.Errorf("expected tool.registry.name='my-registry', got %q", val.AsString())
	}

	val, ok = findAttr(s, "tool.registry.namespace")
	if !ok {
		t.Fatal("missing attribute 'tool.registry.namespace'")
	}
	if val.AsString() != "my-ns" {
		t.Errorf("expected tool.registry.namespace='my-ns', got %q", val.AsString())
	}

	val, ok = findAttr(s, "tool.handler.name")
	if !ok {
		t.Fatal("missing attribute 'tool.handler.name'")
	}
	if val.AsString() != "http-handler" {
		t.Errorf("expected tool.handler.name='http-handler', got %q", val.AsString())
	}

	val, ok = findAttr(s, "tool.handler.type")
	if !ok {
		t.Fatal("missing attribute 'tool.handler.type'")
	}
	if val.AsString() != "http" {
		t.Errorf("expected tool.handler.type='http', got %q", val.AsString())
	}
}

func TestProvider_StartToolSpan_EmptyMeta(t *testing.T) {
	provider, exporter := newTestProvider(t)

	_, span := provider.StartToolSpan(context.Background(), "get_weather", ToolSpanMeta{})
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]

	// Registry attributes should NOT be present when meta is empty
	if _, ok := findAttr(s, "tool.registry.name"); ok {
		t.Error("unexpected attribute 'tool.registry.name' when meta is empty")
	}
}

func TestRecordError(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, _ := NewProvider(context.Background(), cfg)
	_, span := provider.StartConversationSpan(context.Background(), "test", "", "", 0)
	defer span.End()

	// Should not panic with nil error
	RecordError(span, nil)

	// Should not panic with actual error
	RecordError(span, errors.New("test error"))
}

func TestSetSuccess(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, _ := NewProvider(context.Background(), cfg)
	_, span := provider.StartConversationSpan(context.Background(), "test", "", "", 0)
	defer span.End()

	// Should not panic
	SetSuccess(span)
}

func TestAddLLMMetrics(t *testing.T) {
	provider, exporter := newTestProvider(t)

	_, span := provider.StartLLMSpan(context.Background(), "test-model", "openai")
	AddLLMMetrics(span, 100, 200, 0.05)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]

	inputVal, ok := findAttr(s, "gen_ai.usage.input_tokens")
	if !ok {
		t.Fatal("missing attribute 'gen_ai.usage.input_tokens'")
	}
	if inputVal.AsInt64() != 100 {
		t.Errorf("expected gen_ai.usage.input_tokens=100, got %d", inputVal.AsInt64())
	}

	outputVal, ok := findAttr(s, "gen_ai.usage.output_tokens")
	if !ok {
		t.Fatal("missing attribute 'gen_ai.usage.output_tokens'")
	}
	if outputVal.AsInt64() != 200 {
		t.Errorf("expected gen_ai.usage.output_tokens=200, got %d", outputVal.AsInt64())
	}

	costVal, ok := findAttr(s, "gen_ai.usage.cost")
	if !ok {
		t.Fatal("missing attribute 'gen_ai.usage.cost'")
	}
	if costVal.AsFloat64() != 0.05 {
		t.Errorf("expected gen_ai.usage.cost=0.05, got %f", costVal.AsFloat64())
	}

	// Verify removed attributes are not present
	if _, ok := findAttr(s, "llm.total_tokens"); ok {
		t.Error("unexpected attribute 'llm.total_tokens' should have been removed")
	}
}

func TestAddToolResult(t *testing.T) {
	provider, exporter := newTestProvider(t)

	t.Run("success", func(t *testing.T) {
		exporter.Reset()
		_, span := provider.StartToolSpan(context.Background(), "test-tool", ToolSpanMeta{})
		AddToolResult(span, false, 150)
		span.End()

		spans := exporter.GetSpans()
		if len(spans) != 1 {
			t.Fatalf("expected 1 span, got %d", len(spans))
		}

		s := spans[0]
		durVal, ok := findAttr(s, "tool.duration_ms")
		if !ok {
			t.Fatal("missing attribute 'tool.duration_ms'")
		}
		if durVal.AsInt64() != 150 {
			t.Errorf("expected tool.duration_ms=150, got %d", durVal.AsInt64())
		}

		if s.Status.Code == codes.Error {
			t.Error("expected non-error status for success case")
		}
	})

	t.Run("error", func(t *testing.T) {
		exporter.Reset()
		_, span := provider.StartToolSpan(context.Background(), "test-tool", ToolSpanMeta{})
		AddToolResult(span, true, 50)
		span.End()

		spans := exporter.GetSpans()
		if len(spans) != 1 {
			t.Fatalf("expected 1 span, got %d", len(spans))
		}

		s := spans[0]
		durVal, ok := findAttr(s, "tool.duration_ms")
		if !ok {
			t.Fatal("missing attribute 'tool.duration_ms'")
		}
		if durVal.AsInt64() != 50 {
			t.Errorf("expected tool.duration_ms=50, got %d", durVal.AsInt64())
		}

		if s.Status.Code != codes.Error {
			t.Error("expected error status for error case")
		}
		if s.Status.Description != "tool execution failed" {
			t.Errorf("expected status description 'tool execution failed', got %q", s.Status.Description)
		}
	})
}

func TestAddConversationMetrics(t *testing.T) {
	provider, exporter := newTestProvider(t)

	_, span := provider.StartConversationSpan(context.Background(), "test", "", "", 0)
	AddConversationMetrics(span, 150, 500)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]

	promptVal, ok := findAttr(s, "gen_ai.prompt.length")
	if !ok {
		t.Fatal("missing attribute 'gen_ai.prompt.length'")
	}
	if promptVal.AsInt64() != 150 {
		t.Errorf("expected gen_ai.prompt.length=150, got %d", promptVal.AsInt64())
	}

	respVal, ok := findAttr(s, "gen_ai.response.length")
	if !ok {
		t.Fatal("missing attribute 'gen_ai.response.length'")
	}
	if respVal.AsInt64() != 500 {
		t.Errorf("expected gen_ai.response.length=500, got %d", respVal.AsInt64())
	}
}

func TestProvider_TracerProvider_Disabled(t *testing.T) {
	provider, err := NewProvider(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tp := provider.TracerProvider()
	if tp == nil {
		t.Fatal("expected non-nil TracerProvider")
	}
	// Should return the global provider when tracing is disabled (tp is nil)
}

func TestProvider_TracerProvider_NilTP(t *testing.T) {
	// Manually construct a provider with nil tp to test the fallback
	p := &Provider{tracer: nil}
	tp := p.TracerProvider()
	if tp == nil {
		t.Fatal("expected non-nil TracerProvider from global fallback")
	}
}

func TestProvider_TracerProvider_WithTP(t *testing.T) {
	// Construct a provider with a real (no-op) TracerProvider to cover the tp != nil branch
	sdkTP := sdktrace.NewTracerProvider()
	defer func() { _ = sdkTP.Shutdown(context.Background()) }()

	p := &Provider{tp: sdkTP, tracer: sdkTP.Tracer(TracerName)}
	tp := p.TracerProvider()
	if tp == nil {
		t.Fatal("expected non-nil TracerProvider")
	}
	if tp != sdkTP {
		t.Fatal("expected TracerProvider to return the configured provider")
	}
}

func TestProvider_Shutdown_WithTP(t *testing.T) {
	// Test Shutdown with a real TracerProvider to cover the tp != nil branch
	sdkTP := sdktrace.NewTracerProvider()
	p := &Provider{tp: sdkTP, tracer: sdkTP.Tracer(TracerName)}

	err := p.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewProvider_Enabled(t *testing.T) {
	// Create with a non-routable endpoint — provider creation succeeds even
	// though the exporter can't connect (batching is async).
	cfg := Config{
		Enabled:        true,
		Endpoint:       "127.0.0.1:0",
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "test",
		SampleRate:     1.0,
		Insecure:       true,
	}

	provider, err := NewProvider(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = provider.Shutdown(context.Background()) }()

	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if provider.tp == nil {
		t.Fatal("expected non-nil TracerProvider when enabled")
	}
	if provider.Tracer() == nil {
		t.Fatal("expected non-nil tracer")
	}
}

func TestNewProvider_Enabled_Defaults(t *testing.T) {
	// Test that empty ServiceName gets defaulted
	cfg := Config{
		Enabled:    true,
		Endpoint:   "127.0.0.1:0",
		SampleRate: 0, // Should default to 1.0
		Insecure:   true,
	}

	provider, err := NewProvider(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = provider.Shutdown(context.Background()) }()

	if provider.tp == nil {
		t.Fatal("expected non-nil TracerProvider")
	}
}

func TestNewProvider_Enabled_NeverSample(t *testing.T) {
	cfg := Config{
		Enabled:    true,
		Endpoint:   "127.0.0.1:0",
		SampleRate: 0.0,
		Insecure:   true,
	}

	provider, err := NewProvider(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = provider.Shutdown(context.Background()) }()

	if provider.tp == nil {
		t.Fatal("expected non-nil TracerProvider")
	}
}

func TestNewProvider_Enabled_RatioSample(t *testing.T) {
	cfg := Config{
		Enabled:    true,
		Endpoint:   "127.0.0.1:0",
		SampleRate: 0.5,
		Insecure:   true,
	}

	provider, err := NewProvider(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = provider.Shutdown(context.Background()) }()

	if provider.tp == nil {
		t.Fatal("expected non-nil TracerProvider")
	}
}

func TestConfig_SampleRates(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate float64
	}{
		{"always sample", 1.0},
		{"never sample", 0.0},
		{"ratio sample", 0.5},
		{"high ratio", 0.99},
		{"low ratio", 0.01},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Enabled:    false, // Use no-op to avoid needing OTLP endpoint
				SampleRate: tt.sampleRate,
			}

			provider, err := NewProvider(context.Background(), cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if provider == nil {
				t.Fatal("expected non-nil provider")
			}
		})
	}
}
