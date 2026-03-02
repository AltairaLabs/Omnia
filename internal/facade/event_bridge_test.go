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

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session"
)

// mockSessionClient implements EventBridgeSessionClient for testing.
type mockSessionClient struct {
	mu             sync.Mutex
	messages       []session.Message
	statsUpdates   []session.SessionStatsUpdate
	appendErr      error
	updateStatsErr error
}

func (m *mockSessionClient) AppendMessage(_ context.Context, _ string, msg session.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.appendErr != nil {
		return m.appendErr
	}
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockSessionClient) UpdateSessionStats(_ context.Context, _ string, update session.SessionStatsUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateStatsErr != nil {
		return m.updateStatsErr
	}
	m.statsUpdates = append(m.statsUpdates, update)
	return nil
}

func (m *mockSessionClient) getMessages() []session.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]session.Message, len(m.messages))
	copy(result, m.messages)
	return result
}

func (m *mockSessionClient) getStatsUpdates() []session.SessionStatsUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]session.SessionStatsUpdate, len(m.statsUpdates))
	copy(result, m.statsUpdates)
	return result
}

func newTestLogger() logr.Logger {
	return logr.Discard()
}

func TestNewEventBridge(t *testing.T) {
	client := &mockSessionClient{}
	bridge := NewEventBridge(client, "test-agent", "default", newTestLogger())

	if bridge == nil {
		t.Fatal("expected non-nil bridge")
	}
	if bridge.agentName != "test-agent" {
		t.Errorf("agentName = %q, want %q", bridge.agentName, "test-agent")
	}
	if bridge.namespace != "default" {
		t.Errorf("namespace = %q, want %q", bridge.namespace, "default")
	}
	if bridge.IsEnabled() {
		t.Error("bridge should be disabled by default")
	}
}

func TestEventBridge_DisabledByDefault(t *testing.T) {
	client := &mockSessionClient{}
	bridge := NewEventBridge(client, "test-agent", "default", newTestLogger())

	event := EventBusEvent{
		Type:      EventTypeProviderCall,
		SessionID: "sess-1",
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{"model":"gpt-4"}`),
	}

	err := bridge.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("disabled bridge should not return error, got: %v", err)
	}

	messages := client.getMessages()
	if len(messages) != 0 {
		t.Errorf("disabled bridge should not forward events, got %d messages", len(messages))
	}
}

func TestEventBridge_SetEnabled(t *testing.T) {
	client := &mockSessionClient{}
	bridge := NewEventBridge(client, "test-agent", "default", newTestLogger())

	if bridge.IsEnabled() {
		t.Error("bridge should start disabled")
	}

	bridge.SetEnabled(true)
	if !bridge.IsEnabled() {
		t.Error("bridge should be enabled after SetEnabled(true)")
	}

	bridge.SetEnabled(false)
	if bridge.IsEnabled() {
		t.Error("bridge should be disabled after SetEnabled(false)")
	}
}

func TestEventBridge_HandleEvent_ForwardsToSessionClient(t *testing.T) {
	client := &mockSessionClient{}
	bridge := NewEventBridge(client, "test-agent", "default", newTestLogger())
	bridge.SetEnabled(true)

	now := time.Now()
	event := EventBusEvent{
		Type:      EventTypeProviderCall,
		SessionID: "sess-1",
		Timestamp: now,
		Data:      json.RawMessage(`{"model":"gpt-4","tokens":100}`),
	}

	err := bridge.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	messages := client.getMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	msg := messages[0]
	if msg.Role != session.RoleSystem {
		t.Errorf("role = %v, want system", msg.Role)
	}
	if msg.Content != `{"model":"gpt-4","tokens":100}` {
		t.Errorf("content = %q, want event data JSON", msg.Content)
	}
	if msg.Metadata["type"] != "event_bridge" {
		t.Errorf("metadata type = %q, want %q", msg.Metadata["type"], "event_bridge")
	}
	if msg.Metadata["event_type"] != EventTypeProviderCall {
		t.Errorf("metadata event_type = %q, want %q", msg.Metadata["event_type"], EventTypeProviderCall)
	}
	if msg.Metadata["agent"] != "test-agent" {
		t.Errorf("metadata agent = %q, want %q", msg.Metadata["agent"], "test-agent")
	}
	if msg.Metadata["namespace"] != "default" {
		t.Errorf("metadata namespace = %q, want %q", msg.Metadata["namespace"], "default")
	}
	if msg.ID == "" {
		t.Error("expected non-empty message ID")
	}
	if !msg.Timestamp.Equal(now) {
		t.Errorf("timestamp = %v, want %v", msg.Timestamp, now)
	}

	stats := client.getStatsUpdates()
	if len(stats) != 1 {
		t.Fatalf("expected 1 stats update, got %d", len(stats))
	}
	if stats[0].AddMessages != 1 {
		t.Errorf("AddMessages = %d, want 1", stats[0].AddMessages)
	}
}

func TestEventBridge_HandleEvent_DifferentEventTypes(t *testing.T) {
	eventTypes := []string{
		EventTypeProviderCall,
		EventTypeToolExecute,
		EventTypeValidation,
		EventTypeRecordingMessage,
	}

	for _, eventType := range eventTypes {
		t.Run(eventType, func(t *testing.T) {
			client := &mockSessionClient{}
			bridge := NewEventBridge(client, "agent", "ns", newTestLogger())
			bridge.SetEnabled(true)

			event := EventBusEvent{
				Type:      eventType,
				SessionID: "sess-1",
				Timestamp: time.Now(),
				Data:      json.RawMessage(`{"key":"value"}`),
			}

			err := bridge.HandleEvent(context.Background(), event)
			if err != nil {
				t.Fatalf("HandleEvent returned error for type %q: %v", eventType, err)
			}

			messages := client.getMessages()
			if len(messages) != 1 {
				t.Fatalf("expected 1 message for type %q, got %d", eventType, len(messages))
			}
			if messages[0].Metadata["event_type"] != eventType {
				t.Errorf("event_type = %q, want %q", messages[0].Metadata["event_type"], eventType)
			}
		})
	}
}

func TestEventBridge_HandleEvent_AppendMessageError(t *testing.T) {
	client := &mockSessionClient{
		appendErr: errors.New("connection refused"),
	}
	bridge := NewEventBridge(client, "agent", "ns", newTestLogger())
	bridge.SetEnabled(true)

	event := EventBusEvent{
		Type:      EventTypeProviderCall,
		SessionID: "sess-1",
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{}`),
	}

	err := bridge.HandleEvent(context.Background(), event)
	if err == nil {
		t.Fatal("expected error when AppendMessage fails")
	}
	if !errors.Is(err, client.appendErr) {
		t.Errorf("expected wrapped appendErr, got: %v", err)
	}
}

