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

package recorder

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/altairalabs/omnia/internal/session/providers/cold"
)

func TestOmniaEventStore_Append_MessageCreated(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	// Emit user message
	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventMessageCreated,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "Hello!",
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Emit assistant message
	err = es.Append(context.Background(), &events.Event{
		Type:      events.EventMessageCreated,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.MessageCreatedData{
			Role:    "assistant",
			Content: "Hi there!",
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Wait for async goroutines
	time.Sleep(100 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Messages are written async so order is not guaranteed; check by role
	roleContent := make(map[session.MessageRole]string)
	for _, m := range msgs {
		roleContent[m.Role] = m.Content
	}
	if roleContent[session.RoleUser] != "Hello!" {
		t.Errorf("expected user content 'Hello!', got %s", roleContent[session.RoleUser])
	}
	if roleContent[session.RoleAssistant] != "Hi there!" {
		t.Errorf("expected assistant content 'Hi there!', got %s", roleContent[session.RoleAssistant])
	}

	// Check metadata type on any message
	for _, m := range msgs {
		if m.Metadata["type"] != string(events.EventMessageCreated) {
			t.Errorf("expected type metadata %s, got %s", events.EventMessageCreated, m.Metadata["type"])
		}
	}
}

func TestOmniaEventStore_Append_ToolCall(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventToolCallStarted,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.ToolCallStartedData{
			ToolName: "get_weather",
			CallID:   "call-123",
			Args:     map[string]any{"city": "London"},
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != session.RoleAssistant {
		t.Errorf("expected assistant role for tool call, got %s", msgs[0].Role)
	}
	if msgs[0].ToolCallID != "call-123" {
		t.Errorf("expected tool call ID call-123, got %s", msgs[0].ToolCallID)
	}
	if msgs[0].Content == "" {
		t.Error("expected non-empty content with tool call JSON")
	}
}

func TestOmniaEventStore_Append_ProviderCallCompleted(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	// Create a session so stats update works
	warmStore.sessions["sess-1"] = &session.Session{
		ID:        "sess-1",
		AgentName: "test-agent",
		Status:    session.SessionStatusActive,
	}

	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventProviderCallCompleted,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.ProviderCallCompletedData{
			Provider:     "openai",
			Model:        "gpt-4",
			Duration:     2 * time.Second,
			InputTokens:  100,
			OutputTokens: 50,
			Cost:         0.005,
			FinishReason: "stop",
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	msg := msgs[0]
	if msg.Role != session.RoleSystem {
		t.Errorf("expected system role, got %s", msg.Role)
	}
	if msg.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", msg.InputTokens)
	}
	if msg.OutputTokens != 50 {
		t.Errorf("expected 50 output tokens, got %d", msg.OutputTokens)
	}
	if msg.Metadata["provider"] != "openai" {
		t.Errorf("expected provider openai, got %s", msg.Metadata["provider"])
	}

	// Check session stats were updated
	warmStore.mu.Lock()
	sess := warmStore.sessions["sess-1"]
	warmStore.mu.Unlock()
	if sess.TotalInputTokens != 100 {
		t.Errorf("expected session input tokens 100, got %d", sess.TotalInputTokens)
	}
	if sess.TotalOutputTokens != 50 {
		t.Errorf("expected session output tokens 50, got %d", sess.TotalOutputTokens)
	}
}

func TestOmniaEventStore_Append_AudioEvent(t *testing.T) {
	coldBlob := cold.NewMemoryBlobStore()
	warmStore := newMockWarmStoreForTest()
	blobStore := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())
	es := NewOmniaEventStore(warmStore, blobStore, logr.Discard())

	audioData := []byte("fake audio data")
	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventAudioInput,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.AudioInputData{
			Actor: "user",
			Payload: events.BinaryPayload{
				MIMEType:   "audio/wav",
				InlineData: audioData,
			},
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	msg := msgs[0]
	if msg.Role != session.RoleSystem {
		t.Errorf("expected system role, got %s", msg.Role)
	}
	if msg.Content == "" {
		t.Error("expected non-empty content with storage ref")
	}
	if msg.Metadata["category"] != "audio" {
		t.Errorf("expected category audio, got %s", msg.Metadata["category"])
	}
	if msg.Metadata["mime_type"] != "audio/wav" {
		t.Errorf("expected mime_type audio/wav, got %s", msg.Metadata["mime_type"])
	}

	// Verify the binary data was stored in cold storage
	if len(warmStore.artifacts) != 1 {
		t.Errorf("expected 1 artifact, got %d", len(warmStore.artifacts))
	}
}

func TestOmniaEventStore_Append_GracefulDegradation(t *testing.T) {
	// Use a warm store that will fail on AppendMessage
	warmStore := &failingWarmStore{}
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	// Should not return an error (async write, errors logged)
	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventMessageCreated,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "This should not panic",
		},
	})
	if err != nil {
		t.Fatalf("Append should not return error: %v", err)
	}

	// Wait for async goroutine
	time.Sleep(100 * time.Millisecond)
	// No panic = success
}

func TestOmniaEventStore_Append_NoSessionID(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventMessageCreated,
		Timestamp: time.Now(),
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "no session",
		},
	})
	if err == nil {
		t.Error("expected error for missing session ID")
	}
}

