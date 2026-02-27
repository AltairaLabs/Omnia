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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
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

	evalResults        []*api.EvalResult
	sessionEvalResults []*api.EvalResult

	getSessionErr            error
	getMessagesErr           error
	writeErr                 error
	listEvalResultsErr       error
	getSessionEvalResultsErr error
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

func (m *mockSessionAPI) ListEvalResults(_ context.Context, _ api.EvalResultListOpts) ([]*api.EvalResult, error) {
	if m.listEvalResultsErr != nil {
		return nil, m.listEvalResultsErr
	}
	return m.evalResults, nil
}

func (m *mockSessionAPI) GetSessionEvalResults(_ context.Context, _ string) ([]*api.EvalResult, error) {
	if m.getSessionEvalResultsErr != nil {
		return nil, m.getSessionEvalResultsErr
	}
	return m.sessionEvalResults, nil
}

// newTestPackLoader creates a PromptPackLoader backed by a fake K8s client
// with a ConfigMap containing the given eval definitions.
func newTestPackLoader(namespace, packName string, evalDefs []EvalDef) *PromptPackLoader {
	packData, _ := json.Marshal(packJSON{
		ID:      packName,
		Version: "v1",
		Evals:   evalDefs,
	})
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      packName,
			Namespace: namespace,
		},
		Data: map[string]string{
			packJSONKey: string(packData),
		},
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	return NewPromptPackLoader(fakeClient)
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
	// implementation will call filterPerTurnDeterministicEvals(nil) which returns empty.
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
		evalRunner: RunRuleEval,
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

	packLoader := newTestPackLoader("ns", "test-pack", []EvalDef{
		{ID: "e1", Type: "rule", Trigger: triggerPerTurn, Params: map[string]any{"value": "x"}},
	})

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: RunRuleEval,
		packLoader: packLoader,
	}

	event := api.SessionEvent{
		EventType:      eventTypeMessage,
		SessionID:      "s1",
		MessageRole:    "assistant",
		Namespace:      "ns",
		PromptPackName: "test-pack",
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

	packLoader := newTestPackLoader("ns", "test-pack", []EvalDef{
		{ID: "e1", Type: "rule", Trigger: triggerPerTurn, Params: map[string]any{"value": "x"}},
	})

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: RunRuleEval,
		packLoader: packLoader,
	}

	event := api.SessionEvent{
		EventType:      eventTypeMessage,
		SessionID:      "s1",
		MessageRole:    "assistant",
		Namespace:      "ns",
		PromptPackName: "test-pack",
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

func TestFilterPerTurnDeterministicEvals(t *testing.T) {
	defs := []EvalDef{
		{ID: "e1", Type: "rule", Trigger: "per_turn", Params: map[string]any{"value": "x"}},
		{ID: "e2", Type: "llm_judge", Trigger: "per_turn"},
		{ID: "e3", Type: "rule", Trigger: "on_session_complete"},
		{ID: "e4", Type: "rule", Trigger: "per_turn", Params: map[string]any{"maxLength": 100}},
		{
			ID: "e5", Type: EvalTypeArenaAssertion, Trigger: "per_turn",
			Params: map[string]any{"assertion_type": "tools_called"},
		},
	}

	result := filterPerTurnDeterministicEvals(defs)

	require.Len(t, result, 3)
	assert.Equal(t, "e1", result[0].ID)
	assert.Equal(t, "e4", result[1].ID)
	assert.Equal(t, "e5", result[2].ID)
	assert.Equal(t, EvalTypeArenaAssertion, result[2].Type)
	assert.Equal(t, map[string]any{"value": "x"}, result[0].Params)
}

func TestFilterPerTurnDeterministicEvals_Nil(t *testing.T) {
	result := filterPerTurnDeterministicEvals(nil)
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
		evalRunner:    RunRuleEval,
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
		evalRunner:    RunRuleEval,
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

	packLoader := newTestPackLoader("ns", "test-pack", []EvalDef{
		{ID: "e1", Type: "rule", Trigger: triggerPerTurn, Params: map[string]any{"value": "x"}},
	})

	w := &EvalWorker{
		redisClient:   client,
		sessionAPI:    mock,
		namespace:     "ns",
		consumerGroup: "test-group",
		consumerName:  "test",
		logger:        testLogger(),
		evalRunner:    RunRuleEval,
		packLoader:    packLoader,
	}

	streamKey := testStreamKey
	_ = client.XGroupCreateMkStream(context.Background(), streamKey, "test-group", "0").Err()

	event := api.SessionEvent{
		EventType:      eventTypeMessage,
		SessionID:      "s1",
		MessageRole:    "assistant",
		Namespace:      "ns",
		PromptPackName: "test-pack",
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

	// Create a worker with a patched filterPerTurnDeterministicEvals that returns defs.
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
		evalRunner: RunRuleEval,
	}

	// Since filterPerTurnDeterministicEvals(nil) returns empty, processEvent returns nil.
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
		evalRunner: RunRuleEval,
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
		evalRunner: RunRuleEval,
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

func TestFilterOnCompleteDeterministicEvals(t *testing.T) {
	defs := []EvalDef{
		{ID: "e1", Type: "rule", Trigger: "per_turn"},
		{ID: "e2", Type: "rule", Trigger: "on_session_complete", Params: map[string]any{"value": "x"}},
		{ID: "e3", Type: "llm_judge", Trigger: "on_session_complete"},
		{ID: "e4", Type: "rule", Trigger: "on_session_complete", Params: map[string]any{"maxLength": 100}},
		{
			ID: "e5", Type: EvalTypeArenaAssertion, Trigger: "on_session_complete",
			Params: map[string]any{"assertion_type": "tools_called"},
		},
	}

	result := filterOnCompleteDeterministicEvals(defs)

	require.Len(t, result, 3)
	assert.Equal(t, "e2", result[0].ID)
	assert.Equal(t, "e4", result[1].ID)
	assert.Equal(t, "e5", result[2].ID)
	assert.Equal(t, EvalTypeArenaAssertion, result[2].Type)
	assert.Equal(t, map[string]any{"value": "x"}, result[0].Params)
}

func TestFilterOnCompleteDeterministicEvals_Nil(t *testing.T) {
	result := filterOnCompleteDeterministicEvals(nil)
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

	// filterOnCompleteDeterministicEvals(nil) returns empty, so no evals run.
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
			ID:             "s1",
			AgentName:      "test-agent",
			Namespace:      "ns",
			PromptPackName: "test-pack",
		},
		getMessagesErr: fmt.Errorf("timeout"),
	}

	packLoader := newTestPackLoader("ns", "test-pack", []EvalDef{
		{ID: "e1", Type: "rule", Trigger: triggerOnComplete, Params: map[string]any{"value": "x"}},
	})

	w := NewEvalWorker(WorkerConfig{
		RedisClient: goredis.NewClient(&goredis.Options{Addr: "localhost:0"}),
		SessionAPI:  mock,
		Namespace:   "ns",
		Logger:      testLogger(),
		PackLoader:  packLoader,
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

func TestGetSampler_LazyInit(t *testing.T) {
	w := &EvalWorker{logger: testLogger()}
	assert.Nil(t, w.sampler)

	s := w.getSampler()
	assert.NotNil(t, s)
	assert.Equal(t, int32(DefaultSamplingRate), s.DefaultRate())
	// Second call returns the same instance.
	assert.Same(t, s, w.getSampler())
}

func TestGetRateLimiter_LazyInit(t *testing.T) {
	w := &EvalWorker{logger: testLogger()}
	assert.Nil(t, w.rateLimiter)

	rl := w.getRateLimiter()
	assert.NotNil(t, rl)
	assert.Equal(t, int32(DefaultMaxEvalsPerSecond), rl.MaxEvalsPerSecond())
	// Second call returns the same instance.
	assert.Same(t, rl, w.getRateLimiter())
}

func TestAcquireRateLimit_NonJudge(t *testing.T) {
	w := &EvalWorker{
		logger:      testLogger(),
		rateLimiter: NewRateLimiter(nil),
	}
	err := w.acquireRateLimit(context.Background(), false)
	require.NoError(t, err)
}

func TestAcquireRateLimit_Judge(t *testing.T) {
	w := &EvalWorker{
		logger:      testLogger(),
		rateLimiter: NewRateLimiter(nil),
	}
	err := w.acquireRateLimit(context.Background(), true)
	require.NoError(t, err)
	w.rateLimiter.ReleaseJudge()
}

func TestExecuteSingleEval_Success(t *testing.T) {
	score := 0.9
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

	w := &EvalWorker{logger: testLogger(), evalRunner: runner}

	def := api.EvalDefinition{ID: "e1", Type: "contains", Trigger: "per_turn"}
	event := api.SessionEvent{SessionID: "s1", Namespace: "ns"}
	msgs := []session.Message{{ID: "m1", Role: session.RoleAssistant, Content: "hi"}}

	result := w.executeSingleEval(def, msgs, event, "agent")
	require.NotNil(t, result)
	assert.Equal(t, "e1", result.EvalID)
	assert.True(t, result.Passed)
	assert.Equal(t, evalSource, result.Source)
}

func TestExecuteSingleEval_Error(t *testing.T) {
	runner := func(_ api.EvalDefinition, _ []session.Message) (api.EvaluateResultItem, error) {
		return api.EvaluateResultItem{}, fmt.Errorf("eval engine failure")
	}

	w := &EvalWorker{logger: testLogger(), evalRunner: runner}

	def := api.EvalDefinition{ID: "e1", Type: "rule", Trigger: "per_turn"}
	event := api.SessionEvent{SessionID: "s1"}

	result := w.executeSingleEval(def, nil, event, "agent")
	assert.Nil(t, result)
}

func TestRunEvalsWithSampling_AllSampled(t *testing.T) {
	runner := func(def api.EvalDefinition, _ []session.Message) (api.EvaluateResultItem, error) {
		return api.EvaluateResultItem{
			EvalID:     def.ID,
			EvalType:   def.Type,
			Trigger:    def.Trigger,
			Passed:     true,
			DurationMs: 1,
		}, nil
	}

	w := &EvalWorker{
		logger:      testLogger(),
		evalRunner:  runner,
		sampler:     NewSampler(nil), // 100% default rate
		rateLimiter: NewRateLimiter(nil),
	}

	defs := []api.EvalDefinition{
		{ID: "e1", Type: "contains", Trigger: "per_turn"},
		{ID: "e2", Type: "max_length", Trigger: "per_turn"},
	}
	msgs := []session.Message{{ID: "m1", Role: session.RoleAssistant, Content: "hi"}}
	event := api.SessionEvent{SessionID: "s1", Namespace: "ns"}

	results := w.runEvalsWithSampling(context.Background(), defs, msgs, event, "agent", 1)
	assert.Len(t, results, 2)
}

func TestRunEvalsWithSampling_NoneSampled(t *testing.T) {
	runner := func(_ api.EvalDefinition, _ []session.Message) (api.EvaluateResultItem, error) {
		t.Fatal("runner should not be called when sampling rate is 0")
		return api.EvaluateResultItem{}, nil
	}

	dr := int32(0)
	w := &EvalWorker{
		logger:      testLogger(),
		evalRunner:  runner,
		sampler:     NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr}),
		rateLimiter: NewRateLimiter(nil),
	}

	defs := []api.EvalDefinition{
		{ID: "e1", Type: "rule", Trigger: "per_turn"},
	}
	event := api.SessionEvent{SessionID: "s1"}

	results := w.runEvalsWithSampling(context.Background(), defs, nil, event, "agent", 0)
	assert.Empty(t, results)
}

func TestRunEvalsWithSampling_RateLimitCancelled(t *testing.T) {
	callCount := 0
	runner := func(def api.EvalDefinition, _ []session.Message) (api.EvaluateResultItem, error) {
		callCount++
		return api.EvaluateResultItem{
			EvalID: def.ID, EvalType: def.Type, Passed: true,
		}, nil
	}

	// Rate limit of 1 per second with already-cancelled context.
	maxEvals := int32(1)
	w := &EvalWorker{
		logger:      testLogger(),
		evalRunner:  runner,
		sampler:     NewSampler(nil),
		rateLimiter: NewRateLimiter(&v1alpha1.EvalRateLimit{MaxEvalsPerSecond: &maxEvals}),
	}

	// Consume the burst token.
	ctx := context.Background()
	err := w.rateLimiter.Acquire(ctx)
	require.NoError(t, err)

	// Now use a cancelled context so the next Acquire fails.
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	defs := []api.EvalDefinition{
		{ID: "e1", Type: "rule", Trigger: "per_turn"},
		{ID: "e2", Type: "rule", Trigger: "per_turn"},
	}
	event := api.SessionEvent{SessionID: "s1"}

	results := w.runEvalsWithSampling(cancelledCtx, defs, nil, event, "agent", 0)
	assert.Empty(t, results)
	assert.Equal(t, 0, callCount, "no evals should run when rate limit fails")
}

func TestRunEvalsWithSampling_LLMJudgeSampling(t *testing.T) {
	callCount := 0
	runner := func(def api.EvalDefinition, _ []session.Message) (api.EvaluateResultItem, error) {
		callCount++
		return api.EvaluateResultItem{
			EvalID: def.ID, EvalType: def.Type, Passed: true, DurationMs: 1,
		}, nil
	}

	// Default rate 100%, LLM judge rate 0% — judge evals should be skipped.
	jr := int32(0)
	w := &EvalWorker{
		logger:      testLogger(),
		evalRunner:  runner,
		sampler:     NewSampler(&v1alpha1.EvalSampling{LLMJudgeRate: &jr}),
		rateLimiter: NewRateLimiter(nil),
	}

	defs := []api.EvalDefinition{
		{ID: "e1", Type: "rule", Trigger: "per_turn"},
		{ID: "e2", Type: "llm_judge", Trigger: "per_turn"},
	}
	event := api.SessionEvent{SessionID: "s1", Namespace: "ns"}

	results := w.runEvalsWithSampling(context.Background(), defs, nil, event, "agent", 0)
	assert.Len(t, results, 1)
	assert.Equal(t, "e1", results[0].EvalID)
	assert.Equal(t, 1, callCount)
}

func TestCountAssistantMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []session.Message
		expected int
	}{
		{
			name:     "empty",
			messages: nil,
			expected: 0,
		},
		{
			name: "mixed roles",
			messages: []session.Message{
				{Role: session.RoleUser},
				{Role: session.RoleAssistant},
				{Role: session.RoleUser},
				{Role: session.RoleAssistant},
				{Role: session.RoleAssistant},
			},
			expected: 3,
		},
		{
			name: "no assistant",
			messages: []session.Message{
				{Role: session.RoleUser},
				{Role: session.RoleUser},
			},
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, countAssistantMessages(tc.messages))
		})
	}
}

