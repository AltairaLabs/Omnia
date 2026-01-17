/*
Copyright 2025.

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

package queue

import (
	"context"
	"time"
)

// InstrumentedQueue wraps a WorkQueue implementation with Prometheus metrics.
// It delegates all operations to the underlying queue while recording metrics
// for each operation.
type InstrumentedQueue struct {
	queue   WorkQueue
	metrics QueueMetricsRecorder
}

// NewInstrumentedQueue creates a new instrumented queue wrapper.
// The wrapper delegates to the underlying queue and records metrics.
func NewInstrumentedQueue(queue WorkQueue, metrics QueueMetricsRecorder) *InstrumentedQueue {
	return &InstrumentedQueue{
		queue:   queue,
		metrics: metrics,
	}
}

// Push adds work items to the queue for the specified job.
// Records push operation metrics including duration and item count.
func (q *InstrumentedQueue) Push(ctx context.Context, jobID string, items []WorkItem) error {
	start := time.Now()

	err := q.queue.Push(ctx, jobID, items)

	duration := time.Since(start).Seconds()
	q.metrics.RecordOperation(OpPush, duration, err == nil)

	if err == nil {
		q.metrics.RecordItemsPushed(jobID, len(items))
	}

	return err
}

// Pop retrieves the next available work item for the specified job.
// Records pop operation metrics including duration.
func (q *InstrumentedQueue) Pop(ctx context.Context, jobID string) (*WorkItem, error) {
	start := time.Now()

	item, err := q.queue.Pop(ctx, jobID)

	duration := time.Since(start).Seconds()
	// ErrQueueEmpty is not considered an error for metrics purposes
	success := err == nil || err == ErrQueueEmpty
	q.metrics.RecordOperation(OpPop, duration, success)

	if err == nil && item != nil {
		// Item moved from pending to processing
		q.metrics.RecordItemStatusChange(jobID, ItemStatusPending, ItemStatusProcessing)
	}

	return item, err
}

// Ack acknowledges successful processing of a work item.
// Records ack operation metrics and item completion.
func (q *InstrumentedQueue) Ack(ctx context.Context, jobID string, itemID string, result []byte) error {
	start := time.Now()

	err := q.queue.Ack(ctx, jobID, itemID, result)

	duration := time.Since(start).Seconds()
	q.metrics.RecordOperation(OpAck, duration, err == nil)

	if err == nil {
		// Item moved from processing to completed
		q.metrics.RecordItemStatusChange(jobID, ItemStatusProcessing, ItemStatusCompleted)
	}

	return err
}

// Nack indicates that processing of a work item failed.
// Records nack operation metrics and handles retry tracking.
func (q *InstrumentedQueue) Nack(ctx context.Context, jobID string, itemID string, err error) error {
	start := time.Now()

	nackErr := q.queue.Nack(ctx, jobID, itemID, err)

	duration := time.Since(start).Seconds()
	q.metrics.RecordOperation(OpNack, duration, nackErr == nil)

	if nackErr == nil {
		// We don't know if it was requeued or failed without querying progress
		// For simplicity, record a retry attempt
		q.metrics.RecordRetry(jobID)
		// Item moved from processing - the actual destination (pending or failed)
		// depends on retry count, but we record it as leaving processing
		// The next Push or Pop will update the status correctly
		q.metrics.RecordItemStatusChange(jobID, ItemStatusProcessing, "")
	}

	return nackErr
}

// Progress returns the current progress for the specified job.
// This is a read-only operation and does not record operation metrics.
func (q *InstrumentedQueue) Progress(ctx context.Context, jobID string) (*JobProgress, error) {
	return q.queue.Progress(ctx, jobID)
}

// Close releases any resources held by the queue.
func (q *InstrumentedQueue) Close() error {
	return q.queue.Close()
}

// GetCompletedItems returns all completed work items for a job.
// This is a read-only operation and does not record operation metrics.
func (q *InstrumentedQueue) GetCompletedItems(ctx context.Context, jobID string) ([]*WorkItem, error) {
	return q.queue.GetCompletedItems(ctx, jobID)
}

// GetFailedItems returns all failed work items for a job.
// This is a read-only operation and does not record operation metrics.
func (q *InstrumentedQueue) GetFailedItems(ctx context.Context, jobID string) ([]*WorkItem, error) {
	return q.queue.GetFailedItems(ctx, jobID)
}

// Ensure InstrumentedQueue implements WorkQueue interface.
var _ WorkQueue = (*InstrumentedQueue)(nil)
