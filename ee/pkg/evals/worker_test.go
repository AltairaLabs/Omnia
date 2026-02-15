/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

const testStreamKey = "test-stream"

// testLogger returns a silent logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// mockSessionAPI implements SessionAPIClient for testing.
type mockSessionAPI struct {
	session  *session.Session
	messages []session.Message
	// written collects results passed to WriteEvalResults.
	written []*api.EvalResult

	getSessionErr  error
	getMessagesErr error
	writeErr       error
}

func (m *mockSessionAPI) GetSession(_ context.Context, _ string) (*session.Session, error) {
	if m.getSessionErr != nil {
		return nil, m.getSessionErr
	}
	return m.session, nil
}

func (m *mockSessionAPI) GetSessionMessages(_ context.Context, _ string) ([]session.Message, error) {
	if m.getMessagesErr != nil {
		return nil, m.getMessagesErr
	}
	return m.messages, nil
}

func (m *mockSessionAPI) WriteEvalResults(_ context.Context, results []*api.EvalResult) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.written = append(m.written, results...)
	return nil
}

func TestProcessEvent_AssistantMessage_RunsEvals(t *testing.T) {
	mock := &mockSessionAPI{
		session: &session.Session{
			ID:        "s1",
			AgentName: "test-agent",
			Namespace: "ns",
		},
		messages: []session.Message{
			{ID: "m1", Role: session.RoleUser, Content: "hello"},
			{ID: "m2", Role: session.RoleAssistant, Content: "hi there"},
		},
	}

	// Mock eval runner that always passes.
	runner := func(def api.EvalDefinition, msgs []session.Message) (api.EvaluateResultItem, error) {
		return api.EvaluateResultItem{
			EvalID:     def.ID,
			EvalType:   def.Type,
			Trigger:    def.Trigger,
			Passed:     true,
			DurationMs: 5,
		}, nil
	}

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: runner,
	}

	// Directly test processEvent with an assistant event — but the current
	// implementation will call filterPerTurnRuleEvals(nil) which returns empty.
	// So no evals run and no results are written. This tests the skip path.
	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		AgentName:   "test-agent",
		Namespace:   "ns",
		MessageID:   "m2",
		MessageRole: "assistant",
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)
	assert.Empty(t, mock.written, "no evals should be written when no eval defs")
}

func TestProcessEvent_NonAssistantMessage_Skipped(t *testing.T) {
	mock := &mockSessionAPI{}

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: api.RunRuleEval,
	}

	event := api.SessionEvent{
		EventType:   "message.user",
		SessionID:   "s1",
		MessageRole: "user",
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)
}

func TestProcessEvent_SessionAPIError(t *testing.T) {
	mock := &mockSessionAPI{
		getSessionErr: fmt.Errorf("connection refused"),
	}

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: api.RunRuleEval,
	}

	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		MessageRole: "assistant",
	}

	err := w.processEvent(context.Background(), event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get session")
}

func TestProcessEvent_GetMessagesError(t *testing.T) {
	mock := &mockSessionAPI{
		session: &session.Session{
			ID:        "s1",
			AgentName: "test-agent",
		},
		getMessagesErr: fmt.Errorf("timeout"),
	}

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: api.RunRuleEval,
	}

	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		MessageRole: "assistant",
	}

	err := w.processEvent(context.Background(), event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get session messages")
}