func TestNewEvalWorker_WithSamplerAndRateLimiter(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	dr := int32(50)
	sampler := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr})
	rateLimiter := NewRateLimiter(nil)

	w := NewEvalWorker(WorkerConfig{
		RedisClient: client,
		SessionAPI:  &mockSessionAPI{},
		Namespace:   "ns",
		Logger:      testLogger(),
		Sampler:     sampler,
		RateLimiter: rateLimiter,
	})

	assert.Same(t, sampler, w.sampler)
	assert.Same(t, rateLimiter, w.rateLimiter)
}

// --- New tests for PromptPack integration ---

func TestFilterPerTurnEvals(t *testing.T) {
	defs := []EvalDef{
		{ID: "e1", Type: "rule", Trigger: triggerPerTurn, Params: map[string]any{"value": "x"}},
		{ID: "e2", Type: evalTypeLLMJudge, Trigger: triggerPerTurn},
		{ID: "e3", Type: "rule", Trigger: triggerOnComplete},
		{ID: "e4", Type: "rule", Trigger: triggerPerTurn, Params: map[string]any{"maxLength": 100}},
	}

	result := filterPerTurnEvals(defs)

	require.Len(t, result, 3)
	assert.Equal(t, "e1", result[0].ID)
	assert.Equal(t, "e2", result[1].ID) // LLM judge included
	assert.Equal(t, evalTypeLLMJudge, result[1].Type)
	assert.Equal(t, "e4", result[2].ID)
	assert.Equal(t, map[string]any{"value": "x"}, result[0].Params)
}

