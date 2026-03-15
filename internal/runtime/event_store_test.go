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
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/PromptKit/runtime/types"

	"github.com/altairalabs/omnia/internal/runtime/tools"
	"github.com/altairalabs/omnia/internal/session"
)

// mockSessionStore implements session.Store for testing.
type mockSessionStore struct {
	mu            sync.Mutex
	messages      []session.Message
	stats         []session.SessionStatsUpdate
	toolCalls     []session.ToolCall
	providerCalls []session.ProviderCall
	runtimeEvents []session.RuntimeEvent
	evalResults   []session.EvalResult
	appendFn      func(ctx context.Context, sessionID string, msg session.Message) error
}

func (m *mockSessionStore) CreateSession(_ context.Context, _ session.CreateSessionOptions) (*session.Session, error) {
	return nil, nil
}

func (m *mockSessionStore) GetSession(_ context.Context, _ string) (*session.Session, error) {
	return nil, nil
}

func (m *mockSessionStore) DeleteSession(_ context.Context, _ string) error {
	return nil
}

func (m *mockSessionStore) AppendMessage(_ context.Context, _ string, msg session.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	if m.appendFn != nil {
		return m.appendFn(context.Background(), "", msg)
	}
	return nil
}

func (m *mockSessionStore) GetMessages(_ context.Context, _ string) ([]session.Message, error) {
	return nil, nil
}

func (m *mockSessionStore) SetState(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *mockSessionStore) GetState(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func (m *mockSessionStore) RefreshTTL(_ context.Context, _ string, _ time.Duration) error {
	return nil
}

func (m *mockSessionStore) UpdateSessionStats(_ context.Context, _ string, update session.SessionStatsUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats = append(m.stats, update)
	return nil
}

func (m *mockSessionStore) Close() error {
	return nil
}

func (m *mockSessionStore) RecordToolCall(_ context.Context, _ string, tc session.ToolCall) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolCalls = append(m.toolCalls, tc)
	return nil
}

func (m *mockSessionStore) RecordProviderCall(_ context.Context, _ string, pc session.ProviderCall) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providerCalls = append(m.providerCalls, pc)
	return nil
}

func (m *mockSessionStore) GetToolCalls(_ context.Context, _ string) ([]session.ToolCall, error) {
	return nil, nil
}

func (m *mockSessionStore) GetProviderCalls(_ context.Context, _ string) ([]session.ProviderCall, error) {
	return nil, nil
}

func (m *mockSessionStore) RecordEvalResult(_ context.Context, _ string, result session.EvalResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.evalResults = append(m.evalResults, result)
	return nil
}

func (m *mockSessionStore) RecordRuntimeEvent(_ context.Context, _ string, evt session.RuntimeEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtimeEvents = append(m.runtimeEvents, evt)
	return nil
}

func (m *mockSessionStore) GetRuntimeEvents(_ context.Context, _ string) ([]session.RuntimeEvent, error) {
	return nil, nil
}

func (m *mockSessionStore) getRuntimeEvents() []session.RuntimeEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]session.RuntimeEvent, len(m.runtimeEvents))
	copy(result, m.runtimeEvents)
	return result
}

func (m *mockSessionStore) getToolCalls() []session.ToolCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]session.ToolCall, len(m.toolCalls))
	copy(result, m.toolCalls)
	return result
}

func (m *mockSessionStore) getProviderCalls() []session.ProviderCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]session.ProviderCall, len(m.providerCalls))
	copy(result, m.providerCalls)
	return result
}

