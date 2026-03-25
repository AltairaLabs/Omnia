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

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	sdkmetrics "github.com/AltairaLabs/PromptKit/runtime/metrics"
	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
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

// mockResultWriter implements EvalResultWriter for testing.
type mockResultWriter struct {
	written  []*api.EvalResult
	writeErr error
}

func (m *mockResultWriter) WriteEvalResults(_ context.Context, results []*api.EvalResult) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.written = append(m.written, results...)
	return nil
}

// mockMessageStore implements MessageStore for testing.
type mockMessageStore struct {
	sess       *session.Session
	messages   []*session.Message
	getErr     error
	getMsgsErr error
}

func (m *mockMessageStore) GetSession(_ context.Context, _ string) (*session.Session, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.sess, nil
}

func (m *mockMessageStore) GetRecentMessages(_ context.Context, _ string, _ int) ([]*session.Message, error) {
	if m.getMsgsErr != nil {
		return nil, m.getMsgsErr
	}
	return m.messages, nil
}

// containsEvalDef builds a contains-type EvalDef with the given params.
func containsEvalDef(
	id string, trigger runtimeevals.EvalTrigger, patterns ...string,
) runtimeevals.EvalDef {
	p := make([]any, len(patterns))
	for i, s := range patterns {
		p[i] = s
	}
	return runtimeevals.EvalDef{
		ID:      id,
		Type:    "contains",
		Trigger: trigger,
		Params:  map[string]any{"patterns": p},
	}
}

// newTestPackLoader creates a PromptPackLoader backed by a fake K8s client
// with a ConfigMap containing the given eval definitions.
func newTestPackLoader(evalDefs []runtimeevals.EvalDef) *PromptPackLoader {
	const namespace = "ns"
	const packName = "test-pack"
	pack := map[string]any{
		"id":      packName,
		"version": "v1",
		"evals":   evalDefs,
		"prompts": map[string]any{
			"default": map[string]any{
				"id":              "default",
				"name":            "Default",
				"version":         "1.0.0",
				"system_template": "You are a helpful assistant.",
			},
		},
	}
	packData, _ := json.Marshal(pack)
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

// toMessagePtrs converts a value slice to a pointer slice.
func toMessagePtrs(msgs []session.Message) []*session.Message {
	ptrs := make([]*session.Message, len(msgs))
	for i := range msgs {
		ptrs[i] = &msgs[i]
	}
	return ptrs
}

func TestProcessEvent_AssistantMessage_NoPackSkips(t *testing.T) {
	w := &EvalWorker{
		messageStore: &mockMessageStore{
			sess: &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
		},
		resultWriter: &mockResultWriter{},
		namespaces:   []string{"ns"},
		logger:       testLogger(),
	}

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
}

func TestProcessEvent_NonAssistantMessage_Skipped(t *testing.T) {
	w := &EvalWorker{
		messageStore: &mockMessageStore{},
		resultWriter: &mockResultWriter{},
		namespaces:   []string{"ns"},
		logger:       testLogger(),
	}

	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		MessageID:   "m1",
		MessageRole: "user",
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)
}

func TestProcessEvent_GetMessagesError(t *testing.T) {
	store := &mockMessageStore{
		sess:       &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
		getMsgsErr: fmt.Errorf("redis connection refused"),
	}

	packLoader := newTestPackLoader([]runtimeevals.EvalDef{
		containsEvalDef("e1", runtimeevals.TriggerEveryTurn, "hello"),
	})

	w := &EvalWorker{
		messageStore: store,
		resultWriter: &mockResultWriter{},
		namespaces:   []string{"ns"},
		logger:       testLogger(),
		packLoader:   packLoader,
	}

	event := api.SessionEvent{
		EventType:         eventTypeMessage,
		SessionID:         "s1",
		AgentName:         "test-agent",
		Namespace:         "ns",
		MessageID:         "m2",
		MessageRole:       "assistant",
		PromptPackName:    "test-pack",
		PromptPackVersion: "v1",
	}

	err := w.processEvent(context.Background(), event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis connection refused")
}

func TestParseEvent_ValidPayload(t *testing.T) {
	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		MessageID:   "m1",
		MessageRole: "assistant",
		AgentName:   "test-agent",
		Namespace:   "ns",
	}
	payload, _ := json.Marshal(event)
	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]any{streamPayloadField: string(payload)},
	}

	parsed, err := parseEvent(msg)
	require.NoError(t, err)
	assert.Equal(t, "s1", parsed.SessionID)
	assert.Equal(t, "assistant", parsed.MessageRole)
}

func TestParseEvent_MissingPayload(t *testing.T) {
	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]any{},
	}

	_, err := parseEvent(msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestParseEvent_InvalidJSON(t *testing.T) {
	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]any{streamPayloadField: "not-json"},
	}

	_, err := parseEvent(msg)
	require.Error(t, err)
}

func TestParseEvent_NonStringPayload(t *testing.T) {
	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]any{streamPayloadField: 42},
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
			event:    api.SessionEvent{EventType: eventTypeMessage, MessageRole: "user"},
			expected: false,
		},
		{
			name:     "session completed",
			event:    api.SessionEvent{EventType: eventTypeSessionDone},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isAssistantMessageEvent(tt.event))
		})
	}
}

