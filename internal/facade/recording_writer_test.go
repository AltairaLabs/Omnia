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
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session"
)

// mockResponseWriter records calls for verification.
type mockResponseWriter struct {
	mu              sync.Mutex
	chunks          []string
	doneContent     string
	doneParts       []ContentPart
	toolCalls       []*ToolCallInfo
	toolResults     []*ToolResultInfo
	errors          []struct{ code, message string }
	uploadReadys    []*UploadReadyInfo
	uploadCompletes []*UploadCompleteInfo
	mediaChunks     []*MediaChunkInfo
	supportsBinary  bool
	writeErr        error // if set, all write methods return this error
}

func (m *mockResponseWriter) WriteChunk(content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.chunks = append(m.chunks, content)
	return nil
}

func (m *mockResponseWriter) WriteChunkWithParts(parts []ContentPart) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	return nil
}

func (m *mockResponseWriter) WriteDone(content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.doneContent = content
	return nil
}

func (m *mockResponseWriter) WriteDoneWithParts(parts []ContentPart) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.doneParts = parts
	return nil
}

func (m *mockResponseWriter) WriteToolCall(toolCall *ToolCallInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.toolCalls = append(m.toolCalls, toolCall)
	return nil
}

func (m *mockResponseWriter) WriteToolResult(result *ToolResultInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.toolResults = append(m.toolResults, result)
	return nil
}

func (m *mockResponseWriter) WriteError(code, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.errors = append(m.errors, struct{ code, message string }{code, message})
	return nil
}

func (m *mockResponseWriter) WriteUploadReady(uploadReady *UploadReadyInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.uploadReadys = append(m.uploadReadys, uploadReady)
	return nil
}

func (m *mockResponseWriter) WriteUploadComplete(uploadComplete *UploadCompleteInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.uploadCompletes = append(m.uploadCompletes, uploadComplete)
	return nil
}

func (m *mockResponseWriter) WriteMediaChunk(mediaChunk *MediaChunkInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.mediaChunks = append(m.mediaChunks, mediaChunk)
	return nil
}

func (m *mockResponseWriter) WriteBinaryMediaChunk(_ [MediaIDSize]byte, _ uint32, _ bool, _ string, _ []byte) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	return nil
}

func (m *mockResponseWriter) SupportsBinary() bool {
	return m.supportsBinary
}

// waitForAsyncWrites gives goroutines time to complete.
func waitForAsyncWrites() {
	time.Sleep(50 * time.Millisecond)
}

func TestRecordingWriter_WriteDone(t *testing.T) {
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})
	if err != nil {
		t.Fatal(err)
	}

	inner := &mockResponseWriter{}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	if err := rw.WriteDone("Hello from assistant"); err != nil {
		t.Fatal(err)
	}

	// Verify inner was called
	if inner.doneContent != "Hello from assistant" {
		t.Errorf("inner.doneContent = %q, want %q", inner.doneContent, "Hello from assistant")
	}

	// Wait for async write
	waitForAsyncWrites()

	// Verify message was recorded
	messages, err := store.GetMessages(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}
	if messages[0].Role != session.RoleAssistant {
		t.Errorf("role = %v, want assistant", messages[0].Role)
	}
	if messages[0].Content != "Hello from assistant" {
		t.Errorf("content = %q, want %q", messages[0].Content, "Hello from assistant")
	}
	if messages[0].Metadata["latency_ms"] == "" {
		t.Error("expected latency_ms in metadata")
	}
}

func TestRecordingWriter_WriteDoneWithUsage(t *testing.T) {
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, _ := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})

	inner := &mockResponseWriter{}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	rw.ReportUsage(&UsageInfo{
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      0.005,
	})

	if err := rw.WriteDone("response"); err != nil {
		t.Fatal(err)
	}

	waitForAsyncWrites()

	messages, _ := store.GetMessages(ctx, sess.ID)
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}

	msg := messages[0]
	if msg.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", msg.InputTokens)
	}
	if msg.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", msg.OutputTokens)
	}
	if msg.Metadata["cost_usd"] != "0.005" {
		t.Errorf("cost_usd = %q, want %q", msg.Metadata["cost_usd"], "0.005")
	}

	// Verify session stats were updated
	updated, _ := store.GetSession(ctx, sess.ID)
	if updated.TotalInputTokens != 100 {
		t.Errorf("TotalInputTokens = %d, want 100", updated.TotalInputTokens)
	}
	if updated.TotalOutputTokens != 50 {
		t.Errorf("TotalOutputTokens = %d, want 50", updated.TotalOutputTokens)
	}
	if updated.EstimatedCostUSD != 0.005 {
		t.Errorf("EstimatedCostUSD = %f, want 0.005", updated.EstimatedCostUSD)
	}
}

