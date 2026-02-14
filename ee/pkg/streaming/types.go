/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package streaming provides event publishers for real-time session streaming.
// It supports Kafka-based publishing with configurable partitioning,
// compression, and delivery guarantees for session events.
package streaming

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"time"
)

// Event type constants for session lifecycle events.
const (
	EventTypeSessionCreated   = "session_created"
	EventTypeMessageAdded     = "message_added"
	EventTypeSessionCompleted = "session_completed"
	EventTypeToolExecuted     = "tool_executed"
	EventTypeError            = "error"
)

// PartitionStrategy determines how events are distributed across Kafka partitions.
type PartitionStrategy string

const (
	// PartitionBySessionID routes all events for the same session to one partition.
	PartitionBySessionID PartitionStrategy = "session_id"
	// PartitionByAgentID routes all events for the same agent to one partition.
	PartitionByAgentID PartitionStrategy = "agent_id"
	// PartitionRoundRobin distributes events evenly across partitions.
	PartitionRoundRobin PartitionStrategy = "round_robin"
)

// StreamingPublisher publishes session events to a streaming backend.
type StreamingPublisher interface {
	// Publish sends a single event. It is non-blocking for async implementations.
	Publish(ctx context.Context, event *SessionEvent) error
	// PublishBatch sends multiple events. It is non-blocking for async implementations.
	PublishBatch(ctx context.Context, events []*SessionEvent) error
	// Close flushes pending events and releases resources.
	Close() error
}

// SessionEvent represents a session lifecycle event for streaming.
type SessionEvent struct {
	EventID     string          `json:"eventId"`
	EventType   string          `json:"eventType"`
	Timestamp   time.Time       `json:"timestamp"`
	SessionID   string          `json:"sessionId"`
	WorkspaceID string          `json:"workspaceId,omitempty"`
	AgentID     string          `json:"agentId,omitempty"`
	Namespace   string          `json:"namespace,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// KafkaConfig holds configuration for the Kafka streaming publisher.
type KafkaConfig struct {
	// Brokers is the list of Kafka broker addresses.
	Brokers []string
	// Topic is the Kafka topic to publish events to.
	Topic string
	// PartitionStrategy determines how events are routed to partitions.
	PartitionStrategy PartitionStrategy
	// Compression codec: "none", "gzip", "snappy", "lz4".
	Compression string
	// Acks: "0" (fire-and-forget), "1" (leader only), "all" (all replicas).
	Acks string
	// Retries is the maximum number of send retries.
	Retries int
	// BatchSize is the maximum number of messages per batch (bytes).
	BatchSize int
	// LingerMs is the time to wait for batching before sending.
	LingerMs int
	// SASL authentication config. Nil means no SASL.
	SASL *SASLConfig
	// TLS config. Nil means no TLS.
	TLS *TLSConfig
}

// SASLConfig holds SASL authentication settings.
type SASLConfig struct {
	// Mechanism is the SASL mechanism: "PLAIN" or "SCRAM-SHA-256" or "SCRAM-SHA-512".
	Mechanism string
	// Username for SASL authentication.
	Username string
	// Password for SASL authentication.
	Password string
}

// TLSConfig holds TLS connection settings.
type TLSConfig struct {
	// Enable TLS for broker connections.
	Enable bool
	// Config is the Go TLS configuration. If nil and Enable is true,
	// a default configuration is used.
	Config *tls.Config
}
