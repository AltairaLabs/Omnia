/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewMemoryQueue(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	if q == nil {
		t.Fatal("NewMemoryQueueWithDefaults returned nil")
	}
	if q.opts.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", q.opts.MaxRetries)
	}
	if q.opts.VisibilityTimeout != 5*time.Minute {
		t.Errorf("VisibilityTimeout = %v, want 5m", q.opts.VisibilityTimeout)
	}
}

func TestNewMemoryQueueWithOptions(t *testing.T) {
	opts := Options{
		MaxRetries:        5,
		VisibilityTimeout: 10 * time.Minute,
	}
	q := NewMemoryQueue(opts)
	if q.opts.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", q.opts.MaxRetries)
	}
	if q.opts.VisibilityTimeout != 10*time.Minute {
		t.Errorf("VisibilityTimeout = %v, want 10m", q.opts.VisibilityTimeout)
	}
}

func TestMemoryQueuePushAndPop(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1"},
		{ID: "item-2", ScenarioID: "scenario-2"},
		{ID: "item-3", ScenarioID: "scenario-3"},
	}

	if err := q.Push(ctx, "job-1", items); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	// Pop items in FIFO order
	for i, expected := range items {
		item, err := q.Pop(ctx, "job-1")
		if err != nil {
			t.Fatalf("Pop() %d error = %v", i, err)
		}
		if item.ID != expected.ID {
			t.Errorf("Pop() %d ID = %s, want %s", i, item.ID, expected.ID)
		}
		if item.Status != ItemStatusProcessing {
			t.Errorf("Pop() %d Status = %s, want %s", i, item.Status, ItemStatusProcessing)
		}
		if item.JobID != "job-1" {
			t.Errorf("Pop() %d JobID = %s, want job-1", i, item.JobID)
		}
		if item.Attempt != 1 {
			t.Errorf("Pop() %d Attempt = %d, want 1", i, item.Attempt)
		}
	}

	// Queue should be empty now
	_, err := q.Pop(ctx, "job-1")
	if err != ErrQueueEmpty {
		t.Errorf("Pop() on empty queue error = %v, want ErrQueueEmpty", err)
	}
}

func TestMemoryQueuePopNonexistentJob(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	_, err := q.Pop(ctx, "nonexistent-job")
	if err != ErrQueueEmpty {
		t.Errorf("Pop() error = %v, want ErrQueueEmpty", err)
	}
}

func TestMemoryQueueAck(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{{ID: "item-1"}}
	_ = q.Push(ctx, "job-1", items)

	item, _ := q.Pop(ctx, "job-1")

	result := []byte(`{"score": 0.95}`)
	if err := q.Ack(ctx, "job-1", item.ID, result); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	// Verify progress
	progress, _ := q.Progress(ctx, "job-1")
	if progress.Completed != 1 {
		t.Errorf("Progress.Completed = %d, want 1", progress.Completed)
	}
	if progress.Processing != 0 {
		t.Errorf("Progress.Processing = %d, want 0", progress.Processing)
	}
}

func TestMemoryQueueAckNotFound(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	// Ack on nonexistent job
	err := q.Ack(ctx, "nonexistent-job", "item-1", nil)
	if err != ErrJobNotFound {
		t.Errorf("Ack() error = %v, want ErrJobNotFound", err)
	}

	// Create job and try to ack nonexistent item
	_ = q.Push(ctx, "job-1", []WorkItem{{ID: "item-1"}})
	_, _ = q.Pop(ctx, "job-1")

	err = q.Ack(ctx, "job-1", "nonexistent-item", nil)
	if err != ErrItemNotFound {
		t.Errorf("Ack() error = %v, want ErrItemNotFound", err)
	}
}

