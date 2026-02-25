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

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

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
	cfg := Config{
		Enabled: false,
	}

	provider, _ := NewProvider(context.Background(), cfg)

	ctx, span := provider.StartConversationSpan(context.Background(), "test-session")
	defer span.End()

	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if span == nil {
		t.Fatal("expected non-nil span")
	}
}

func TestProvider_StartLLMSpan(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, _ := NewProvider(context.Background(), cfg)

	ctx, span := provider.StartLLMSpan(context.Background(), "claude-3-opus")
	defer span.End()

	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if span == nil {
		t.Fatal("expected non-nil span")
	}
}

func TestProvider_StartToolSpan(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, _ := NewProvider(context.Background(), cfg)

	ctx, span := provider.StartToolSpan(context.Background(), "get_weather")
	defer span.End()

	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if span == nil {
		t.Fatal("expected non-nil span")
	}
}

func TestRecordError(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, _ := NewProvider(context.Background(), cfg)
	_, span := provider.StartConversationSpan(context.Background(), "test")
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
	_, span := provider.StartConversationSpan(context.Background(), "test")
	defer span.End()

	// Should not panic
	SetSuccess(span)
}

func TestAddLLMMetrics(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, _ := NewProvider(context.Background(), cfg)
	_, span := provider.StartLLMSpan(context.Background(), "test-model")
	defer span.End()

	// Should not panic
	AddLLMMetrics(span, 100, 200, 0.05)
}

func TestAddToolResult(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, _ := NewProvider(context.Background(), cfg)
	_, span := provider.StartToolSpan(context.Background(), "test-tool")
	defer span.End()

	// Test success case
	AddToolResult(span, false, 1024)

	// Test error case
	AddToolResult(span, true, 50)
}

func TestAddConversationMetrics(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, _ := NewProvider(context.Background(), cfg)
	_, span := provider.StartConversationSpan(context.Background(), "test")
	defer span.End()

	// Should not panic
	AddConversationMetrics(span, 150, 500)
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
