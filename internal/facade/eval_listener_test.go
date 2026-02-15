/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/altairalabs/omnia/ee/pkg/evals"
	"github.com/altairalabs/omnia/internal/session"
)

// --- Mock implementations ---

type mockEvalLoader struct {
	mu        sync.Mutex
	evals     *evals.PromptPackEvals
	loadErr   error
	loadCalls int
}

func (m *mockEvalLoader) LoadEvals(_ context.Context, _, _, _ string) (*evals.PromptPackEvals, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadCalls++
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.evals, nil
}

func (m *mockEvalLoader) ResolveEvals(packEvals *evals.PromptPackEvals, trigger string) []evals.EvalDef {
	if packEvals == nil {
		return nil
	}
	if trigger == "" {
		result := make([]evals.EvalDef, len(packEvals.Evals))
		copy(result, packEvals.Evals)
		return result
	}
	var matched []evals.EvalDef
	for _, e := range packEvals.Evals {
		if e.Trigger == trigger {
			matched = append(matched, e)
		}
	}
	return matched
}

type mockMessageFetcher struct {
	mu       sync.Mutex
	messages []session.Message
	fetchErr error
}

func (m *mockMessageFetcher) GetMessages(_ context.Context, _ string) ([]session.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	result := make([]session.Message, len(m.messages))
	copy(result, m.messages)
	return result, nil
}

type mockResultWriter struct {
	mu       sync.Mutex
	results  []EvalResultInput
	writeErr error
}

func (m *mockResultWriter) WriteEvalResults(_ context.Context, results []EvalResultInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.results = append(m.results, results...)
	return nil
}

func (m *mockResultWriter) getResults() []EvalResultInput {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]EvalResultInput, len(m.results))
	copy(result, m.results)
	return result
}

// --- Helper functions ---

const (
	testEvalAgentName = "test-agent"
	testEvalNamespace = "default"
	testEvalSessionID = "sess-1"
)

func defaultConfig() EvalListenerConfig {
	return EvalListenerConfig{
		AgentName:    testEvalAgentName,
		Namespace:    testEvalNamespace,
		PackName:     "test-pack",
		PackVersion:  "v1.0.0",
		Enabled:      true,
		SamplingRate: 100,
		LLMJudgeRate: 10,
	}
}

func assistantEvent() EventBusEvent {
	return EventBusEvent{
		Type:      EventTypeRecordingMessage,
		SessionID: testEvalSessionID,
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{"role":"assistant","messageId":"msg-1"}`),
	}
}

func userEvent() EventBusEvent {
	return EventBusEvent{
		Type:      EventTypeRecordingMessage,
		SessionID: testEvalSessionID,
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{"role":"user","messageId":"msg-2"}`),
	}
}

