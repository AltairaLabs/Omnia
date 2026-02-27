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

package otlp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// MockSessionWriter is a test double for SessionWriter.
type MockSessionWriter struct {
	sessions map[string]*session.Session
	messages map[string][]*session.Message
	stats    map[string]session.SessionStatsUpdate

	getSessionErr   error
	createErr       error
	appendErr       error
	updateStatsErr  error
	createCallCount int
}

func newMockWriter() *MockSessionWriter {
	return &MockSessionWriter{
		sessions: make(map[string]*session.Session),
		messages: make(map[string][]*session.Message),
		stats:    make(map[string]session.SessionStatsUpdate),
	}
}

func (m *MockSessionWriter) GetSession(_ context.Context, sessionID string) (*session.Session, error) {
	if m.getSessionErr != nil {
		return nil, m.getSessionErr
	}
	if s, ok := m.sessions[sessionID]; ok {
		return s, nil
	}
	return nil, session.ErrSessionNotFound
}

func (m *MockSessionWriter) CreateSession(_ context.Context, sess *session.Session) error {
	m.createCallCount++
	if m.createErr != nil {
		return m.createErr
	}
	m.sessions[sess.ID] = sess
	return nil
}

func (m *MockSessionWriter) AppendMessage(_ context.Context, sessionID string, msg *session.Message) error {
	if m.appendErr != nil {
		return m.appendErr
	}
	m.messages[sessionID] = append(m.messages[sessionID], msg)
	return nil
}

func (m *MockSessionWriter) UpdateSessionStats(_ context.Context, sessionID string, update session.SessionStatsUpdate) error {
	if m.updateStatsErr != nil {
		return m.updateStatsErr
	}
	m.stats[sessionID] = update
	return nil
}

// --- helpers for building OTLP test data ---

func makeSpan(conversationID string, startNano uint64, attrs []*commonpb.KeyValue) *tracepb.Span {
	if conversationID != "" {
		attrs = append([]*commonpb.KeyValue{
			{Key: AttrGenAIConversationID, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: conversationID}}},
		}, attrs...)
	}
	return &tracepb.Span{
		TraceId:           []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		SpanId:            []byte{0x01},
		StartTimeUnixNano: startNano,
		Attributes:        attrs,
	}
}

func makeMessageValue(role, content string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{
		KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{
			{Key: "role", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: role}}},
			{Key: "content", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: content}}},
		}},
	}}
}

func makeResourceSpans(namespace, agentName string, spans ...*tracepb.Span) *tracepb.ResourceSpans {
	return &tracepb.ResourceSpans{
		Resource: &resourcepb.Resource{
			Attributes: []*commonpb.KeyValue{
				{Key: AttrServiceNamespace, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: namespace}}},
				{Key: AttrServiceName, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: agentName}}},
			},
		},
		ScopeSpans: []*tracepb.ScopeSpans{
			{Spans: spans},
		},
	}
}

func outputMsgAttrs(msgs ...*commonpb.AnyValue) []*commonpb.KeyValue {
	return []*commonpb.KeyValue{
		{Key: AttrGenAIOutputMessages, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{
			ArrayValue: &commonpb.ArrayValue{Values: msgs},
		}}},
	}
}

func tokenAttrs(input, output int64) []*commonpb.KeyValue {
	return []*commonpb.KeyValue{
		{Key: AttrGenAIUsageInput, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: input}}},
		{Key: AttrGenAIUsageOutput, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: output}}},
	}
}

func combineAttrs(groups ...[]*commonpb.KeyValue) []*commonpb.KeyValue {
	var result []*commonpb.KeyValue
	for _, g := range groups {
		result = append(result, g...)
	}
	return result
}

// --- tests ---