func TestToEvalResult(t *testing.T) {
	score := 0.9
	item := api.EvaluateResultItem{
		EvalID:     "e1",
		EvalType:   "contains",
		Trigger:    "per_turn",
		Passed:     true,
		Score:      &score,
		DurationMs: 5,
		Source:     "worker",
	}
	event := api.SessionEvent{
		SessionID: "s1",
		MessageID: "m1",
		Namespace: "ns",
	}

	result := toEvalResult(item, event, "test-agent")
	assert.Equal(t, "s1", result.SessionID)
	assert.Equal(t, "m1", result.MessageID)
	assert.Equal(t, "test-agent", result.AgentName)
	assert.Equal(t, "e1", result.EvalID)
	assert.Equal(t, "contains", result.EvalType)
	assert.True(t, result.Passed)
	assert.Equal(t, &score, result.Score)
	require.NotNil(t, result.DurationMs)
	assert.Equal(t, 5, *result.DurationMs)
}

func TestToEvalResult_ZeroDuration(t *testing.T) {
	item := api.EvaluateResultItem{EvalID: "e1"}
	event := api.SessionEvent{SessionID: "s1"}

	result := toEvalResult(item, event, "agent")
	assert.Nil(t, result.DurationMs)
}

func TestIsConsumerGroupExistsError(t *testing.T) {
	assert.True(t, isConsumerGroupExistsError(fmt.Errorf("BUSYGROUP Consumer Group name already exists")))
	assert.False(t, isConsumerGroupExistsError(fmt.Errorf("other error")))
	assert.False(t, isConsumerGroupExistsError(nil))
}

func TestEnsureConsumerGroup(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := &EvalWorker{
		redisClient:   client,
		consumerGroup: "test-group",
		logger:        testLogger(),
	}

	err = w.ensureConsumerGroup(context.Background(), "test-stream")
	require.NoError(t, err)

	// Second call should succeed (group already exists).
	err = w.ensureConsumerGroup(context.Background(), "test-stream")
	require.NoError(t, err)
}

func TestStartAndShutdown(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		MessageStore: &mockMessageStore{},
		Namespaces:   []string{"ns"},
		Logger:       testLogger(),
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- w.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not shut down in time")
	}
}

func TestHandleMessage_ParseError(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// Create the stream and consumer group.
	client.XGroupCreateMkStream(context.Background(), testStreamKey, "test-group", "0")

	w := &EvalWorker{
		redisClient:   client,
		resultWriter:  &mockResultWriter{},
		messageStore:  &mockMessageStore{},
		namespaces:    []string{"ns"},
		streamKeys:    []string{testStreamKey},
		consumerGroup: "test-group",
		consumerName:  "test-consumer",
		logger:        testLogger(),
	}

	msg := goredis.XMessage{
		ID:     "1-0",
		Values: map[string]any{streamPayloadField: "invalid-json"},
	}

	w.handleMessage(context.Background(), testStreamKey, msg)
}

func TestNewEvalWorker_DefaultSDKRunner(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespaces:   []string{"ns"},
		Logger:       testLogger(),
	})

	require.NotNil(t, w.sdkRunner)
	require.NotNil(t, w.messageStore)
}

func TestNewEvalWorker_CustomSDKRunner(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	customRunner := NewSDKRunner()
	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespaces:   []string{"ns"},
		Logger:       testLogger(),
		SDKRunner:    customRunner,
	})

	assert.Equal(t, customRunner, w.sdkRunner)
}

func TestNewEvalWorker_TracerProviderWiring(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	tp := noop.NewTracerProvider()
	w := NewEvalWorker(WorkerConfig{
		RedisClient:    client,
		ResultWriter:   &mockResultWriter{},
		Namespaces:     []string{"ns"},
		Logger:         testLogger(),
		TracerProvider: tp,
	})

	assert.Equal(t, tp, w.TracerProvider(), "TracerProvider should be wired to SDKRunner")
}

func TestNewEvalWorker_NoTracerProvider(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespaces:   []string{"ns"},
		Logger:       testLogger(),
	})

	assert.Nil(t, w.TracerProvider(), "TracerProvider should be nil when not configured")
}

func TestNewEvalWorker_LoggerWiring(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	logger := testLogger()
	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespaces:   []string{"ns"},
		Logger:       logger,
	})

	require.NotNil(t, w.sdkRunner)
	assert.Equal(t, logger, w.sdkRunner.logger, "Logger should be wired to SDKRunner")
}

func TestHostname(t *testing.T) {
	h := hostname()
	assert.NotEmpty(t, h)
}

