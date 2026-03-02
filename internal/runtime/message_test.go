/*
Copyright 2026.

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

package runtime

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/tracing"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// newTracingTestProvider creates a Provider backed by an in-memory span exporter.
func newTracingTestProvider(t *testing.T) (*tracing.Provider, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	return tracing.NewTestProvider(tp), exporter
}

// findSpanByName returns the first span with the given name, or nil.
func findSpanByName(spans []tracetest.SpanStub, name string) *tracetest.SpanStub {
	for i := range spans {
		if spans[i].Name == name {
			return &spans[i]
		}
	}
	return nil
}

// findSpanAttr looks up an attribute by key in a span's attribute set.
func findSpanAttr(span tracetest.SpanStub, key string) (attribute.Value, bool) {
	for _, a := range span.Attributes {
		if string(a.Key) == key {
			return a.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestConverse_EmitsConversationSpan(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	require.NoError(t, writeTestFile(t, packPath, packContent))

	provider, exporter := newTracingTestProvider(t)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
		WithTracingProvider(provider),
		WithProviderInfo("mock", "mock-model"),
	)
	defer func() { _ = server.Close() }()

	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "test-session-tracing", Content: "Hello"},
	})

	_ = server.Converse(stream)

	spans := exporter.GetSpans()

	// Verify conversation.turn span
	convSpan := findSpanByName(spans, "conversation.turn")
	if convSpan == nil {
		t.Fatal("expected 'conversation.turn' span to be recorded")
	}

	if convSpan.SpanKind != trace.SpanKindServer {
		t.Errorf("expected SpanKindServer, got %v", convSpan.SpanKind)
	}

	val, ok := findSpanAttr(*convSpan, "session.id")
	if !ok {
		t.Fatal("missing attribute 'session.id' on conversation.turn span")
	}
	assert.Equal(t, "test-session-tracing", val.AsString())
}

func TestConverse_EmitsLLMSpanWithGenAIAttributes(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	require.NoError(t, writeTestFile(t, packPath, packContent))

	provider, exporter := newTracingTestProvider(t)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
		WithTracingProvider(provider),
		WithProviderInfo("ollama", "llama3"),
	)
	defer func() { _ = server.Close() }()

	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "test-session-llm", Content: "Hello"},
	})

	_ = server.Converse(stream)

	spans := exporter.GetSpans()

	// Find LLM span
	llmSpan := findSpanByName(spans, "chat llama3")
	if llmSpan == nil {
		t.Fatal("expected 'chat llama3' span to be recorded")
	}

	if llmSpan.SpanKind != trace.SpanKindClient {
		t.Errorf("expected SpanKindClient, got %v", llmSpan.SpanKind)
	}

	// Verify GenAI semantic convention attributes
	systemVal, ok := findSpanAttr(*llmSpan, "gen_ai.system")
	if !ok {
		t.Fatal("missing attribute 'gen_ai.system'")
	}
	assert.Equal(t, "ollama", systemVal.AsString())

	opVal, ok := findSpanAttr(*llmSpan, "gen_ai.operation.name")
	if !ok {
		t.Fatal("missing attribute 'gen_ai.operation.name'")
	}
	assert.Equal(t, "chat", opVal.AsString())

	modelVal, ok := findSpanAttr(*llmSpan, "gen_ai.request.model")
	if !ok {
		t.Fatal("missing attribute 'gen_ai.request.model'")
	}
	assert.Equal(t, "llama3", modelVal.AsString())
}

func TestConverse_NoTracingProvider_NoSpans(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	require.NoError(t, writeTestFile(t, packPath, packContent))

	// Create server WITHOUT tracing provider
	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
	)
	defer func() { _ = server.Close() }()

	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "test-no-tracing", Content: "Hello"},
	})

	// Should succeed without panic
	_ = server.Converse(stream)

	// Verify responses were sent
	assert.NotEmpty(t, stream.sentMessages)
}
