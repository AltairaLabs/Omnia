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

// kinesisMaxRecordsPerBatch is the maximum number of records per PutRecords call.
const kinesisMaxRecordsPerBatch = 500

// KinesisRecord represents a single record to be sent to Kinesis.
type KinesisRecord struct {
	// Data is the record payload.
	Data []byte
	// PartitionKey determines which shard receives the record.
	PartitionKey string
}

// KinesisClient abstracts the AWS Kinesis API operations needed by the publisher.
// Implementations should wrap the real AWS SDK Kinesis client.
type KinesisClient interface {
	// PutRecord sends a single record to a Kinesis stream.
	PutRecord(ctx context.Context, streamName string, data []byte, partitionKey string) error

	// PutRecords sends a batch of records to a Kinesis stream.
	PutRecords(ctx context.Context, streamName string, records []KinesisRecord) error
}

// KinesisPublisherConfig holds configuration for the Kinesis publisher.
type KinesisPublisherConfig struct {
	// StreamName is the Kinesis stream to publish to.
	StreamName string

	// Region is the AWS region of the Kinesis stream.
	Region string

	// PartitionKeyField is the event field used as the partition key.
	// Defaults to "session_id" if empty.
	PartitionKeyField string
}

// KinesisPublisher implements StreamingPublisher for AWS Kinesis.
type KinesisPublisher struct {
	client KinesisClient
	config KinesisPublisherConfig
}

// Compile-time interface check.
var _ StreamingPublisher = (*KinesisPublisher)(nil)

// NewKinesisPublisher creates a new KinesisPublisher with the given client and config.
func NewKinesisPublisher(client KinesisClient, config KinesisPublisherConfig) *KinesisPublisher {
	if config.PartitionKeyField == "" {
		config.PartitionKeyField = "session_id"
	}
	return &KinesisPublisher{
		client: client,
		config: config,
	}
}

// Publish sends a single event to the Kinesis stream.
func (p *KinesisPublisher) Publish(ctx context.Context, event *SessionEvent) error {
	data, err := marshalEvent(event)
	if err != nil {
		return err
	}

	partitionKey := extractPartitionKey(event, p.config.PartitionKeyField)

	if err := p.client.PutRecord(ctx, p.config.StreamName, data, partitionKey); err != nil {
		return fmt.Errorf("failed to put record to Kinesis: %w", err)
	}

	return nil
}

// PublishBatch sends multiple events to the Kinesis stream, batching in groups
// of up to 500 records per API call (Kinesis limit).
func (p *KinesisPublisher) PublishBatch(ctx context.Context, events []*SessionEvent) error {
	if len(events) == 0 {
		return nil
	}

	records, err := p.marshalEvents(events)
	if err != nil {
		return err
	}

	return p.sendBatches(ctx, records)
}

// marshalEvents converts events to KinesisRecords.
func (p *KinesisPublisher) marshalEvents(events []*SessionEvent) ([]KinesisRecord, error) {
	records := make([]KinesisRecord, 0, len(events))
	for i, event := range events {
		data, err := marshalEvent(event)
		if err != nil {
			return nil, fmt.Errorf("event at index %d: %w", i, err)
		}

		records = append(records, KinesisRecord{
			Data:         data,
			PartitionKey: extractPartitionKey(event, p.config.PartitionKeyField),
		})
	}
	return records, nil
}

// sendBatches sends records in chunks of kinesisMaxRecordsPerBatch.
func (p *KinesisPublisher) sendBatches(ctx context.Context, records []KinesisRecord) error {
	for i := 0; i < len(records); i += kinesisMaxRecordsPerBatch {
		end := i + kinesisMaxRecordsPerBatch
		if end > len(records) {
			end = len(records)
		}
		batch := records[i:end]

		if err := p.client.PutRecords(ctx, p.config.StreamName, batch); err != nil {
			return fmt.Errorf("failed to put records batch to Kinesis: %w", err)
		}
	}
	return nil
}

// Close is a no-op for Kinesis as the client is externally managed.
func (p *KinesisPublisher) Close() error {
	return nil
}

// extractPartitionKey extracts the partition key value from an event based on the field name.
func extractPartitionKey(event *SessionEvent, field string) string {
	switch field {
	case "session_id":
		return event.SessionID
	case "workspace_id":
		return event.WorkspaceID
	case "agent_id":
		return event.AgentID
	case "event_type":
		return event.EventType
	case "event_id":
		return event.EventID
	default:
		return event.SessionID
	}
}