func TestRecordingWriter_WriteDoneWithParts(t *testing.T) {
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, _ := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})

	inner := &mockResponseWriter{}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	parts := []ContentPart{
		{Type: ContentPartTypeText, Text: "Hello from parts"},
		{Type: ContentPartTypeImage, Media: &MediaContent{URL: "http://example.com/img.png", MimeType: "image/png"}},
	}

	if err := rw.WriteDoneWithParts(parts); err != nil {
		t.Fatal(err)
	}

	// Verify inner was called
	if len(inner.doneParts) != 2 {
		t.Errorf("inner.doneParts length = %d, want 2", len(inner.doneParts))
	}

	waitForAsyncWrites()

	messages, _ := store.GetMessages(ctx, sess.ID)
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}
	if messages[0].Content != "Hello from parts" {
		t.Errorf("content = %q, want %q", messages[0].Content, "Hello from parts")
	}
}

func TestRecordingWriter_WriteToolCall(t *testing.T) {
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, _ := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})

	inner := &mockResponseWriter{}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	tc := &ToolCallInfo{
		ID:   "tc-1",
		Name: "search",
		Arguments: map[string]interface{}{
			"query": "test query",
		},
	}
	if err := rw.WriteToolCall(tc); err != nil {
		t.Fatal(err)
	}

	// Verify inner was called
	if len(inner.toolCalls) != 1 {
		t.Fatalf("inner.toolCalls = %d, want 1", len(inner.toolCalls))
	}

	waitForAsyncWrites()

	messages, _ := store.GetMessages(ctx, sess.ID)
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}
	if messages[0].Role != session.RoleAssistant {
		t.Errorf("role = %v, want assistant", messages[0].Role)
	}
	if messages[0].ToolCallID != "tc-1" {
		t.Errorf("ToolCallID = %q, want %q", messages[0].ToolCallID, "tc-1")
	}
	if messages[0].Metadata["type"] != "tool_call" {
		t.Errorf("type = %q, want %q", messages[0].Metadata["type"], "tool_call")
	}
	if !strings.Contains(messages[0].Content, "search") {
		t.Errorf("content should contain tool name, got %q", messages[0].Content)
	}

	// Verify tool call count was incremented
	updated, _ := store.GetSession(ctx, sess.ID)
	if updated.ToolCallCount != 1 {
		t.Errorf("ToolCallCount = %d, want 1", updated.ToolCallCount)
	}
}

func TestRecordingWriter_WriteToolResult(t *testing.T) {
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, _ := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})

	inner := &mockResponseWriter{}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	result := &ToolResultInfo{
		ID:     "tc-1",
		Result: "search results here",
	}
	if err := rw.WriteToolResult(result); err != nil {
		t.Fatal(err)
	}

	waitForAsyncWrites()

	messages, _ := store.GetMessages(ctx, sess.ID)
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}
	if messages[0].Role != session.RoleSystem {
		t.Errorf("role = %v, want system", messages[0].Role)
	}
	if messages[0].ToolCallID != "tc-1" {
		t.Errorf("ToolCallID = %q, want %q", messages[0].ToolCallID, "tc-1")
	}
	if messages[0].Metadata["type"] != "tool_result" {
		t.Errorf("type = %q, want %q", messages[0].Metadata["type"], "tool_result")
	}
}

func TestRecordingWriter_WriteToolResult_Error(t *testing.T) {
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, _ := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})

	inner := &mockResponseWriter{}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	result := &ToolResultInfo{
		ID:    "tc-1",
		Error: "tool failed: timeout",
	}
	if err := rw.WriteToolResult(result); err != nil {
		t.Fatal(err)
	}

	waitForAsyncWrites()

	messages, _ := store.GetMessages(ctx, sess.ID)
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}
	if messages[0].Content != "tool failed: timeout" {
		t.Errorf("content = %q, want %q", messages[0].Content, "tool failed: timeout")
	}
	if messages[0].Metadata["is_error"] != "true" {
		t.Errorf("is_error = %q, want %q", messages[0].Metadata["is_error"], "true")
	}
}

func TestRecordingWriter_WriteError(t *testing.T) {
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, _ := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})

	inner := &mockResponseWriter{}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	if err := rw.WriteError("INTERNAL_ERROR", "something went wrong"); err != nil {
		t.Fatal(err)
	}

	// Verify inner was called
	if len(inner.errors) != 1 {
		t.Fatalf("inner.errors = %d, want 1", len(inner.errors))
	}

	waitForAsyncWrites()

	messages, _ := store.GetMessages(ctx, sess.ID)
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}
	if messages[0].Role != session.RoleSystem {
		t.Errorf("role = %v, want system", messages[0].Role)
	}
	if messages[0].Metadata["type"] != "error" {
		t.Errorf("type = %q, want %q", messages[0].Metadata["type"], "error")
	}
	if !strings.Contains(messages[0].Content, "INTERNAL_ERROR") {
		t.Errorf("content should contain error code, got %q", messages[0].Content)
	}

	// Verify session status was set to error
	updated, _ := store.GetSession(ctx, sess.ID)
	if updated.Status != session.SessionStatusError {
		t.Errorf("Status = %q, want %q", updated.Status, session.SessionStatusError)
	}
}