func TestMemoryQueueNackWithRetry(t *testing.T) {
	q := NewMemoryQueue(Options{MaxRetries: 3})
	ctx := context.Background()

	items := []WorkItem{{ID: "item-1"}}
	_ = q.Push(ctx, "job-1", items)

	// First attempt - should be requeued
	item, _ := q.Pop(ctx, "job-1")
	if item.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1", item.Attempt)
	}

	testErr := errors.New("temporary failure")
	if err := q.Nack(ctx, "job-1", item.ID, testErr); err != nil {
		t.Fatalf("Nack() error = %v", err)
	}

	// Should be back in pending
	progress, _ := q.Progress(ctx, "job-1")
	if progress.Pending != 1 {
		t.Errorf("Progress.Pending = %d, want 1", progress.Pending)
	}

	// Second attempt
	item, _ = q.Pop(ctx, "job-1")
	if item.Attempt != 2 {
		t.Errorf("Attempt = %d, want 2", item.Attempt)
	}
	_ = q.Nack(ctx, "job-1", item.ID, testErr)

	// Third attempt
	item, _ = q.Pop(ctx, "job-1")
	if item.Attempt != 3 {
		t.Errorf("Attempt = %d, want 3", item.Attempt)
	}
	_ = q.Nack(ctx, "job-1", item.ID, testErr)

	// Should now be in failed (max retries exceeded)
	progress, _ = q.Progress(ctx, "job-1")
	if progress.Failed != 1 {
		t.Errorf("Progress.Failed = %d, want 1", progress.Failed)
	}
	if progress.Pending != 0 {
		t.Errorf("Progress.Pending = %d, want 0", progress.Pending)
	}
}

func TestMemoryQueueNackNotFound(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	// Nack on nonexistent job
	err := q.Nack(ctx, "nonexistent-job", "item-1", nil)
	if err != ErrJobNotFound {
		t.Errorf("Nack() error = %v, want ErrJobNotFound", err)
	}

	// Create job and try to nack nonexistent item
	_ = q.Push(ctx, "job-1", []WorkItem{{ID: "item-1"}})
	_, _ = q.Pop(ctx, "job-1")

	err = q.Nack(ctx, "job-1", "nonexistent-item", nil)
	if err != ErrItemNotFound {
		t.Errorf("Nack() error = %v, want ErrItemNotFound", err)
	}
}

func TestMemoryQueueProgress(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1"},
		{ID: "item-2"},
		{ID: "item-3"},
	}
	_ = q.Push(ctx, "job-1", items)

	// Initial progress
	progress, err := q.Progress(ctx, "job-1")
	if err != nil {
		t.Fatalf("Progress() error = %v", err)
	}
	if progress.Total != 3 {
		t.Errorf("Total = %d, want 3", progress.Total)
	}
	if progress.Pending != 3 {
		t.Errorf("Pending = %d, want 3", progress.Pending)
	}

	// Pop one item
	item1, _ := q.Pop(ctx, "job-1")
	progress, _ = q.Progress(ctx, "job-1")
	if progress.Processing != 1 {
		t.Errorf("Processing = %d, want 1", progress.Processing)
	}
	if progress.Pending != 2 {
		t.Errorf("Pending = %d, want 2", progress.Pending)
	}

	// Ack it
	_ = q.Ack(ctx, "job-1", item1.ID, nil)
	progress, _ = q.Progress(ctx, "job-1")
	if progress.Completed != 1 {
		t.Errorf("Completed = %d, want 1", progress.Completed)
	}

	// Complete all items
	item2, _ := q.Pop(ctx, "job-1")
	item3, _ := q.Pop(ctx, "job-1")
	_ = q.Ack(ctx, "job-1", item2.ID, nil)
	_ = q.Ack(ctx, "job-1", item3.ID, nil)

	progress, _ = q.Progress(ctx, "job-1")
	if !progress.IsComplete() {
		t.Error("IsComplete() = false, want true")
	}
	if progress.CompletedAt == nil {
		t.Error("CompletedAt = nil, want non-nil")
	}
}

func TestMemoryQueueProgressNotFound(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	_, err := q.Progress(ctx, "nonexistent-job")
	if err != ErrJobNotFound {
		t.Errorf("Progress() error = %v, want ErrJobNotFound", err)
	}
}

