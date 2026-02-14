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
	"fmt"
	"testing"
	"time"
)

// MockKinesisClient implements KinesisClient for testing.
type MockKinesisClient struct {
	PutRecordFunc  func(ctx context.Context, streamName string, data []byte, partitionKey string) error
	PutRecordsFunc func(ctx context.Context, streamName string, records []KinesisRecord) error

	PutRecordCalls  []mockPutRecordCall
	PutRecordsCalls []mockPutRecordsCall
}

type mockPutRecordCall struct {
	StreamName   string
	Data         []byte
	PartitionKey string
}

type mockPutRecordsCall struct {
	StreamName string
	Records    []KinesisRecord
}

func (m *MockKinesisClient) PutRecord(ctx context.Context, streamName string, data []byte, partitionKey string) error {
	m.PutRecordCalls = append(m.PutRecordCalls, mockPutRecordCall{
		StreamName:   streamName,
		Data:         data,
		PartitionKey: partitionKey,
	})
	if m.PutRecordFunc != nil {
		return m.PutRecordFunc(ctx, streamName, data, partitionKey)
	}
	return nil
}

func (m *MockKinesisClient) PutRecords(ctx context.Context, streamName string, records []KinesisRecord) error {
	m.PutRecordsCalls = append(m.PutRecordsCalls, mockPutRecordsCall{
		StreamName: streamName,
		Records:    records,
	})
	if m.PutRecordsFunc != nil {
		return m.PutRecordsFunc(ctx, streamName, records)
	}
	return nil
}

func newTestEvent(id, sessionID string) *SessionEvent {
	return &SessionEvent{
		EventID:     id,
		EventType:   "session_created",
		SessionID:   sessionID,
		WorkspaceID: "ws-1",
		AgentID:     "agent-1",
		Timestamp:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Payload:     json.RawMessage(`{"key":"value"}`),
	}
}

func TestNewKinesisPublisher_DefaultPartitionKey(t *testing.T) {
	client := &MockKinesisClient{}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName: "test-stream",
		Region:     "us-east-1",
	})
	if pub.config.PartitionKeyField != "session_id" {
		t.Errorf("expected default partition key field 'session_id', got %q", pub.config.PartitionKeyField)
	}
}

func TestNewKinesisPublisher_CustomPartitionKey(t *testing.T) {
	client := &MockKinesisClient{}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName:        "test-stream",
		Region:            "us-east-1",
		PartitionKeyField: "workspace_id",
	})
	if pub.config.PartitionKeyField != "workspace_id" {
		t.Errorf("expected partition key field 'workspace_id', got %q", pub.config.PartitionKeyField)
	}
}

func TestKinesisPublisher_Publish_Success(t *testing.T) {
	client := &MockKinesisClient{}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName: "test-stream",
		Region:     "us-east-1",
	})

	event := newTestEvent("evt-1", "sess-1")
	err := pub.Publish(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.PutRecordCalls) != 1 {
		t.Fatalf("expected 1 PutRecord call, got %d", len(client.PutRecordCalls))
	}

	call := client.PutRecordCalls[0]
	if call.StreamName != "test-stream" {
		t.Errorf("expected stream name 'test-stream', got %q", call.StreamName)
	}
	if call.PartitionKey != "sess-1" {
		t.Errorf("expected partition key 'sess-1', got %q", call.PartitionKey)
	}
}

func TestKinesisPublisher_Publish_NilEvent(t *testing.T) {
	client := &MockKinesisClient{}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName: "test-stream",
	})

	err := pub.Publish(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil event")
	}
}

func TestKinesisPublisher_Publish_ClientError(t *testing.T) {
	client := &MockKinesisClient{
		PutRecordFunc: func(_ context.Context, _ string, _ []byte, _ string) error {
			return fmt.Errorf("kinesis error")
		},
	}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName: "test-stream",
	})

	err := pub.Publish(context.Background(), newTestEvent("evt-1", "sess-1"))
	if err == nil {
		t.Fatal("expected error from client")
	}
}

