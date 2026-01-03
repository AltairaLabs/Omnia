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