func (m *mockSessionStore) waitForToolCalls(t *testing.T, count int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		m.mu.Lock()
		n := len(m.toolCalls)
		m.mu.Unlock()
		if n >= count {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d tool calls (got %d)", count, n)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (m *mockSessionStore) waitForProviderCalls(t *testing.T, count int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		m.mu.Lock()
		n := len(m.providerCalls)
		m.mu.Unlock()
		if n >= count {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d provider calls (got %d)", count, n)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (m *mockSessionStore) waitForEvalResults(t *testing.T, count int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		m.mu.Lock()
		n := len(m.evalResults)
		m.mu.Unlock()
		if n >= count {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d eval results (got %d)", count, n)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (m *mockSessionStore) getEvalResults() []session.EvalResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]session.EvalResult, len(m.evalResults))
	copy(result, m.evalResults)
	return result
}

func (m *mockSessionStore) waitForRuntimeEvents(t *testing.T, count int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		m.mu.Lock()
		n := len(m.runtimeEvents)
		m.mu.Unlock()
		if n >= count {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d runtime events (got %d)", count, n)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (m *mockSessionStore) getMessages() []session.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]session.Message, len(m.messages))
	copy(result, m.messages)
	return result
}

func (m *mockSessionStore) getStats() []session.SessionStatsUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]session.SessionStatsUpdate, len(m.stats))
	copy(result, m.stats)
	return result
}

// waitForMessages waits until the expected number of messages is appended.
func (m *mockSessionStore) waitForMessages(t *testing.T, count int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		m.mu.Lock()
		n := len(m.messages)
		m.mu.Unlock()
		if n >= count {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d messages, got %d", count, n)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// waitForStats waits until the expected number of stats updates is recorded.
// UpdateSessionStats is called after AppendMessage in the same goroutine,
// so waitForMessages alone is not sufficient to guarantee stats are available.
func (m *mockSessionStore) waitForStats(t *testing.T, count int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		m.mu.Lock()
		n := len(m.stats)
		m.mu.Unlock()
		if n >= count {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d stats updates, got %d", count, n)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// --- Tool call events ---

func TestOmniaEventStore_AppendToolCallStarted(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventToolCallStarted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ToolCallStartedData{
			ToolName: "weather",
			CallID:   "call-123",
			Args:     map[string]interface{}{"city": "Tokyo"},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// No legacy message — only first-class tool call record.
	store.waitForToolCalls(t, 1)
	msgs := store.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for tool call started, got %d", len(msgs))
	}

	tcs := store.getToolCalls()
	if tcs[0].Name != "weather" {
		t.Errorf("expected tool call name weather, got %s", tcs[0].Name)
	}
	if tcs[0].CallID != "call-123" {
		t.Errorf("expected callID call-123, got %s", tcs[0].CallID)
	}
	if tcs[0].Status != session.ToolCallStatusPending {
		t.Errorf("expected tool call status pending, got %s", tcs[0].Status)
	}
	if tcs[0].Execution != session.ToolCallExecutionServer {
		t.Errorf("expected execution server, got %s", tcs[0].Execution)
	}
}

func TestOmniaEventStore_AppendToolCallCompleted(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventToolCallCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ToolCallCompletedData{
			ToolName: "weather",
			CallID:   "call-123",
			Status:   "success",
			Duration: 500 * time.Millisecond,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// No legacy message — only first-class tool call record.
	store.waitForToolCalls(t, 1)
	msgs := store.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for tool call completed, got %d", len(msgs))
	}

	tcs := store.getToolCalls()
	if tcs[0].Name != "weather" {
		t.Errorf("expected tool call name weather, got %s", tcs[0].Name)
	}
	if tcs[0].Status != session.ToolCallStatusSuccess {
		t.Errorf("expected status success, got %s", tcs[0].Status)
	}
	if tcs[0].DurationMs != 500 {
		t.Errorf("expected durationMs=500, got %d", tcs[0].DurationMs)
	}
}

func TestOmniaEventStore_ToolCallCompletedWithResultBody(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventToolCallCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ToolCallCompletedData{
			ToolName: "calculate",
			CallID:   "call-456",
			Status:   "complete",
			Duration: 200 * time.Millisecond,
			Parts:    []types.ContentPart{types.NewTextPart("91")},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForToolCalls(t, 1)
	tcs := store.getToolCalls()
	if tcs[0].Result != "91" {
		t.Errorf("expected result=91, got %v", tcs[0].Result)
	}
	if tcs[0].Name != "calculate" {
		t.Errorf("expected name=calculate, got %s", tcs[0].Name)
	}
}

func TestOmniaEventStore_ToolCallCompletedWithoutParts(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventToolCallCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ToolCallCompletedData{
			ToolName: "search",
			CallID:   "call-789",
			Status:   "complete",
			Duration: 100 * time.Millisecond,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForToolCalls(t, 1)
	tcs := store.getToolCalls()
	if tcs[0].Result != nil {
		t.Errorf("expected no result when Parts is empty, got %v", tcs[0].Result)
	}
}

func TestOmniaEventStore_AppendToolCallFailed(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventToolCallFailed,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ToolCallFailedData{
			ToolName: "weather",
			CallID:   "call-456",
			Error:    errors.New("API timeout"),
			Duration: 30 * time.Second,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// No legacy message — only first-class tool call record.
	store.waitForToolCalls(t, 1)
	msgs := store.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for tool call failed, got %d", len(msgs))
	}

	tcs := store.getToolCalls()
	if tcs[0].Status != session.ToolCallStatusError {
		t.Errorf("expected status error, got %s", tcs[0].Status)
	}
	if tcs[0].ErrorMessage != "API timeout" {
		t.Errorf("expected errorMessage 'API timeout', got %s", tcs[0].ErrorMessage)
	}
	if tcs[0].CallID != "call-456" {
		t.Errorf("expected callID call-456, got %s", tcs[0].CallID)
	}
}

func TestOmniaEventStore_ToolCallFailedNilError(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventToolCallFailed,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ToolCallFailedData{
			ToolName: "weather",
			CallID:   "call-789",
			Error:    nil,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForToolCalls(t, 1)
	tcs := store.getToolCalls()
	if tcs[0].ErrorMessage != "unknown error" {
		t.Errorf("expected 'unknown error' for nil error, got %s", tcs[0].ErrorMessage)
	}
}

// --- Provider call events ---

func TestOmniaEventStore_AppendProviderCallStarted(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventProviderCallStarted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ProviderCallStartedData{
			Provider:     "claude",
			Model:        "claude-3-sonnet",
			MessageCount: 5,
			ToolCount:    2,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// No legacy message — only first-class provider call record.
	store.waitForProviderCalls(t, 1)
	msgs := store.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for provider call started, got %d", len(msgs))
	}

	pcs := store.getProviderCalls()
	if pcs[0].Provider != "claude" {
		t.Errorf("expected provider=claude, got %s", pcs[0].Provider)
	}
	if pcs[0].Model != "claude-3-sonnet" {
		t.Errorf("expected model=claude-3-sonnet, got %s", pcs[0].Model)
	}
	if pcs[0].Status != session.ProviderCallStatusPending {
		t.Errorf("expected status pending, got %s", pcs[0].Status)
	}
}

func TestOmniaEventStore_AppendProviderCallCompleted(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ProviderCallCompletedData{
			Provider:     "claude",
			Model:        "claude-3-sonnet",
			InputTokens:  100,
			OutputTokens: 200,
			CachedTokens: 50,
			Cost:         0.005,
			FinishReason: "end_turn",
			Duration:     2 * time.Second,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// No legacy message — only first-class provider call record.
	store.waitForProviderCalls(t, 1)
	msgs := store.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for provider call completed, got %d", len(msgs))
	}

	pcs := store.getProviderCalls()
	if pcs[0].Status != session.ProviderCallStatusCompleted {
		t.Errorf("expected status completed, got %s", pcs[0].Status)
	}
	if pcs[0].InputTokens != 100 {
		t.Errorf("expected inputTokens=100, got %d", pcs[0].InputTokens)
	}
	if pcs[0].OutputTokens != 200 {
		t.Errorf("expected outputTokens=200, got %d", pcs[0].OutputTokens)
	}
	if pcs[0].CostUSD != 0.005 {
		t.Errorf("expected costUSD=0.005, got %f", pcs[0].CostUSD)
	}
	if pcs[0].FinishReason != "end_turn" {
		t.Errorf("expected finishReason=end_turn, got %s", pcs[0].FinishReason)
	}

	// Stats should still be updated for session counters.
	store.waitForStats(t, 1)
	stats := store.getStats()
	if stats[0].AddInputTokens != 100 {
		t.Errorf("expected AddInputTokens=100, got %d", stats[0].AddInputTokens)
	}
	if stats[0].AddCostUSD != 0.005 {
		t.Errorf("expected AddCostUSD=0.005, got %f", stats[0].AddCostUSD)
	}
}

func TestOmniaEventStore_AppendProviderCallFailed(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventProviderCallFailed,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ProviderCallFailedData{
			Provider: "claude",
			Model:    "claude-3-sonnet",
			Error:    errors.New("rate limited"),
			Duration: 1 * time.Second,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// No legacy message — only first-class provider call record.
	store.waitForProviderCalls(t, 1)
	msgs := store.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for provider call failed, got %d", len(msgs))
	}

	pcs := store.getProviderCalls()
	if pcs[0].Status != session.ProviderCallStatusFailed {
		t.Errorf("expected status failed, got %s", pcs[0].Status)
	}
	if pcs[0].ErrorMessage != "rate limited" {
		t.Errorf("expected errorMessage 'rate limited', got %s", pcs[0].ErrorMessage)
	}
}

// --- Message events ---

func TestOmniaEventStore_AppendMessageCreated_User(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventMessageCreated,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "Hello, what's the weather?",
			Index:   0,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForMessages(t, 1)
	msg := store.getMessages()[0]

	if msg.Role != session.RoleUser {
		t.Errorf("expected role user, got %s", msg.Role)
	}
	if msg.Content != "Hello, what's the weather?" {
		t.Errorf("expected user content, got %s", msg.Content)
	}
	if msg.Metadata["index"] != "0" {
		t.Errorf("expected index=0, got %s", msg.Metadata["index"])
	}
}

func TestOmniaEventStore_AppendMessageCreated_Assistant(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventMessageCreated,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.MessageCreatedData{
			Role:    "assistant",
			Content: "Let me check the weather for you.",
			Index:   1,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForMessages(t, 1)
	msg := store.getMessages()[0]

	if msg.Role != session.RoleAssistant {
		t.Errorf("expected role assistant, got %s", msg.Role)
	}
	if msg.Content != "Let me check the weather for you." {
		t.Errorf("unexpected content: %s", msg.Content)
	}
}

func TestOmniaEventStore_AppendMessageCreated_WithToolCalls(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventMessageCreated,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.MessageCreatedData{
			Role:    "assistant",
			Content: "",
			Index:   1,
			ToolCalls: []events.MessageToolCall{
				{ID: "tc-1", Name: "get_weather", Args: `{"city":"Tokyo"}`},
				{ID: "tc-2", Name: "get_time", Args: `{"timezone":"JST"}`},
			},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForMessages(t, 1)
	msg := store.getMessages()[0]

	// Message should NOT have tool_call metadata — tool calls are first-class records now.
	if msg.Metadata["type"] == "tool_call" {
		t.Error("expected no tool_call type metadata on message")
	}
	if msg.Metadata["tool_calls"] != "" {
		t.Error("expected no tool_calls metadata on message")
	}

	// Stats should still count tool calls.
	store.waitForStats(t, 1)
	stats := store.getStats()
	if stats[0].AddToolCalls != 2 {
		t.Errorf("expected AddToolCalls=2, got %d", stats[0].AddToolCalls)
	}
}

func TestOmniaEventStore_AppendMessageCreated_WithToolResult(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventMessageCreated,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.MessageCreatedData{
			Role:  "tool",
			Index: 2,
			ToolResult: &events.MessageToolResult{
				ID:        "tc-1",
				Name:      "get_weather",
				Parts:     []types.ContentPart{types.NewTextPart(`{"temp":22,"condition":"sunny"}`)},
				LatencyMs: 150,
			},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForMessages(t, 1)
	msg := store.getMessages()[0]

	if msg.Metadata["type"] != "tool_result" {
		t.Errorf("expected type=tool_result, got %s", msg.Metadata["type"])
	}
	if msg.ToolCallID != "tc-1" {
		t.Errorf("expected toolCallID=tc-1, got %s", msg.ToolCallID)
	}
	if msg.Metadata["tool_name"] != "get_weather" {
		t.Errorf("expected tool_name=get_weather, got %s", msg.Metadata["tool_name"])
	}
	if msg.Content != `{"temp":22,"condition":"sunny"}` {
		t.Errorf("expected tool result content, got %s", msg.Content)
	}
	if msg.Metadata["duration_ms"] != "150" {
		t.Errorf("expected duration_ms=150, got %s", msg.Metadata["duration_ms"])
	}
}

func TestOmniaEventStore_AppendMessageCreated_WithToolResultError(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventMessageCreated,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.MessageCreatedData{
			Role:  "tool",
			Index: 2,
			ToolResult: &events.MessageToolResult{
				ID:    "tc-1",
				Name:  "get_weather",
				Error: "city not found",
			},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForMessages(t, 1)
	msg := store.getMessages()[0]

	if msg.Metadata["is_error"] != "true" {
		t.Errorf("expected is_error=true, got %s", msg.Metadata["is_error"])
	}
	if msg.Content != "city not found" {
		t.Errorf("expected error content, got %s", msg.Content)
	}
}

func TestOmniaEventStore_AppendMessageUpdated(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventMessageUpdated,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.MessageUpdatedData{
			Index:        1,
			LatencyMs:    350,
			InputTokens:  100,
			OutputTokens: 200,
			TotalCost:    0.003,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForMessages(t, 1)
	msg := store.getMessages()[0]

	if msg.Metadata["type"] != "message_updated" {
		t.Errorf("expected type=message_updated, got %s", msg.Metadata["type"])
	}
	if msg.InputTokens != 100 {
		t.Errorf("expected inputTokens=100, got %d", msg.InputTokens)
	}
	if msg.OutputTokens != 200 {
		t.Errorf("expected outputTokens=200, got %d", msg.OutputTokens)
	}

	store.waitForStats(t, 1)
	stats := store.getStats()
	if stats[0].AddInputTokens != 100 {
		t.Errorf("expected AddInputTokens=100, got %d", stats[0].AddInputTokens)
	}
	if stats[0].AddCostUSD != 0.003 {
		t.Errorf("expected AddCostUSD=0.003, got %f", stats[0].AddCostUSD)
	}
}

func TestOmniaEventStore_AppendConversationStarted(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventConversationStarted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ConversationStartedData{
			SystemPrompt: "You are a helpful assistant.",
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForMessages(t, 1)
	msg := store.getMessages()[0]

	if msg.Metadata["type"] != "conversation_started" {
		t.Errorf("expected type=conversation_started, got %s", msg.Metadata["type"])
	}
	if msg.Content != "You are a helpful assistant." {
		t.Errorf("expected system prompt content, got %s", msg.Content)
	}
	if msg.Role != session.RoleSystem {
		t.Errorf("expected role=system, got %s", msg.Role)
	}
}

// --- Generic events (pipeline, stage, validation, etc.) ---

func TestOmniaEventStore_AppendPipelineStarted(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventPipelineStarted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data:      &events.PipelineStartedData{MiddlewareCount: 3},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForRuntimeEvents(t, 1)

	// Should NOT produce a message — goes to runtime_events table.
	msgs := store.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for pipeline event, got %d", len(msgs))
	}

	evt := store.getRuntimeEvents()[0]
	if evt.EventType != "pipeline.started" {
		t.Errorf("expected eventType=pipeline.started, got %s", evt.EventType)
	}
	if evt.Data["MiddlewareCount"] != float64(3) {
		t.Errorf("expected MiddlewareCount=3, got %v", evt.Data["MiddlewareCount"])
	}
}

func TestOmniaEventStore_AppendPipelineCompleted(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventPipelineCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.PipelineCompletedData{
			Duration:     5 * time.Second,
			TotalCost:    0.01,
			InputTokens:  500,
			OutputTokens: 300,
			MessageCount: 4,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForRuntimeEvents(t, 1)
	evt := store.getRuntimeEvents()[0]

	if evt.EventType != "pipeline.completed" {
		t.Errorf("expected eventType=pipeline.completed, got %s", evt.EventType)
	}
}

func TestOmniaEventStore_AppendValidationFailed(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventValidationFailed,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ValidationFailedData{
			ValidatorName: "safety",
			ValidatorType: "output",
			Duration:      100 * time.Millisecond,
			Violations:    []string{"harmful content detected"},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForRuntimeEvents(t, 1)
	evt := store.getRuntimeEvents()[0]

	if evt.EventType != "validation.failed" {
		t.Errorf("expected eventType=validation.failed, got %s", evt.EventType)
	}
}

func TestOmniaEventStore_AppendStageCompleted(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventStageCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.StageCompletedData{
			Name:      "generate",
			Index:     0,
			StageType: "generate",
			Duration:  2 * time.Second,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForRuntimeEvents(t, 1)
	if store.getRuntimeEvents()[0].EventType != "stage.completed" {
		t.Errorf("expected eventType=stage.completed")
	}
}

func TestOmniaEventStore_AppendWorkflowTransitioned(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventWorkflowTransitioned,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.WorkflowTransitionedData{
			FromState:  "greeting",
			ToState:    "collecting_info",
			Event:      "user_responded",
			PromptTask: "ask_name",
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForRuntimeEvents(t, 1)
	if store.getRuntimeEvents()[0].EventType != "workflow.transitioned" {
		t.Errorf("expected eventType=workflow.transitioned")
	}
}

// --- Multimodal content metadata ---

func TestOmniaEventStore_AppendMessageCreated_WithImageParts(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	base64Data := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAA"
	detail := "high"
	caption := "A photo of a sunset"
	sizeKB := int64(256)
	width := 1920
	height := 1080

	event := &events.Event{
		Type:      events.EventMessageCreated,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "What's in this image?",
			Index:   0,
			Parts: []types.ContentPart{
				types.NewTextPart("What's in this image?"),
				{
					Type: types.ContentTypeImage,
					Media: &types.MediaContent{
						Data:     &base64Data,
						MIMEType: types.MIMETypeImagePNG,
						Detail:   &detail,
						Caption:  &caption,
						SizeKB:   &sizeKB,
						Width:    &width,
						Height:   &height,
					},
				},
			},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForMessages(t, 1)
	msg := store.getMessages()[0]

	// Should have multimodal metadata
	if msg.Metadata["multimodal"] != "true" {
		t.Errorf("expected multimodal=true, got %s", msg.Metadata["multimodal"])
	}
	if msg.Metadata["part_count"] != "2" {
		t.Errorf("expected part_count=2, got %s", msg.Metadata["part_count"])
	}

	// Parse parts metadata — only media parts should be included
	var parts []partMetadata
	if err := json.Unmarshal([]byte(msg.Metadata["parts"]), &parts); err != nil {
		t.Fatalf("failed to unmarshal parts: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 media part metadata, got %d", len(parts))
	}

	p := parts[0]
	if p.Type != "image" {
		t.Errorf("expected type=image, got %s", p.Type)
	}
	if p.MIMEType != "image/png" {
		t.Errorf("expected mime_type=image/png, got %s", p.MIMEType)
	}
	if p.Detail != "high" {
		t.Errorf("expected detail=high, got %s", p.Detail)
	}
	if p.Caption != "A photo of a sunset" {
		t.Errorf("expected caption, got %s", p.Caption)
	}
	if p.SizeKB == nil || *p.SizeKB != 256 {
		t.Errorf("expected size_kb=256")
	}
	if p.Width == nil || *p.Width != 1920 {
		t.Errorf("expected width=1920")
	}
	if p.Height == nil || *p.Height != 1080 {
		t.Errorf("expected height=1080")
	}
	if !p.HasData {
		t.Error("expected has_data=true since base64 data was present")
	}

	// Content should still be the text content (not the blob)
	if msg.Content != "What's in this image?" {
		t.Errorf("expected text content preserved, got %s", msg.Content)
	}

	// Structured multi-modal fields should be set
	if !msg.HasMedia {
		t.Error("expected HasMedia=true")
	}
	if len(msg.MediaTypes) != 1 || msg.MediaTypes[0] != "image" {
		t.Errorf("expected MediaTypes=[image], got %v", msg.MediaTypes)
	}
}

func TestOmniaEventStore_AppendMessageCreated_WithAudioPart(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	url := "https://example.com/audio.mp3"
	duration := 120
	channels := 2
	bitRate := 128
	sizeKB := int64(1800)

	event := &events.Event{
		Type:      events.EventMessageCreated,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "Transcribe this audio",
			Index:   0,
			Parts: []types.ContentPart{
				types.NewTextPart("Transcribe this audio"),
				{
					Type: types.ContentTypeAudio,
					Media: &types.MediaContent{
						URL:      &url,
						MIMEType: types.MIMETypeAudioMP3,
						Duration: &duration,
						Channels: &channels,
						BitRate:  &bitRate,
						SizeKB:   &sizeKB,
					},
				},
			},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForMessages(t, 1)
	msg := store.getMessages()[0]

	var parts []partMetadata
	if err := json.Unmarshal([]byte(msg.Metadata["parts"]), &parts); err != nil {
		t.Fatalf("failed to unmarshal parts: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}

	p := parts[0]
	if p.Type != "audio" {
		t.Errorf("expected type=audio, got %s", p.Type)
	}
	if p.MIMEType != "audio/mpeg" {
		t.Errorf("expected mime_type=audio/mpeg, got %s", p.MIMEType)
	}
	if p.Duration == nil || *p.Duration != 120 {
		t.Errorf("expected duration=120")
	}
	if p.Channels == nil || *p.Channels != 2 {
		t.Errorf("expected channels=2")
	}
	if p.BitRate == nil || *p.BitRate != 128 {
		t.Errorf("expected bit_rate=128")
	}
	if !p.HasData {
		t.Error("expected has_data=true since URL was present")
	}

	// Structured multi-modal fields
	if !msg.HasMedia {
		t.Error("expected HasMedia=true")
	}
	if len(msg.MediaTypes) != 1 || msg.MediaTypes[0] != "audio" {
		t.Errorf("expected MediaTypes=[audio], got %v", msg.MediaTypes)
	}
}

func TestOmniaEventStore_AppendMessageCreated_TextOnlyParts(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventMessageCreated,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "Hello",
			Index:   0,
			Parts: []types.ContentPart{
				types.NewTextPart("Hello"),
			},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForMessages(t, 1)
	msg := store.getMessages()[0]

	// Text-only parts should NOT produce multimodal metadata
	if msg.Metadata["multimodal"] != "" {
		t.Errorf("expected no multimodal metadata for text-only parts, got %s", msg.Metadata["multimodal"])
	}
	if msg.Metadata["parts"] != "" {
		t.Errorf("expected no parts metadata for text-only parts, got %s", msg.Metadata["parts"])
	}
}

func TestExtractPartsMetadata_StripsBlob(t *testing.T) {
	base64Data := "AAAAAAA="
	filePath := "/tmp/image.png"
	url := "https://example.com/img.png"
	storageRef := "s3://bucket/key"

	parts := []types.ContentPart{
		{
			Type: types.ContentTypeImage,
			Media: &types.MediaContent{
				Data:             &base64Data,
				MIMEType:         types.MIMETypeImagePNG,
				StorageReference: &storageRef,
			},
		},
		{
			Type: types.ContentTypeImage,
			Media: &types.MediaContent{
				FilePath: &filePath,
				MIMEType: types.MIMETypeImagePNG,
			},
		},
		{
			Type: types.ContentTypeImage,
			Media: &types.MediaContent{
				URL:      &url,
				MIMEType: types.MIMETypeImagePNG,
			},
		},
	}

	metas := extractPartsMetadata(parts)
	if len(metas) != 3 {
		t.Fatalf("expected 3 metadata entries, got %d", len(metas))
	}

	// Verify serialized JSON does NOT contain blob data
	data, err := json.Marshal(metas)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	jsonStr := string(data)

	if containsAny(jsonStr, base64Data, filePath, url, storageRef) {
		t.Errorf("serialized metadata should NOT contain blob data: %s", jsonStr)
	}

	// All should have has_data=true
	for i, m := range metas {
		if !m.HasData {
			t.Errorf("part %d: expected has_data=true", i)
		}
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(sub) > 0 && json.Valid([]byte(`"`+sub+`"`)) {
			// Check for the value in JSON
			if contains(s, sub) {
				return true
			}
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- Edge cases ---

func TestOmniaEventStore_AppendEmptySessionID(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventToolCallStarted,
		SessionID: "",
		Timestamp: time.Now(),
		Data:      &events.ToolCallStartedData{ToolName: "weather", CallID: "call-123"},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if len(store.getToolCalls()) != 0 {
		t.Error("expected no tool calls for empty sessionID")
	}
}

// TestOmniaEventStore_AppendEmptySessionID_BackfillsFromFallback verifies the
// PromptKit#705 workaround: when the event has an empty SessionID but the store
// has a fallback sessionID set, the event is backfilled and persisted.
func TestOmniaEventStore_AppendEmptySessionID_BackfillsFromFallback(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())
	es.SetSessionID("fallback-sess")

	event := &events.Event{
		Type:      events.EventEvalCompleted,
		SessionID: "", // empty — simulates PromptKit#705
		Timestamp: time.Now(),
		Data: &events.EvalEventData{
			EvalID:   "conciseness",
			EvalType: "contains",
			Trigger:  "every_turn",
			Passed:   true,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// SessionID should be backfilled on the event itself.
	if event.SessionID != "fallback-sess" {
		t.Errorf("expected event.SessionID='fallback-sess', got %q", event.SessionID)
	}

	store.waitForEvalResults(t, 1)
	er := store.getEvalResults()[0]
	if er.EvalID != "conciseness" {
		t.Errorf("expected EvalID 'conciseness', got %q", er.EvalID)
	}
}

// TestOmniaEventStore_AppendPreservesExistingSessionID verifies that events
// with an existing SessionID are NOT overwritten by the fallback.
func TestOmniaEventStore_AppendPreservesExistingSessionID(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())
	es.SetSessionID("fallback-sess")

	event := &events.Event{
		Type:      events.EventEvalCompleted,
		SessionID: "real-sess", // already set by the pipeline emitter
		Timestamp: time.Now(),
		Data: &events.EvalEventData{
			EvalID:   "e1",
			EvalType: "contains",
			Trigger:  "every_turn",
			Passed:   true,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// Must NOT overwrite the existing SessionID.
	if event.SessionID != "real-sess" {
		t.Errorf("expected event.SessionID='real-sess', got %q", event.SessionID)
	}

	store.waitForEvalResults(t, 1)
}

func TestOmniaEventStore_SetSessionID(t *testing.T) {
	es := NewOmniaEventStore(nil, logr.Discard())
	if es.sessionID != "" {
		t.Error("expected empty sessionID initially")
	}
	es.SetSessionID("abc")
	if es.sessionID != "abc" {
		t.Errorf("expected sessionID='abc', got %q", es.sessionID)
	}
}

func TestOmniaEventStore_StoreErrorsAreLoggedNotPropagated(t *testing.T) {
	store := &mockSessionStore{
		appendFn: func(_ context.Context, _ string, _ session.Message) error {
			return errors.New("store unavailable")
		},
	}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventToolCallStarted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data:      &events.ToolCallStartedData{ToolName: "weather", CallID: "call-123"},
	}

	err := es.Append(context.Background(), event)
	if err != nil {
		t.Fatalf("Append() should not return error, got %v", err)
	}

	time.Sleep(100 * time.Millisecond)
}

func TestOmniaEventStore_QueryReturnsNil(t *testing.T) {
	es := NewOmniaEventStore(&mockSessionStore{}, logr.Discard())
	result, err := es.Query(context.Background(), &events.EventFilter{})
	if err != nil || result != nil {
		t.Errorf("Query() should return nil, nil; got %v, %v", result, err)
	}
}

func TestOmniaEventStore_QueryRawReturnsNil(t *testing.T) {
	es := NewOmniaEventStore(&mockSessionStore{}, logr.Discard())
	result, err := es.QueryRaw(context.Background(), &events.EventFilter{})
	if err != nil || result != nil {
		t.Errorf("QueryRaw() should return nil, nil; got %v, %v", result, err)
	}
}

func TestOmniaEventStore_StreamReturnsClosed(t *testing.T) {
	es := NewOmniaEventStore(&mockSessionStore{}, logr.Discard())
	ch, err := es.Stream(context.Background(), "test-session")
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, ok := <-ch; ok {
		t.Error("Stream() channel should be closed")
	}
}

func TestOmniaEventStore_CloseReturnsNil(t *testing.T) {
	es := NewOmniaEventStore(&mockSessionStore{}, logr.Discard())
	if err := es.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// --- Timestamp preservation ---

func TestOmniaEventStore_PreservesTimestamp(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	ts := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	event := &events.Event{
		Type:      events.EventMessageCreated,
		SessionID: "test-session",
		Timestamp: ts,
		Data: &events.MessageCreatedData{
			Role:    "assistant",
			Content: "test",
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForMessages(t, 1)
	if !store.getMessages()[0].Timestamp.Equal(ts) {
		t.Errorf("expected timestamp %v, got %v", ts, store.getMessages()[0].Timestamp)
	}
}

// --- Value type assertions (emitter passes values, not pointers) ---

// TestOmniaEventStore_ValueTypeToolCall verifies that tool call events passed as
// struct values (as the PromptKit emitter does) are correctly handled by asPtr.
func TestOmniaEventStore_ValueTypeToolCall(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	// Emitter passes value types, not pointers
	event := &events.Event{
		Type:      events.EventToolCallStarted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: events.ToolCallStartedData{
			ToolName: "get_weather",
			CallID:   "call-val-1",
			Args:     map[string]interface{}{"city": "London"},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForToolCalls(t, 1)
	tcs := store.getToolCalls()
	if tcs[0].Name != "get_weather" {
		t.Errorf("expected name=get_weather, got %s", tcs[0].Name)
	}
	if tcs[0].CallID != "call-val-1" {
		t.Errorf("expected callID=call-val-1, got %s", tcs[0].CallID)
	}
}

// TestOmniaEventStore_ValueTypeToolCallCompleted verifies completed events
// passed as values are handled.
func TestOmniaEventStore_ValueTypeToolCallCompleted(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventToolCallCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: events.ToolCallCompletedData{
			ToolName: "get_weather",
			CallID:   "call-val-2",
			Duration: 500 * time.Millisecond,
			Status:   "success",
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForToolCalls(t, 1)
	tcs := store.getToolCalls()
	if tcs[0].Name != "get_weather" {
		t.Errorf("expected name=get_weather, got %s", tcs[0].Name)
	}
	if tcs[0].DurationMs != 500 {
		t.Errorf("expected durationMs=500, got %d", tcs[0].DurationMs)
	}
}

// TestOmniaEventStore_ValueTypeProviderCallStarted verifies provider call
// started events passed as values are handled.
func TestOmniaEventStore_ValueTypeProviderCallStarted(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventProviderCallStarted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: events.ProviderCallStartedData{
			Provider:     "ollama",
			Model:        "llama3",
			MessageCount: 3,
			ToolCount:    2,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForProviderCalls(t, 1)
	pcs := store.getProviderCalls()
	if pcs[0].Provider != "ollama" {
		t.Errorf("expected provider=ollama, got %s", pcs[0].Provider)
	}
}

// TestOmniaEventStore_ValueTypeProviderCallFailed verifies provider call
// failed events passed as values are handled.
func TestOmniaEventStore_ValueTypeProviderCallFailed(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventProviderCallFailed,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: events.ProviderCallFailedData{
			Provider: "ollama",
			Error:    errors.New("timeout"),
			Duration: 30 * time.Second,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForProviderCalls(t, 1)
	pcs := store.getProviderCalls()
	if pcs[0].Status != session.ProviderCallStatusFailed {
		t.Errorf("expected status=failed, got %s", pcs[0].Status)
	}
	if pcs[0].ErrorMessage != "timeout" {
		t.Errorf("expected errorMessage=timeout, got %s", pcs[0].ErrorMessage)
	}
}

// TestOmniaEventStore_ToolCallWithRegistryMeta verifies that tool call records
// are enriched with registry/handler labels when toolMetaFn is set.
func TestOmniaEventStore_ToolCallWithRegistryMeta(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())
	es.SetToolMetaFn(func(toolName string) (tools.ToolMeta, bool) {
		if toolName == "search" {
			return tools.ToolMeta{
				RegistryName:      "my-registry",
				RegistryNamespace: "my-ns",
				HandlerName:       "mcp-handler",
				HandlerType:       "mcp",
				Endpoint:          "http://mcp:8080/sse",
			}, true
		}
		return tools.ToolMeta{}, false
	})

	event := &events.Event{
		Type:      events.EventToolCallStarted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ToolCallStartedData{
			ToolName: "search",
			CallID:   "call-1",
			Args:     map[string]any{"q": "test"},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForToolCalls(t, 1)
	tcs := store.getToolCalls()
	if tcs[0].Labels["handler_name"] != "mcp-handler" {
		t.Errorf("expected label handler_name=mcp-handler, got %s", tcs[0].Labels["handler_name"])
	}
	if tcs[0].Labels["handler_type"] != "mcp" {
		t.Errorf("expected label handler_type=mcp, got %s", tcs[0].Labels["handler_type"])
	}
	if tcs[0].Labels["registry_name"] != "my-registry" {
		t.Errorf("expected label registry_name=my-registry, got %s", tcs[0].Labels["registry_name"])
	}
	if tcs[0].Labels["registry_namespace"] != "my-ns" {
		t.Errorf("expected label registry_namespace=my-ns, got %s", tcs[0].Labels["registry_namespace"])
	}
}

// TestOmniaEventStore_ToolCallCompletedWithRegistryMeta verifies completed events get labels.
func TestOmniaEventStore_ToolCallCompletedWithRegistryMeta(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())
	es.SetToolMetaFn(func(toolName string) (tools.ToolMeta, bool) {
		return tools.ToolMeta{
			RegistryName: "reg",
			HandlerName:  "h",
			HandlerType:  "http",
		}, true
	})

	event := &events.Event{
		Type:      events.EventToolCallCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ToolCallCompletedData{
			ToolName: "search",
			CallID:   "call-1",
			Status:   "success",
			Duration: 100 * time.Millisecond,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForToolCalls(t, 1)
	tcs := store.getToolCalls()
	if tcs[0].Labels["handler_type"] != "http" {
		t.Errorf("expected label handler_type=http, got %s", tcs[0].Labels["handler_type"])
	}
	if tcs[0].Labels["registry_name"] != "reg" {
		t.Errorf("expected label registry_name=reg, got %s", tcs[0].Labels["registry_name"])
	}
}

// TestOmniaEventStore_ToolCallWithoutMetaFn verifies graceful behavior with no toolMetaFn.
func TestOmniaEventStore_ToolCallWithoutMetaFn(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())
	// No SetToolMetaFn — should work without enrichment

	event := &events.Event{
		Type:      events.EventToolCallStarted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.ToolCallStartedData{
			ToolName: "search",
			CallID:   "call-1",
			Args:     map[string]any{"q": "test"},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForToolCalls(t, 1)
	tcs := store.getToolCalls()
	if tcs[0].Labels != nil {
		if _, ok := tcs[0].Labels["handler_name"]; ok {
			t.Error("expected no handler_name label when toolMetaFn is nil")
		}
	}
}

// --- Eval events ---

func TestOmniaEventStore_EvalCompleted(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	score := 0.85
	event := &events.Event{
		Type:      events.EventEvalCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.EvalEventData{
			EvalID:     "conciseness",
			EvalType:   "regex",
			Trigger:    "every_turn",
			Passed:     true,
			Score:      &score,
			DurationMs: 5,
			Message:    "Eval passed",
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// Eval events now go to eval_results, not messages.
	store.waitForEvalResults(t, 1)
	msgs := store.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for eval event, got %d", len(msgs))
	}

	er := store.getEvalResults()[0]
	if er.EvalID != "conciseness" {
		t.Errorf("expected EvalID 'conciseness', got %q", er.EvalID)
	}
	if er.EvalType != "regex" {
		t.Errorf("expected EvalType 'regex', got %q", er.EvalType)
	}
	if !er.Passed {
		t.Errorf("expected Passed true, got %v", er.Passed)
	}
	if er.DurationMs == nil || *er.DurationMs != 5 {
		t.Errorf("expected DurationMs=5, got %v", er.DurationMs)
	}
}

func TestOmniaEventStore_EvalFailed(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventEvalFailed,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.EvalEventData{
			EvalID:      "accuracy",
			EvalType:    "llm_judge",
			Trigger:     "on_session_complete",
			Passed:      false,
			DurationMs:  2500,
			Message:     "Eval failed: accuracy below threshold",
			Explanation: "Score was 0.3, threshold is 0.7",
			Violations:  []string{"accuracy too low"},
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForEvalResults(t, 1)
	er := store.getEvalResults()[0]
	if er.EvalID != "accuracy" {
		t.Errorf("expected EvalID 'accuracy', got %q", er.EvalID)
	}
	if er.Passed {
		t.Errorf("expected Passed false, got true")
	}
	// Explanation should be preserved in the Details JSON.
	var details map[string]any
	if err := json.Unmarshal(er.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal Details: %v", err)
	}
	if details["explanation"] != "Score was 0.3, threshold is 0.7" {
		t.Errorf("expected explanation preserved, got %v", details["explanation"])
	}
	if er.DurationMs == nil || *er.DurationMs != 2500 {
		t.Errorf("expected DurationMs=2500, got %v", er.DurationMs)
	}
}

func TestOmniaEventStore_EvalSkipped(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventEvalCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.EvalEventData{
			EvalID:     "sampled_eval",
			EvalType:   "llm_judge",
			Trigger:    "sample_turns",
			Skipped:    true,
			SkipReason: "sampling",
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForEvalResults(t, 1)
	er := store.getEvalResults()[0]
	var details map[string]any
	if err := json.Unmarshal(er.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal Details: %v", err)
	}
	if details["skipped"] != true {
		t.Errorf("expected skipped true, got %v", details["skipped"])
	}
	if details["skipReason"] != "sampling" {
		t.Errorf("expected skipReason 'sampling', got %v", details["skipReason"])
	}
}

func TestOmniaEventStore_EvalWithError(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventEvalFailed,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.EvalEventData{
			EvalID:     "broken",
			EvalType:   "regex",
			Trigger:    "every_turn",
			Error:      "invalid regex pattern",
			DurationMs: 1,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForEvalResults(t, 1)
	er := store.getEvalResults()[0]
	var details map[string]any
	if err := json.Unmarshal(er.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal Details: %v", err)
	}
	if details["error"] != "invalid regex pattern" {
		t.Errorf("expected error 'invalid regex pattern', got %v", details["error"])
	}
}

func TestOmniaEventStore_EvalPersistsAsEvalResult(t *testing.T) {
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventEvalCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: &events.EvalEventData{
			EvalID:   "test-eval",
			EvalType: "contains",
			Trigger:  "every_turn",
			Passed:   true,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForEvalResults(t, 1)
	er := store.getEvalResults()[0]
	if er.EvalID != "test-eval" {
		t.Errorf("expected EvalID 'test-eval', got %q", er.EvalID)
	}
	if er.EvalType != "contains" {
		t.Errorf("expected EvalType 'contains', got %q", er.EvalType)
	}
	if !er.Passed {
		t.Errorf("expected Passed true, got false")
	}
}

func TestOmniaEventStore_EvalValueTypedData(t *testing.T) {
	// Verify asPtr handles value-typed EvalEventData (not just *EvalEventData)
	store := &mockSessionStore{}
	es := NewOmniaEventStore(store, logr.Discard())

	event := &events.Event{
		Type:      events.EventEvalCompleted,
		SessionID: "test-session",
		Timestamp: time.Now(),
		Data: events.EvalEventData{ // value type, not pointer
			EvalID:   "value-typed",
			EvalType: "contains",
			Trigger:  "every_turn",
			Passed:   true,
		},
	}

	if err := es.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	store.waitForEvalResults(t, 1)
	er := store.getEvalResults()[0]
	if er.EvalID != "value-typed" {
		t.Errorf("expected EvalID 'value-typed', got %q", er.EvalID)
	}
	if er.EvalType != "contains" {
		t.Errorf("expected EvalType 'contains', got %q", er.EvalType)
	}
}