func TestKinesisPublisher_Publish_WorkspacePartitionKey(t *testing.T) {
	client := &MockKinesisClient{}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName:        "test-stream",
		PartitionKeyField: "workspace_id",
	})

	event := newTestEvent("evt-1", "sess-1")
	err := pub.Publish(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.PutRecordCalls[0].PartitionKey != "ws-1" {
		t.Errorf("expected partition key 'ws-1', got %q", client.PutRecordCalls[0].PartitionKey)
	}
}

func TestKinesisPublisher_PublishBatch_Success(t *testing.T) {
	client := &MockKinesisClient{}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName: "test-stream",
	})

	events := []*SessionEvent{
		newTestEvent("evt-1", "sess-1"),
		newTestEvent("evt-2", "sess-2"),
	}

	err := pub.PublishBatch(context.Background(), events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.PutRecordsCalls) != 1 {
		t.Fatalf("expected 1 PutRecords call, got %d", len(client.PutRecordsCalls))
	}
	if len(client.PutRecordsCalls[0].Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(client.PutRecordsCalls[0].Records))
	}
}

func TestKinesisPublisher_PublishBatch_Empty(t *testing.T) {
	client := &MockKinesisClient{}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName: "test-stream",
	})

	err := pub.PublishBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.PutRecordsCalls) != 0 {
		t.Errorf("expected no PutRecords calls, got %d", len(client.PutRecordsCalls))
	}
}

func TestKinesisPublisher_PublishBatch_NilEvent(t *testing.T) {
	client := &MockKinesisClient{}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName: "test-stream",
	})

	events := []*SessionEvent{newTestEvent("evt-1", "sess-1"), nil}
	err := pub.PublishBatch(context.Background(), events)
	if err == nil {
		t.Fatal("expected error for nil event in batch")
	}
}

func TestKinesisPublisher_PublishBatch_ClientError(t *testing.T) {
	client := &MockKinesisClient{
		PutRecordsFunc: func(_ context.Context, _ string, _ []KinesisRecord) error {
			return fmt.Errorf("batch error")
		},
	}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName: "test-stream",
	})

	events := []*SessionEvent{newTestEvent("evt-1", "sess-1")}
	err := pub.PublishBatch(context.Background(), events)
	if err == nil {
		t.Fatal("expected error from client")
	}
}

func TestKinesisPublisher_PublishBatch_MultipleBatches(t *testing.T) {
	client := &MockKinesisClient{}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName: "test-stream",
	})

	// Create 501 events to trigger 2 batches (500 + 1)
	events := make([]*SessionEvent, 501)
	for i := range events {
		events[i] = newTestEvent(fmt.Sprintf("evt-%d", i), fmt.Sprintf("sess-%d", i))
	}

	err := pub.PublishBatch(context.Background(), events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.PutRecordsCalls) != 2 {
		t.Fatalf("expected 2 PutRecords calls, got %d", len(client.PutRecordsCalls))
	}
	if len(client.PutRecordsCalls[0].Records) != 500 {
		t.Errorf("expected 500 records in first batch, got %d", len(client.PutRecordsCalls[0].Records))
	}
	if len(client.PutRecordsCalls[1].Records) != 1 {
		t.Errorf("expected 1 record in second batch, got %d", len(client.PutRecordsCalls[1].Records))
	}
}

func TestKinesisPublisher_Close(t *testing.T) {
	client := &MockKinesisClient{}
	pub := NewKinesisPublisher(client, KinesisPublisherConfig{
		StreamName: "test-stream",
	})

	err := pub.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractPartitionKey_AllFields(t *testing.T) {
	event := &SessionEvent{
		EventID:     "evt-1",
		EventType:   "session_created",
		SessionID:   "sess-1",
		WorkspaceID: "ws-1",
		AgentID:     "agent-1",
	}

	tests := []struct {
		field    string
		expected string
	}{
		{"session_id", "sess-1"},
		{"workspace_id", "ws-1"},
		{"agent_id", "agent-1"},
		{"event_type", "session_created"},
		{"event_id", "evt-1"},
		{"unknown_field", "sess-1"}, // defaults to session_id
	}

	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			result := extractPartitionKey(event, tc.field)
			if result != tc.expected {
				t.Errorf("extractPartitionKey(%q) = %q, want %q", tc.field, result, tc.expected)
			}
		})
	}
}