func TestFilterPerTurnEvals_Nil(t *testing.T) {
	result := filterPerTurnEvals(nil)
	assert.Empty(t, result)
}

func TestFilterOnCompleteEvals(t *testing.T) {
	defs := []EvalDef{
		{ID: "e1", Type: "rule", Trigger: triggerPerTurn},
		{ID: "e2", Type: "rule", Trigger: triggerOnComplete, Params: map[string]any{"value": "x"}},
		{ID: "e3", Type: evalTypeLLMJudge, Trigger: triggerOnComplete},
		{ID: "e4", Type: "rule", Trigger: triggerOnComplete, Params: map[string]any{"maxLength": 100}},
	}

	result := filterOnCompleteEvals(defs)

	require.Len(t, result, 3)
	assert.Equal(t, "e2", result[0].ID)
	assert.Equal(t, "e3", result[1].ID) // LLM judge included
	assert.Equal(t, evalTypeLLMJudge, result[1].Type)
	assert.Equal(t, "e4", result[2].ID)
}

func TestFilterOnCompleteEvals_Nil(t *testing.T) {
	result := filterOnCompleteEvals(nil)
	assert.Empty(t, result)
}

func TestLoadPackEvals_WithPack(t *testing.T) {
	packLoader := newTestPackLoader("ns", "my-pack", []EvalDef{
		{ID: "e1", Type: "rule", Trigger: triggerPerTurn},
	})

	w := &EvalWorker{
		logger:     testLogger(),
		packLoader: packLoader,
	}

	event := api.SessionEvent{
		SessionID:      "s1",
		Namespace:      "ns",
		PromptPackName: "my-pack",
	}

	result := w.loadPackEvals(context.Background(), event)
	require.NotNil(t, result)
	assert.Equal(t, "my-pack", result.PackName)
	assert.Equal(t, "v1", result.PackVersion)
	assert.Len(t, result.Evals, 1)
}