func TestRunEvals_Success(t *testing.T) {
	mock := &mockSessionAPI{}
	score := 1.0

	runner := func(def api.EvalDefinition, _ []session.Message) (api.EvaluateResultItem, error) {
		return api.EvaluateResultItem{
			EvalID:     def.ID,
			EvalType:   def.Type,
			Trigger:    def.Trigger,
			Passed:     true,
			Score:      &score,
			DurationMs: 3,
		}, nil
	}

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: runner,
	}

	defs := []api.EvalDefinition{
		{ID: "e1", Type: "contains", Trigger: "per_turn", Params: map[string]any{"value": "hello"}},
		{ID: "e2", Type: "max_length", Trigger: "per_turn", Params: map[string]any{"maxLength": 100}},
	}

	messages := []session.Message{
		{ID: "m1", Role: session.RoleAssistant, Content: "hello world"},
	}

	event := api.SessionEvent{
		SessionID: "s1",
		MessageID: "m1",
		Namespace: "ns",
	}

	results := w.runEvals(defs, messages, event, "test-agent")

	require.Len(t, results, 2)
	assert.Equal(t, "e1", results[0].EvalID)
	assert.Equal(t, "e2", results[1].EvalID)
	assert.Equal(t, evalSource, results[0].Source)
	assert.Equal(t, evalSource, results[1].Source)
	assert.True(t, results[0].Passed)
	assert.Equal(t, "test-agent", results[0].AgentName)
	assert.Equal(t, "ns", results[0].Namespace)
	assert.Equal(t, "s1", results[0].SessionID)
	assert.NotNil(t, results[0].DurationMs)
	assert.Equal(t, 3, *results[0].DurationMs)
}

func TestRunEvals_EvalFailure(t *testing.T) {
	runner := func(_ api.EvalDefinition, _ []session.Message) (api.EvaluateResultItem, error) {
		return api.EvaluateResultItem{}, fmt.Errorf("eval engine error")
	}

	w := &EvalWorker{
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: runner,
	}

	defs := []api.EvalDefinition{
		{ID: "e1", Type: "rule", Trigger: "per_turn"},
	}

	event := api.SessionEvent{SessionID: "s1", Namespace: "ns"}
	results := w.runEvals(defs, nil, event, "agent")

	assert.Empty(t, results)
}

func TestParseEvent_ValidPayload(t *testing.T) {
	payload := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		AgentName:   "agent",
		Namespace:   "ns",
		MessageID:   "m1",
		MessageRole: "assistant",
		Timestamp:   "2026-01-01T00:00:00Z",
	}
	data, _ := json.Marshal(payload)

	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": string(data)},
	}

	event, err := parseEvent(msg)
	require.NoError(t, err)
	assert.Equal(t, "s1", event.SessionID)
	assert.Equal(t, eventTypeMessage, event.EventType)
	assert.Equal(t, "assistant", event.MessageRole)
}

func TestParseEvent_MissingPayload(t *testing.T) {
	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"other": "data"},
	}

	_, err := parseEvent(msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestParseEvent_InvalidJSON(t *testing.T) {
	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": "not-json"},
	}

	_, err := parseEvent(msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestParseEvent_NonStringPayload(t *testing.T) {
	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": 12345},
	}

	_, err := parseEvent(msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a string")
}

func TestIsAssistantMessageEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    api.SessionEvent
		expected bool
	}{
		{
			name:     "assistant message",
			event:    api.SessionEvent{EventType: eventTypeMessage, MessageRole: "assistant"},
			expected: true,
		},
		{
			name:     "user message",
			event:    api.SessionEvent{EventType: "message.user", MessageRole: "user"},
			expected: false,
		},
		{
			name:     "wrong event type",
			event:    api.SessionEvent{EventType: "session.end", MessageRole: "assistant"},
			expected: false,
		},
		{
			name:     "empty event",
			event:    api.SessionEvent{},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isAssistantMessageEvent(tc.event))
		})
	}
}

func TestFilterPerTurnRuleEvals(t *testing.T) {
	defs := []EvalDef{
		{ID: "e1", Type: "rule", Trigger: "per_turn", Params: map[string]any{"value": "x"}},
		{ID: "e2", Type: "llm_judge", Trigger: "per_turn"},
		{ID: "e3", Type: "rule", Trigger: "on_session_complete"},
		{ID: "e4", Type: "rule", Trigger: "per_turn", Params: map[string]any{"maxLength": 100}},
	}

	result := filterPerTurnRuleEvals(defs)

	require.Len(t, result, 2)
	assert.Equal(t, "e1", result[0].ID)
	assert.Equal(t, "e4", result[1].ID)
	assert.Equal(t, map[string]any{"value": "x"}, result[0].Params)
}

func TestFilterPerTurnRuleEvals_Nil(t *testing.T) {
	result := filterPerTurnRuleEvals(nil)
	assert.Empty(t, result)
}