func TestProcessExport_CurrentOTelFormat(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	attrs := combineAttrs(
		outputMsgAttrs(makeMessageValue("assistant", "Hello! How can I help?")),
		tokenAttrs(50, 20),
	)
	span := makeSpan("conv-123", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("default", "my-agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	sess := writer.sessions["conv-123"]
	require.NotNil(t, sess)
	assert.Equal(t, "my-agent", sess.AgentName)
	assert.Equal(t, "default", sess.Namespace)

	msgs := writer.messages["conv-123"]
	require.Len(t, msgs, 1)
	assert.Equal(t, session.RoleAssistant, msgs[0].Role)
	assert.Equal(t, "Hello! How can I help?", msgs[0].Content)

	stats := writer.stats["conv-123"]
	assert.Equal(t, int32(50), stats.AddInputTokens)
	assert.Equal(t, int32(20), stats.AddOutputTokens)
}

func TestProcessExport_LegacyOpenLLMetryFormat(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	attrs := []*commonpb.KeyValue{
		{Key: "gen_ai.system", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "openai"}}},
		{Key: "gen_ai.request.model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gpt-4"}}},
		{Key: "gen_ai.prompt.0.role", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "system"}}},
		{Key: "gen_ai.prompt.0.content", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "You are a mathematician."}}},
		{Key: "gen_ai.prompt.1.role", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "user"}}},
		{Key: "gen_ai.prompt.1.content", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "calculate 5 + 5"}}},
		{Key: "gen_ai.completion.0.role", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "assistant"}}},
		{Key: "gen_ai.completion.0.content", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "5 + 5 = 10"}}},
		{Key: "gen_ai.usage.prompt_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 125}}},
		{Key: "gen_ai.usage.completion_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 47}}},
	}

	span := makeSpan("conv-legacy", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("default", "langchain-agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	msgs := writer.messages["conv-legacy"]
	require.Len(t, msgs, 3)
	assert.Equal(t, session.RoleSystem, msgs[0].Role)
	assert.Equal(t, "You are a mathematician.", msgs[0].Content)
	assert.Equal(t, session.RoleUser, msgs[1].Role)
	assert.Equal(t, "calculate 5 + 5", msgs[1].Content)
	assert.Equal(t, session.RoleAssistant, msgs[2].Role)
	assert.Equal(t, "5 + 5 = 10", msgs[2].Content)

	// Model metadata on messages.
	assert.Equal(t, "gpt-4", msgs[0].Metadata["gen_ai.model"])

	// Deprecated token names should work.
	stats := writer.stats["conv-legacy"]
	assert.Equal(t, int32(125), stats.AddInputTokens)
	assert.Equal(t, int32(47), stats.AddOutputTokens)

	// Session state should contain provider.
	sess := writer.sessions["conv-legacy"]
	assert.Equal(t, "openai", sess.State["gen_ai.provider"])
}

func TestProcessExport_SpanEvents(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	// Build a span with messages in the event, not attributes.
	eventAttrs := combineAttrs(
		outputMsgAttrs(makeMessageValue("assistant", "From event")),
	)
	span := makeSpan("conv-events", uint64(time.Now().UnixNano()), nil)
	span.Events = []*tracepb.Span_Event{
		{
			Name:       "gen_ai.client.inference.operation.details",
			Attributes: eventAttrs,
		},
	}

	rs := makeResourceSpans("ns", "agent", span)
	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	msgs := writer.messages["conv-events"]
	require.Len(t, msgs, 1)
	assert.Equal(t, "From event", msgs[0].Content)
}

func TestProcessExport_SessionIDFallbackToTraceID(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	// Span with no conversation ID — should use trace ID.
	span := &tracepb.Span{
		TraceId:           []byte{0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89},
		SpanId:            []byte{0x01},
		StartTimeUnixNano: uint64(time.Now().UnixNano()),
		Attributes: []*commonpb.KeyValue{
			{Key: AttrGenAIRequestModel, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gpt-4"}}},
		},
	}

	rs := makeResourceSpans("ns", "agent", span)
	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	// Session should be created with hex trace ID.
	assert.Contains(t, writer.sessions, "abcdef0123456789")
}

func TestProcessExport_ExistingSession(t *testing.T) {
	writer := newMockWriter()
	writer.sessions["conv-123"] = &session.Session{ID: "conv-123"}

	transformer := NewTransformer(writer, logr.Discard())

	attrs := outputMsgAttrs(makeMessageValue("assistant", "Follow-up"))
	span := makeSpan("conv-123", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("default", "my-agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, writer.createCallCount, "should not create session again")
}

func TestProcessExport_SkipsSpanWithNoSessionID(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	// Span with no conversation ID and no trace ID.
	span := &tracepb.Span{
		SpanId:            []byte{0x01},
		StartTimeUnixNano: uint64(time.Now().UnixNano()),
		Attributes: []*commonpb.KeyValue{
			{Key: "http.method", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "GET"}}},
		},
	}

	rs := makeResourceSpans("default", "my-agent", span)
	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Empty(t, writer.sessions)
}

func TestProcessExport_NoTokenUsage(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	attrs := outputMsgAttrs(makeMessageValue("assistant", "response"))
	span := makeSpan("conv-456", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("default", "agent", span)

	_, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Empty(t, writer.stats)
}

func TestProcessExport_CreateSessionError(t *testing.T) {
	writer := newMockWriter()
	writer.createErr = errors.New("db error")

	transformer := NewTransformer(writer, logr.Discard())

	span := makeSpan("conv-err", uint64(time.Now().UnixNano()), nil)
	rs := makeResourceSpans("default", "agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	assert.Error(t, err)
	assert.Equal(t, 0, processed)
}

func TestProcessExport_AppendMessageError(t *testing.T) {
	writer := newMockWriter()
	writer.appendErr = errors.New("append failed")

	transformer := NewTransformer(writer, logr.Discard())

	attrs := outputMsgAttrs(makeMessageValue("assistant", "response"))
	span := makeSpan("conv-append-err", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("default", "agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	assert.Error(t, err)
	assert.Equal(t, 0, processed)
}

func TestProcessExport_UpdateStatsError(t *testing.T) {
	writer := newMockWriter()
	writer.updateStatsErr = errors.New("stats failed")

	transformer := NewTransformer(writer, logr.Discard())

	span := makeSpan("conv-stats-err", uint64(time.Now().UnixNano()), tokenAttrs(10, 5))
	rs := makeResourceSpans("default", "agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	assert.Error(t, err)
	assert.Equal(t, 0, processed)
}

func TestProcessExport_MultipleSpansSortedByTime(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	base := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	span2 := makeSpan("conv-sort", uint64(base.Add(2*time.Second).UnixNano()),
		outputMsgAttrs(makeMessageValue("assistant", "second")))
	span1 := makeSpan("conv-sort", uint64(base.Add(1*time.Second).UnixNano()),
		outputMsgAttrs(makeMessageValue("assistant", "first")))

	rs := makeResourceSpans("ns", "agent", span2, span1)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 2, processed)

	msgs := writer.messages["conv-sort"]
	require.Len(t, msgs, 2)
	assert.Equal(t, "first", msgs[0].Content)
	assert.Equal(t, "second", msgs[1].Content)
}

func TestProcessExport_NilResource(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	attrs := outputMsgAttrs(makeMessageValue("assistant", "hello"))
	span := makeSpan("conv-nil-res", uint64(time.Now().UnixNano()), attrs)

	rs := &tracepb.ResourceSpans{
		Resource: nil,
		ScopeSpans: []*tracepb.ScopeSpans{
			{Spans: []*tracepb.Span{span}},
		},
	}

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	sess := writer.sessions["conv-nil-res"]
	require.NotNil(t, sess)
	assert.Equal(t, "", sess.AgentName)
}

func TestProcessExport_EmptyResourceSpans(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	processed, err := transformer.ProcessExport(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, processed)
}

func TestProcessExport_EmptyOutputMessages(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	span := makeSpan("conv-no-msg", uint64(time.Now().UnixNano()), tokenAttrs(10, 5))
	rs := makeResourceSpans("ns", "agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Empty(t, writer.messages)
}

func TestProcessExport_GetSessionErr_NonNotFound(t *testing.T) {
	writer := newMockWriter()
	writer.getSessionErr = errors.New("db connection failed")

	transformer := NewTransformer(writer, logr.Discard())

	span := makeSpan("conv-db-err", uint64(time.Now().UnixNano()), nil)
	rs := makeResourceSpans("ns", "agent", span)

	_, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db connection failed")
}

func TestProcessExport_InputAndOutputMessages(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	inputMsgs := []*commonpb.AnyValue{
		makeMessageValue("user", "What is AI?"),
	}
	outputMsgs := []*commonpb.AnyValue{
		makeMessageValue("assistant", "AI is..."),
	}

	attrs := []*commonpb.KeyValue{
		{Key: AttrGenAIInputMessages, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{
			ArrayValue: &commonpb.ArrayValue{Values: inputMsgs},
		}}},
		{Key: AttrGenAIOutputMessages, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{
			ArrayValue: &commonpb.ArrayValue{Values: outputMsgs},
		}}},
	}

	span := makeSpan("conv-io", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("ns", "agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	msgs := writer.messages["conv-io"]
	require.Len(t, msgs, 2)
	assert.Equal(t, session.RoleUser, msgs[0].Role)
	assert.Equal(t, session.RoleAssistant, msgs[1].Role)
}

func TestSpanTimestamp(t *testing.T) {
	expected := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	span := &tracepb.Span{StartTimeUnixNano: uint64(expected.UnixNano())}

	result := spanTimestamp(span)
	assert.True(t, expected.Equal(result))
}

func TestSpanTimestamp_Zero(t *testing.T) {
	span := &tracepb.Span{StartTimeUnixNano: 0}
	result := spanTimestamp(span)
	assert.WithinDuration(t, time.Now(), result, 2*time.Second)
}

func TestExtractContentFromParts(t *testing.T) {
	// Test the new OTel "parts" format.
	partsValue := &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{
		ArrayValue: &commonpb.ArrayValue{Values: []*commonpb.AnyValue{
			{Value: &commonpb.AnyValue_KvlistValue{
				KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{
					{Key: "type", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "text"}}},
					{Key: "content", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "Hello from parts"}}},
				}},
			}},
		}},
	}}

	result := extractContentFromParts(partsValue)
	assert.Equal(t, "Hello from parts", result)
}

func TestExtractContentFromParts_Empty(t *testing.T) {
	assert.Equal(t, "", extractContentFromParts(nil))
	assert.Equal(t, "", extractContentFromParts(&commonpb.AnyValue{}))
}

func TestBuildSessionState(t *testing.T) {
	attrs := []*commonpb.KeyValue{
		{Key: AttrGenAIProviderName, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "openai"}}},
		{Key: AttrGenAIRequestModel, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gpt-4"}}},
	}

	state := buildSessionState(attrs)
	assert.Equal(t, "openai", state["gen_ai.provider"])
	assert.Equal(t, "gpt-4", state["gen_ai.model"])
}

func TestBuildSessionState_Empty(t *testing.T) {
	assert.Nil(t, buildSessionState(nil))
}

// --- tool & workflow span tests ---

func makeNamedSpan(name, conversationID string, startNano uint64, attrs []*commonpb.KeyValue) *tracepb.Span {
	if conversationID != "" {
		attrs = append([]*commonpb.KeyValue{
			{Key: AttrGenAIConversationID, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: conversationID}}},
		}, attrs...)
	}
	return &tracepb.Span{
		Name:              name,
		TraceId:           []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		SpanId:            []byte{0x01},
		StartTimeUnixNano: startNano,
		Attributes:        attrs,
	}
}

