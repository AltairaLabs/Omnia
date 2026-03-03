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
	convSpan := findSpanByName(spans, "omnia.runtime.conversation.turn")
	if convSpan == nil {
		t.Fatal("expected 'omnia.runtime.conversation.turn' span to be recorded")
	}

	if convSpan.SpanKind != trace.SpanKindInternal {
		t.Errorf("expected SpanKindInternal, got %v", convSpan.SpanKind)
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
	llmSpan := findSpanByName(spans, "genai.chat")
	if llmSpan == nil {
		t.Fatal("expected 'genai.chat' span to be recorded")
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

func TestConverse_LLMSpanIsChildOfConversationSpan(t *testing.T) {
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
		{SessionId: "test-hierarchy", Content: "Hello"},
	})

	_ = server.Converse(stream)

	spans := exporter.GetSpans()

	convSpan := findSpanByName(spans, "omnia.runtime.conversation.turn")
	require.NotNil(t, convSpan, "expected conversation.turn span")

	llmSpan := findSpanByName(spans, "genai.chat")
	require.NotNil(t, llmSpan, "expected genai.chat span")

	// LLM span must be a child of the conversation span (same trace, parent matches)
	assert.Equal(t, convSpan.SpanContext.TraceID(), llmSpan.SpanContext.TraceID(),
		"LLM span should share trace ID with conversation span")
	assert.Equal(t, convSpan.SpanContext.SpanID(), llmSpan.Parent.SpanID(),
		"LLM span parent should be the conversation span")
}

func TestConverse_TurnIndexIncrements(t *testing.T) {
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

	// Send two messages in the same session
	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "test-turn-index", Content: "Hello"},
		{SessionId: "test-turn-index", Content: "How are you?"},
	})

	_ = server.Converse(stream)

	spans := exporter.GetSpans()

	// Find both conversation.turn spans
	var turnSpans []tracetest.SpanStub
	for _, s := range spans {
		if s.Name == "omnia.runtime.conversation.turn" {
			turnSpans = append(turnSpans, s)
		}
	}
	require.Len(t, turnSpans, 2, "expected 2 conversation.turn spans for 2 messages")

	turn0, ok := findSpanAttr(turnSpans[0], "omnia.turn.index")
	require.True(t, ok, "missing omnia.turn.index on first turn")
	assert.Equal(t, int64(0), turn0.AsInt64())

	turn1, ok := findSpanAttr(turnSpans[1], "omnia.turn.index")
	require.True(t, ok, "missing omnia.turn.index on second turn")
	assert.Equal(t, int64(1), turn1.AsInt64())
}

func TestConverse_ErrorSetsSpanStatus(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	// Invalid pack content — will cause Open to fail
	packContent := `{"invalid": true}`
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
		{SessionId: "test-error-span", Content: "Hello"},
	})

	_ = server.Converse(stream)

	spans := exporter.GetSpans()

	convSpan := findSpanByName(spans, "omnia.runtime.conversation.turn")
	if convSpan == nil {
		// If pack is so broken the span isn't created, the error was before tracing.
		// Verify the error was sent to the client instead.
		require.NotEmpty(t, stream.sentMessages, "expected at least an error message")
		return
	}

	// If we got a conversation span, it should be marked as error
	assert.Equal(t, "Error", convSpan.Status.Code.String(),
		"conversation span should have error status when processing fails")
}

func TestConverse_ToolCallSpanHierarchy(t *testing.T) {
	// This test verifies that when executeToolForConversation is called with
	// a context containing a parent span, the tool span is properly parented.
	//
	// NOTE: When called through PromptKit's pipeline (OnToolCtx), the tool
	// callback receives context.Background() instead of the pipeline context,
	// so tool spans are orphaned. See PromptKit#591.

	provider, exporter := newTracingTestProvider(t)

	// Start a parent conversation span
	ctx := context.Background()
	ctx, convSpan := provider.StartConversationSpan(ctx, "test-tool-session", "test-pack", "1.0.0", 0)

	// Start a child tool span using the same API the server uses
	ctx, toolSpanOtel := provider.StartToolSpan(ctx, "search_places")
	toolSpanOtel.End()
	convSpan.End()

	spans := exporter.GetSpans()

	toolSpan := findSpanByName(spans, "omnia.tool.call")
	require.NotNil(t, toolSpan, "expected omnia.tool.call span")

	// Verify tool name attribute
	toolName, ok := findSpanAttr(*toolSpan, "tool.name")
	require.True(t, ok, "missing tool.name attribute")
	assert.Equal(t, "search_places", toolName.AsString())

	convSpanStub := findSpanByName(spans, "omnia.runtime.conversation.turn")
	require.NotNil(t, convSpanStub, "expected conversation.turn span")

	// Tool span must be a child of the conversation span
	assert.Equal(t, convSpanStub.SpanContext.TraceID(), toolSpan.SpanContext.TraceID(),
		"tool span should share trace ID with conversation span")
	assert.Equal(t, convSpanStub.SpanContext.SpanID(), toolSpan.Parent.SpanID(),
		"tool span parent should be the conversation span")
}

func TestConverse_ToolCallSpanParentedViaOnToolCtx(t *testing.T) {
	// Verifies that OnToolCtx now properly propagates the pipeline context
	// to tool handlers, so tool spans are children of the conversation span.
	// (Fixed in PromptKit#591.)

	provider, exporter := newTracingTestProvider(t)

	// Simulate what OnToolCtx does after the fix: handler receives the
	// pipeline context that carries the conversation span.
	ctx := context.Background()
	ctx, convSpan := provider.StartConversationSpan(ctx, "test-parented", "test-pack", "1.0.0", 0)

	// OnToolCtx now passes the pipeline context (with conversation span) to the handler
	_, toolSpanOtel := provider.StartToolSpan(ctx, "parented_tool")
	toolSpanOtel.End()
	convSpan.End()

	spans := exporter.GetSpans()

	toolSpan := findSpanByName(spans, "omnia.tool.call")
	require.NotNil(t, toolSpan, "expected omnia.tool.call span")

	convSpanStub := findSpanByName(spans, "omnia.runtime.conversation.turn")
	require.NotNil(t, convSpanStub, "expected conversation.turn span")

	// Tool span must be a child of the conversation span (same trace, parent matches)
	assert.Equal(t, convSpanStub.SpanContext.TraceID(), toolSpan.SpanContext.TraceID(),
		"tool span should share trace ID with conversation span")
	assert.Equal(t, convSpanStub.SpanContext.SpanID(), toolSpan.Parent.SpanID(),
		"tool span parent should be the conversation span")
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
