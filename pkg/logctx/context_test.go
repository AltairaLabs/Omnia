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

package logctx

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
)

func TestWithSessionID(t *testing.T) {
	ctx := context.Background()
	ctx = WithSessionID(ctx, "sess-123")

	if got := SessionID(ctx); got != "sess-123" {
		t.Errorf("SessionID() = %q, want %q", got, "sess-123")
	}
}

func TestWithRequestID(t *testing.T) {
	ctx := context.Background()
	ctx = WithRequestID(ctx, "req-456")

	if got := RequestID(ctx); got != "req-456" {
		t.Errorf("RequestID() = %q, want %q", got, "req-456")
	}
}

func TestWithAgent(t *testing.T) {
	ctx := context.Background()
	ctx = WithAgent(ctx, "my-agent")

	if got := Agent(ctx); got != "my-agent" {
		t.Errorf("Agent() = %q, want %q", got, "my-agent")
	}
}

func TestWithNamespace(t *testing.T) {
	ctx := context.Background()
	ctx = WithNamespace(ctx, "my-ns")

	if got := Namespace(ctx); got != "my-ns" {
		t.Errorf("Namespace() = %q, want %q", got, "my-ns")
	}
}

func TestWithCorrelationID(t *testing.T) {
	ctx := context.Background()
	ctx = WithCorrelationID(ctx, "corr-789")

	fields := ExtractLoggingFields(ctx)
	if fields.CorrelationID != "corr-789" {
		t.Errorf("CorrelationID = %q, want %q", fields.CorrelationID, "corr-789")
	}
}

func TestWithProvider(t *testing.T) {
	ctx := context.Background()
	ctx = WithProvider(ctx, "anthropic")

	fields := ExtractLoggingFields(ctx)
	if fields.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", fields.Provider, "anthropic")
	}
}

func TestWithModel(t *testing.T) {
	ctx := context.Background()
	ctx = WithModel(ctx, "claude-3")

	fields := ExtractLoggingFields(ctx)
	if fields.Model != "claude-3" {
		t.Errorf("Model = %q, want %q", fields.Model, "claude-3")
	}
}

func TestWithHandler(t *testing.T) {
	ctx := context.Background()
	ctx = WithHandler(ctx, "demo")

	fields := ExtractLoggingFields(ctx)
	if fields.Handler != "demo" {
		t.Errorf("Handler = %q, want %q", fields.Handler, "demo")
	}
}

func TestWithTool(t *testing.T) {
	ctx := context.Background()
	ctx = WithTool(ctx, "search")

	fields := ExtractLoggingFields(ctx)
	if fields.Tool != "search" {
		t.Errorf("Tool = %q, want %q", fields.Tool, "search")
	}
}

func TestWithStage(t *testing.T) {
	ctx := context.Background()
	ctx = WithStage(ctx, "execution")

	fields := ExtractLoggingFields(ctx)
	if fields.Stage != "execution" {
		t.Errorf("Stage = %q, want %q", fields.Stage, "execution")
	}
}

func TestWithLoggingContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithLoggingContext(ctx, &LoggingFields{
		SessionID:     "sess-1",
		RequestID:     "req-1",
		CorrelationID: "corr-1",
		Agent:         "agent-1",
		Namespace:     "ns-1",
		Provider:      "provider-1",
		Model:         "model-1",
		Handler:       "handler-1",
		Tool:          "tool-1",
		Stage:         "stage-1",
	})

	fields := ExtractLoggingFields(ctx)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"SessionID", fields.SessionID, "sess-1"},
		{"RequestID", fields.RequestID, "req-1"},
		{"CorrelationID", fields.CorrelationID, "corr-1"},
		{"Agent", fields.Agent, "agent-1"},
		{"Namespace", fields.Namespace, "ns-1"},
		{"Provider", fields.Provider, "provider-1"},
		{"Model", fields.Model, "model-1"},
		{"Handler", fields.Handler, "handler-1"},
		{"Tool", fields.Tool, "tool-1"},
		{"Stage", fields.Stage, "stage-1"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestWithLoggingContextNil(t *testing.T) {
	ctx := context.Background()
	result := WithLoggingContext(ctx, nil)

	if result != ctx {
		t.Error("WithLoggingContext(ctx, nil) should return the same context")
	}
}

func TestWithLoggingContextPartial(t *testing.T) {
	ctx := context.Background()
	ctx = WithLoggingContext(ctx, &LoggingFields{
		SessionID: "sess-only",
		// Other fields empty
	})

	fields := ExtractLoggingFields(ctx)

	if fields.SessionID != "sess-only" {
		t.Errorf("SessionID = %q, want %q", fields.SessionID, "sess-only")
	}
	if fields.Agent != "" {
		t.Errorf("Agent = %q, want empty", fields.Agent)
	}
}

func TestExtractLoggingFieldsEmpty(t *testing.T) {
	ctx := context.Background()
	fields := ExtractLoggingFields(ctx)

	if fields.SessionID != "" {
		t.Errorf("SessionID = %q, want empty", fields.SessionID)
	}
	if fields.Agent != "" {
		t.Errorf("Agent = %q, want empty", fields.Agent)
	}
}

func TestLogrValues(t *testing.T) {
	ctx := context.Background()
	ctx = WithSessionID(ctx, "sess-123")
	ctx = WithAgent(ctx, "my-agent")

	values := LogrValues(ctx)

	// Should have 4 elements (2 key-value pairs)
	if len(values) != 4 {
		t.Errorf("len(LogrValues) = %d, want 4", len(values))
	}

	// Check that values contain expected keys and values
	found := make(map[string]string)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			t.Errorf("key at index %d is not a string", i)
			continue
		}
		val, ok := values[i+1].(string)
		if !ok {
			t.Errorf("value at index %d is not a string", i+1)
			continue
		}
		found[key] = val
	}

	if found["session_id"] != "sess-123" {
		t.Errorf("session_id = %q, want %q", found["session_id"], "sess-123")
	}
	if found["agent"] != "my-agent" {
		t.Errorf("agent = %q, want %q", found["agent"], "my-agent")
	}
}

func TestLogrValuesEmpty(t *testing.T) {
	ctx := context.Background()
	values := LogrValues(ctx)

	if len(values) != 0 {
		t.Errorf("len(LogrValues) = %d, want 0", len(values))
	}
}

func TestLogrValuesSkipsEmpty(t *testing.T) {
	ctx := context.Background()
	// Set an empty string - should be skipped
	ctx = context.WithValue(ctx, ContextKeySessionID, "")
	ctx = WithAgent(ctx, "my-agent")

	values := LogrValues(ctx)

	// Should only have 2 elements (1 key-value pair for agent)
	if len(values) != 2 {
		t.Errorf("len(LogrValues) = %d, want 2", len(values))
	}
}

func TestLoggerWithContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithSessionID(ctx, "sess-123")
	ctx = WithAgent(ctx, "my-agent")

	log := logr.Discard()
	enriched := LoggerWithContext(log, ctx)

	// Just verify it doesn't panic and returns a logger
	// logr.Discard() has nil sink but is still valid
	enriched.Info("test message") // Should not panic
}

func TestLoggerWithContextEmpty(t *testing.T) {
	ctx := context.Background()
	log := logr.Discard()

	enriched := LoggerWithContext(log, ctx)

	// Should return same logger when no context values
	enriched.Info("test message") // Should not panic
}

func TestGettersReturnEmptyOnWrongType(t *testing.T) {
	ctx := context.Background()
	// Set non-string values
	ctx = context.WithValue(ctx, ContextKeySessionID, 123)
	ctx = context.WithValue(ctx, ContextKeyAgent, true)
	ctx = context.WithValue(ctx, ContextKeyNamespace, []string{"test"})
	ctx = context.WithValue(ctx, ContextKeyRequestID, struct{}{})

	if got := SessionID(ctx); got != "" {
		t.Errorf("SessionID() = %q, want empty for int value", got)
	}
	if got := Agent(ctx); got != "" {
		t.Errorf("Agent() = %q, want empty for bool value", got)
	}
	if got := Namespace(ctx); got != "" {
		t.Errorf("Namespace() = %q, want empty for slice value", got)
	}
	if got := RequestID(ctx); got != "" {
		t.Errorf("RequestID() = %q, want empty for struct value", got)
	}
}

func TestChainedContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithSessionID(ctx, "sess-1")
	ctx = WithAgent(ctx, "agent-1")
	ctx = WithNamespace(ctx, "ns-1")

	// Update session ID - should override
	ctx = WithSessionID(ctx, "sess-2")

	if got := SessionID(ctx); got != "sess-2" {
		t.Errorf("SessionID() = %q, want %q", got, "sess-2")
	}
	// Other values should remain
	if got := Agent(ctx); got != "agent-1" {
		t.Errorf("Agent() = %q, want %q", got, "agent-1")
	}
	if got := Namespace(ctx); got != "ns-1" {
		t.Errorf("Namespace() = %q, want %q", got, "ns-1")
	}
}