func TestRecordingWriter_StoreFailure_GracefulDegradation(t *testing.T) {
	// Use a store that will fail on AppendMessage by closing it
	store := session.NewMemoryStore()

	ctx := context.Background()
	sess, _ := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})

	inner := &mockResponseWriter{}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	// Close the store to make it fail
	_ = store.Close()

	// WriteDone should still succeed (inner writer works fine)
	if err := rw.WriteDone("response"); err != nil {
		t.Fatalf("WriteDone should not fail even when store is broken: %v", err)
	}

	// Verify inner was called
	if inner.doneContent != "response" {
		t.Errorf("inner.doneContent = %q, want %q", inner.doneContent, "response")
	}

	waitForAsyncWrites()
	// No panic, no error propagated -- graceful degradation
}

func TestRecordingWriter_PassThrough(t *testing.T) {
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, _ := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})

	inner := &mockResponseWriter{supportsBinary: true}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	// WriteChunk -- pass-through, no recording
	if err := rw.WriteChunk("chunk data"); err != nil {
		t.Fatal(err)
	}
	if len(inner.chunks) != 1 {
		t.Errorf("chunks = %d, want 1", len(inner.chunks))
	}

	// WriteUploadReady -- pass-through
	if err := rw.WriteUploadReady(&UploadReadyInfo{UploadID: "up-1"}); err != nil {
		t.Fatal(err)
	}
	if len(inner.uploadReadys) != 1 {
		t.Errorf("uploadReadys = %d, want 1", len(inner.uploadReadys))
	}

	// WriteUploadComplete -- pass-through
	if err := rw.WriteUploadComplete(&UploadCompleteInfo{UploadID: "up-1"}); err != nil {
		t.Fatal(err)
	}
	if len(inner.uploadCompletes) != 1 {
		t.Errorf("uploadCompletes = %d, want 1", len(inner.uploadCompletes))
	}

	// WriteMediaChunk -- pass-through
	if err := rw.WriteMediaChunk(&MediaChunkInfo{MediaID: "m-1"}); err != nil {
		t.Fatal(err)
	}
	if len(inner.mediaChunks) != 1 {
		t.Errorf("mediaChunks = %d, want 1", len(inner.mediaChunks))
	}

	// SupportsBinary -- pass-through
	if !rw.SupportsBinary() {
		t.Error("SupportsBinary should delegate to inner")
	}

	waitForAsyncWrites()

	// Verify NO messages were recorded for pass-through methods
	messages, _ := store.GetMessages(ctx, sess.ID)
	if len(messages) != 0 {
		t.Errorf("pass-through methods should not record messages, got %d", len(messages))
	}
}

func TestRecordingWriter_Latency(t *testing.T) {
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, _ := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})

	inner := &mockResponseWriter{}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	// Wait a bit to have measurable latency
	time.Sleep(10 * time.Millisecond)

	if err := rw.WriteDone("done"); err != nil {
		t.Fatal(err)
	}

	waitForAsyncWrites()

	messages, _ := store.GetMessages(ctx, sess.ID)
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}

	latencyStr := messages[0].Metadata["latency_ms"]
	if latencyStr == "" {
		t.Fatal("expected latency_ms in metadata")
	}
	// Should be >= 10ms since we slept 10ms
	if latencyStr == "0" {
		t.Error("latency should be > 0")
	}
}

func TestRecordingWriter_ReportUsage(t *testing.T) {
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, _ := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})

	inner := &mockResponseWriter{}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	// Verify UsageReporter interface
	var reporter UsageReporter = rw
	reporter.ReportUsage(&UsageInfo{
		InputTokens:  200,
		OutputTokens: 100,
		CostUSD:      0.01,
	})

	// Usage should be stored and applied to next Done
	if err := rw.WriteDone("response with usage"); err != nil {
		t.Fatal(err)
	}

	waitForAsyncWrites()

	messages, _ := store.GetMessages(ctx, sess.ID)
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}
	if messages[0].InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200", messages[0].InputTokens)
	}
	if messages[0].OutputTokens != 100 {
		t.Errorf("OutputTokens = %d, want 100", messages[0].OutputTokens)
	}
}

func TestRecordingWriter_InnerWriteError_Propagated(t *testing.T) {
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, _ := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test", Namespace: "default",
	})

	writeErr := errors.New("connection closed")
	inner := &mockResponseWriter{writeErr: writeErr}
	rw := newRecordingWriter(inner, store, sess.ID, logr.Discard())

	// Inner write errors should be propagated
	if err := rw.WriteDone("test"); err == nil {
		t.Error("expected error from inner writer")
	}
	if err := rw.WriteToolCall(&ToolCallInfo{ID: "tc-1", Name: "test"}); err == nil {
		t.Error("expected error from inner writer")
	}
	if err := rw.WriteToolResult(&ToolResultInfo{ID: "tc-1", Result: "ok"}); err == nil {
		t.Error("expected error from inner writer")
	}
	if err := rw.WriteError("ERR", "msg"); err == nil {
		t.Error("expected error from inner writer")
	}
}