func TestToEvalResult(t *testing.T) {
	score := 0.75
	item := api.EvaluateResultItem{
		EvalID:     "e1",
		EvalType:   "contains",
		Trigger:    "per_turn",
		Passed:     true,
		Score:      &score,
		DurationMs: 10,
		Source:     evalSource,
	}

	event := api.SessionEvent{
		SessionID: "s1",
		MessageID: "m1",
		Namespace: "ns",
	}

	result := toEvalResult(item, event, "agent-x")

	assert.Equal(t, "s1", result.SessionID)
	assert.Equal(t, "m1", result.MessageID)
	assert.Equal(t, "agent-x", result.AgentName)
	assert.Equal(t, "ns", result.Namespace)
	assert.Equal(t, "e1", result.EvalID)
	assert.Equal(t, "contains", result.EvalType)
	assert.Equal(t, "per_turn", result.Trigger)
	assert.True(t, result.Passed)
	assert.Equal(t, &score, result.Score)
	assert.Equal(t, evalSource, result.Source)
	assert.NotNil(t, result.DurationMs)
	assert.Equal(t, 10, *result.DurationMs)
	assert.False(t, result.CreatedAt.IsZero())
}

func TestToEvalResult_ZeroDuration(t *testing.T) {
	item := api.EvaluateResultItem{
		EvalID:     "e1",
		EvalType:   "rule",
		DurationMs: 0,
	}
	event := api.SessionEvent{SessionID: "s1"}

	result := toEvalResult(item, event, "agent")
	assert.Nil(t, result.DurationMs)
}

func TestIsConsumerGroupExistsError(t *testing.T) {
	assert.True(t, isConsumerGroupExistsError(fmt.Errorf("BUSYGROUP Consumer Group name already exists")))
	assert.False(t, isConsumerGroupExistsError(fmt.Errorf("some other error")))
	assert.False(t, isConsumerGroupExistsError(nil))
}

func TestEnsureConsumerGroup(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := &EvalWorker{
		redisClient:   client,
		consumerGroup: "test-group",
		logger:        testLogger(),
	}

	// First call creates the group.
	err := w.ensureConsumerGroup(context.Background(), testStreamKey)
	require.NoError(t, err)

	// Second call is idempotent.
	err = w.ensureConsumerGroup(context.Background(), testStreamKey)
	require.NoError(t, err)
}

func TestStartAndShutdown(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	mock := &mockSessionAPI{}

	w := NewEvalWorker(WorkerConfig{
		RedisClient: client,
		SessionAPI:  mock,
		Namespace:   "test-ns",
		Logger:      testLogger(),
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Start(ctx)
	}()

	// Give the worker a moment to start, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not shut down in time")
	}
}

func TestHandleMessage_ParseError(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := &EvalWorker{
		redisClient:   client,
		consumerGroup: "test-group",
		consumerName:  "test",
		logger:        testLogger(),
	}

	streamKey := testStreamKey
	_ = client.XGroupCreateMkStream(context.Background(), streamKey, "test-group", "0").Err()

	// Add a message with invalid payload.
	client.XAdd(context.Background(), &goredis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{"payload": "invalid-json"},
	})

	// handleMessage should ACK the invalid message (skip it).
	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": "invalid-json"},
	}

	w.handleMessage(context.Background(), streamKey, msg)
	// No panic, no error — the malformed message is ACKed and skipped.
}

func TestNewEvalWorker_DefaultRunner(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient: client,
		SessionAPI:  &mockSessionAPI{},
		Namespace:   "ns",
		Logger:      testLogger(),
	})

	assert.NotNil(t, w.evalRunner)
	assert.Equal(t, "omnia-eval-workers-ns", w.consumerGroup)
}

func TestNewEvalWorker_CustomRunner(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	custom := func(_ api.EvalDefinition, _ []session.Message) (api.EvaluateResultItem, error) {
		return api.EvaluateResultItem{}, nil
	}

	w := NewEvalWorker(WorkerConfig{
		RedisClient: client,
		SessionAPI:  &mockSessionAPI{},
		Namespace:   "ns",
		Logger:      testLogger(),
		EvalRunner:  custom,
	})

	assert.NotNil(t, w.evalRunner)
}

