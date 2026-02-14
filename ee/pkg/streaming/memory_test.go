/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package streaming

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestMemoryPublisher_Publish(t *testing.T) {
	pub := NewMemoryPublisher()

	event := &SessionEvent{
		EventID:   "evt-1",
		EventType: EventTypeSessionCreated,
		Timestamp: time.Now(),
		SessionID: "sess-abc",
	}

	err := pub.Publish(context.Background(), event)
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	events := pub.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventID != "evt-1" {
		t.Errorf("expected eventId evt-1, got %s", events[0].EventID)
	}
}

func TestMemoryPublisher_PublishNilEvent(t *testing.T) {
	pub := NewMemoryPublisher()

	err := pub.Publish(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil event")
	}
}

func TestMemoryPublisher_PublishAfterClose(t *testing.T) {
	pub := NewMemoryPublisher()

	if err := pub.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	err := pub.Publish(context.Background(), &SessionEvent{})
	if err == nil {
		t.Fatal("expected error after close")
	}
	if err.Error() != errMsgPublisherClosed {
		t.Errorf("expected error %q, got %q", errMsgPublisherClosed, err.Error())
	}
}

func TestMemoryPublisher_PublishBatch(t *testing.T) {
	pub := NewMemoryPublisher()

	events := []*SessionEvent{
		{EventID: "evt-1", EventType: EventTypeSessionCreated, SessionID: "sess-1"},
		{EventID: "evt-2", EventType: EventTypeMessageAdded, SessionID: "sess-1"},
		nil, // nil events should be skipped
		{EventID: "evt-3", EventType: EventTypeSessionCompleted, SessionID: "sess-1"},
	}

	err := pub.PublishBatch(context.Background(), events)
	if err != nil {
		t.Fatalf("PublishBatch returned error: %v", err)
	}

	stored := pub.Events()
	if len(stored) != 3 {
		t.Fatalf("expected 3 events (nil skipped), got %d", len(stored))
	}
}

func TestMemoryPublisher_PublishBatchAfterClose(t *testing.T) {
	pub := NewMemoryPublisher()
	_ = pub.Close()

	err := pub.PublishBatch(context.Background(), []*SessionEvent{{EventID: "evt-1"}})
	if err == nil {
		t.Fatal("expected error after close")
	}
}

func TestMemoryPublisher_Reset(t *testing.T) {
	pub := NewMemoryPublisher()

	_ = pub.Publish(context.Background(), &SessionEvent{EventID: "evt-1"})
	_ = pub.Publish(context.Background(), &SessionEvent{EventID: "evt-2"})

	if len(pub.Events()) != 2 {
		t.Fatalf("expected 2 events before reset")
	}

	pub.Reset()

	if len(pub.Events()) != 0 {
		t.Fatalf("expected 0 events after reset, got %d", len(pub.Events()))
	}
}

func TestMemoryPublisher_EventsCopiesSlice(t *testing.T) {
	pub := NewMemoryPublisher()
	_ = pub.Publish(context.Background(), &SessionEvent{EventID: "evt-1"})

	events := pub.Events()
	events[0] = nil // modifying the copy should not affect the publisher

	stored := pub.Events()
	if stored[0] == nil {
		t.Error("modifying Events() return value should not affect publisher state")
	}
}

func TestMemoryPublisher_ConcurrentAccess(t *testing.T) {
	pub := NewMemoryPublisher()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event := &SessionEvent{
				EventID:   "evt-" + time.Now().String(),
				EventType: EventTypeMessageAdded,
				SessionID: "sess-1",
			}
			_ = pub.Publish(ctx, event)
		}()
	}
	wg.Wait()

	events := pub.Events()
	if len(events) != 100 {
		t.Errorf("expected 100 events, got %d", len(events))
	}
}

func TestMemoryPublisher_ImplementsInterface(t *testing.T) {
	var _ StreamingPublisher = &MemoryPublisher{}
}

func TestSessionEvent_JSONRoundTrip(t *testing.T) {
	event := &SessionEvent{
		EventID:     "evt-1",
		EventType:   EventTypeToolExecuted,
		Timestamp:   time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		SessionID:   "sess-abc",
		WorkspaceID: "ws-1",
		AgentID:     "agent-1",
		Namespace:   "default",
		Payload:     json.RawMessage(`{"tool":"search","result":"ok"}`),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var decoded SessionEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	if decoded.EventID != event.EventID {
		t.Errorf("eventId mismatch: %s != %s", decoded.EventID, event.EventID)
	}
	if decoded.EventType != EventTypeToolExecuted {
		t.Errorf("eventType mismatch: %s != %s", decoded.EventType, EventTypeToolExecuted)
	}
	if decoded.SessionID != event.SessionID {
		t.Errorf("sessionId mismatch: %s != %s", decoded.SessionID, event.SessionID)
	}
	if string(decoded.Payload) != string(event.Payload) {
		t.Errorf("payload mismatch: %s != %s", decoded.Payload, event.Payload)
	}
}

func TestSessionEvent_JSONOmitsEmptyFields(t *testing.T) {
	event := &SessionEvent{
		EventID:   "evt-1",
		EventType: EventTypeError,
		Timestamp: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		SessionID: "sess-abc",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	for _, key := range []string{"workspaceId", "agentId", "namespace", "payload"} {
		if _, ok := raw[key]; ok {
			t.Errorf("expected %s to be omitted from JSON", key)
		}
	}
}

func TestDurationFromMs(t *testing.T) {
	d := durationFromMs(100)
	if d != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", d)
	}

	d = durationFromMs(0)
	if d != 0 {
		t.Errorf("expected 0, got %v", d)
	}
}

func TestEventTypeConstants(t *testing.T) {
	// Verify constants are defined with expected values
	constants := map[string]string{
		"session_created":   EventTypeSessionCreated,
		"message_added":     EventTypeMessageAdded,
		"session_completed": EventTypeSessionCompleted,
		"tool_executed":     EventTypeToolExecuted,
		"error":             EventTypeError,
	}

	for expected, got := range constants {
		if got != expected {
			t.Errorf("expected constant value %q, got %q", expected, got)
		}
	}
}