func providerCallEvent() EventBusEvent {
	return EventBusEvent{
		Type:      EventTypeProviderCall,
		SessionID: testEvalSessionID,
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{"model":"gpt-4"}`),
	}
}

func sampleEvalDefs() *evals.PromptPackEvals {
	return &evals.PromptPackEvals{
		PackName:    "test-pack",
		PackVersion: "v1.0.0",
		Evals: []evals.EvalDef{
			{
				ID:      "eval-contains",
				Type:    "contains",
				Trigger: "per_turn",
				Params:  map[string]any{"value": "hello"},
			},
			{
				ID:      "eval-max-len",
				Type:    "max_length",
				Trigger: "per_turn",
				Params:  map[string]any{"maxLength": float64(1000)},
			},
			{
				ID:      "eval-session-contains",
				Type:    "contains",
				Trigger: "on_session_complete",
				Params:  map[string]any{"value": "goodbye"},
			},
			{
				ID:      "eval-judge",
				Type:    "llm_judge",
				Trigger: "per_turn",
				Params:  map[string]any{"judgeName": "fast-judge"},
			},
		},
	}
}

func sampleMessages() []session.Message {
	return []session.Message{
		{
			ID:        "msg-1",
			Role:      session.RoleUser,
			Content:   "say hello",
			Timestamp: time.Now().Add(-2 * time.Second),
		},
		{
			ID:        "msg-2",
			Role:      session.RoleAssistant,
			Content:   "hello world",
			Timestamp: time.Now().Add(-time.Second),
		},
	}
}

// --- Tests ---

func TestNewEvalListener(t *testing.T) {
	loader := &mockEvalLoader{}
	fetcher := &mockMessageFetcher{}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	if listener == nil {
		t.Fatal("expected non-nil listener")
	}
	if listener.config.AgentName != testEvalAgentName {
		t.Errorf("AgentName = %q, want %q", listener.config.AgentName, testEvalAgentName)
	}
	if listener.config.Enabled != true {
		t.Error("expected listener to be enabled")
	}
}

func TestEvalListener_OnEvent_TriggersPerTurnEvalsForAssistantMessages(t *testing.T) {
	loader := &mockEvalLoader{evals: sampleEvalDefs()}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	err := listener.OnEvent(context.Background(), assistantEvent())
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}

	results := writer.getResults()
	// Should have 2 results: contains + max_length (llm_judge is skipped)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Source != evalSourceInProc {
			t.Errorf("source = %q, want %q", r.Source, evalSourceInProc)
		}
		if r.SessionID != testEvalSessionID {
			t.Errorf("sessionID = %q, want %q", r.SessionID, testEvalSessionID)
		}
		if r.AgentName != testEvalAgentName {
			t.Errorf("agentName = %q, want %q", r.AgentName, testEvalAgentName)
		}
		if r.Namespace != testEvalNamespace {
			t.Errorf("namespace = %q, want %q", r.Namespace, testEvalNamespace)
		}
		if r.PromptPackName != "test-pack" {
			t.Errorf("promptpackName = %q, want %q", r.PromptPackName, "test-pack")
		}
		if r.Trigger != evalTriggerPerTurn {
			t.Errorf("trigger = %q, want %q", r.Trigger, evalTriggerPerTurn)
		}
	}

	// Check specific eval results
	if results[0].EvalID != "eval-contains" {
		t.Errorf("first eval ID = %q, want %q", results[0].EvalID, "eval-contains")
	}
	if results[1].EvalID != "eval-max-len" {
		t.Errorf("second eval ID = %q, want %q", results[1].EvalID, "eval-max-len")
	}
}

func TestEvalListener_OnEvent_SkipsNonAssistantMessages(t *testing.T) {
	loader := &mockEvalLoader{evals: sampleEvalDefs()}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	// User message should be skipped
	err := listener.OnEvent(context.Background(), userEvent())
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}

	results := writer.getResults()
	if len(results) != 0 {
		t.Errorf("expected 0 results for user message, got %d", len(results))
	}
}

func TestEvalListener_OnEvent_SkipsNonRecordingMessageEvents(t *testing.T) {
	loader := &mockEvalLoader{evals: sampleEvalDefs()}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	// provider.call event should be skipped
	err := listener.OnEvent(context.Background(), providerCallEvent())
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}

	results := writer.getResults()
	if len(results) != 0 {
		t.Errorf("expected 0 results for provider.call event, got %d", len(results))
	}
}

func TestEvalListener_OnEvent_DisabledSkipsAllEvents(t *testing.T) {
	loader := &mockEvalLoader{evals: sampleEvalDefs()}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()
	config.Enabled = false

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	err := listener.OnEvent(context.Background(), assistantEvent())
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}

	results := writer.getResults()
	if len(results) != 0 {
		t.Errorf("expected 0 results when disabled, got %d", len(results))
	}

	// Loader should not be called when disabled
	loader.mu.Lock()
	calls := loader.loadCalls
	loader.mu.Unlock()
	if calls != 0 {
		t.Errorf("loader called %d times when disabled, expected 0", calls)
	}
}

func TestEvalListener_OnEvent_SamplingFilters(t *testing.T) {
	loader := &mockEvalLoader{evals: sampleEvalDefs()}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()
	config.SamplingRate = 0 // 0% sampling = skip everything

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	err := listener.OnEvent(context.Background(), assistantEvent())
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}

	results := writer.getResults()
	if len(results) != 0 {
		t.Errorf("expected 0 results with 0%% sampling, got %d", len(results))
	}
}

func TestEvalListener_ShouldSample_Deterministic(t *testing.T) {
	config := defaultConfig()
	config.SamplingRate = 50

	listener := NewEvalListener(config, nil, nil, nil, newTestLogger())

	// Same input should always give same result
	result1 := listener.shouldSample("sess-1", evalTriggerPerTurn)
	result2 := listener.shouldSample("sess-1", evalTriggerPerTurn)
	if result1 != result2 {
		t.Error("shouldSample is not deterministic for same input")
	}
}

func TestEvalListener_ShouldSample_BoundaryRates(t *testing.T) {
	tests := []struct {
		name     string
		rate     int32
		expected bool
	}{
		{"rate_0_always_false", 0, false},
		{"rate_negative_always_false", -1, false},
		{"rate_100_always_true", 100, true},
		{"rate_above_100_always_true", 150, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := defaultConfig()
			config.SamplingRate = tt.rate
			listener := NewEvalListener(config, nil, nil, nil, newTestLogger())

			// Test multiple session IDs to verify boundary behavior
			for i := 0; i < 10; i++ {
				got := listener.shouldSample("sess-"+string(rune('a'+i)), evalTriggerPerTurn)
				if got != tt.expected {
					t.Errorf("shouldSample with rate %d = %v, want %v", tt.rate, got, tt.expected)
					break
				}
			}
		})
	}
}

func TestEvalListener_OnSessionComplete_TriggersSessionCompleteEvals(t *testing.T) {
	loader := &mockEvalLoader{evals: sampleEvalDefs()}
	msgs := []session.Message{
		{ID: "msg-1", Role: session.RoleAssistant, Content: "goodbye friend", Timestamp: time.Now()},
	}
	fetcher := &mockMessageFetcher{messages: msgs}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	err := listener.OnSessionComplete(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("OnSessionComplete returned error: %v", err)
	}

	results := writer.getResults()
	// Only the on_session_complete eval should run
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].EvalID != "eval-session-contains" {
		t.Errorf("evalID = %q, want %q", results[0].EvalID, "eval-session-contains")
	}
	if results[0].Trigger != evalTriggerOnSessionComplete {
		t.Errorf("trigger = %q, want %q", results[0].Trigger, evalTriggerOnSessionComplete)
	}
	if results[0].Source != evalSourceInProc {
		t.Errorf("source = %q, want %q", results[0].Source, evalSourceInProc)
	}
	if results[0].MessageID != "" {
		t.Errorf("messageID = %q, want empty for session complete", results[0].MessageID)
	}
}

func TestEvalListener_OnSessionComplete_DisabledSkips(t *testing.T) {
	loader := &mockEvalLoader{evals: sampleEvalDefs()}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()
	config.Enabled = false

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	err := listener.OnSessionComplete(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("OnSessionComplete returned error: %v", err)
	}

	results := writer.getResults()
	if len(results) != 0 {
		t.Errorf("expected 0 results when disabled, got %d", len(results))
	}
}

func TestEvalListener_OnEvent_LoaderError(t *testing.T) {
	loader := &mockEvalLoader{loadErr: errors.New("configmap not found")}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	err := listener.OnEvent(context.Background(), assistantEvent())
	if err == nil {
		t.Fatal("expected error when loader fails")
	}
	if !errors.Is(err, loader.loadErr) {
		t.Errorf("expected wrapped loadErr, got: %v", err)
	}
}

func TestEvalListener_OnEvent_FetcherError(t *testing.T) {
	loader := &mockEvalLoader{evals: sampleEvalDefs()}
	fetcher := &mockMessageFetcher{fetchErr: errors.New("session not found")}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	err := listener.OnEvent(context.Background(), assistantEvent())
	if err == nil {
		t.Fatal("expected error when fetcher fails")
	}
	if !errors.Is(err, fetcher.fetchErr) {
		t.Errorf("expected wrapped fetchErr, got: %v", err)
	}
}

func TestEvalListener_OnEvent_WriterError(t *testing.T) {
	loader := &mockEvalLoader{evals: sampleEvalDefs()}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{writeErr: errors.New("database unavailable")}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	err := listener.OnEvent(context.Background(), assistantEvent())
	if err == nil {
		t.Fatal("expected error when writer fails")
	}
	if !errors.Is(err, writer.writeErr) {
		t.Errorf("expected wrapped writeErr, got: %v", err)
	}
}

func TestEvalListener_OnEvent_NoEvalsForTrigger(t *testing.T) {
	// Create evals with only on_session_complete trigger
	packEvals := &evals.PromptPackEvals{
		PackName:    "test-pack",
		PackVersion: "v1.0.0",
		Evals: []evals.EvalDef{
			{
				ID:      "eval-session-only",
				Type:    "contains",
				Trigger: "on_session_complete",
				Params:  map[string]any{"value": "test"},
			},
		},
	}
	loader := &mockEvalLoader{evals: packEvals}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	// per_turn event should not match on_session_complete evals
	err := listener.OnEvent(context.Background(), assistantEvent())
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}

	results := writer.getResults()
	if len(results) != 0 {
		t.Errorf("expected 0 results when no per_turn evals exist, got %d", len(results))
	}
}

func TestEvalListener_OnEvent_SkipsLLMJudgeEvals(t *testing.T) {
	// Create evals with only llm_judge type
	packEvals := &evals.PromptPackEvals{
		PackName:    "test-pack",
		PackVersion: "v1.0.0",
		Evals: []evals.EvalDef{
			{
				ID:      "eval-judge-only",
				Type:    "llm_judge",
				Trigger: "per_turn",
				Params:  map[string]any{"judgeName": "strong-judge"},
			},
		},
	}
	loader := &mockEvalLoader{evals: packEvals}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	err := listener.OnEvent(context.Background(), assistantEvent())
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}

	results := writer.getResults()
	if len(results) != 0 {
		t.Errorf("expected 0 results when only llm_judge evals exist, got %d", len(results))
	}
}

func TestEvalListener_OnEvent_EvalPassedAndFailed(t *testing.T) {
	packEvals := &evals.PromptPackEvals{
		PackName:    "test-pack",
		PackVersion: "v1.0.0",
		Evals: []evals.EvalDef{
			{
				ID:      "eval-pass",
				Type:    "contains",
				Trigger: "per_turn",
				Params:  map[string]any{"value": "hello"},
			},
			{
				ID:      "eval-fail",
				Type:    "contains",
				Trigger: "per_turn",
				Params:  map[string]any{"value": "nonexistent-string-xyz"},
			},
		},
	}
	loader := &mockEvalLoader{evals: packEvals}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	err := listener.OnEvent(context.Background(), assistantEvent())
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}

	results := writer.getResults()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First eval should pass (content contains "hello")
	if results[0].EvalID != "eval-pass" || !results[0].Passed {
		t.Errorf("eval-pass: passed = %v, want true", results[0].Passed)
	}

	// Second eval should fail (content does not contain "nonexistent-string-xyz")
	if results[1].EvalID != "eval-fail" || results[1].Passed {
		t.Errorf("eval-fail: passed = %v, want false", results[1].Passed)
	}
}

func TestEvalListener_OnEvent_MessageIDFromEventData(t *testing.T) {
	packEvals := &evals.PromptPackEvals{
		PackName:    "test-pack",
		PackVersion: "v1.0.0",
		Evals: []evals.EvalDef{
			{
				ID:      "eval-1",
				Type:    "max_length",
				Trigger: "per_turn",
				Params:  map[string]any{"maxLength": float64(5000)},
			},
		},
	}
	loader := &mockEvalLoader{evals: packEvals}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	event := EventBusEvent{
		Type:      EventTypeRecordingMessage,
		SessionID: "sess-1",
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{"role":"assistant","messageId":"msg-abc-123"}`),
	}

	err := listener.OnEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}

	results := writer.getResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].MessageID != "msg-abc-123" {
		t.Errorf("messageID = %q, want %q", results[0].MessageID, "msg-abc-123")
	}
}