func TestProcessStreams(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	packLoader := newTestPackLoader([]runtimeevals.EvalDef{
		containsEvalDef("e1", runtimeevals.TriggerEveryTurn, "hello"),
	})

	msgs := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "say hello"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hello world"},
	}

	writer := &mockResultWriter{}
	store := &mockMessageStore{
		sess:     &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
		messages: toMessagePtrs(msgs),
	}

	w := &EvalWorker{
		redisClient:   client,
		resultWriter:  writer,
		messageStore:  store,
		namespaces:    []string{"ns"},
		streamKeys:    []string{testStreamKey},
		consumerGroup: "test-group",
		consumerName:  "test-consumer",
		logger:        testLogger(),
		packLoader:    packLoader,
	}

	event := api.SessionEvent{
		EventType:         eventTypeMessage,
		SessionID:         "s1",
		AgentName:         "test-agent",
		Namespace:         "ns",
		MessageID:         "m2",
		MessageRole:       "assistant",
		PromptPackName:    "test-pack",
		PromptPackVersion: "v1",
	}
	payload, _ := json.Marshal(event)

	// Create consumer group, add message, and process.
	client.XGroupCreateMkStream(context.Background(), testStreamKey, "test-group", "0")
	client.XAdd(context.Background(), &goredis.XAddArgs{
		Stream: testStreamKey,
		Values: map[string]any{streamPayloadField: string(payload)},
	})

	streams, err := client.XReadGroup(context.Background(), &goredis.XReadGroupArgs{
		Group:    "test-group",
		Consumer: "test-consumer",
		Streams:  []string{testStreamKey, ">"},
		Count:    10,
		Block:    time.Second,
	}).Result()
	require.NoError(t, err)

	w.processStreams(context.Background(), streams)

	// The SDK's contains handler should have run.
	assert.NotEmpty(t, writer.written)
}

func TestHandleMessage_SuccessfulProcess(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	packLoader := newTestPackLoader([]runtimeevals.EvalDef{
		containsEvalDef("e1", runtimeevals.TriggerEveryTurn, "hello"),
	})

	msgs := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "test"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hello"},
	}

	writer := &mockResultWriter{}
	store := &mockMessageStore{
		sess:     &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
		messages: toMessagePtrs(msgs),
	}

	w := &EvalWorker{
		redisClient:   client,
		resultWriter:  writer,
		messageStore:  store,
		namespaces:    []string{"ns"},
		streamKeys:    []string{testStreamKey},
		consumerGroup: "test-group",
		consumerName:  "test-consumer",
		logger:        testLogger(),
		packLoader:    packLoader,
	}

	event := api.SessionEvent{
		EventType:         eventTypeMessage,
		SessionID:         "s1",
		AgentName:         "test-agent",
		Namespace:         "ns",
		MessageID:         "m2",
		MessageRole:       "assistant",
		PromptPackName:    "test-pack",
		PromptPackVersion: "v1",
	}
	payload, _ := json.Marshal(event)

	client.XGroupCreateMkStream(context.Background(), testStreamKey, "test-group", "0")
	client.XAdd(context.Background(), &goredis.XAddArgs{
		Stream: testStreamKey,
		Values: map[string]any{streamPayloadField: string(payload)},
	})

	streams, err := client.XReadGroup(context.Background(), &goredis.XReadGroupArgs{
		Group:    "test-group",
		Consumer: "test-consumer",
		Streams:  []string{testStreamKey, ">"},
		Count:    10,
		Block:    time.Second,
	}).Result()
	require.NoError(t, err)
	require.NotEmpty(t, streams)

	w.handleMessage(context.Background(), testStreamKey, streams[0].Messages[0])
	assert.NotEmpty(t, writer.written)
}

func TestAckMessage(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := &EvalWorker{
		redisClient:   client,
		consumerGroup: "test-group",
		logger:        testLogger(),
	}

	// ACK on a non-existent stream should not panic.
	w.ackMessage(context.Background(), "non-existent-stream", "1-0")
}

func TestProcessEvent_WriteEvalResults(t *testing.T) {
	packLoader := newTestPackLoader([]runtimeevals.EvalDef{
		containsEvalDef("e1", runtimeevals.TriggerEveryTurn, "world"),
	})

	msgs := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "test"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hello world"},
	}

	writer := &mockResultWriter{}
	store := &mockMessageStore{
		sess:     &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
		messages: toMessagePtrs(msgs),
	}

	w := &EvalWorker{
		resultWriter: writer,
		messageStore: store,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
		packLoader:   packLoader,
	}

	event := api.SessionEvent{
		EventType:         eventTypeMessage,
		SessionID:         "s1",
		AgentName:         "test-agent",
		Namespace:         "ns",
		MessageID:         "m2",
		MessageRole:       "assistant",
		PromptPackName:    "test-pack",
		PromptPackVersion: "v1",
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)
	assert.NotEmpty(t, writer.written, "eval results should be written")
	assert.Equal(t, "e1", writer.written[0].EvalID)
	assert.True(t, writer.written[0].Passed)
}