func TestMemoryQueueClose(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	_ = q.Push(ctx, "job-1", []WorkItem{{ID: "item-1"}})

	if err := q.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// All operations should return ErrQueueClosed
	if err := q.Push(ctx, "job-1", nil); err != ErrQueueClosed {
		t.Errorf("Push() after close error = %v, want ErrQueueClosed", err)
	}
	if _, err := q.Pop(ctx, "job-1"); err != ErrQueueClosed {
		t.Errorf("Pop() after close error = %v, want ErrQueueClosed", err)
	}
	if err := q.Ack(ctx, "job-1", "item-1", nil); err != ErrQueueClosed {
		t.Errorf("Ack() after close error = %v, want ErrQueueClosed", err)
	}
	if err := q.Nack(ctx, "job-1", "item-1", nil); err != ErrQueueClosed {
		t.Errorf("Nack() after close error = %v, want ErrQueueClosed", err)
	}
	if _, err := q.Progress(ctx, "job-1"); err != ErrQueueClosed {
		t.Errorf("Progress() after close error = %v, want ErrQueueClosed", err)
	}
}

func TestMemoryQueueConcurrency(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	// Push 100 items
	items := make([]WorkItem, 100)
	for i := range items {
		items[i] = WorkItem{ID: string(rune('A'+i%26)) + string(rune('0'+i/26))}
	}
	_ = q.Push(ctx, "job-1", items)

	// Process items concurrently
	var wg sync.WaitGroup
	processed := make(chan string, 100)

	for range 10 { // 10 workers
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				item, err := q.Pop(ctx, "job-1")
				if err == ErrQueueEmpty {
					return
				}
				if err != nil {
					t.Errorf("Pop() error = %v", err)
					return
				}
				processed <- item.ID
				if err := q.Ack(ctx, "job-1", item.ID, nil); err != nil {
					t.Errorf("Ack() error = %v", err)
				}
			}
		}()
	}

	wg.Wait()
	close(processed)

	// Verify all items were processed
	count := 0
	for range processed {
		count++
	}
	if count != 100 {
		t.Errorf("Processed %d items, want 100", count)
	}

	progress, _ := q.Progress(ctx, "job-1")
	if progress.Completed != 100 {
		t.Errorf("Completed = %d, want 100", progress.Completed)
	}
}

func TestMemoryQueueMultipleJobs(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	// Push items to multiple jobs
	_ = q.Push(ctx, "job-1", []WorkItem{{ID: "1a"}, {ID: "1b"}})
	_ = q.Push(ctx, "job-2", []WorkItem{{ID: "2a"}, {ID: "2b"}, {ID: "2c"}})

	// Verify progress for each job
	p1, _ := q.Progress(ctx, "job-1")
	p2, _ := q.Progress(ctx, "job-2")

	if p1.Total != 2 {
		t.Errorf("job-1 Total = %d, want 2", p1.Total)
	}
	if p2.Total != 3 {
		t.Errorf("job-2 Total = %d, want 3", p2.Total)
	}

	// Process job-1 items
	item, _ := q.Pop(ctx, "job-1")
	_ = q.Ack(ctx, "job-1", item.ID, nil)

	// Verify jobs are independent
	p1, _ = q.Progress(ctx, "job-1")
	p2, _ = q.Progress(ctx, "job-2")

	if p1.Completed != 1 {
		t.Errorf("job-1 Completed = %d, want 1", p1.Completed)
	}
	if p2.Completed != 0 {
		t.Errorf("job-2 Completed = %d, want 0", p2.Completed)
	}
}

func TestMemoryQueueNackWithNilError(t *testing.T) {
	q := NewMemoryQueue(Options{MaxRetries: 1})
	ctx := context.Background()

	_ = q.Push(ctx, "job-1", []WorkItem{{ID: "item-1"}})
	item, _ := q.Pop(ctx, "job-1")

	// Nack with nil error should still work
	if err := q.Nack(ctx, "job-1", item.ID, nil); err != nil {
		t.Fatalf("Nack() error = %v", err)
	}

	progress, _ := q.Progress(ctx, "job-1")
	if progress.Failed != 1 {
		t.Errorf("Failed = %d, want 1", progress.Failed)
	}
}

