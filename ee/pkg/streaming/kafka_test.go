/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package streaming

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/IBM/sarama"
)

// mockAsyncProducer implements saramaProducer for testing.
type mockAsyncProducer struct {
	input    chan *sarama.ProducerMessage
	errors   chan *sarama.ProducerError
	closed   bool
	messages []*sarama.ProducerMessage
}

func newMockAsyncProducer() *mockAsyncProducer {
	return &mockAsyncProducer{
		input:  make(chan *sarama.ProducerMessage, 100),
		errors: make(chan *sarama.ProducerError, 100),
	}
}

func (m *mockAsyncProducer) Input() chan<- *sarama.ProducerMessage {
	return m.input
}

func (m *mockAsyncProducer) Errors() <-chan *sarama.ProducerError {
	return m.errors
}

func (m *mockAsyncProducer) AsyncClose() {
	m.closed = true
	close(m.errors)
}

func (m *mockAsyncProducer) Close() error {
	m.closed = true
	close(m.errors)
	return nil
}

// drain reads all messages from the input channel.
func (m *mockAsyncProducer) drain() {
	for {
		select {
		case msg := <-m.input:
			m.messages = append(m.messages, msg)
		default:
			return
		}
	}
}

func testEvent() *SessionEvent {
	return &SessionEvent{
		EventID:     "evt-1",
		EventType:   EventTypeSessionCreated,
		Timestamp:   time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		SessionID:   "sess-abc",
		WorkspaceID: "ws-1",
		AgentID:     "agent-1",
		Namespace:   "default",
		Payload:     json.RawMessage(`{"key":"value"}`),
	}
}

