/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package streaming

import (
	"context"
	"fmt"
	"testing"
)

// MockJetStreamPublisher implements JetStreamPublisher for testing.
type MockJetStreamPublisher struct {
	PublishFunc func(ctx context.Context, subject string, data []byte) error

	PublishCalls []mockJetStreamPublishCall
}

type mockJetStreamPublishCall struct {
	Subject string
	Data    []byte
}

func (m *MockJetStreamPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	m.PublishCalls = append(m.PublishCalls, mockJetStreamPublishCall{
		Subject: subject,
		Data:    data,
	})
	if m.PublishFunc != nil {
		return m.PublishFunc(ctx, subject, data)
	}
	return nil
}

func TestNewNATSPublisher(t *testing.T) {
	js := &MockJetStreamPublisher{}
	config := NATSPublisherConfig{
		URL:     "nats://localhost:4222",
		Stream:  "test-stream",
		Subject: "events.session",
	}
	pub := NewNATSPublisher(js, config)
	if pub.config.URL != "nats://localhost:4222" {
		t.Errorf("expected URL 'nats://localhost:4222', got %q", pub.config.URL)
	}
	if pub.config.Stream != "test-stream" {
		t.Errorf("expected Stream 'test-stream', got %q", pub.config.Stream)
	}
	if pub.config.Subject != "events.session" {
		t.Errorf("expected Subject 'events.session', got %q", pub.config.Subject)
	}
}

func TestNATSPublisher_Publish_Success(t *testing.T) {
	js := &MockJetStreamPublisher{}
	pub := NewNATSPublisher(js, NATSPublisherConfig{
		URL:     "nats://localhost:4222",
		Stream:  "test-stream",
		Subject: "events.session",
	})

	event := newTestEvent("evt-1", "sess-1")
	err := pub.Publish(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(js.PublishCalls) != 1 {
		t.Fatalf("expected 1 Publish call, got %d", len(js.PublishCalls))
	}

	call := js.PublishCalls[0]
	if call.Subject != "events.session" {
		t.Errorf("expected subject 'events.session', got %q", call.Subject)
	}
}

func TestNATSPublisher_Publish_NilEvent(t *testing.T) {
	js := &MockJetStreamPublisher{}
	pub := NewNATSPublisher(js, NATSPublisherConfig{
		Subject: "events.session",
	})

	err := pub.Publish(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil event")
	}
}

func TestNATSPublisher_Publish_JetStreamError(t *testing.T) {
	js := &MockJetStreamPublisher{
		PublishFunc: func(_ context.Context, _ string, _ []byte) error {
			return fmt.Errorf("jetstream error")
		},
	}
	pub := NewNATSPublisher(js, NATSPublisherConfig{
		Subject: "events.session",
	})

	err := pub.Publish(context.Background(), newTestEvent("evt-1", "sess-1"))
	if err == nil {
		t.Fatal("expected error from JetStream")
	}
}

func TestNATSPublisher_PublishBatch_Success(t *testing.T) {
	js := &MockJetStreamPublisher{}
	pub := NewNATSPublisher(js, NATSPublisherConfig{
		Subject: "events.session",
	})

	events := []*SessionEvent{
		newTestEvent("evt-1", "sess-1"),
		newTestEvent("evt-2", "sess-2"),
	}

	err := pub.PublishBatch(context.Background(), events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(js.PublishCalls) != 2 {
		t.Errorf("expected 2 Publish calls, got %d", len(js.PublishCalls))
	}
}

func TestNATSPublisher_PublishBatch_Empty(t *testing.T) {
	js := &MockJetStreamPublisher{}
	pub := NewNATSPublisher(js, NATSPublisherConfig{
		Subject: "events.session",
	})

	err := pub.PublishBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(js.PublishCalls) != 0 {
		t.Errorf("expected no Publish calls, got %d", len(js.PublishCalls))
	}
}

func TestNATSPublisher_PublishBatch_ErrorMidBatch(t *testing.T) {
	callCount := 0
	js := &MockJetStreamPublisher{
		PublishFunc: func(_ context.Context, _ string, _ []byte) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("publish error on second call")
			}
			return nil
		},
	}
	pub := NewNATSPublisher(js, NATSPublisherConfig{
		Subject: "events.session",
	})

	events := []*SessionEvent{
		newTestEvent("evt-1", "sess-1"),
		newTestEvent("evt-2", "sess-2"),
		newTestEvent("evt-3", "sess-3"),
	}

	err := pub.PublishBatch(context.Background(), events)
	if err == nil {
		t.Fatal("expected error from batch publish")
	}
}

func TestNATSPublisher_PublishBatch_NilEvent(t *testing.T) {
	js := &MockJetStreamPublisher{}
	pub := NewNATSPublisher(js, NATSPublisherConfig{
		Subject: "events.session",
	})

	events := []*SessionEvent{newTestEvent("evt-1", "sess-1"), nil}
	err := pub.PublishBatch(context.Background(), events)
	if err == nil {
		t.Fatal("expected error for nil event in batch")
	}
}

func TestNATSPublisher_Close(t *testing.T) {
	js := &MockJetStreamPublisher{}
	pub := NewNATSPublisher(js, NATSPublisherConfig{
		Subject: "events.session",
	})

	err := pub.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