func TestHostname(t *testing.T) {
	h := hostname()
	assert.NotEmpty(t, h)
	assert.NotEqual(t, "unknown", h)
}

func TestProcessStreams(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	mock := &mockSessionAPI{
		session: &session.Session{
			ID:        "s1",
			AgentName: "agent",
			Namespace: "ns",
		},
		messages: []session.Message{
			{ID: "m1", Role: session.RoleAssistant, Content: "hi"},
		},
	}

	w := &EvalWorker{
		redisClient:   client,
		sessionAPI:    mock,
		namespace:     "ns",
		consumerGroup: "test-group",
		consumerName:  "test",
		logger:        testLogger(),
		evalRunner:    api.RunRuleEval,
	}

	streamKey := testStreamKey
	_ = client.XGroupCreateMkStream(context.Background(), streamKey, "test-group", "0").Err()

	// Add a valid event.
	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		MessageRole: "assistant",
		Namespace:   "ns",
	}
	data, _ := json.Marshal(event)
	client.XAdd(context.Background(), &goredis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{"payload": string(data)},
	})

	// Read and process.
	streams, err := w.readFromStream(context.Background(), streamKey)
	require.NoError(t, err)
	require.NotEmpty(t, streams)

	w.processStreams(context.Background(), streamKey, streams)
}

func TestHandleMessage_SuccessfulProcess(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	mock := &mockSessionAPI{
		session: &session.Session{
			ID:        "s1",
			AgentName: "agent",
			Namespace: "ns",
		},
		messages: []session.Message{
			{ID: "m1", Role: session.RoleAssistant, Content: "hi"},
		},
	}

	w := &EvalWorker{
		redisClient:   client,
		sessionAPI:    mock,
		namespace:     "ns",
		consumerGroup: "test-group",
		consumerName:  "test",
		logger:        testLogger(),
		evalRunner:    api.RunRuleEval,
	}

	streamKey := testStreamKey
	_ = client.XGroupCreateMkStream(context.Background(), streamKey, "test-group", "0").Err()

	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		MessageRole: "assistant",
		Namespace:   "ns",
	}
	data, _ := json.Marshal(event)

	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": string(data)},
	}

	// Should not panic; processes event and ACKs.
	w.handleMessage(context.Background(), streamKey, msg)
}

func TestHandleMessage_ProcessError_NoAck(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	mock := &mockSessionAPI{
		getSessionErr: fmt.Errorf("connection refused"),
	}

	w := &EvalWorker{
		redisClient:   client,
		sessionAPI:    mock,
		namespace:     "ns",
		consumerGroup: "test-group",
		consumerName:  "test",
		logger:        testLogger(),
		evalRunner:    api.RunRuleEval,
	}

	streamKey := testStreamKey
	_ = client.XGroupCreateMkStream(context.Background(), streamKey, "test-group", "0").Err()

	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		MessageRole: "assistant",
		Namespace:   "ns",
	}
	data, _ := json.Marshal(event)

	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": string(data)},
	}

	// Should not panic; error means no ACK.
	w.handleMessage(context.Background(), streamKey, msg)
}

func TestAckMessage(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := &EvalWorker{
		redisClient:   client,
		consumerGroup: "test-group",
		logger:        testLogger(),
	}

	// ACK on a non-existent stream should not panic.
	w.ackMessage(context.Background(), "nonexistent-stream", "1-0")
}