func TestMemoryQueueGetCompletedItems(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1"},
		{ID: "item-2", ScenarioID: "scenario-2"},
		{ID: "item-3", ScenarioID: "scenario-3"},
	}
	_ = q.Push(ctx, "job-1", items)

	// Complete two items
	item1, _ := q.Pop(ctx, "job-1")
	item2, _ := q.Pop(ctx, "job-1")
	result1 := []byte(`{"score": 0.95}`)
	result2 := []byte(`{"score": 0.85}`)
	_ = q.Ack(ctx, "job-1", item1.ID, result1)
	_ = q.Ack(ctx, "job-1", item2.ID, result2)

	// Get completed items
	completed, err := q.GetCompletedItems(ctx, "job-1")
	if err != nil {
		t.Fatalf("GetCompletedItems() error = %v", err)
	}

	if len(completed) != 2 {
		t.Errorf("GetCompletedItems() returned %d items, want 2", len(completed))
	}

	// Verify result data is preserved
	foundResults := make(map[string]bool)
	for _, item := range completed {
		if item.Result != nil {
			foundResults[string(item.Result)] = true
		}
		if item.Status != ItemStatusCompleted {
			t.Errorf("Item status = %s, want %s", item.Status, ItemStatusCompleted)
		}
	}
	if !foundResults[`{"score": 0.95}`] || !foundResults[`{"score": 0.85}`] {
		t.Error("Result data not preserved in completed items")
	}
}

func TestMemoryQueueGetCompletedItemsNotFound(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	_, err := q.GetCompletedItems(ctx, "nonexistent-job")
	if err != ErrJobNotFound {
		t.Errorf("GetCompletedItems() error = %v, want ErrJobNotFound", err)
	}
}

func TestMemoryQueueGetCompletedItemsClosed(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	_ = q.Close()
	_, err := q.GetCompletedItems(ctx, "job-1")
	if err != ErrQueueClosed {
		t.Errorf("GetCompletedItems() error = %v, want ErrQueueClosed", err)
	}
}

func TestMemoryQueueGetFailedItems(t *testing.T) {
	q := NewMemoryQueue(Options{MaxRetries: 1})
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1"},
		{ID: "item-2", ScenarioID: "scenario-2"},
		{ID: "item-3", ScenarioID: "scenario-3"},
	}
	_ = q.Push(ctx, "job-1", items)

	// Fail two items (max retries = 1, so first Nack fails them)
	item1, _ := q.Pop(ctx, "job-1")
	item2, _ := q.Pop(ctx, "job-1")
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	_ = q.Nack(ctx, "job-1", item1.ID, err1)
	_ = q.Nack(ctx, "job-1", item2.ID, err2)

	// Get failed items
	failed, err := q.GetFailedItems(ctx, "job-1")
	if err != nil {
		t.Fatalf("GetFailedItems() error = %v", err)
	}

	if len(failed) != 2 {
		t.Errorf("GetFailedItems() returned %d items, want 2", len(failed))
	}

	// Verify error data is preserved
	foundErrors := make(map[string]bool)
	for _, item := range failed {
		if item.Error != "" {
			foundErrors[item.Error] = true
		}
		if item.Status != ItemStatusFailed {
			t.Errorf("Item status = %s, want %s", item.Status, ItemStatusFailed)
		}
	}
	if !foundErrors["error 1"] || !foundErrors["error 2"] {
		t.Error("Error data not preserved in failed items")
	}
}

func TestMemoryQueueGetFailedItemsNotFound(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	_, err := q.GetFailedItems(ctx, "nonexistent-job")
	if err != ErrJobNotFound {
		t.Errorf("GetFailedItems() error = %v, want ErrJobNotFound", err)
	}
}

func TestMemoryQueueGetFailedItemsClosed(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	_ = q.Close()
	_, err := q.GetFailedItems(ctx, "job-1")
	if err != ErrQueueClosed {
		t.Errorf("GetFailedItems() error = %v, want ErrQueueClosed", err)
	}
}

func TestMemoryQueueGetItemsEmpty(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	// Push items but don't complete any
	_ = q.Push(ctx, "job-1", []WorkItem{{ID: "item-1"}})

	// Should return empty slice, not error
	completed, err := q.GetCompletedItems(ctx, "job-1")
	if err != nil {
		t.Fatalf("GetCompletedItems() error = %v", err)
	}
	if len(completed) != 0 {
		t.Errorf("GetCompletedItems() returned %d items, want 0", len(completed))
	}

	failed, err := q.GetFailedItems(ctx, "job-1")
	if err != nil {
		t.Fatalf("GetFailedItems() error = %v", err)
	}
	if len(failed) != 0 {
		t.Errorf("GetFailedItems() returned %d items, want 0", len(failed))
	}
}
