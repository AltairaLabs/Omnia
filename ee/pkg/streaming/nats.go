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
	data, err := marshalEvent(event)
	if err != nil {
		return err
	}

	if err := p.js.Publish(ctx, p.config.Subject, data); err != nil {
		return fmt.Errorf("failed to publish message to NATS: %w", err)
	}

	return nil
}

// PublishBatch sends multiple events to the configured NATS subject by iterating
// and publishing each individually.
func (p *NATSPublisher) PublishBatch(ctx context.Context, events []*SessionEvent) error {
	return defaultPublishBatch(ctx, events, p.Publish)
}

// Close is a no-op for NATS as the connection is externally managed.
func (p *NATSPublisher) Close() error {
	return nil
}