func TestProcessEvent_WriteError(t *testing.T) {
	packLoader := newTestPackLoader([]runtimeevals.EvalDef{
		containsEvalDef("e1", runtimeevals.TriggerEveryTurn, "hello"),
	})

	msgs := []session.Message{
		{ID: "m1", Role: session.RoleAssistant, Content: "hello"},
	}

	writer := &mockResultWriter{writeErr: fmt.Errorf("write failed")}
	store := &mockMessageStore{
		sess:     &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
		messages: toMessagePtrs(msgs),
	}

	w := &EvalWorker{
		resultWriter: writer,
		messageStore: store,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
		packLoader:   packLoader,
	}

	event := api.SessionEvent{
		EventType:         eventTypeMessage,
		SessionID:         "s1",
		AgentName:         "test-agent",
		Namespace:         "ns",
		MessageID:         "m1",
		MessageRole:       "assistant",
		PromptPackName:    "test-pack",
		PromptPackVersion: "v1",
	}

	err := w.processEvent(context.Background(), event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

func TestProcessEvent_SessionCompleted_TriggersCompletion(t *testing.T) {
	writer := &mockResultWriter{}
	store := &mockMessageStore{
		sess: &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
	}

	w := &EvalWorker{
		resultWriter: writer,
		messageStore: store,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
	}
	w.completionTracker = NewCompletionTracker(DefaultInactivityTimeout, nil, testLogger())

	event := api.SessionEvent{
		EventType: eventTypeSessionDone,
		SessionID: "s1",
		Namespace: "ns",
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)
}

func TestProcessEvent_AssistantMessage_RecordsActivity(t *testing.T) {
	writer := &mockResultWriter{}
	store := &mockMessageStore{
		sess: &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
	}

	w := &EvalWorker{
		resultWriter: writer,
		messageStore: store,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
	}
	w.completionTracker = NewCompletionTracker(DefaultInactivityTimeout, nil, testLogger())

	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		AgentName:   "test-agent",
		Namespace:   "ns",
		MessageID:   "m2",
		MessageRole: "assistant",
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)
}

func TestIsSessionCompletedEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    api.SessionEvent
		expected bool
	}{
		{
			name:     "session completed",
			event:    api.SessionEvent{EventType: eventTypeSessionDone},
			expected: true,
		},
		{
			name:     "assistant message",
			event:    api.SessionEvent{EventType: eventTypeMessage, MessageRole: "assistant"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isSessionCompletedEvent(tt.event))
		})
	}
}

func TestNewEvalWorker_CompletionTracker(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespaces:   []string{"ns"},
		Logger:       testLogger(),
	})

	require.NotNil(t, w.completionTracker)
}

func TestNewEvalWorker_CustomInactivityTimeout(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient:       client,
		ResultWriter:      &mockResultWriter{},
		Namespaces:        []string{"ns"},
		Logger:            testLogger(),
		InactivityTimeout: 60 * time.Second,
	})

	require.NotNil(t, w.completionTracker)
}

func TestOnSessionComplete_NoEvals(t *testing.T) {
	store := &mockMessageStore{
		sess: &session.Session{
			ID:        "s1",
			AgentName: "test-agent",
			Namespace: "ns",
		},
	}

	w := &EvalWorker{
		resultWriter: &mockResultWriter{},
		messageStore: store,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
	}
	w.completionTracker = NewCompletionTracker(DefaultInactivityTimeout, nil, testLogger())

	err := w.onSessionComplete(context.Background(), "s1")
	require.NoError(t, err)
}

func TestOnSessionComplete_GetSessionError(t *testing.T) {
	store := &mockMessageStore{
		getErr: fmt.Errorf("session not found"),
	}

	w := &EvalWorker{
		resultWriter: &mockResultWriter{},
		messageStore: store,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
	}
	w.completionTracker = NewCompletionTracker(DefaultInactivityTimeout, nil, testLogger())

	err := w.onSessionComplete(context.Background(), "s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestOnSessionComplete_GetMessagesError(t *testing.T) {
	packLoader := newTestPackLoader([]runtimeevals.EvalDef{
		containsEvalDef("e1", runtimeevals.TriggerOnSessionComplete, "hello"),
	})

	store := &mockMessageStore{
		sess: &session.Session{
			ID:                "s1",
			AgentName:         "test-agent",
			Namespace:         "ns",
			PromptPackName:    "test-pack",
			PromptPackVersion: "v1",
		},
		getMsgsErr: fmt.Errorf("redis error"),
	}

	w := &EvalWorker{
		resultWriter: &mockResultWriter{},
		messageStore: store,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
		packLoader:   packLoader,
	}
	w.completionTracker = NewCompletionTracker(DefaultInactivityTimeout, nil, testLogger())

	err := w.onSessionComplete(context.Background(), "s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis error")
}

func TestWriteResults_Empty(t *testing.T) {
	writer := &mockResultWriter{}
	w := &EvalWorker{
		resultWriter: writer,
		logger:       testLogger(),
	}

	err := w.writeResults(context.Background(), nil, "s1")
	require.NoError(t, err)
	assert.Empty(t, writer.written)
}

func TestWriteResults_Success(t *testing.T) {
	writer := &mockResultWriter{}
	w := &EvalWorker{
		resultWriter: writer,
		logger:       testLogger(),
	}

	results := []*api.EvalResult{{EvalID: "e1"}}
	err := w.writeResults(context.Background(), results, "s1")
	require.NoError(t, err)
	assert.Len(t, writer.written, 1)
}

func TestWriteResults_Error(t *testing.T) {
	writer := &mockResultWriter{writeErr: fmt.Errorf("db error")}
	w := &EvalWorker{
		resultWriter: writer,
		logger:       testLogger(),
	}

	results := []*api.EvalResult{{EvalID: "e1"}}
	err := w.writeResults(context.Background(), results, "s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestGetTracker_LazyInit(t *testing.T) {
	w := &EvalWorker{logger: testLogger()}
	tracker := w.getTracker()
	require.NotNil(t, tracker)
	assert.Equal(t, tracker, w.getTracker(), "should return the same tracker")
}

func TestGetRateLimiter_LazyInit(t *testing.T) {
	w := &EvalWorker{}
	rl := w.getRateLimiter()
	require.NotNil(t, rl)
	assert.Equal(t, rl, w.getRateLimiter(), "should return the same limiter")
}

func TestGetSDKRunner_LazyInit(t *testing.T) {
	w := &EvalWorker{}
	runner := w.getSDKRunner()
	require.NotNil(t, runner)
	assert.Equal(t, runner, w.getSDKRunner(), "should return the same runner")
}

func TestCountAssistantMessages(t *testing.T) {
	tests := []struct {
		name     string
		msgs     []session.Message
		expected int
	}{
		{"empty", nil, 0},
		{"no assistant", []session.Message{{Role: session.RoleUser}}, 0},
		{"one assistant", []session.Message{
			{Role: session.RoleUser},
			{Role: session.RoleAssistant},
		}, 1},
		{"two assistant", []session.Message{
			{Role: session.RoleUser},
			{Role: session.RoleAssistant},
			{Role: session.RoleUser},
			{Role: session.RoleAssistant},
		}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, countAssistantMessages(tt.msgs))
		})
	}
}

