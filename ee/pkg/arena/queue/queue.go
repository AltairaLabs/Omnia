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

	// CompleteItem acknowledges a work item and updates accumulators atomically.
	// This is the preferred path over Ack for typed result handling.
	CompleteItem(ctx context.Context, jobID string, itemID string, result *ItemResult) error

	// FailItem marks an item as terminally failed and updates failure accumulators.
	// Unlike Nack, this does not retry — the item is marked as permanently failed.
	FailItem(ctx context.Context, jobID string, itemID string, err error) error

	// GetStats returns the current accumulator statistics for a job.
	// Returns zero-value stats for a new or unknown job.
	GetStats(ctx context.Context, jobID string) (*JobStats, error)

	// Close releases any resources held by the queue.
	// After Close is called, all other methods will return ErrQueueClosed.
	Close() error
}

// ItemResult is the typed execution result shared between worker and aggregator.
type ItemResult struct {
	// Status indicates the execution outcome: "pass" or "fail".
	Status string `json:"status"`

	// DurationMs is the execution time in milliseconds.
	DurationMs float64 `json:"durationMs"`

	// Error contains the error message if execution failed.
	Error string `json:"error,omitempty"`

	// Metrics contains additional numeric metrics like tokens, cost.
	Metrics map[string]float64 `json:"metrics,omitempty"`

	// Assertions contains individual assertion outcomes.
	Assertions []AssertionResult `json:"assertions,omitempty"`

	// SessionID is the optional session identifier for this execution.
	SessionID string `json:"sessionId,omitempty"`
}

// AssertionResult represents a single assertion outcome.
type AssertionResult struct {
	// Name is the assertion identifier or description.
	Name string `json:"name"`

	// Passed indicates whether the assertion passed.
	Passed bool `json:"passed"`

	// Message contains additional details about the assertion result.
	Message string `json:"message,omitempty"`
}

// JobStats contains accumulated statistics readable at any time during or after execution.
type JobStats struct {
	// Total is the total number of completed or failed items.
	Total int64 `json:"total"`

	// Passed is the number of items that passed.
	Passed int64 `json:"passed"`

	// Failed is the number of items that failed.
	Failed int64 `json:"failed"`

	// TotalDurationMs is the sum of all execution durations in milliseconds.
	TotalDurationMs float64 `json:"totalDurationMs"`

	// TotalTokens is the total token count across all executions.
	TotalTokens int64 `json:"totalTokens"`

	// TotalCost is the total cost across all executions.
	TotalCost float64 `json:"totalCost"`

	// ByScenario contains per-scenario statistics.
	ByScenario map[string]*GroupStats `json:"byScenario,omitempty"`

	// ByProvider contains per-provider statistics.
	ByProvider map[string]*GroupStats `json:"byProvider,omitempty"`
}

// GroupStats contains accumulated statistics for a scenario or provider group.
type GroupStats struct {
	// Total is the total number of completed or failed items in this group.
	Total int64 `json:"total"`

	// Passed is the number of items that passed.
	Passed int64 `json:"passed"`

	// Failed is the number of items that failed.
	Failed int64 `json:"failed"`

	// TotalDurationMs is the sum of all execution durations in milliseconds.
	TotalDurationMs float64 `json:"totalDurationMs"`

	// TotalTokens is the total token count.
	TotalTokens int64 `json:"totalTokens"`

	// TotalCost is the total cost.
	TotalCost float64 `json:"totalCost"`
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

// extractTokens returns the token count from a metrics map.
// Checks "totalTokens", "tokens", and the sum of "totalInputTokens" + "totalOutputTokens".
func extractTokens(metrics map[string]float64) int64 {
	if v, ok := metrics["totalTokens"]; ok {
		return int64(v)
	}
	if v, ok := metrics["tokens"]; ok {
		return int64(v)
	}
	// Sum input + output tokens if reported separately.
	var total float64
	if v, ok := metrics["totalInputTokens"]; ok {
		total += v
	}
	if v, ok := metrics["totalOutputTokens"]; ok {
		total += v
	}
	if total > 0 {
		return int64(total)
	}
	return 0
}

// extractCost returns the cost from a metrics map,
// checking both "totalCost" and "cost" keys.
func extractCost(metrics map[string]float64) float64 {
	if v, ok := metrics["totalCost"]; ok {
		return v
	}
	if v, ok := metrics["cost"]; ok {
		return v
	}
	return 0
}

// DefaultOptions returns the default queue options.
func DefaultOptions() Options {
	return Options{
		VisibilityTimeout: 5 * time.Minute,
		MaxRetries:        3,
	}
}