func TestEventBridge_HandleEvent_UpdateStatsError(t *testing.T) {
	client := &mockSessionClient{
		updateStatsErr: errors.New("timeout"),
	}
	bridge := NewEventBridge(client, "agent", "ns", newTestLogger())
	bridge.SetEnabled(true)

	event := EventBusEvent{
		Type:      EventTypeToolExecute,
		SessionID: "sess-1",
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{}`),
	}

	err := bridge.HandleEvent(context.Background(), event)
	if err == nil {
		t.Fatal("expected error when UpdateSessionStats fails")
	}
	if !errors.Is(err, client.updateStatsErr) {
		t.Errorf("expected wrapped updateStatsErr, got: %v", err)
	}

	// AppendMessage should still have succeeded
	messages := client.getMessages()
	if len(messages) != 1 {
		t.Errorf("expected 1 message even though stats update failed, got %d", len(messages))
	}
}

func TestEventBridge_HandleEvent_MissingSessionID(t *testing.T) {
	client := &mockSessionClient{}
	bridge := NewEventBridge(client, "agent", "ns", newTestLogger())
	bridge.SetEnabled(true)

	event := EventBusEvent{
		Type:      EventTypeProviderCall,
		SessionID: "",
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{}`),
	}

	err := bridge.HandleEvent(context.Background(), event)
	if err == nil {
		t.Fatal("expected error for missing session ID")
	}

	messages := client.getMessages()
	if len(messages) != 0 {
		t.Errorf("should not forward event with missing session ID, got %d messages", len(messages))
	}
}

func TestEventBridge_HandleEvent_EmptyData(t *testing.T) {
	client := &mockSessionClient{}
	bridge := NewEventBridge(client, "agent", "ns", newTestLogger())
	bridge.SetEnabled(true)

	event := EventBusEvent{
		Type:      EventTypeProviderCall,
		SessionID: "sess-1",
		Timestamp: time.Now(),
		Data:      nil,
	}

	err := bridge.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleEvent should handle empty data, got: %v", err)
	}

	messages := client.getMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Content != "{}" {
		t.Errorf("content = %q, want %q for nil data", messages[0].Content, "{}")
	}
}

func TestEventBridge_HandleEvent_ZeroTimestamp(t *testing.T) {
	client := &mockSessionClient{}
	bridge := NewEventBridge(client, "agent", "ns", newTestLogger())
	bridge.SetEnabled(true)

	before := time.Now()
	event := EventBusEvent{
		Type:      EventTypeValidation,
		SessionID: "sess-1",
		Timestamp: time.Time{}, // zero value
		Data:      json.RawMessage(`{"valid":true}`),
	}

	err := bridge.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}
	after := time.Now()

	messages := client.getMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	ts := messages[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v should be between %v and %v", ts, before, after)
	}
}

func TestEventBridge_HandleEvent_EnabledDisabledToggle(t *testing.T) {
	client := &mockSessionClient{}
	bridge := NewEventBridge(client, "agent", "ns", newTestLogger())

	event := EventBusEvent{
		Type:      EventTypeProviderCall,
		SessionID: "sess-1",
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{}`),
	}

	// Disabled: should not forward
	_ = bridge.HandleEvent(context.Background(), event)
	if len(client.getMessages()) != 0 {
		t.Error("disabled bridge should not forward events")
	}

	// Enable: should forward
	bridge.SetEnabled(true)
	_ = bridge.HandleEvent(context.Background(), event)
	if len(client.getMessages()) != 1 {
		t.Error("enabled bridge should forward events")
	}

	// Disable again: should not forward
	bridge.SetEnabled(false)
	_ = bridge.HandleEvent(context.Background(), event)
	if len(client.getMessages()) != 1 {
		t.Error("re-disabled bridge should not forward additional events")
	}
}

func TestEventBridge_ConcurrentAccess(t *testing.T) {
	client := &mockSessionClient{}
	bridge := NewEventBridge(client, "agent", "ns", newTestLogger())
	bridge.SetEnabled(true)

	var wg sync.WaitGroup
	eventCount := 20

	for i := 0; i < eventCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event := EventBusEvent{
				Type:      EventTypeProviderCall,
				SessionID: "sess-1",
				Timestamp: time.Now(),
				Data:      json.RawMessage(`{"concurrent":true}`),
			}
			_ = bridge.HandleEvent(context.Background(), event)
		}()
	}

	// Also toggle enabled concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			bridge.SetEnabled(true)
			bridge.IsEnabled()
		}
	}()

	wg.Wait()

	// Just verify no panics occurred and some messages were recorded
	messages := client.getMessages()
	if len(messages) == 0 {
		t.Error("expected at least some messages from concurrent calls")
	}
}
