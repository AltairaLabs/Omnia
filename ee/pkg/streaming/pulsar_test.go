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

// MockPulsarProducer implements PulsarProducer for testing.
type MockPulsarProducer struct {
	SendFunc  func(ctx context.Context, payload []byte) error
	CloseFunc func() error

	SendCalls  [][]byte
	CloseCalls int
}

func (m *MockPulsarProducer) Send(ctx context.Context, payload []byte) error {
	m.SendCalls = append(m.SendCalls, payload)
	if m.SendFunc != nil {
		return m.SendFunc(ctx, payload)
	}
	return nil
}

func (m *MockPulsarProducer) Close() error {
	m.CloseCalls++
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func TestNewPulsarPublisher(t *testing.T) {
	producer := &MockPulsarProducer{}
	config := PulsarPublisherConfig{
		ServiceURL: "pulsar://localhost:6650",
		Topic:      "test-topic",
	}
	pub := NewPulsarPublisher(producer, config)
	if pub.config.ServiceURL != "pulsar://localhost:6650" {
		t.Errorf("expected ServiceURL 'pulsar://localhost:6650', got %q", pub.config.ServiceURL)
	}
	if pub.config.Topic != "test-topic" {
		t.Errorf("expected Topic 'test-topic', got %q", pub.config.Topic)
	}
}

func TestPulsarPublisher_Publish_Success(t *testing.T) {
	producer := &MockPulsarProducer{}
	pub := NewPulsarPublisher(producer, PulsarPublisherConfig{
		ServiceURL: "pulsar://localhost:6650",
		Topic:      "test-topic",
	})

	event := newTestEvent("evt-1", "sess-1")
	err := pub.Publish(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(producer.SendCalls) != 1 {
		t.Fatalf("expected 1 Send call, got %d", len(producer.SendCalls))
	}
}

func TestPulsarPublisher_Publish_NilEvent(t *testing.T) {
	producer := &MockPulsarProducer{}
	pub := NewPulsarPublisher(producer, PulsarPublisherConfig{
		Topic: "test-topic",
	})

	err := pub.Publish(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil event")
	}
}

func TestPulsarPublisher_Publish_SendError(t *testing.T) {
	producer := &MockPulsarProducer{
		SendFunc: func(_ context.Context, _ []byte) error {
			return fmt.Errorf("send error")
		},
	}
	pub := NewPulsarPublisher(producer, PulsarPublisherConfig{
		Topic: "test-topic",
	})

	err := pub.Publish(context.Background(), newTestEvent("evt-1", "sess-1"))
	if err == nil {
		t.Fatal("expected error from producer")
	}
}

func TestPulsarPublisher_PublishBatch_Success(t *testing.T) {
	producer := &MockPulsarProducer{}
	pub := NewPulsarPublisher(producer, PulsarPublisherConfig{
		Topic: "test-topic",
	})

	events := []*SessionEvent{
		newTestEvent("evt-1", "sess-1"),
		newTestEvent("evt-2", "sess-2"),
		newTestEvent("evt-3", "sess-3"),
	}

	err := pub.PublishBatch(context.Background(), events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(producer.SendCalls) != 3 {
		t.Errorf("expected 3 Send calls, got %d", len(producer.SendCalls))
	}
}

func TestPulsarPublisher_PublishBatch_Empty(t *testing.T) {
	producer := &MockPulsarProducer{}
	pub := NewPulsarPublisher(producer, PulsarPublisherConfig{
		Topic: "test-topic",
	})

	err := pub.PublishBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(producer.SendCalls) != 0 {
		t.Errorf("expected no Send calls, got %d", len(producer.SendCalls))
	}
}

func TestPulsarPublisher_PublishBatch_ErrorMidBatch(t *testing.T) {
	callCount := 0
	producer := &MockPulsarProducer{
		SendFunc: func(_ context.Context, _ []byte) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("send error on second call")
			}
			return nil
		},
	}
	pub := NewPulsarPublisher(producer, PulsarPublisherConfig{
		Topic: "test-topic",
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

func TestPulsarPublisher_PublishBatch_NilEvent(t *testing.T) {
	producer := &MockPulsarProducer{}
	pub := NewPulsarPublisher(producer, PulsarPublisherConfig{
		Topic: "test-topic",
	})

	events := []*SessionEvent{newTestEvent("evt-1", "sess-1"), nil}
	err := pub.PublishBatch(context.Background(), events)
	if err == nil {
		t.Fatal("expected error for nil event in batch")
	}
}

func TestPulsarPublisher_Close_Success(t *testing.T) {
	producer := &MockPulsarProducer{}
	pub := NewPulsarPublisher(producer, PulsarPublisherConfig{
		Topic: "test-topic",
	})

	err := pub.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if producer.CloseCalls != 1 {
		t.Errorf("expected 1 Close call, got %d", producer.CloseCalls)
	}
}

func TestPulsarPublisher_Close_Error(t *testing.T) {
	producer := &MockPulsarProducer{
		CloseFunc: func() error {
			return fmt.Errorf("close error")
		},
	}
	pub := NewPulsarPublisher(producer, PulsarPublisherConfig{
		Topic: "test-topic",
	})

	err := pub.Close()
	if err == nil {
		t.Fatal("expected error from close")
	}
}