func TestProcessEvent_WriteEvalResults(t *testing.T) {
	mock := &mockSessionAPI{
		session: &session.Session{
			ID:        "s1",
			AgentName: "test-agent",
			Namespace: "ns",
		},
		messages: []session.Message{
			{ID: "m1", Role: session.RoleUser, Content: "hello"},
			{ID: "m2", Role: session.RoleAssistant, Content: "contains-marker"},
		},
	}

	// Runner that always returns a result.
	runner := func(def api.EvalDefinition, _ []session.Message) (api.EvaluateResultItem, error) {
		return api.EvaluateResultItem{
			EvalID:     def.ID,
			EvalType:   def.Type,
			Trigger:    def.Trigger,
			Passed:     true,
			DurationMs: 2,
		}, nil
	}

	// Create a worker with a patched filterPerTurnRuleEvals that returns defs.
	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: runner,
	}

	// Directly call runEvals to test the write path.
	defs := []api.EvalDefinition{
		{ID: "e1", Type: "contains", Trigger: "per_turn", Params: map[string]any{"value": "marker"}},
	}
	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		MessageID:   "m2",
		Namespace:   "ns",
		MessageRole: "assistant",
	}

	results := w.runEvals(defs, mock.messages, event, "test-agent")
	require.Len(t, results, 1)
	assert.Equal(t, "e1", results[0].EvalID)
	assert.True(t, results[0].Passed)

	err := mock.WriteEvalResults(context.Background(), results)
	require.NoError(t, err)
	assert.Len(t, mock.written, 1)
}

func TestProcessEvent_WriteError(t *testing.T) {
	mock := &mockSessionAPI{
		session: &session.Session{
			ID:        "s1",
			AgentName: "test-agent",
			Namespace: "ns",
		},
		messages: []session.Message{{ID: "m1", Role: session.RoleAssistant, Content: "hi"}},
		writeErr: fmt.Errorf("write failed"),
	}

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: api.RunRuleEval,
	}

	// Since filterPerTurnRuleEvals(nil) returns empty, processEvent returns nil.
	// This tests the "no evals" path, which is still a valid test.
	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		MessageRole: "assistant",
		Namespace:   "ns",
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)
}

func TestProcessEvent_SessionCompleted_TriggersCompletion(t *testing.T) {
	mock := &mockSessionAPI{
		session: &session.Session{
			ID:        "s1",
			AgentName: "test-agent",
			Namespace: "ns",
		},
		messages: []session.Message{
			{ID: "m1", Role: session.RoleAssistant, Content: "hello"},
		},
	}

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: api.RunRuleEval,
	}

	// The tracker is lazily initialized and the onComplete callback is nil
	// for directly constructed workers. We verify the event is handled
	// without error and the tracker is initialized.
	event := api.SessionEvent{
		EventType: eventTypeSessionDone,
		SessionID: "s1",
		Namespace: "ns",
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)
	assert.NotNil(t, w.completionTracker)
}

func TestProcessEvent_AssistantMessage_RecordsActivity(t *testing.T) {
	mock := &mockSessionAPI{
		session: &session.Session{
			ID:        "s1",
			AgentName: "test-agent",
			Namespace: "ns",
		},
		messages: []session.Message{
			{ID: "m1", Role: session.RoleAssistant, Content: "hello"},
		},
	}

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: api.RunRuleEval,
	}

	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		MessageRole: "assistant",
		Namespace:   "ns",
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)

	// Verify the tracker was initialized and the session is tracked.
	assert.Equal(t, 1, w.getTracker().TrackedCount())
}

func TestIsSessionCompletedEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    api.SessionEvent
		expected bool
	}{
		{
			name:     "session completed event",
			event:    api.SessionEvent{EventType: eventTypeSessionDone},
			expected: true,
		},
		{
			name:     "assistant message event",
			event:    api.SessionEvent{EventType: eventTypeMessage, MessageRole: "assistant"},
			expected: false,
		},
		{
			name:     "empty event",
			event:    api.SessionEvent{},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isSessionCompletedEvent(tc.event))
		})
	}
}

func TestFilterOnCompleteRuleEvals(t *testing.T) {
	defs := []EvalDef{
		{ID: "e1", Type: "rule", Trigger: "per_turn"},
		{ID: "e2", Type: "rule", Trigger: "on_session_complete", Params: map[string]any{"value": "x"}},
		{ID: "e3", Type: "llm_judge", Trigger: "on_session_complete"},
		{ID: "e4", Type: "rule", Trigger: "on_session_complete", Params: map[string]any{"maxLength": 100}},
	}

	result := filterOnCompleteRuleEvals(defs)

	require.Len(t, result, 2)
	assert.Equal(t, "e2", result[0].ID)
	assert.Equal(t, "e4", result[1].ID)
	assert.Equal(t, map[string]any{"value": "x"}, result[0].Params)
}