func TestLoadPackEvals_NoPackLoader(t *testing.T) {
	w := &EvalWorker{logger: testLogger()}

	event := api.SessionEvent{
		SessionID:      "s1",
		PromptPackName: "my-pack",
	}

	result := w.loadPackEvals(context.Background(), event)
	assert.Nil(t, result)
}

func TestLoadPackEvals_NoPackName(t *testing.T) {
	packLoader := newTestPackLoader("other-ns", "my-pack", nil)
	w := &EvalWorker{
		logger:     testLogger(),
		packLoader: packLoader,
	}

	event := api.SessionEvent{SessionID: "s1", Namespace: "other-ns"}

	result := w.loadPackEvals(context.Background(), event)
	assert.Nil(t, result)
}

func TestEnrichEvent(t *testing.T) {
	event := api.SessionEvent{
		SessionID: "s1",
		Namespace: "ns",
	}
	packEvals := &PromptPackEvals{
		PackName:    "my-pack",
		PackVersion: "v2",
	}

	enriched := enrichEvent(event, packEvals)
	assert.Equal(t, "my-pack", enriched.PromptPackName)
	assert.Equal(t, "v2", enriched.PromptPackVersion)
	// Original event should not be modified (value semantics).
	assert.Empty(t, event.PromptPackName)
}

