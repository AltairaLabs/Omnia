/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package queue

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// MemoryQueue implements WorkQueue using in-memory data structures.
// It is suitable for development, testing, and single-node deployments.
// Not recommended for production multi-worker scenarios.
type MemoryQueue struct {
	mu     sync.RWMutex
	closed bool
	opts   Options

	// jobs maps jobID to job state
	jobs map[string]*jobState
}

// jobState holds the state for a single job's work items.
type jobState struct {
	mu           sync.Mutex
	pending      []*WorkItem          // Items waiting to be processed
	processing   map[string]*WorkItem // Items currently being processed (by itemID)
	completed    map[string]*WorkItem // Successfully completed items
	failed       map[string]*WorkItem // Failed items
	statsCounted map[string]bool      // Item IDs already counted in stats (idempotency guard)
	startedAt    *time.Time
	stats        *JobStats // Accumulated statistics
}

// NewMemoryQueue creates a new in-memory work queue with the given options.
func NewMemoryQueue(opts Options) *MemoryQueue {
	if opts.VisibilityTimeout == 0 {
		opts.VisibilityTimeout = DefaultOptions().VisibilityTimeout
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = DefaultOptions().MaxRetries
	}
	return &MemoryQueue{
		opts: opts,
		jobs: make(map[string]*jobState),
	}
}

// NewMemoryQueueWithDefaults creates a new in-memory work queue with default options.
func NewMemoryQueueWithDefaults() *MemoryQueue {
	return NewMemoryQueue(DefaultOptions())
}

// Push adds work items to the queue for the specified job.
func (q *MemoryQueue) Push(ctx context.Context, jobID string, items []WorkItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrQueueClosed
	}

	state := q.getOrCreateJobState(jobID)
	state.mu.Lock()
	defer state.mu.Unlock()

	now := time.Now()
	for i := range items {
		item := items[i]
		item.JobID = jobID
		item.Status = ItemStatusPending
		item.CreatedAt = now
		if item.MaxAttempts == 0 {
			item.MaxAttempts = q.opts.MaxRetries
		}
		state.pending = append(state.pending, &item)
	}

	return nil
}

// Pop retrieves the next available work item for the specified job.
func (q *MemoryQueue) Pop(ctx context.Context, jobID string) (*WorkItem, error) {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return nil, ErrQueueClosed
	}

	state, exists := q.jobs[jobID]
	q.mu.RUnlock()

	if !exists {
		return nil, ErrQueueEmpty
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if len(state.pending) == 0 {
		return nil, ErrQueueEmpty
	}

	// Pop from front of queue (FIFO)
	item := state.pending[0]
	state.pending = state.pending[1:]

	// Mark as processing
	now := time.Now()
	item.Status = ItemStatusProcessing
	item.StartedAt = &now
	item.Attempt++

	// Track job start time
	if state.startedAt == nil {
		state.startedAt = &now
	}

	state.processing[item.ID] = item

	// Return a copy to prevent external modification
	itemCopy := *item
	return &itemCopy, nil
}

// Ack acknowledges successful processing of a work item.
func (q *MemoryQueue) Ack(ctx context.Context, jobID string, itemID string, result []byte) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return ErrQueueClosed
	}

	state, exists := q.jobs[jobID]
	q.mu.RUnlock()

	if !exists {
		return ErrJobNotFound
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	item, exists := state.processing[itemID]
	if !exists {
		return ErrItemNotFound
	}

	// Mark as completed
	now := time.Now()
	item.Status = ItemStatusCompleted
	item.CompletedAt = &now
	item.Result = result

	delete(state.processing, itemID)
	state.completed[itemID] = item

	return nil
}

// Nack indicates that processing of a work item failed.
func (q *MemoryQueue) Nack(ctx context.Context, jobID string, itemID string, err error) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return ErrQueueClosed
	}

	state, exists := q.jobs[jobID]
	q.mu.RUnlock()

	if !exists {
		return ErrJobNotFound
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	item, exists := state.processing[itemID]
	if !exists {
		return ErrItemNotFound
	}

	delete(state.processing, itemID)

	// Check if we can retry
	if item.Attempt < item.MaxAttempts {
		// Requeue for retry
		item.Status = ItemStatusPending
		item.StartedAt = nil
		if err != nil {
			item.Error = err.Error()
		}
		state.pending = append(state.pending, item)
	} else {
		// Max retries exceeded, mark as failed
		now := time.Now()
		item.Status = ItemStatusFailed
		item.CompletedAt = &now
		if err != nil {
			item.Error = err.Error()
		}
		state.failed[itemID] = item
	}

	return nil
}

