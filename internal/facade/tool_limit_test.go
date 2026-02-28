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

package facade

import (
	"sync"
	"testing"
)

func TestToolCallLimiter_Allow(t *testing.T) {
	limiter := NewToolCallLimiter(3)

	// First three calls should succeed
	for i := 0; i < 3; i++ {
		if err := limiter.Allow(); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
	}

	// Fourth call should fail
	if err := limiter.Allow(); err == nil {
		t.Fatal("expected error on fourth call")
	}

	if limiter.Count() != 4 {
		t.Errorf("Count() = %d, want 4", limiter.Count())
	}
}

func TestToolCallLimiter_ZeroLimit(t *testing.T) {
	limiter := NewToolCallLimiter(0)

	// Zero limit means no enforcement
	for i := 0; i < 100; i++ {
		if err := limiter.Allow(); err != nil {
			t.Fatalf("unexpected error with zero limit: %v", err)
		}
	}
}

func TestToolCallLimiter_NegativeLimit(t *testing.T) {
	limiter := NewToolCallLimiter(-1)

	if err := limiter.Allow(); err != nil {
		t.Fatalf("unexpected error with negative limit: %v", err)
	}
}

func TestToolCallLimiter_NilLimiter(t *testing.T) {
	var limiter *ToolCallLimiter

	if err := limiter.Allow(); err != nil {
		t.Fatalf("unexpected error with nil limiter: %v", err)
	}
	if limiter.Count() != 0 {
		t.Errorf("Count() = %d, want 0", limiter.Count())
	}
	if limiter.Limit() != 0 {
		t.Errorf("Limit() = %d, want 0", limiter.Limit())
	}
}

func TestToolCallLimiter_Concurrent(t *testing.T) {
	limiter := NewToolCallLimiter(100)

	var wg sync.WaitGroup
	errors := make(chan error, 200)

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := limiter.Allow(); err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	errCount := 0
	for range errors {
		errCount++
	}

	// Exactly 100 should have failed
	if errCount != 100 {
		t.Errorf("error count = %d, want 100", errCount)
	}
}

func TestToolCallLimiter_Limit(t *testing.T) {
	limiter := NewToolCallLimiter(42)
	if limiter.Limit() != 42 {
		t.Errorf("Limit() = %d, want 42", limiter.Limit())
	}
}

func TestRateLimitedWriter_PassesThrough(t *testing.T) {
	inner := &mockResponseWriter{}
	limiter := NewToolCallLimiter(10)
	writer := newRateLimitedWriter(inner, limiter)

	// Regular methods pass through
	if err := writer.WriteChunk("test"); err != nil {
		t.Fatal(err)
	}
	if len(inner.chunks) != 1 {
		t.Errorf("chunks = %d, want 1", len(inner.chunks))
	}

	if err := writer.WriteDone("done"); err != nil {
		t.Fatal(err)
	}
	if inner.doneContent != "done" {
		t.Errorf("doneContent = %q, want %q", inner.doneContent, "done")
	}

	if err := writer.WriteError("CODE", "msg"); err != nil {
		t.Fatal(err)
	}
	if len(inner.errors) != 1 {
		t.Errorf("errors = %d, want 1", len(inner.errors))
	}
}

func TestRateLimitedWriter_ToolCallAllowed(t *testing.T) {
	inner := &mockResponseWriter{}
	limiter := NewToolCallLimiter(2)
	writer := newRateLimitedWriter(inner, limiter)

	tc := &ToolCallInfo{ID: "tc-1", Name: "search"}

	// First two should succeed
	if err := writer.WriteToolCall(tc); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteToolCall(tc); err != nil {
		t.Fatal(err)
	}
	if len(inner.toolCalls) != 2 {
		t.Errorf("toolCalls = %d, want 2", len(inner.toolCalls))
	}
}

func TestRateLimitedWriter_ToolCallExceeded(t *testing.T) {
	inner := &mockResponseWriter{}
	limiter := NewToolCallLimiter(1)
	writer := newRateLimitedWriter(inner, limiter)

	tc := &ToolCallInfo{ID: "tc-1", Name: "search"}

	// First call succeeds
	if err := writer.WriteToolCall(tc); err != nil {
		t.Fatal(err)
	}

	// Second call should send error instead
	if err := writer.WriteToolCall(tc); err != nil {
		t.Fatal(err)
	}

	// Only one tool call forwarded
	if len(inner.toolCalls) != 1 {
		t.Errorf("toolCalls = %d, want 1", len(inner.toolCalls))
	}

	// Error sent instead
	if len(inner.errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(inner.errors))
	}
	if inner.errors[0].code != ErrorCodeToolCallLimitExceeded {
		t.Errorf("error code = %q, want %q", inner.errors[0].code, ErrorCodeToolCallLimitExceeded)
	}
}

func TestRateLimitedWriter_NilLimiter(t *testing.T) {
	inner := &mockResponseWriter{}
	writer := newRateLimitedWriter(inner, nil)

	// With nil limiter, should return inner directly
	if writer != inner {
		t.Error("expected writer to be inner when limiter is nil")
	}
}

func TestRateLimitedWriter_ZeroLimit(t *testing.T) {
	inner := &mockResponseWriter{}
	limiter := NewToolCallLimiter(0)
	writer := newRateLimitedWriter(inner, limiter)

	// With zero limit, should return inner directly (no wrapping)
	if writer != inner {
		t.Error("expected writer to be inner when limit is zero")
	}
}

func TestRateLimitedWriter_ToolResult(t *testing.T) {
	inner := &mockResponseWriter{}
	limiter := NewToolCallLimiter(10)
	writer := newRateLimitedWriter(inner, limiter)

	result := &ToolResultInfo{ID: "tc-1", Result: "ok"}
	if err := writer.WriteToolResult(result); err != nil {
		t.Fatal(err)
	}
	if len(inner.toolResults) != 1 {
		t.Errorf("toolResults = %d, want 1", len(inner.toolResults))
	}
}

func TestRateLimitedWriter_SupportsBinary(t *testing.T) {
	inner := &mockResponseWriter{supportsBinary: true}
	limiter := NewToolCallLimiter(10)
	writer := newRateLimitedWriter(inner, limiter)

	if !writer.SupportsBinary() {
		t.Error("expected SupportsBinary() to be true")
	}
}