func TestEvalListener_OnEvent_NilEventData(t *testing.T) {
	loader := &mockEvalLoader{evals: sampleEvalDefs()}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	event := EventBusEvent{
		Type:      EventTypeRecordingMessage,
		SessionID: "sess-1",
		Timestamp: time.Now(),
		Data:      nil,
	}

	err := listener.OnEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}

	// Nil data means no role, so should be skipped
	results := writer.getResults()
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil data, got %d", len(results))
	}
}

func TestEvalListener_FilterRuleEvals(t *testing.T) {
	defs := []evals.EvalDef{
		{ID: "rule-1", Type: "contains"},
		{ID: "judge-1", Type: "llm_judge"},
		{ID: "rule-2", Type: "max_length"},
		{ID: "judge-2", Type: "llm_judge"},
		{ID: "rule-3", Type: "regex_match"},
	}

	filtered := filterRuleEvals(defs)
	if len(filtered) != 3 {
		t.Fatalf("expected 3 rule evals, got %d", len(filtered))
	}

	expectedIDs := []string{"rule-1", "rule-2", "rule-3"}
	for i, f := range filtered {
		if f.ID != expectedIDs[i] {
			t.Errorf("filtered[%d].ID = %q, want %q", i, f.ID, expectedIDs[i])
		}
	}
}