// Progress returns the current progress for the specified job.
func (q *MemoryQueue) Progress(ctx context.Context, jobID string) (*JobProgress, error) {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return nil, ErrQueueClosed
	}

	state, exists := q.jobs[jobID]
	q.mu.RUnlock()

	if !exists {
		return nil, ErrJobNotFound
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	progress := &JobProgress{
		JobID:      jobID,
		Pending:    len(state.pending),
		Processing: len(state.processing),
		Completed:  len(state.completed),
		Failed:     len(state.failed),
		StartedAt:  state.startedAt,
	}
	progress.Total = progress.Pending + progress.Processing + progress.Completed + progress.Failed

	// Set completion time if all items are done
	if progress.IsComplete() && progress.Total > 0 {
		progress.CompletedAt = findLatestCompletionTime(state.completed, state.failed)
	}

	return progress, nil
}

// findLatestCompletionTime returns a pointer to the latest completion time from the given item maps.
func findLatestCompletionTime(completed, failed map[string]*WorkItem) *time.Time {
	var latest time.Time
	for _, item := range completed {
		if item.CompletedAt != nil && item.CompletedAt.After(latest) {
			latest = *item.CompletedAt
		}
	}
	for _, item := range failed {
		if item.CompletedAt != nil && item.CompletedAt.After(latest) {
			latest = *item.CompletedAt
		}
	}
	if latest.IsZero() {
		return nil
	}
	return &latest
}

// Close releases resources and marks the queue as closed.
func (q *MemoryQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.jobs = nil
	return nil
}

// getOrCreateJobState returns the job state, creating it if necessary.
// Must be called with q.mu held.
func (q *MemoryQueue) getOrCreateJobState(jobID string) *jobState {
	state, exists := q.jobs[jobID]
	if !exists {
		state = &jobState{
			pending:      make([]*WorkItem, 0),
			processing:   make(map[string]*WorkItem),
			completed:    make(map[string]*WorkItem),
			failed:       make(map[string]*WorkItem),
			statsCounted: make(map[string]bool),
			stats: &JobStats{
				ByScenario: make(map[string]*GroupStats),
				ByProvider: make(map[string]*GroupStats),
			},
		}
		q.jobs[jobID] = state
	}
	return state
}

// GetCompletedItems returns all completed work items for a job.
func (q *MemoryQueue) GetCompletedItems(ctx context.Context, jobID string) ([]*WorkItem, error) {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return nil, ErrQueueClosed
	}

	state, exists := q.jobs[jobID]
	q.mu.RUnlock()

	if !exists {
		return nil, ErrJobNotFound
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	items := make([]*WorkItem, 0, len(state.completed))
	for _, item := range state.completed {
		// Return a copy to prevent external modification
		itemCopy := *item
		items = append(items, &itemCopy)
	}

	return items, nil
}

// GetFailedItems returns all failed work items for a job.
func (q *MemoryQueue) GetFailedItems(ctx context.Context, jobID string) ([]*WorkItem, error) {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return nil, ErrQueueClosed
	}

	state, exists := q.jobs[jobID]
	q.mu.RUnlock()

	if !exists {
		return nil, ErrJobNotFound
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	items := make([]*WorkItem, 0, len(state.failed))
	for _, item := range state.failed {
		// Return a copy to prevent external modification
		itemCopy := *item
		items = append(items, &itemCopy)
	}

	return items, nil
}

// CompleteItem acknowledges a work item and updates accumulators atomically.
func (q *MemoryQueue) CompleteItem(ctx context.Context, jobID string, itemID string, result *ItemResult) error {
	// Marshal result to JSON for the Ack path
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}

	// Do normal Ack bookkeeping
	if err := q.Ack(ctx, jobID, itemID, resultJSON); err != nil {
		return err
	}

	// Update accumulators
	q.mu.RLock()
	state := q.jobs[jobID]
	q.mu.RUnlock()

	state.mu.Lock()
	defer state.mu.Unlock()

	// Idempotent: skip stats if this item was already counted.
	if state.statsCounted[itemID] {
		return nil
	}
	state.statsCounted[itemID] = true

	item := state.completed[itemID]
	q.updateMemoryStats(state.stats, item, result)

	return nil
}