func TestNewEvalWorker_WithRateLimiter(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	rateLimiter := NewRateLimiter(nil)

	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespaces:   []string{"ns"},
		Logger:       testLogger(),
		RateLimiter:  rateLimiter,
	})

	assert.Equal(t, rateLimiter, w.rateLimiter)
}

func TestLoadPackEvals_WithPack(t *testing.T) {
	packLoader := newTestPackLoader([]runtimeevals.EvalDef{
		{ID: "e1", Type: "contains", Trigger: runtimeevals.TriggerEveryTurn},
	})

	w := &EvalWorker{
		packLoader: packLoader,
		logger:     testLogger(),
	}

	event := api.SessionEvent{
		SessionID:         "s1",
		Namespace:         "ns",
		PromptPackName:    "test-pack",
		PromptPackVersion: "v1",
	}

	result := w.loadPackEvals(context.Background(), event)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.PackData)
}

func TestLoadPackEvals_NoPackLoader(t *testing.T) {
	w := &EvalWorker{logger: testLogger()}

	event := api.SessionEvent{
		PromptPackName: "test-pack",
	}

	result := w.loadPackEvals(context.Background(), event)
	assert.Nil(t, result)
}

func TestLoadPackEvals_NoPackName(t *testing.T) {
	w := &EvalWorker{
		packLoader: &PromptPackLoader{},
		logger:     testLogger(),
	}

	event := api.SessionEvent{}
	result := w.loadPackEvals(context.Background(), event)
	assert.Nil(t, result)
}

func TestEnrichEvent(t *testing.T) {
	event := api.SessionEvent{SessionID: "s1"}
	packEvals := &CachedPack{
		PackName:    "my-pack",
		PackVersion: "v2",
	}

	enriched := enrichEvent(event, packEvals)
	assert.Equal(t, "my-pack", enriched.PromptPackName)
	assert.Equal(t, "v2", enriched.PromptPackVersion)
	assert.Equal(t, "s1", enriched.SessionID)
}

func TestToEvalResult_WithPromptPack(t *testing.T) {
	item := api.EvaluateResultItem{
		EvalID:   "e1",
		EvalType: "contains",
		Passed:   true,
	}
	event := api.SessionEvent{
		SessionID:         "s1",
		Namespace:         "ns",
		PromptPackName:    "pack-1",
		PromptPackVersion: "v1",
	}

	result := toEvalResult(item, event, "agent")
	assert.Equal(t, "pack-1", result.PromptPackName)
	assert.Equal(t, "v1", result.PromptPackVersion)
}

func TestToEvalResult_WithDetails(t *testing.T) {
	details := json.RawMessage(`{"explanation":"Too informal","error":"threshold exceeded"}`)
	item := api.EvaluateResultItem{
		EvalID:   "e1",
		EvalType: "llm_judge",
		Passed:   false,
		Details:  details,
	}
	event := api.SessionEvent{SessionID: "s1", Namespace: "ns"}

	result := toEvalResult(item, event, "agent")
	require.NotNil(t, result.Details)
	assert.JSONEq(t, `{"explanation":"Too informal","error":"threshold exceeded"}`, string(result.Details))
}

func TestNewEvalWorker_WithPackLoader(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	packLoader := NewPromptPackLoader(fakeClient)

	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespaces:   []string{"ns"},
		Logger:       testLogger(),
		PackLoader:   packLoader,
	})

	assert.Equal(t, packLoader, w.packLoader)
}