func TestEvalListener_OnEvent_InvalidEvalType(t *testing.T) {
	// An unsupported eval type should be logged as a warning but not fail the batch
	packEvals := &evals.PromptPackEvals{
		PackName:    "test-pack",
		PackVersion: "v1.0.0",
		Evals: []evals.EvalDef{
			{
				ID:      "eval-bad-type",
				Type:    "unsupported_type",
				Trigger: "per_turn",
				Params:  map[string]any{},
			},
			{
				ID:      "eval-good",
				Type:    "max_length",
				Trigger: "per_turn",
				Params:  map[string]any{"maxLength": float64(1000)},
			},
		},
	}
	loader := &mockEvalLoader{evals: packEvals}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())

	err := listener.OnEvent(context.Background(), assistantEvent())
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}

	// Only the valid eval should produce a result
	results := writer.getResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result (invalid type skipped), got %d", len(results))
	}
	if results[0].EvalID != "eval-good" {
		t.Errorf("evalID = %q, want %q", results[0].EvalID, "eval-good")
	}
}

func TestEventBridge_SetEvalListener(t *testing.T) {
	client := &mockSessionClient{}
	bridge := NewEventBridge(client, "agent", "ns", newTestLogger())

	if bridge.getEvalListener() != nil {
		t.Error("eval listener should be nil by default")
	}

	config := defaultConfig()
	listener := NewEvalListener(config, nil, nil, nil, newTestLogger())
	bridge.SetEvalListener(listener)

	if bridge.getEvalListener() != listener {
		t.Error("eval listener should be set after SetEvalListener")
	}

	bridge.SetEvalListener(nil)
	if bridge.getEvalListener() != nil {
		t.Error("eval listener should be nil after SetEvalListener(nil)")
	}
}

