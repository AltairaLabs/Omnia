/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package queue provides interfaces and implementations for distributing
// Arena work items across multiple workers.
package queue

import (
	"context"
	"errors"
	"time"
)

// Common errors returned by WorkQueue implementations.
var (
	// ErrQueueEmpty is returned when Pop is called on an empty queue.
	ErrQueueEmpty = errors.New("queue is empty")

	// ErrItemNotFound is returned when an item cannot be found for Ack/Nack.
	ErrItemNotFound = errors.New("work item not found")

	// ErrQueueClosed is returned when operations are attempted on a closed queue.
	ErrQueueClosed = errors.New("queue is closed")

	// ErrJobNotFound is returned when a job cannot be found.
	ErrJobNotFound = errors.New("job not found")
)

// ItemStatus represents the status of a work item.
type ItemStatus string

const (
	// ItemStatusPending indicates the item is waiting to be processed.
	ItemStatusPending ItemStatus = "pending"

	// ItemStatusProcessing indicates the item is currently being processed.
	ItemStatusProcessing ItemStatus = "processing"

	// ItemStatusCompleted indicates the item was processed successfully.
	ItemStatusCompleted ItemStatus = "completed"

	// ItemStatusFailed indicates the item processing failed.
	ItemStatusFailed ItemStatus = "failed"
)

// WorkItem represents a unit of work to be processed by an Arena worker.
type WorkItem struct {
	// ID is the unique identifier for this work item.
	ID string `json:"id"`

	// JobID is the ID of the parent ArenaJob.
	JobID string `json:"jobId"`

	// ScenarioID identifies which scenario from the bundle to execute.
	ScenarioID string `json:"scenarioId"`

	// ProviderID identifies which provider to use for this scenario.
	ProviderID string `json:"providerId"`

	// BundleURL is the URL to fetch the PromptKit bundle from.
	BundleURL string `json:"bundleUrl"`

	// Config contains the scenario configuration as JSON.
	Config []byte `json:"config,omitempty"`

	// Status is the current status of the work item.
	Status ItemStatus `json:"status"`

	// Attempt tracks the current attempt number (1-based).
	Attempt int `json:"attempt"`

	// MaxAttempts is the maximum number of retry attempts.
	MaxAttempts int `json:"maxAttempts"`

	// CreatedAt is when the work item was created.
	CreatedAt time.Time `json:"createdAt"`

	// StartedAt is when the work item started processing.
	StartedAt *time.Time `json:"startedAt,omitempty"`

	// CompletedAt is when the work item finished (success or failure).
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	// Error contains the error message if the item failed.
	Error string `json:"error,omitempty"`

	// Result contains the execution result as JSON.
	Result []byte `json:"result,omitempty"`
}

// JobProgress represents the progress of an Arena job's work items.
type JobProgress struct {
	// JobID is the ID of the ArenaJob.
	JobID string `json:"jobId"`

	// Total is the total number of work items.
	Total int `json:"total"`

	// Pending is the number of items waiting to be processed.
	Pending int `json:"pending"`

	// Processing is the number of items currently being processed.
	Processing int `json:"processing"`

	// Completed is the number of items that completed successfully.
	Completed int `json:"completed"`

	// Failed is the number of items that failed.
	Failed int `json:"failed"`

	// StartedAt is when the first item started processing.
	StartedAt *time.Time `json:"startedAt,omitempty"`

	// CompletedAt is when all items finished processing.
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// IsComplete returns true if all work items have been processed.
func (p *JobProgress) IsComplete() bool {
	return p.Pending == 0 && p.Processing == 0
}

// SuccessRate returns the success rate as a percentage (0-100).
func (p *JobProgress) SuccessRate() float64 {
	finished := p.Completed + p.Failed
	if finished == 0 {
		return 0
	}
	return float64(p.Completed) / float64(finished) * 100
}

// WorkQueue defines the interface for distributing work items to Arena workers.
type WorkQueue interface {
	// Push adds work items to the queue for the specified job.
	// Items are added in the order provided and will be processed FIFO.
	Push(ctx context.Context, jobID string, items []WorkItem) error

	// Pop retrieves the next available work item for the specified job.
	// The item is marked as processing and must be acknowledged or rejected.
	// Returns ErrQueueEmpty if no items are available.
	Pop(ctx context.Context, jobID string) (*WorkItem, error)

	// Ack acknowledges successful processing of a work item.
	// The item is marked as completed and removed from the processing set.
	// The result parameter contains the execution result as JSON.
	Ack(ctx context.Context, jobID string, itemID string, result []byte) error

	// Nack indicates that processing of a work item failed.
	// If retries remain, the item is requeued; otherwise, it's marked as failed.
	// The error parameter contains the failure reason.
	Nack(ctx context.Context, jobID string, itemID string, err error) error

	// Progress returns the current progress for the specified job.
	// Returns ErrJobNotFound if the job doesn't exist.
	Progress(ctx context.Context, jobID string) (*JobProgress, error)

	// GetCompletedItems returns all completed work items for a job.
	// Returns ErrJobNotFound if the job doesn't exist.
	GetCompletedItems(ctx context.Context, jobID string) ([]*WorkItem, error)

	// GetFailedItems returns all failed work items for a job.
	// Returns ErrJobNotFound if the job doesn't exist.
	GetFailedItems(ctx context.Context, jobID string) ([]*WorkItem, error)

	// Close releases any resources held by the queue.
	// After Close is called, all other methods will return ErrQueueClosed.
	Close() error
}

// Options contains configuration options for WorkQueue implementations.
type Options struct {
	// VisibilityTimeout is how long an item remains invisible after Pop.
	// If not acknowledged within this time, the item becomes visible again.
	// Default: 5 minutes.
	VisibilityTimeout time.Duration

	// MaxRetries is the maximum number of times an item can be retried.
	// Default: 3.
	MaxRetries int
}

// DefaultOptions returns the default queue options.
func DefaultOptions() Options {
	return Options{
		VisibilityTimeout: 5 * time.Minute,
		MaxRetries:        3,
	}
}