func TestProcessAssistantMessage_WithPackEvals(t *testing.T) {
	packLoader := newTestPackLoader([]runtimeevals.EvalDef{
		containsEvalDef("e1", runtimeevals.TriggerEveryTurn, "hello"),
		{ID: "e2", Type: "regex", Trigger: runtimeevals.TriggerEveryTurn, Params: map[string]any{"pattern": "world"}},
	})

	msgs := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "test"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hello world"},
	}

	writer := &mockResultWriter{}
	store := &mockMessageStore{
		sess:     &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
		messages: toMessagePtrs(msgs),
	}

	w := &EvalWorker{
		resultWriter: writer,
		messageStore: store,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
		packLoader:   packLoader,
	}

	event := api.SessionEvent{
		EventType:         eventTypeMessage,
		SessionID:         "s1",
		AgentName:         "test-agent",
		Namespace:         "ns",
		MessageID:         "m2",
		MessageRole:       "assistant",
		PromptPackName:    "test-pack",
		PromptPackVersion: "v1",
	}

	err := w.processAssistantMessage(context.Background(), event)
	require.NoError(t, err)
	assert.NotEmpty(t, writer.written)
}

func TestProcessAssistantMessage_NoPackName_SkipsEvals(t *testing.T) {
	writer := &mockResultWriter{}
	store := &mockMessageStore{
		sess: &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
	}

	w := &EvalWorker{
		resultWriter: writer,
		messageStore: store,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
		packLoader:   newTestPackLoader([]runtimeevals.EvalDef{}),
	}

	event := api.SessionEvent{
		EventType:   eventTypeMessage,
		SessionID:   "s1",
		AgentName:   "test-agent",
		Namespace:   "ns",
		MessageID:   "m2",
		MessageRole: "assistant",
		// No PromptPackName — should skip.
	}

	err := w.processAssistantMessage(context.Background(), event)
	require.NoError(t, err)
	assert.Empty(t, writer.written)
}

func TestOnSessionComplete_WithPackEvals(t *testing.T) {
	packLoader := newTestPackLoader([]runtimeevals.EvalDef{
		containsEvalDef("e1", runtimeevals.TriggerOnSessionComplete, "hello"),
	})

	msgs := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "test"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hello"},
	}

	writer := &mockResultWriter{}
	store := &mockMessageStore{
		sess: &session.Session{
			ID:                "s1",
			AgentName:         "test-agent",
			Namespace:         "ns",
			PromptPackName:    "test-pack",
			PromptPackVersion: "v1",
		},
		messages: toMessagePtrs(msgs),
	}

	w := &EvalWorker{
		resultWriter: writer,
		messageStore: store,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
		packLoader:   packLoader,
	}
	w.completionTracker = NewCompletionTracker(DefaultInactivityTimeout, nil, testLogger())

	err := w.onSessionComplete(context.Background(), "s1")
	require.NoError(t, err)
	assert.NotEmpty(t, writer.written)
}

func TestNewEvalWorker_MultiNamespace(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespaces:   []string{"ns1", "ns2"},
		Logger:       testLogger(),
	})

	assert.Equal(t, []string{"ns1", "ns2"}, w.Namespaces())
	assert.Len(t, w.StreamKeys(), 2)
	assert.Equal(t, consumerGroupPrefix+"cluster", w.ConsumerGroup())
}

func TestNewEvalWorker_SingleNamespace(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespaces:   []string{"ns"},
		Logger:       testLogger(),
	})

	assert.Equal(t, []string{"ns"}, w.Namespaces())
	assert.Len(t, w.StreamKeys(), 1)
	assert.Equal(t, consumerGroupPrefix+"ns", w.ConsumerGroup())
}

func TestNewEvalWorker_BackwardCompat_DeprecatedNamespace(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespace:    "legacy-ns",
		Logger:       testLogger(),
	})

	assert.Equal(t, []string{"legacy-ns"}, w.Namespaces())
}

func TestNewEvalWorker_NamespacesOverridesNamespace(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespaces:   []string{"ns1"},
		Namespace:    "ignored",
		Logger:       testLogger(),
	})

	assert.Equal(t, []string{"ns1"}, w.Namespaces())
}

func TestStart_CreatesConsumerGroupsOnAllStreams(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	w := NewEvalWorker(WorkerConfig{
		RedisClient:  client,
		ResultWriter: &mockResultWriter{},
		Namespaces:   []string{"ns1", "ns2"},
		Logger:       testLogger(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not shut down")
	}

	// Verify both streams have the consumer group.
	for _, key := range w.StreamKeys() {
		groups, err := client.XInfoGroups(context.Background(), key).Result()
		require.NoError(t, err)
		require.Len(t, groups, 1, "expected consumer group on %s", key)
	}
}

func TestRepeatedGt(t *testing.T) {
	assert.Equal(t, []string{">", ">", ">"}, repeatedGt(3))
	assert.Equal(t, []string{">"}, repeatedGt(1))
}

func TestBuildConsumerGroup(t *testing.T) {
	assert.Equal(t, consumerGroupPrefix+"cluster", buildConsumerGroup([]string{"a", "b"}))
	assert.Equal(t, consumerGroupPrefix+"a", buildConsumerGroup([]string{"a"}))
	assert.Equal(t, consumerGroupPrefix+"default", buildConsumerGroup(nil))
}

func TestBuildStreamKeys(t *testing.T) {
	keys := buildStreamKeys([]string{"ns1", "ns2"})
	assert.Len(t, keys, 2)
}

func TestResolveNamespaces(t *testing.T) {
	tests := []struct {
		name     string
		config   WorkerConfig
		expected []string
	}{
		{"namespaces set", WorkerConfig{Namespaces: []string{"a", "b"}}, []string{"a", "b"}},
		{"fallback to namespace", WorkerConfig{Namespace: "x"}, []string{"x"}},
		{"empty", WorkerConfig{}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveNamespaces(tt.config))
		})
	}
}