func toolAttrs(name, callID, args, status string, durationMs int64) []*commonpb.KeyValue {
	return []*commonpb.KeyValue{
		{Key: AttrToolName, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: name}}},
		{Key: AttrToolCallID, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: callID}}},
		{Key: AttrToolArgs, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: args}}},
		{Key: AttrToolStatus, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: status}}},
		{Key: AttrToolDurationMs, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: durationMs}}},
	}
}

func workflowTransitionAttrs(from, to, event, promptTask string) []*commonpb.KeyValue {
	return []*commonpb.KeyValue{
		{Key: AttrWorkflowFromState, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: from}}},
		{Key: AttrWorkflowToState, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: to}}},
		{Key: AttrWorkflowEvent, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: event}}},
		{Key: AttrWorkflowPromptTask, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: promptTask}}},
	}
}

func workflowCompletedAttrs(finalState string, transitionCount int64) []*commonpb.KeyValue {
	return []*commonpb.KeyValue{
		{Key: AttrWorkflowFinalState, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: finalState}}},
		{Key: AttrWorkflowTransitionCount, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: transitionCount}}},
	}
}

func TestProcessExport_ToolSpan(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	attrs := toolAttrs("search", "call-1", `{"query":"test"}`, "success", 150)
	span := makeNamedSpan("tool.search", "conv-tool", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("default", "agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	msgs := writer.messages["conv-tool"]
	require.Len(t, msgs, 1)
	msg := msgs[0]
	assert.Equal(t, session.RoleSystem, msg.Role)
	assert.Equal(t, "call-1", msg.ToolCallID)
	assert.Equal(t, "tool.call.completed", msg.Metadata["type"])
	assert.Equal(t, "search", msg.Metadata["tool_name"])
	assert.Equal(t, `{"query":"test"}`, msg.Metadata["tool_args"])
	assert.Equal(t, "success", msg.Metadata["status"])
	assert.Equal(t, "150", msg.Metadata["duration_ms"])
	assert.NotEmpty(t, msg.ID)
}

func TestProcessExport_ToolSpan_NoTokenUpdate(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	attrs := toolAttrs("calculator", "call-2", "{}", "error", 50)
	span := makeNamedSpan("tool.calculator", "conv-tool-no-tokens", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("default", "agent", span)

	_, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Empty(t, writer.stats, "tool spans should not trigger UpdateSessionStats")

	msgs := writer.messages["conv-tool-no-tokens"]
	require.Len(t, msgs, 1)
	assert.Equal(t, "error", msgs[0].Metadata["status"])
}

func TestProcessExport_WorkflowTransitionSpan(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	attrs := workflowTransitionAttrs("intake", "processing", "InfoComplete", "process")
	span := makeNamedSpan("workflow.transition", "conv-wf-trans", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("default", "agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	msgs := writer.messages["conv-wf-trans"]
	require.Len(t, msgs, 1)
	msg := msgs[0]
	assert.Equal(t, session.RoleSystem, msg.Role)
	assert.Equal(t, "workflow.transitioned", msg.Metadata["type"])
	assert.Equal(t, "intake", msg.Metadata["from_state"])
	assert.Equal(t, "processing", msg.Metadata["to_state"])
	assert.Equal(t, "InfoComplete", msg.Metadata["event"])
	assert.Equal(t, "process", msg.Metadata["prompt_task"])
}

func TestProcessExport_WorkflowCompletedSpan(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	attrs := workflowCompletedAttrs("done", 5)
	span := makeNamedSpan("workflow.completed", "conv-wf-done", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("default", "agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	msgs := writer.messages["conv-wf-done"]
	require.Len(t, msgs, 1)
	msg := msgs[0]
	assert.Equal(t, session.RoleSystem, msg.Role)
	assert.Equal(t, "workflow.completed", msg.Metadata["type"])
	assert.Equal(t, "done", msg.Metadata["final_state"])
	assert.Equal(t, "5", msg.Metadata["transition_count"])
}

func TestProcessExport_MixedSpans(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	base := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// GenAI span.
	genaiSpan := makeSpan("conv-mixed", uint64(base.UnixNano()),
		combineAttrs(
			outputMsgAttrs(makeMessageValue("assistant", "Hello")),
			tokenAttrs(10, 5),
		))

	// Tool span.
	toolSpan := makeNamedSpan("tool.search", "conv-mixed",
		uint64(base.Add(1*time.Second).UnixNano()),
		toolAttrs("search", "c1", "{}", "success", 100))

	// Workflow transition span.
	wfSpan := makeNamedSpan("workflow.transition", "conv-mixed",
		uint64(base.Add(2*time.Second).UnixNano()),
		workflowTransitionAttrs("s1", "s2", "Next", "task2"))

	// Workflow completed span.
	wfDoneSpan := makeNamedSpan("workflow.completed", "conv-mixed",
		uint64(base.Add(3*time.Second).UnixNano()),
		workflowCompletedAttrs("done", 3))

	rs := makeResourceSpans("ns", "agent", genaiSpan, toolSpan, wfSpan, wfDoneSpan)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 4, processed)

	msgs := writer.messages["conv-mixed"]
	require.Len(t, msgs, 4)
	// GenAI message.
	assert.Equal(t, session.RoleAssistant, msgs[0].Role)
	assert.Equal(t, "Hello", msgs[0].Content)
	// Tool message.
	assert.Equal(t, "tool.call.completed", msgs[1].Metadata["type"])
	assert.Equal(t, "search", msgs[1].Metadata["tool_name"])
	// Workflow transition.
	assert.Equal(t, "workflow.transitioned", msgs[2].Metadata["type"])
	// Workflow completed.
	assert.Equal(t, "workflow.completed", msgs[3].Metadata["type"])

	// Token stats should only come from the GenAI span.
	stats := writer.stats["conv-mixed"]
	assert.Equal(t, int32(10), stats.AddInputTokens)
	assert.Equal(t, int32(5), stats.AddOutputTokens)
}

func TestProcessExport_PromptPackAttributes(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	// Span attributes carry PromptPack info (as emitted by the facade).
	spanAttrs := combineAttrs(
		outputMsgAttrs(makeMessageValue("assistant", "Hi")),
		[]*commonpb.KeyValue{
			{Key: AttrOmniaPromptPackName, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "my-pack"}}},
			{Key: AttrOmniaPromptPackVersion, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "v2.1"}}},
		},
	)
	span := makeSpan("conv-pp", uint64(time.Now().UnixNano()), spanAttrs)
	rs := makeResourceSpans("default", "my-agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	sess := writer.sessions["conv-pp"]
	require.NotNil(t, sess)
	assert.Equal(t, "my-pack", sess.PromptPackName)
	assert.Equal(t, "v2.1", sess.PromptPackVersion)

	// State map should also include PromptPack attributes.
	assert.Equal(t, "my-pack", sess.State[AttrOmniaPromptPackName])
	assert.Equal(t, "v2.1", sess.State[AttrOmniaPromptPackVersion])
}

func TestProcessExport_PromptPackFromResourceAttrs(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())

	// PromptPack on resource attrs, not span attrs.
	span := makeSpan("conv-pp-res", uint64(time.Now().UnixNano()), nil)
	rs := &tracepb.ResourceSpans{
		Resource: &resourcepb.Resource{
			Attributes: []*commonpb.KeyValue{
				{Key: AttrServiceNamespace, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "ns"}}},
				{Key: AttrServiceName, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "agent"}}},
				{Key: AttrOmniaPromptPackName, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "res-pack"}}},
				{Key: AttrOmniaPromptPackVersion, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "v3"}}},
			},
		},
		ScopeSpans: []*tracepb.ScopeSpans{
			{Spans: []*tracepb.Span{span}},
		},
	}

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	sess := writer.sessions["conv-pp-res"]
	require.NotNil(t, sess)
	assert.Equal(t, "res-pack", sess.PromptPackName)
	assert.Equal(t, "v3", sess.PromptPackVersion)
}

func TestBuildSessionState_PromptPackAttributes(t *testing.T) {
	attrs := []*commonpb.KeyValue{
		{Key: AttrOmniaPromptPackName, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "pack-1"}}},
		{Key: AttrOmniaPromptPackVersion, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "v1"}}},
	}

	state := buildSessionState(attrs)
	assert.Equal(t, "pack-1", state[AttrOmniaPromptPackName])
	assert.Equal(t, "v1", state[AttrOmniaPromptPackVersion])
}

func TestProcessExport_ToolSpan_AppendError(t *testing.T) {
	writer := newMockWriter()
	writer.appendErr = errors.New("append failed")

	transformer := NewTransformer(writer, logr.Discard())

	attrs := toolAttrs("search", "call-1", "{}", "success", 100)
	span := makeNamedSpan("tool.search", "conv-tool-err", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("default", "agent", span)

	processed, err := transformer.ProcessExport(context.Background(), []*tracepb.ResourceSpans{rs})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "append failed")
	assert.Equal(t, 0, processed)
}