func TestEventBridge_HandleEvent_CallsEvalListener(t *testing.T) {
	sessionClient := &mockSessionClient{}
	bridge := NewEventBridge(sessionClient, "agent", "ns", newTestLogger())
	bridge.SetEnabled(true)

	packEvals := &evals.PromptPackEvals{
		PackName:    "test-pack",
		PackVersion: "v1.0.0",
		Evals: []evals.EvalDef{
			{
				ID:      "eval-1",
				Type:    "max_length",
				Trigger: "per_turn",
				Params:  map[string]any{"maxLength": float64(5000)},
			},
		},
	}
	loader := &mockEvalLoader{evals: packEvals}
	fetcher := &mockMessageFetcher{messages: sampleMessages()}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())
	bridge.SetEvalListener(listener)

	event := EventBusEvent{
		Type:      EventTypeRecordingMessage,
		SessionID: "sess-1",
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{"role":"assistant","messageId":"msg-1"}`),
	}

	err := bridge.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	// Verify both the session message and eval result were written
	msgs := sessionClient.getMessages()
	if len(msgs) != 1 {
		t.Errorf("expected 1 session message, got %d", len(msgs))
	}

	results := writer.getResults()
	if len(results) != 1 {
		t.Errorf("expected 1 eval result, got %d", len(results))
	}
}

func TestEventBridge_HandleEvent_EvalListenerErrorDoesNotFailPipeline(t *testing.T) {
	sessionClient := &mockSessionClient{}
	bridge := NewEventBridge(sessionClient, "agent", "ns", newTestLogger())
	bridge.SetEnabled(true)

	// Loader will fail, causing eval listener to error
	loader := &mockEvalLoader{loadErr: errors.New("eval loader broken")}
	fetcher := &mockMessageFetcher{}
	writer := &mockResultWriter{}
	config := defaultConfig()

	listener := NewEvalListener(config, loader, fetcher, writer, newTestLogger())
	bridge.SetEvalListener(listener)

	event := EventBusEvent{
		Type:      EventTypeRecordingMessage,
		SessionID: "sess-1",
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{"role":"assistant","messageId":"msg-1"}`),
	}

	// HandleEvent should succeed even though eval listener fails
	err := bridge.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleEvent should not fail when eval listener errors, got: %v", err)
	}

	// Session message should still be recorded
	msgs := sessionClient.getMessages()
	if len(msgs) != 1 {
		t.Errorf("expected 1 session message even when eval fails, got %d", len(msgs))
	}
}