func TestConvertToEvalResults(t *testing.T) {
	w := &EvalWorker{logger: testLogger()}

	score := 0.9
	items := []api.EvaluateResultItem{
		{EvalID: "e1", EvalType: "contains", Passed: true, Score: &score, DurationMs: 5, Source: evalSource},
		{EvalID: "e2", EvalType: "regex", Passed: false, DurationMs: 3, Source: evalSource},
	}
	event := api.SessionEvent{
		SessionID:         "s1",
		MessageID:         "m1",
		Namespace:         "ns",
		PromptPackName:    "pack",
		PromptPackVersion: "v1",
	}

	results := w.convertToEvalResults(items, event, "agent")
	require.Len(t, results, 2)
	assert.Equal(t, "e1", results[0].EvalID)
	assert.True(t, results[0].Passed)
	assert.Equal(t, "e2", results[1].EvalID)
	assert.False(t, results[1].Passed)
}

func TestGetMessages(t *testing.T) {
	msgs := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "hello"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hi"},
	}

	store := &mockMessageStore{
		sess:     &session.Session{ID: "s1"},
		messages: toMessagePtrs(msgs),
	}

	w := &EvalWorker{messageStore: store, logger: testLogger()}

	result, err := w.getMessages(context.Background(), "s1")
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "m1", result[0].ID)
	assert.Equal(t, "m2", result[1].ID)
}

func TestGetMessages_Error(t *testing.T) {
	store := &mockMessageStore{
		getMsgsErr: fmt.Errorf("redis down"),
	}

	w := &EvalWorker{messageStore: store, logger: testLogger()}

	_, err := w.getMessages(context.Background(), "s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis down")
}

// TestProviderResolver_Nil verifies that resolveProviders returns nil when no resolver is configured.
func TestProviderResolver_Nil(t *testing.T) {
	w := &EvalWorker{logger: testLogger()}

	specs := w.resolveProviders(context.Background(), api.SessionEvent{
		AgentName: "agent",
		Namespace: "ns",
	})
	assert.Nil(t, specs)
}

// TestProviderResolverWithFakeK8s verifies provider resolution with a fake K8s client.
func TestProviderResolverWithFakeK8s(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	w := &EvalWorker{
		providerResolver: NewProviderResolver(fakeClient),
		logger:           testLogger(),
	}

	// Resolution fails because no AgentRuntime exists, but should not panic.
	specs := w.resolveProviders(context.Background(), api.SessionEvent{
		AgentName: "nonexistent",
		Namespace: "ns",
	})
	assert.Nil(t, specs)
}

func TestEvalCollector_DelegatesToSDKRunner(t *testing.T) {
	collector := sdkmetrics.NewEvalOnlyCollector(sdkmetrics.CollectorOpts{
		Registerer: prometheus.NewRegistry(),
		Namespace:  "omnia_eval",
	})
	runner := NewSDKRunner(WithEvalCollector(collector))
	w := &EvalWorker{
		sdkRunner: runner,
		logger:    testLogger(),
	}

	assert.Equal(t, collector, w.EvalCollector())
}

func TestEvalCollector_NilWhenRunnerHasNone(t *testing.T) {
	runner := NewSDKRunner()
	w := &EvalWorker{
		sdkRunner: runner,
		logger:    testLogger(),
	}

	assert.Nil(t, w.EvalCollector())
}

func TestProcessEvent_EvaluateRequest_RunsAllEvals(t *testing.T) {
	packLoader := newTestPackLoader([]runtimeevals.EvalDef{
		containsEvalDef("e1", runtimeevals.TriggerEveryTurn, "hello"),
		containsEvalDef("e2", runtimeevals.TriggerOnSessionComplete, "bye"),
	})
	rw := &mockResultWriter{}
	store := &mockMessageStore{
		sess: &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
		messages: toMessagePtrs([]session.Message{
			{ID: "m1", Role: session.RoleUser, Content: "hi"},
			{ID: "m2", Role: session.RoleAssistant, Content: "hello and bye"},
		}),
	}

	w := &EvalWorker{
		messageStore: store,
		resultWriter: rw,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
		packLoader:   packLoader,
	}

	event := api.SessionEvent{
		EventType:      eventTypeEvaluate,
		SessionID:      "s1",
		AgentName:      "test-agent",
		Namespace:      "ns",
		PromptPackName: "test-pack",
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)

	// Verify results were written with source "manual".
	require.NotEmpty(t, rw.written)
	for _, r := range rw.written {
		assert.Equal(t, "manual", r.Source, "evaluate results should have source=manual")
	}
}