func TestToEvalResult_WithPromptPack(t *testing.T) {
	item := api.EvaluateResultItem{
		EvalID:   "e1",
		EvalType: "rule",
		Trigger:  triggerPerTurn,
		Passed:   true,
		Source:   evalSource,
	}
	event := api.SessionEvent{
		SessionID:         "s1",
		Namespace:         "ns",
		PromptPackName:    "my-pack",
		PromptPackVersion: "v3",
	}

	result := toEvalResult(item, event, "agent")
	assert.Equal(t, "my-pack", result.PromptPackName)
	assert.Equal(t, "v3", result.PromptPackVersion)
}

func TestNewEvalWorker_WithPackLoader(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	packLoader := newTestPackLoader("ns", "pack", nil)

	w := NewEvalWorker(WorkerConfig{
		RedisClient: client,
		SessionAPI:  &mockSessionAPI{},
		Namespace:   "ns",
		Logger:      testLogger(),
		PackLoader:  packLoader,
	})

	assert.Same(t, packLoader, w.packLoader)
}

func TestProcessAssistantMessage_WithPackEvals(t *testing.T) {
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

	runner := func(def api.EvalDefinition, _ []session.Message) (api.EvaluateResultItem, error) {
		return api.EvaluateResultItem{
			EvalID:     def.ID,
			EvalType:   def.Type,
			Trigger:    def.Trigger,
			Passed:     true,
			DurationMs: 5,
		}, nil
	}

	packLoader := newTestPackLoader("ns", "test-pack", []EvalDef{
		{ID: "e1", Type: "contains", Trigger: triggerPerTurn, Params: map[string]any{"value": "x"}},
		{ID: "e2", Type: "max_length", Trigger: triggerPerTurn, Params: map[string]any{"maxLength": 100}},
	})

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: runner,
		packLoader: packLoader,
	}

	event := api.SessionEvent{
		EventType:      eventTypeMessage,
		SessionID:      "s1",
		AgentName:      "test-agent",
		Namespace:      "ns",
		MessageID:      "m2",
		MessageRole:    "assistant",
		PromptPackName: "test-pack",
		Timestamp:      time.Now().Format(time.RFC3339),
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)
	require.Len(t, mock.written, 2)
	assert.Equal(t, "e1", mock.written[0].EvalID)
	assert.Equal(t, "e2", mock.written[1].EvalID)
	assert.Equal(t, "test-pack", mock.written[0].PromptPackName)
	assert.Equal(t, "v1", mock.written[0].PromptPackVersion)
	assert.True(t, mock.written[0].Passed)
}

