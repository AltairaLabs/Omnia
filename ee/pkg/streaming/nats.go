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

// JetStreamPublisher abstracts the NATS JetStream publish operations needed by the publisher.
// Implementations should wrap the real NATS JetStream context.
type JetStreamPublisher interface {
	// Publish sends a message to the specified subject on JetStream.
	Publish(ctx context.Context, subject string, data []byte) error
}

// NATSPublisherConfig holds configuration for the NATS JetStream publisher.
type NATSPublisherConfig struct {
	// URL is the NATS server URL.
	URL string

	// Stream is the JetStream stream name.
	Stream string

	// Subject is the NATS subject to publish to.
	Subject string
}

// NATSPublisher implements StreamingPublisher for NATS JetStream.
type NATSPublisher struct {
	js     JetStreamPublisher
	config NATSPublisherConfig
}

// Compile-time interface check.
var _ StreamingPublisher = (*NATSPublisher)(nil)

// NewNATSPublisher creates a new NATSPublisher with the given JetStream publisher and config.
func NewNATSPublisher(js JetStreamPublisher, config NATSPublisherConfig) *NATSPublisher {
	return &NATSPublisher{
		js:     js,
		config: config,
	}
}

// Publish sends a single event to the configured NATS subject.
func (p *NATSPublisher) Publish(ctx context.Context, event *SessionEvent) error {
	if event == nil {
		return fmt.Errorf("event must not be nil")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if err := p.js.Publish(ctx, p.config.Subject, data); err != nil {
		return fmt.Errorf("failed to publish message to NATS: %w", err)
	}

	return nil
}

// PublishBatch sends multiple events to the configured NATS subject by iterating
// and publishing each individually.
func (p *NATSPublisher) PublishBatch(ctx context.Context, events []*SessionEvent) error {
	for i, event := range events {
		if err := p.Publish(ctx, event); err != nil {
			return fmt.Errorf("failed to publish event at index %d: %w", i, err)
		}
	}
	return nil
}

// Close is a no-op for NATS as the connection is externally managed.
func (p *NATSPublisher) Close() error {
	return nil
}