func TestProcessEvent_EvaluateRequest_NoPack(t *testing.T) {
	rw := &mockResultWriter{}
	store := &mockMessageStore{
		sess: &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
	}

	w := &EvalWorker{
		messageStore: store,
		resultWriter: rw,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
	}

	event := api.SessionEvent{
		EventType: eventTypeEvaluate,
		SessionID: "s1",
		AgentName: "test-agent",
		Namespace: "ns",
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)
	assert.Empty(t, rw.written, "no evals should run without a pack")
}

func TestIsEvaluateEvent(t *testing.T) {
	assert.True(t, isEvaluateEvent(api.SessionEvent{EventType: eventTypeEvaluate}))
	assert.False(t, isEvaluateEvent(api.SessionEvent{EventType: eventTypeMessage}))
	assert.False(t, isEvaluateEvent(api.SessionEvent{EventType: eventTypeSessionDone}))
}

func TestReportStreamLag(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	streamKey := "omnia:session-events:ns"
	group := "omnia-eval-workers-ns"

	// Create consumer group, add messages, then read (without ACK) to create pending entries.
	client.XGroupCreateMkStream(context.Background(), streamKey, group, "0")
	for i := 0; i < 3; i++ {
		client.XAdd(context.Background(), &goredis.XAddArgs{
			Stream: streamKey,
			Values: map[string]any{streamPayloadField: `{"eventType":"message.assistant"}`},
		})
	}
	// Read messages to make them pending (not ACKed).
	_, err = client.XReadGroup(context.Background(), &goredis.XReadGroupArgs{
		Group:    group,
		Consumer: "test-consumer",
		Streams:  []string{streamKey, ">"},
		Count:    10,
		Block:    time.Second,
	}).Result()
	require.NoError(t, err)

	spy := &spyMetrics{}
	w := &EvalWorker{
		redisClient:   client,
		streamKeys:    []string{streamKey},
		consumerGroup: group,
		logger:        testLogger(),
		metrics:       spy,
	}

	w.reportStreamLag(context.Background())

	require.Len(t, spy.streamLag, 1)
	assert.Equal(t, streamKey, spy.streamLag[0].stream)
	assert.Equal(t, float64(3), spy.streamLag[0].lag)
}

func TestReportStreamLag_NoMessages(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	streamKey := "omnia:session-events:ns"
	group := "omnia-eval-workers-ns"
	client.XGroupCreateMkStream(context.Background(), streamKey, group, "0")

	spy := &spyMetrics{}
	w := &EvalWorker{
		redisClient:   client,
		streamKeys:    []string{streamKey},
		consumerGroup: group,
		logger:        testLogger(),
		metrics:       spy,
	}

	w.reportStreamLag(context.Background())

	require.Len(t, spy.streamLag, 1)
	assert.Equal(t, float64(0), spy.streamLag[0].lag)
}

func TestProcessAssistantMessage_RecordsEvalMetrics(t *testing.T) {
	packLoader := newTestPackLoader([]runtimeevals.EvalDef{
		containsEvalDef("e1", runtimeevals.TriggerEveryTurn, "hello"),
	})

	spy := &spyMetrics{}
	rw := &mockResultWriter{}
	store := &mockMessageStore{
		sess: &session.Session{ID: "s1", AgentName: "test-agent", Namespace: "ns"},
		messages: toMessagePtrs([]session.Message{
			{ID: "m1", Role: session.RoleUser, Content: "say hello"},
			{ID: "m2", Role: session.RoleAssistant, Content: "hello world"},
		}),
	}

	runner := NewSDKRunner(WithMetrics(spy))
	w := &EvalWorker{
		messageStore: store,
		resultWriter: rw,
		namespaces:   []string{"ns"},
		logger:       testLogger(),
		packLoader:   packLoader,
		sdkRunner:    runner,
		metrics:      spy,
	}

	event := api.SessionEvent{
		EventType:         eventTypeMessage,
		SessionID:         "s1",
		AgentName:         "test-agent",
		Namespace:         "ns",
		MessageID:         "m2",
		MessageRole:       "assistant",
		PromptPackName:    "test-pack",
		PromptPackVersion: "v1",
	}

	err := w.processEvent(context.Background(), event)
	require.NoError(t, err)

	// Verify eval execution metrics were recorded.
	require.Len(t, spy.evalExecuted, 1)
	assert.Equal(t, "contains", spy.evalExecuted[0].evalType)
	assert.Equal(t, "every_turn", spy.evalExecuted[0].trigger)
	assert.Equal(t, MetricStatusSuccess, spy.evalExecuted[0].status)

	// Verify sampling decision was recorded.
	require.Len(t, spy.samplingDecision, 1)
	assert.Equal(t, "contains", spy.samplingDecision[0].evalType)
	assert.Equal(t, MetricStatusSampled, spy.samplingDecision[0].decision)

	// Verify results were also written.
	require.NotEmpty(t, rw.written)
}

// Ensure unused import suppressors compile.
var _ = v1alpha1.GroupVersion