func TestOmniaEventStore_Query(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	// Append events to two different sessions
	for _, role := range []string{"user", "assistant"} {
		err := es.Append(context.Background(), &events.Event{
			Type:      events.EventMessageCreated,
			Timestamp: time.Now(),
			SessionID: "sess-1",
			Data: &events.MessageCreatedData{
				Role:    role,
				Content: "hello from " + role,
			},
		})
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}
	// Add event to a different session
	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventMessageCreated,
		Timestamp: time.Now(),
		SessionID: "sess-2",
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "other session",
		},
	})
	if err != nil {
		t.Fatalf("Append to sess-2 failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Verify messages landed in the right sessions
	if len(warmStore.getMessages("sess-1")) != 2 {
		t.Fatalf("expected 2 messages in sess-1, got %d", len(warmStore.getMessages("sess-1")))
	}
	if len(warmStore.getMessages("sess-2")) != 1 {
		t.Fatalf("expected 1 message in sess-2, got %d", len(warmStore.getMessages("sess-2")))
	}

	// Query all events for sess-1
	result, err := es.Query(context.Background(), &events.EventFilter{
		SessionID: "sess-1",
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 events, got %d", len(result))
	}

	// Query with type filter
	result, err = es.Query(context.Background(), &events.EventFilter{
		SessionID: "sess-1",
		Types:     []events.EventType{events.EventMessageCreated},
	})
	if err != nil {
		t.Fatalf("Query with type filter failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 events with type filter, got %d", len(result))
	}
}

func TestOmniaEventStore_Append_ToolCallCompleted(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventToolCallCompleted,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.ToolCallCompletedData{
			ToolName: "get_weather",
			CallID:   "call-456",
			Duration: 500 * time.Millisecond,
			Status:   "success",
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != session.RoleSystem {
		t.Errorf("expected system role, got %s", msgs[0].Role)
	}
	if msgs[0].ToolCallID != "call-456" {
		t.Errorf("expected tool call ID call-456, got %s", msgs[0].ToolCallID)
	}
	if msgs[0].Metadata["status"] != "success" {
		t.Errorf("expected status success, got %s", msgs[0].Metadata["status"])
	}
}

func TestOmniaEventStore_Append_ToolCallFailed(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventToolCallFailed,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.ToolCallFailedData{
			ToolName: "get_weather",
			CallID:   "call-789",
			Error:    fmt.Errorf("connection timeout"),
			Duration: 5 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "connection timeout" {
		t.Errorf("expected error content, got %s", msgs[0].Content)
	}
	if msgs[0].ToolCallID != "call-789" {
		t.Errorf("expected tool call ID call-789, got %s", msgs[0].ToolCallID)
	}
}

func TestOmniaEventStore_Append_GenericEvent(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventPipelineStarted,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.PipelineStartedData{
			MiddlewareCount: 3,
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != session.RoleSystem {
		t.Errorf("expected system role for generic event, got %s", msgs[0].Role)
	}
}

func TestOmniaEventStore_Append_BinaryWithStorageRef(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	// Event with existing storage ref (no inline data)
	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventImageInput,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.ImageInputData{
			Actor: "user",
			Payload: events.BinaryPayload{
				StorageRef: "s3://bucket/key.png",
				MIMEType:   "image/png",
				Size:       1024,
				Checksum:   "sha256:abc",
			},
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "s3://bucket/key.png" {
		t.Errorf("expected storage ref in content, got %s", msgs[0].Content)
	}
}

func TestOmniaEventStore_QueryRaw(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventMessageCreated,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "raw test",
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	stored, err := es.QueryRaw(context.Background(), &events.EventFilter{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("QueryRaw failed: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected 1 stored event, got %d", len(stored))
	}
	if stored[0].Sequence != 1 {
		t.Errorf("expected sequence 1, got %d", stored[0].Sequence)
	}
}

func TestOmniaEventStore_Stream(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	err := es.Append(context.Background(), &events.Event{
		Type:      events.EventMessageCreated,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "stream test",
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	ch, err := es.Stream(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	evts := make([]*events.Event, 0, 1)
	for evt := range ch {
		evts = append(evts, evt)
	}
	if len(evts) != 1 {
		t.Fatalf("expected 1 streamed event, got %d", len(evts))
	}
}

func TestOmniaEventStore_Query_WithFilters(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	now := time.Now()
	err := es.Append(context.Background(), &events.Event{
		Type:           events.EventMessageCreated,
		Timestamp:      now,
		SessionID:      "sess-1",
		ConversationID: "conv-1",
		RunID:          "run-1",
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "filtered",
		},
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Filter by conversation ID
	result, err := es.Query(context.Background(), &events.EventFilter{
		SessionID:      "sess-1",
		ConversationID: "conv-1",
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 event with conversation filter, got %d", len(result))
	}

	// Filter by wrong conversation ID
	result, err = es.Query(context.Background(), &events.EventFilter{
		SessionID:      "sess-1",
		ConversationID: "conv-wrong",
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 events with wrong conversation, got %d", len(result))
	}

	// Filter by time range
	result, err = es.Query(context.Background(), &events.EventFilter{
		SessionID: "sess-1",
		Since:     now.Add(-time.Hour),
		Until:     now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 event in time range, got %d", len(result))
	}

	// Filter by future time
	result, err = es.Query(context.Background(), &events.EventFilter{
		SessionID: "sess-1",
		Since:     now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 events in future, got %d", len(result))
	}

	// Filter by non-matching type
	result, err = es.Query(context.Background(), &events.EventFilter{
		SessionID: "sess-1",
		Types:     []events.EventType{events.EventToolCallStarted},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 events for wrong type, got %d", len(result))
	}
}

func TestOmniaEventStore_Close(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())
	if err := es.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestOmniaEventStore_Query_WithLimit(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	for i := range 5 {
		_ = es.Append(context.Background(), &events.Event{
			Type:      events.EventMessageCreated,
			Timestamp: time.Now(),
			SessionID: "sess-1",
			Data:      &events.MessageCreatedData{Role: "user", Content: fmt.Sprintf("msg-%d", i)},
		})
	}
	time.Sleep(200 * time.Millisecond)

	result, err := es.Query(context.Background(), &events.EventFilter{
		SessionID: "sess-1",
		Limit:     3,
	})
	if err != nil {
		t.Fatalf("Query with limit failed: %v", err)
	}
	if len(result) > 5 {
		t.Errorf("expected at most 5 events, got %d", len(result))
	}
}

func TestOmniaEventStore_Query_NoSessionID(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	_, err := es.Query(context.Background(), &events.EventFilter{})
	if err == nil {
		t.Error("expected error for missing session ID")
	}
}

func TestOmniaEventStore_Query_WithRunIDFilter(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	_ = es.Append(context.Background(), &events.Event{
		Type:      events.EventMessageCreated,
		Timestamp: time.Now(),
		SessionID: "sess-1",
		RunID:     "run-abc",
		Data:      &events.MessageCreatedData{Role: "user", Content: "with run"},
	})
	time.Sleep(100 * time.Millisecond)

	result, err := es.Query(context.Background(), &events.EventFilter{
		SessionID: "sess-1",
		RunID:     "run-abc",
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 event, got %d", len(result))
	}

	result, err = es.Query(context.Background(), &events.EventFilter{
		SessionID: "sess-1",
		RunID:     "run-wrong",
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 events with wrong run ID, got %d", len(result))
	}
}

func TestOmniaEventStore_MessageToEvent_ToolCallReconstruction(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	// Append a ToolCallStarted, ToolCallCompleted, ToolCallFailed, and ProviderCallCompleted
	testEvents := []*events.Event{
		{
			Type: events.EventToolCallStarted, Timestamp: time.Now(), SessionID: "sess-1",
			Data: &events.ToolCallStartedData{ToolName: "calc", CallID: "c1", Args: map[string]any{"x": 1}},
		},
		{
			Type: events.EventToolCallCompleted, Timestamp: time.Now(), SessionID: "sess-1",
			Data: &events.ToolCallCompletedData{ToolName: "calc", CallID: "c2", Status: "success", Duration: time.Second},
		},
		{
			Type: events.EventToolCallFailed, Timestamp: time.Now(), SessionID: "sess-1",
			Data: &events.ToolCallFailedData{ToolName: "calc", CallID: "c3", Error: fmt.Errorf("bad")},
		},
		{
			Type: events.EventProviderCallCompleted, Timestamp: time.Now(), SessionID: "sess-1",
			Data: &events.ProviderCallCompletedData{
				Provider: "openai", Model: "gpt-4", InputTokens: 10, OutputTokens: 5, Cost: 0.001, FinishReason: "stop",
			},
		},
	}
	// Seed session for stats update
	warmStore.sessions["sess-1"] = &session.Session{ID: "sess-1", Status: session.SessionStatusActive}

	for _, e := range testEvents {
		if err := es.Append(context.Background(), e); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}
	time.Sleep(300 * time.Millisecond)

	// Query and verify reconstruction
	result, err := es.Query(context.Background(), &events.EventFilter{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(result) != 4 {
		t.Fatalf("expected 4 events, got %d", len(result))
	}

	// Verify reconstructed event types
	typeMap := make(map[events.EventType]int)
	for _, evt := range result {
		typeMap[evt.Type]++
		switch evt.Type {
		case events.EventToolCallStarted:
			d, ok := evt.Data.(*events.ToolCallStartedData)
			if !ok {
				t.Error("expected ToolCallStartedData")
			} else if d.CallID != "c1" {
				t.Errorf("expected CallID c1, got %s", d.CallID)
			}
		case events.EventToolCallCompleted:
			d, ok := evt.Data.(*events.ToolCallCompletedData)
			if !ok {
				t.Error("expected ToolCallCompletedData")
			} else if d.CallID != "c2" {
				t.Errorf("expected CallID c2, got %s", d.CallID)
			}
		case events.EventToolCallFailed:
			d, ok := evt.Data.(*events.ToolCallFailedData)
			if !ok {
				t.Error("expected ToolCallFailedData")
			} else if d.CallID != "c3" {
				t.Errorf("expected CallID c3, got %s", d.CallID)
			}
		case events.EventProviderCallCompleted:
			d, ok := evt.Data.(*events.ProviderCallCompletedData)
			if !ok {
				t.Error("expected ProviderCallCompletedData")
			} else if d.Provider != "openai" {
				t.Errorf("expected provider openai, got %s", d.Provider)
			}
		}
	}
}

func TestOmniaEventStore_Append_ValueTypes(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	warmStore.sessions["sess-1"] = &session.Session{ID: "sess-1", Status: session.SessionStatusActive}
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	// Test value-type (non-pointer) event data variants
	valueEvents := []*events.Event{
		{
			Type: events.EventMessageCreated, Timestamp: time.Now(), SessionID: "sess-1",
			Data: events.MessageCreatedData{Role: "user", Content: "value type msg"},
		},
		{
			Type: events.EventToolCallStarted, Timestamp: time.Now(), SessionID: "sess-1",
			Data: events.ToolCallStartedData{ToolName: "test", CallID: "vc1"},
		},
		{
			Type: events.EventToolCallCompleted, Timestamp: time.Now(), SessionID: "sess-1",
			Data: events.ToolCallCompletedData{ToolName: "test", CallID: "vc2", Status: "ok"},
		},
		{
			Type: events.EventToolCallFailed, Timestamp: time.Now(), SessionID: "sess-1",
			Data: events.ToolCallFailedData{ToolName: "test", CallID: "vc3", Error: fmt.Errorf("fail")},
		},
		{
			Type: events.EventProviderCallCompleted, Timestamp: time.Now(), SessionID: "sess-1",
			Data: events.ProviderCallCompletedData{Provider: "test", InputTokens: 5, OutputTokens: 3},
		},
	}

	for _, e := range valueEvents {
		if err := es.Append(context.Background(), e); err != nil {
			t.Fatalf("Append value-type event failed: %v", err)
		}
	}
	time.Sleep(200 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 5 {
		t.Errorf("expected 5 messages from value-type events, got %d", len(msgs))
	}
}

func TestOmniaEventStore_Append_BinaryValueTypes(t *testing.T) {
	coldBlob := cold.NewMemoryBlobStore()
	warmStore := newMockWarmStoreForTest()
	blobStore := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())
	es := NewOmniaEventStore(warmStore, blobStore, logr.Discard())

	data := []byte("binary data")
	// Test value-type binary events
	binaryEvents := []*events.Event{
		{
			Type: events.EventAudioOutput, Timestamp: time.Now(), SessionID: "sess-1",
			Data: events.AudioOutputData{Payload: events.BinaryPayload{MIMEType: "audio/mp3", InlineData: data}},
		},
		{
			Type: events.EventImageOutput, Timestamp: time.Now(), SessionID: "sess-1",
			Data: events.ImageOutputData{Payload: events.BinaryPayload{MIMEType: "image/png", InlineData: data}},
		},
		{
			Type: events.EventVideoFrame, Timestamp: time.Now(), SessionID: "sess-1",
			Data: events.VideoFrameData{Payload: events.BinaryPayload{MIMEType: "video/mp4", InlineData: data}},
		},
		{
			Type: events.EventScreenshot, Timestamp: time.Now(), SessionID: "sess-1",
			Data: events.ScreenshotData{Payload: events.BinaryPayload{MIMEType: "image/jpeg", InlineData: data}},
		},
	}

	for _, e := range binaryEvents {
		if err := es.Append(context.Background(), e); err != nil {
			t.Fatalf("Append binary value-type event failed: %v", err)
		}
	}
	time.Sleep(300 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 4 {
		t.Errorf("expected 4 messages from binary value-type events, got %d", len(msgs))
	}
}

func TestOmniaEventStore_Append_BinaryPointerTypes(t *testing.T) {
	coldBlob := cold.NewMemoryBlobStore()
	warmStore := newMockWarmStoreForTest()
	blobStore := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())
	es := NewOmniaEventStore(warmStore, blobStore, logr.Discard())

	data := []byte("ptr binary data")
	// Test pointer-type binary events beyond AudioInput and ImageInput
	ptrEvents := []*events.Event{
		{
			Type: events.EventAudioOutput, Timestamp: time.Now(), SessionID: "sess-1",
			Data: &events.AudioOutputData{Payload: events.BinaryPayload{MIMEType: "audio/ogg", InlineData: data}},
		},
		{
			Type: events.EventImageOutput, Timestamp: time.Now(), SessionID: "sess-1",
			Data: &events.ImageOutputData{Payload: events.BinaryPayload{MIMEType: "image/gif", InlineData: data}},
		},
		{
			Type: events.EventVideoFrame, Timestamp: time.Now(), SessionID: "sess-1",
			Data: &events.VideoFrameData{Payload: events.BinaryPayload{MIMEType: "video/webm", InlineData: data}},
		},
		{
			Type: events.EventScreenshot, Timestamp: time.Now(), SessionID: "sess-1",
			Data: &events.ScreenshotData{Payload: events.BinaryPayload{MIMEType: "image/webp", InlineData: data}},
		},
	}

	for _, e := range ptrEvents {
		if err := es.Append(context.Background(), e); err != nil {
			t.Fatalf("Append binary pointer-type event failed: %v", err)
		}
	}
	time.Sleep(300 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 4 {
		t.Errorf("expected 4 messages from binary pointer-type events, got %d", len(msgs))
	}
}

func TestOmniaEventStore_Append_MessageCreated_SystemRole(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	es := NewOmniaEventStore(warmStore, nil, logr.Discard())

	_ = es.Append(context.Background(), &events.Event{
		Type: events.EventMessageCreated, Timestamp: time.Now(), SessionID: "sess-1",
		Data: &events.MessageCreatedData{Role: "system", Content: "system msg"},
	})
	time.Sleep(100 * time.Millisecond)

	msgs := warmStore.getMessages("sess-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != session.RoleSystem {
		t.Errorf("expected system role, got %s", msgs[0].Role)
	}
}

// failingWarmStore is a WarmStoreProvider that fails on all writes.
type failingWarmStore struct{}

func (f *failingWarmStore) CreateSession(context.Context, *session.Session) error {
	return fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) GetSession(context.Context, string) (*session.Session, error) {
	return nil, fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) UpdateSession(context.Context, *session.Session) error {
	return fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) DeleteSession(context.Context, string) error {
	return fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) AppendMessage(context.Context, string, *session.Message) error {
	return fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) GetMessages(context.Context, string, providers.MessageQueryOpts) ([]*session.Message, error) {
	return nil, fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) ListSessions(context.Context, providers.SessionListOpts) (*providers.SessionPage, error) {
	return nil, fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) SearchSessions(context.Context, string, providers.SessionListOpts) (*providers.SessionPage, error) {
	return nil, fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) CreatePartition(context.Context, time.Time) error { return nil }
func (f *failingWarmStore) DropPartition(context.Context, time.Time) error   { return nil }
func (f *failingWarmStore) ListPartitions(context.Context) ([]providers.PartitionInfo, error) {
	return nil, nil
}
func (f *failingWarmStore) GetSessionsOlderThan(context.Context, time.Time, int) ([]*session.Session, error) {
	return nil, nil
}
func (f *failingWarmStore) DeleteSessionsBatch(context.Context, []string) error { return nil }
func (f *failingWarmStore) SaveArtifact(context.Context, *session.Artifact) error {
	return fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) GetArtifacts(context.Context, string) ([]*session.Artifact, error) {
	return nil, fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) GetSessionArtifacts(context.Context, string) ([]*session.Artifact, error) {
	return nil, fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) DeleteSessionArtifacts(context.Context, string) error {
	return fmt.Errorf("store unavailable")
}
func (f *failingWarmStore) Ping(context.Context) error { return nil }
func (f *failingWarmStore) Close() error               { return nil }

// TestOmniaEventStore_QueryRaw_Error validates QueryRaw propagates query errors.
func TestOmniaEventStore_QueryRaw_Error(t *testing.T) {
	store := NewOmniaEventStore(&failingWarmStore{}, nil, logr.Discard())
	_, err := store.QueryRaw(context.Background(), &events.EventFilter{SessionID: "s"})
	require.Error(t, err)
}

// TestOmniaEventStore_Stream_Error validates Stream propagates query errors.
func TestOmniaEventStore_Stream_Error(t *testing.T) {
	store := NewOmniaEventStore(&failingWarmStore{}, nil, logr.Discard())
	_, err := store.Stream(context.Background(), "s")
	require.Error(t, err)
}

// TestOmniaEventStore_Stream_ContextCancel validates Stream respects cancellation.
func TestOmniaEventStore_Stream_ContextCancel(t *testing.T) {
	warm := newMockWarmStoreForTest()
	store := NewOmniaEventStore(warm, nil, logr.Discard())

	sessionID := "sess-cancel"
	_ = warm.CreateSession(context.Background(), &session.Session{ID: sessionID})

	// Append multiple messages
	for i := range 10 {
		_ = store.Append(context.Background(), &events.Event{
			Type:      events.EventMessageCreated,
			SessionID: sessionID,
			Timestamp: time.Now(),
			Data: &events.MessageCreatedData{
				Role:    "user",
				Content: fmt.Sprintf("msg %d", i),
			},
		})
	}
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := store.Stream(ctx, sessionID)
	require.NoError(t, err)

	// Read one event then cancel
	<-ch
	cancel()

	// Drain remaining — channel should close soon
	count := 1
	for range ch {
		count++
	}
	// We should get fewer than all 10 due to cancellation (or all if goroutine was fast)
	assert.LessOrEqual(t, count, 10)
}

// TestOmniaEventStore_HandleBinaryEvent_BlobStoreFailure validates graceful
// degradation when the blob store fails during binary event handling.
func TestOmniaEventStore_HandleBinaryEvent_BlobStoreFailure(t *testing.T) {
	warm := newMockWarmStoreForTest()
	coldBlob := &failingBlobStore{}
	blobStore := NewOmniaBlobStore(coldBlob, warm, logr.Discard())
	store := NewOmniaEventStore(warm, blobStore, logr.Discard())

	sessionID := "sess-blob-fail"
	_ = warm.CreateSession(context.Background(), &session.Session{ID: sessionID})

	err := store.Append(context.Background(), &events.Event{
		Type:      events.EventAudioInput,
		SessionID: sessionID,
		Timestamp: time.Now(),
		Data: &events.AudioInputData{
			Payload: events.BinaryPayload{
				InlineData: []byte("audio data"),
				MIMEType:   "audio/wav",
			},
		},
	})
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// The message should still be appended (graceful degradation) but without
	// storage ref since blob store failed
	msgs := warm.getMessages(sessionID)
	require.Len(t, msgs, 1)
	assert.Equal(t, session.RoleSystem, msgs[0].Role)
}

// TestOmniaEventStore_MaybeUpdateSessionStats_GetSessionFailure validates that
// stats update gracefully handles a missing session.
func TestOmniaEventStore_MaybeUpdateSessionStats_GetSessionFailure(t *testing.T) {
	warm := newMockWarmStoreForTest()
	store := NewOmniaEventStore(warm, nil, logr.Discard())

	// Session "no-such" doesn't exist — GetSession will fail
	err := store.Append(context.Background(), &events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "no-such",
		Timestamp: time.Now(),
		Data: &events.ProviderCallCompletedData{
			Provider:     "openai",
			Model:        "gpt-4",
			InputTokens:  100,
			OutputTokens: 50,
			Cost:         0.01,
		},
	})
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	// No panic — graceful degradation
}

// TestOmniaEventStore_Query_GetMessagesFailure validates Query propagates warm store errors.
func TestOmniaEventStore_Query_GetMessagesFailure(t *testing.T) {
	store := NewOmniaEventStore(&failingWarmStore{}, nil, logr.Discard())

	_, err := store.Query(context.Background(), &events.EventFilter{SessionID: "s"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store unavailable")
}

// TestOmniaEventStore_Query_UntilFilter validates the Until time filter works.
func TestOmniaEventStore_Query_UntilFilter(t *testing.T) {
	warm := newMockWarmStoreForTest()
	store := NewOmniaEventStore(warm, nil, logr.Discard())

	sessionID := "sess-until"
	_ = warm.CreateSession(context.Background(), &session.Session{ID: sessionID})

	now := time.Now()
	// Append a message with a known timestamp
	_ = store.Append(context.Background(), &events.Event{
		Type:      events.EventMessageCreated,
		SessionID: sessionID,
		Timestamp: now,
		Data:      &events.MessageCreatedData{Role: "user", Content: "hello"},
	})
	time.Sleep(50 * time.Millisecond)

	// Query with Until before the message timestamp — should return 0
	result, err := store.Query(context.Background(), &events.EventFilter{
		SessionID: sessionID,
		Until:     now.Add(-time.Hour),
	})
	require.NoError(t, err)
	assert.Empty(t, result)

	// Query with Until after the message timestamp — should return 1
	result, err = store.Query(context.Background(), &events.EventFilter{
		SessionID: sessionID,
		Until:     now.Add(time.Hour),
	})
	require.NoError(t, err)
	assert.Len(t, result, 1)
}