func TestKafkaPublisher_Publish(t *testing.T) {
	mock := newMockAsyncProducer()
	logger := slog.Default()
	pub := newKafkaPublisherWithProducer(mock, "test-topic", PartitionBySessionID, logger)

	err := pub.Publish(context.Background(), testEvent())
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	mock.drain()
	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.messages))
	}

	msg := mock.messages[0]
	if msg.Topic != "test-topic" {
		t.Errorf("expected topic test-topic, got %s", msg.Topic)
	}

	keyBytes, err := msg.Key.Encode()
	if err != nil {
		t.Fatalf("failed to encode key: %v", err)
	}
	if string(keyBytes) != "sess-abc" {
		t.Errorf("expected key sess-abc, got %s", string(keyBytes))
	}

	valBytes, err := msg.Value.Encode()
	if err != nil {
		t.Fatalf("failed to encode value: %v", err)
	}

	var decoded SessionEvent
	if err := json.Unmarshal(valBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal value: %v", err)
	}
	if decoded.EventID != "evt-1" {
		t.Errorf("expected eventId evt-1, got %s", decoded.EventID)
	}

	if err := pub.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestKafkaPublisher_PublishNilEvent(t *testing.T) {
	mock := newMockAsyncProducer()
	pub := newKafkaPublisherWithProducer(mock, "test-topic", PartitionBySessionID, nil)

	err := pub.Publish(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil event")
	}
	if err.Error() != ErrMsgNilEvent {
		t.Errorf("expected error %q, got %q", ErrMsgNilEvent, err.Error())
	}

	if err := pub.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestKafkaPublisher_PublishAfterClose(t *testing.T) {
	mock := newMockAsyncProducer()
	pub := newKafkaPublisherWithProducer(mock, "test-topic", PartitionBySessionID, nil)

	if err := pub.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	err := pub.Publish(context.Background(), testEvent())
	if err == nil {
		t.Fatal("expected error after close")
	}
	if err.Error() != errMsgPublisherClosed {
		t.Errorf("expected error %q, got %q", errMsgPublisherClosed, err.Error())
	}
}

func TestKafkaPublisher_PublishBatch(t *testing.T) {
	mock := newMockAsyncProducer()
	pub := newKafkaPublisherWithProducer(mock, "test-topic", PartitionByAgentID, nil)

	events := []*SessionEvent{
		testEvent(),
		{
			EventID:   "evt-2",
			EventType: EventTypeMessageAdded,
			Timestamp: time.Now(),
			SessionID: "sess-def",
			AgentID:   "agent-2",
		},
		nil, // nil events should be skipped
	}

	err := pub.PublishBatch(context.Background(), events)
	if err != nil {
		t.Fatalf("PublishBatch returned error: %v", err)
	}

	mock.drain()
	if len(mock.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(mock.messages))
	}

	// Verify agent_id partitioning key
	keyBytes, _ := mock.messages[0].Key.Encode()
	if string(keyBytes) != "agent-1" {
		t.Errorf("expected key agent-1, got %s", string(keyBytes))
	}

	keyBytes, _ = mock.messages[1].Key.Encode()
	if string(keyBytes) != "agent-2" {
		t.Errorf("expected key agent-2, got %s", string(keyBytes))
	}

	if err := pub.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestKafkaPublisher_PublishBatchAfterClose(t *testing.T) {
	mock := newMockAsyncProducer()
	pub := newKafkaPublisherWithProducer(mock, "test-topic", PartitionBySessionID, nil)

	if err := pub.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	err := pub.PublishBatch(context.Background(), []*SessionEvent{testEvent()})
	if err == nil {
		t.Fatal("expected error after close")
	}
}

func TestKafkaPublisher_RoundRobinPartitioning(t *testing.T) {
	mock := newMockAsyncProducer()
	pub := newKafkaPublisherWithProducer(mock, "test-topic", PartitionRoundRobin, nil)

	err := pub.Publish(context.Background(), testEvent())
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	mock.drain()
	if mock.messages[0].Key != nil {
		t.Error("expected nil key for round_robin strategy")
	}

	if err := pub.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestKafkaPublisher_CloseIdempotent(t *testing.T) {
	mock := newMockAsyncProducer()
	pub := newKafkaPublisherWithProducer(mock, "test-topic", PartitionBySessionID, nil)

	if err := pub.Close(); err != nil {
		t.Fatalf("first Close returned error: %v", err)
	}
	if err := pub.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

func TestKafkaPublisher_DrainErrors(t *testing.T) {
	mock := newMockAsyncProducer()
	logger := slog.Default()
	pub := newKafkaPublisherWithProducer(mock, "test-topic", PartitionBySessionID, logger)

	// Send an error to the error channel
	mock.errors <- &sarama.ProducerError{
		Msg: &sarama.ProducerMessage{Topic: "test-topic"},
		Err: sarama.ErrOutOfBrokers,
	}

	// Give the drain goroutine time to process
	time.Sleep(50 * time.Millisecond)

	if err := pub.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestBuildSaramaConfig_Defaults(t *testing.T) {
	cfg := &KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "test",
	}

	sc, err := buildSaramaConfig(cfg)
	if err != nil {
		t.Fatalf("buildSaramaConfig returned error: %v", err)
	}

	if sc.Producer.RequiredAcks != sarama.WaitForAll {
		t.Errorf("expected WaitForAll, got %v", sc.Producer.RequiredAcks)
	}
	if sc.Producer.Compression != sarama.CompressionNone {
		t.Errorf("expected CompressionNone, got %v", sc.Producer.Compression)
	}
	if !sc.Producer.Return.Errors {
		t.Error("expected Return.Errors to be true")
	}
}

func TestBuildSaramaConfig_AllOptions(t *testing.T) {
	cfg := &KafkaConfig{
		Brokers:           []string{"localhost:9092"},
		Topic:             "test",
		PartitionStrategy: PartitionBySessionID,
		Compression:       "gzip",
		Acks:              "1",
		Retries:           5,
		BatchSize:         16384,
		LingerMs:          100,
		SASL: &SASLConfig{
			Mechanism: "PLAIN",
			Username:  "user",
			Password:  "pass",
		},
		TLS: &TLSConfig{
			Enable: true,
		},
	}

	sc, err := buildSaramaConfig(cfg)
	if err != nil {
		t.Fatalf("buildSaramaConfig returned error: %v", err)
	}

	if sc.Producer.RequiredAcks != sarama.WaitForLocal {
		t.Errorf("expected WaitForLocal, got %v", sc.Producer.RequiredAcks)
	}
	if sc.Producer.Compression != sarama.CompressionGZIP {
		t.Errorf("expected CompressionGZIP, got %v", sc.Producer.Compression)
	}
	if sc.Producer.Retry.Max != 5 {
		t.Errorf("expected Retry.Max 5, got %d", sc.Producer.Retry.Max)
	}
	if sc.Producer.Flush.Bytes != 16384 {
		t.Errorf("expected Flush.Bytes 16384, got %d", sc.Producer.Flush.Bytes)
	}
	if !sc.Net.SASL.Enable {
		t.Error("expected SASL to be enabled")
	}
	if sc.Net.SASL.User != "user" {
		t.Errorf("expected SASL user 'user', got %s", sc.Net.SASL.User)
	}
	if !sc.Net.TLS.Enable {
		t.Error("expected TLS to be enabled")
	}
	if sc.Net.TLS.Config == nil {
		t.Error("expected TLS config to be set")
	}
}

func TestBuildSaramaConfig_InvalidAcks(t *testing.T) {
	cfg := &KafkaConfig{
		Acks: "invalid",
	}

	_, err := buildSaramaConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid acks")
	}
}

func TestBuildSaramaConfig_AcksZero(t *testing.T) {
	cfg := &KafkaConfig{Acks: "0"}
	sc, err := buildSaramaConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc.Producer.RequiredAcks != sarama.NoResponse {
		t.Errorf("expected NoResponse, got %v", sc.Producer.RequiredAcks)
	}
}

func TestBuildSaramaConfig_Compression(t *testing.T) {
	tests := []struct {
		name        string
		compression string
		want        sarama.CompressionCodec
	}{
		{"snappy", "snappy", sarama.CompressionSnappy},
		{"lz4", "lz4", sarama.CompressionLZ4},
		{"none", "none", sarama.CompressionNone},
		{"unknown defaults to none", "unknown", sarama.CompressionNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &KafkaConfig{Compression: tt.compression}
			sc, err := buildSaramaConfig(cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sc.Producer.Compression != tt.want {
				t.Errorf("expected %v, got %v", tt.want, sc.Producer.Compression)
			}
		})
	}
}

func TestBuildSaramaConfig_TLSWithCustomConfig(t *testing.T) {
	cfg := &KafkaConfig{
		TLS: &TLSConfig{
			Enable: true,
			Config: &tls.Config{MinVersion: tls.VersionTLS13}, //nolint:gosec
		},
	}

	sc, err := buildSaramaConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc.Net.TLS.Config.MinVersion != tls.VersionTLS13 {
		t.Errorf("expected TLS 1.3, got %v", sc.Net.TLS.Config.MinVersion)
	}
}

func TestBuildSaramaConfig_TLSDisabled(t *testing.T) {
	cfg := &KafkaConfig{
		TLS: &TLSConfig{Enable: false},
	}

	sc, err := buildSaramaConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc.Net.TLS.Enable {
		t.Error("expected TLS to be disabled")
	}
}