// FailItem marks an item as terminally failed and updates failure accumulators.
func (q *MemoryQueue) FailItem(ctx context.Context, jobID string, itemID string, failErr error) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return ErrQueueClosed
	}

	state, exists := q.jobs[jobID]
	q.mu.RUnlock()

	if !exists {
		return ErrJobNotFound
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	item, exists := state.processing[itemID]
	if !exists {
		return ErrItemNotFound
	}

	// Mark as terminally failed (no retry)
	now := time.Now()
	item.Status = ItemStatusFailed
	item.CompletedAt = &now
	if failErr != nil {
		item.Error = failErr.Error()
	}

	delete(state.processing, itemID)
	state.failed[itemID] = item

	// Idempotent: skip stats if this item was already counted.
	if !state.statsCounted[itemID] {
		state.statsCounted[itemID] = true
		q.incrementFailureStats(state.stats, item)
	}

	return nil
}

// GetStats returns the current accumulator statistics for a job.
func (q *MemoryQueue) GetStats(_ context.Context, jobID string) (*JobStats, error) {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return nil, ErrQueueClosed
	}

	state, exists := q.jobs[jobID]
	q.mu.RUnlock()

	if !exists {
		// Return zero stats for unknown jobs
		return &JobStats{
			ByScenario: make(map[string]*GroupStats),
			ByProvider: make(map[string]*GroupStats),
		}, nil
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	// Return a deep copy
	return q.copyJobStats(state.stats), nil
}

// updateMemoryStats updates accumulated stats for a completed item.
func (q *MemoryQueue) updateMemoryStats(stats *JobStats, item *WorkItem, result *ItemResult) {
	stats.Total++
	stats.TotalDurationMs += result.DurationMs

	if result.Status == "pass" {
		stats.Passed++
	} else {
		stats.Failed++
	}

	tokens := extractTokens(result.Metrics)
	cost := extractCost(result.Metrics)
	stats.TotalTokens += tokens
	stats.TotalCost += cost

	// Update scenario stats
	if item.ScenarioID != "" {
		gs := q.getOrCreateGroupStats(stats.ByScenario, item.ScenarioID)
		q.updateGroupStats(gs, result, tokens, cost)
	}

	// Update provider stats
	if item.ProviderID != "" {
		gs := q.getOrCreateGroupStats(stats.ByProvider, item.ProviderID)
		q.updateGroupStats(gs, result, tokens, cost)
	}
}

// incrementFailureStats updates accumulated stats for a failed item.
func (q *MemoryQueue) incrementFailureStats(stats *JobStats, item *WorkItem) {
	stats.Total++
	stats.Failed++

	if item.ScenarioID != "" {
		gs := q.getOrCreateGroupStats(stats.ByScenario, item.ScenarioID)
		gs.Total++
		gs.Failed++
	}

	if item.ProviderID != "" {
		gs := q.getOrCreateGroupStats(stats.ByProvider, item.ProviderID)
		gs.Total++
		gs.Failed++
	}
}

// getOrCreateGroupStats returns or creates a GroupStats entry in the given map.
func (q *MemoryQueue) getOrCreateGroupStats(m map[string]*GroupStats, key string) *GroupStats {
	gs, ok := m[key]
	if !ok {
		gs = &GroupStats{}
		m[key] = gs
	}
	return gs
}

// updateGroupStats updates a GroupStats from an ItemResult.
func (q *MemoryQueue) updateGroupStats(gs *GroupStats, result *ItemResult, tokens int64, cost float64) {
	gs.Total++
	gs.TotalDurationMs += result.DurationMs
	gs.TotalTokens += tokens
	gs.TotalCost += cost

	if result.Status == "pass" {
		gs.Passed++
	} else {
		gs.Failed++
	}
}

// copyJobStats returns a deep copy of JobStats.
func (q *MemoryQueue) copyJobStats(src *JobStats) *JobStats {
	dst := &JobStats{
		Total:           src.Total,
		Passed:          src.Passed,
		Failed:          src.Failed,
		TotalDurationMs: src.TotalDurationMs,
		TotalTokens:     src.TotalTokens,
		TotalCost:       src.TotalCost,
		ByScenario:      make(map[string]*GroupStats, len(src.ByScenario)),
		ByProvider:      make(map[string]*GroupStats, len(src.ByProvider)),
	}
	for k, v := range src.ByScenario {
		cp := *v
		dst.ByScenario[k] = &cp
	}
	for k, v := range src.ByProvider {
		cp := *v
		dst.ByProvider[k] = &cp
	}
	return dst
}

// Ensure MemoryQueue implements WorkQueue interface.
var _ WorkQueue = (*MemoryQueue)(nil)