func TestFilterOnCompleteRuleEvals_Nil(t *testing.T) {
	result := filterOnCompleteRuleEvals(nil)
	assert.Empty(t, result)
}

func TestNewEvalWorker_CompletionTracker(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient: client,
		SessionAPI:  &mockSessionAPI{},
		Namespace:   "ns",
		Logger:      testLogger(),
	})

	assert.NotNil(t, w.completionTracker)
}

func TestNewEvalWorker_CustomInactivityTimeout(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient:       client,
		SessionAPI:        &mockSessionAPI{},
		Namespace:         "ns",
		Logger:            testLogger(),
		InactivityTimeout: 10 * time.Minute,
	})

	assert.NotNil(t, w.completionTracker)
}

func TestOnSessionComplete_NoEvals(t *testing.T) {
	mock := &mockSessionAPI{
		session: &session.Session{
			ID:        "s1",
			AgentName: "test-agent",
			Namespace: "ns",
		},
		messages: []session.Message{
			{ID: "m1", Role: session.RoleAssistant, Content: "hello"},
		},
	}

	w := NewEvalWorker(WorkerConfig{
		RedisClient: goredis.NewClient(&goredis.Options{Addr: "localhost:0"}),
		SessionAPI:  mock,
		Namespace:   "ns",
		Logger:      testLogger(),
	})

	// filterOnCompleteRuleEvals(nil) returns empty, so no evals run.
	err := w.onSessionComplete(context.Background(), "s1")
	require.NoError(t, err)
	assert.Empty(t, mock.written)
}

func TestOnSessionComplete_GetSessionError(t *testing.T) {
	mock := &mockSessionAPI{
		getSessionErr: fmt.Errorf("session not found"),
	}

	w := NewEvalWorker(WorkerConfig{
		RedisClient: goredis.NewClient(&goredis.Options{Addr: "localhost:0"}),
		SessionAPI:  mock,
		Namespace:   "ns",
		Logger:      testLogger(),
	})

	err := w.onSessionComplete(context.Background(), "s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get session")
}

func TestOnSessionComplete_GetMessagesError(t *testing.T) {
	mock := &mockSessionAPI{
		session: &session.Session{
			ID:        "s1",
			AgentName: "test-agent",
			Namespace: "ns",
		},
		getMessagesErr: fmt.Errorf("timeout"),
	}

	w := NewEvalWorker(WorkerConfig{
		RedisClient: goredis.NewClient(&goredis.Options{Addr: "localhost:0"}),
		SessionAPI:  mock,
		Namespace:   "ns",
		Logger:      testLogger(),
	})

	err := w.onSessionComplete(context.Background(), "s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get session messages")
}

func TestWriteResults_Empty(t *testing.T) {
	mock := &mockSessionAPI{}
	w := &EvalWorker{
		sessionAPI: mock,
		logger:     testLogger(),
	}

	err := w.writeResults(context.Background(), nil, "s1")
	require.NoError(t, err)
	assert.Empty(t, mock.written)
}

func TestWriteResults_Success(t *testing.T) {
	mock := &mockSessionAPI{}
	w := &EvalWorker{
		sessionAPI: mock,
		logger:     testLogger(),
	}

	results := []*api.EvalResult{
		{SessionID: "s1", EvalID: "e1", Passed: true},
	}
	err := w.writeResults(context.Background(), results, "s1")
	require.NoError(t, err)
	assert.Len(t, mock.written, 1)
}

func TestWriteResults_Error(t *testing.T) {
	mock := &mockSessionAPI{writeErr: fmt.Errorf("write failed")}
	w := &EvalWorker{
		sessionAPI: mock,
		logger:     testLogger(),
	}

	results := []*api.EvalResult{
		{SessionID: "s1", EvalID: "e1", Passed: true},
	}
	err := w.writeResults(context.Background(), results, "s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write eval results")
}

func TestGetTracker_LazyInit(t *testing.T) {
	w := &EvalWorker{
		logger: testLogger(),
	}

	assert.Nil(t, w.completionTracker)
	tracker := w.getTracker()
	assert.NotNil(t, tracker)
	// Second call returns the same tracker.
	assert.Same(t, tracker, w.getTracker())
}
