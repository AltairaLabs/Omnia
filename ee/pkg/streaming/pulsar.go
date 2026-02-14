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
)

// PulsarProducer abstracts the Apache Pulsar producer operations needed by the publisher.
// Implementations should wrap the real Pulsar client producer.
type PulsarProducer interface {
	// Send synchronously publishes a message to the Pulsar topic.
	Send(ctx context.Context, payload []byte) error

	// Close flushes pending messages and releases resources.
	Close() error
}

// PulsarPublisherConfig holds configuration for the Pulsar publisher.
type PulsarPublisherConfig struct {
	// ServiceURL is the Pulsar service URL.
	ServiceURL string

	// Topic is the Pulsar topic to publish to.
	Topic string
}

// PulsarPublisher implements StreamingPublisher for Apache Pulsar.
type PulsarPublisher struct {
	producer PulsarProducer
	config   PulsarPublisherConfig
}

// Compile-time interface check.
var _ StreamingPublisher = (*PulsarPublisher)(nil)

// NewPulsarPublisher creates a new PulsarPublisher with the given producer and config.
func NewPulsarPublisher(producer PulsarProducer, config PulsarPublisherConfig) *PulsarPublisher {
	return &PulsarPublisher{
		producer: producer,
		config:   config,
	}
}

// Publish sends a single event to the Pulsar topic.
func (p *PulsarPublisher) Publish(ctx context.Context, event *SessionEvent) error {
	if event == nil {
		return fmt.Errorf("event must not be nil")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if err := p.producer.Send(ctx, data); err != nil {
		return fmt.Errorf("failed to send message to Pulsar: %w", err)
	}

	return nil
}

// PublishBatch sends multiple events to the Pulsar topic by iterating and sending each.
// Pulsar does not have a native batch API at the producer level.
func (p *PulsarPublisher) PublishBatch(ctx context.Context, events []*SessionEvent) error {
	for i, event := range events {
		if err := p.Publish(ctx, event); err != nil {
			return fmt.Errorf("failed to publish event at index %d: %w", i, err)
		}
	}
	return nil
}

// Close closes the underlying Pulsar producer.
func (p *PulsarPublisher) Close() error {
	if err := p.producer.Close(); err != nil {
		return fmt.Errorf("failed to close Pulsar producer: %w", err)
	}
	return nil
}