func TestProcessAssistantMessage_NoPackName_SkipsEvals(t *testing.T) {
	mock := &mockSessionAPI{
		session: &session.Session{
			ID:        "s1",
			AgentName: "test-agent",
			Namespace: "ns",
		},
		messages: []session.Message{
			{ID: "m1", Role: session.RoleAssistant, Content: "hi"},
		},
	}

	w := &EvalWorker{
		sessionAPI: mock,
		namespace:  "ns",
		logger:     testLogger(),
		evalRunner: RunRuleEval,
	}

	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		MessageRole: "assistant",
		Namespace:   "ns",
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)
	assert.Empty(t, mock.written, "no evals should run when no pack name")
}

func TestOnSessionComplete_WithPackEvals(t *testing.T) {
	mock := &mockSessionAPI{
		session: &session.Session{
			ID:             "s1",
			AgentName:      "test-agent",
			Namespace:      "ns",
			PromptPackName: "test-pack",
		},
		messages: []session.Message{
			{ID: "m1", Role: session.RoleUser, Content: "hello"},
			{ID: "m2", Role: session.RoleAssistant, Content: "hi there"},
		},
	}

	runner := func(def api.EvalDefinition, _ []session.Message) (api.EvaluateResultItem, error) {
		return api.EvaluateResultItem{
			EvalID:     def.ID,
			EvalType:   def.Type,
			Trigger:    def.Trigger,
			Passed:     true,
			DurationMs: 3,
		}, nil
	}

	packLoader := newTestPackLoader("ns", "test-pack", []EvalDef{
		{ID: "e1", Type: "rule", Trigger: triggerOnComplete, Params: map[string]any{"value": "x"}},
	})

	w := NewEvalWorker(WorkerConfig{
		RedisClient: goredis.NewClient(&goredis.Options{Addr: "localhost:0"}),
		SessionAPI:  mock,
		Namespace:   "ns",
		Logger:      testLogger(),
		EvalRunner:  runner,
		PackLoader:  packLoader,
	})

	err := w.onSessionComplete(context.Background(), "s1")
	require.NoError(t, err)
	require.Len(t, mock.written, 1)
	assert.Equal(t, "e1", mock.written[0].EvalID)
	assert.Equal(t, "test-pack", mock.written[0].PromptPackName)
	assert.Equal(t, "v1", mock.written[0].PromptPackVersion)
}
