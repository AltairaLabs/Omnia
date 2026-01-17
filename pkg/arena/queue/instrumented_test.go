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
	"errors"
	"testing"
)

// testJobID is a constant job ID used across tests.
const testJobID = "job-1"

// mockMetrics tracks method calls for testing.
type mockMetrics struct {
	operations       []operationCall
	statusChanges    []statusChangeCall
	itemsPushed      []itemsPushedCall
	retries          []string
	activeJobsChange int
}

type operationCall struct {
	operation       string
	durationSeconds float64
	success         bool
}

type statusChangeCall struct {
	jobID     string
	oldStatus ItemStatus
	newStatus ItemStatus
}

type itemsPushedCall struct {
	jobID string
	count int
}

func newMockMetrics() *mockMetrics {
	return &mockMetrics{}
}

func (m *mockMetrics) RecordOperation(operation string, durationSeconds float64, success bool) {
	m.operations = append(m.operations, operationCall{
		operation:       operation,
		durationSeconds: durationSeconds,
		success:         success,
	})
}

func (m *mockMetrics) RecordItemStatusChange(jobID string, oldStatus, newStatus ItemStatus) {
	m.statusChanges = append(m.statusChanges, statusChangeCall{
		jobID:     jobID,
		oldStatus: oldStatus,
		newStatus: newStatus,
	})
}

func (m *mockMetrics) RecordItemsPushed(jobID string, count int) {
	m.itemsPushed = append(m.itemsPushed, itemsPushedCall{
		jobID: jobID,
		count: count,
	})
}

func (m *mockMetrics) RecordRetry(jobID string) {
	m.retries = append(m.retries, jobID)
}

func (m *mockMetrics) IncrementActiveJobs() {
	m.activeJobsChange++
}

func (m *mockMetrics) DecrementActiveJobs() {
	m.activeJobsChange--
}

func TestInstrumentedQueuePush(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()
	items := []WorkItem{
		{ID: "item-1"},
		{ID: "item-2"},
		{ID: "item-3"},
	}

	err := q.Push(ctx, testJobID, items)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	// Verify operation was recorded
	if len(metrics.operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(metrics.operations))
	}

	op := metrics.operations[0]
	if op.operation != OpPush {
		t.Errorf("Operation = %s, want %s", op.operation, OpPush)
	}
	if !op.success {
		t.Error("Success = false, want true")
	}
	if op.durationSeconds <= 0 {
		t.Error("Duration should be positive")
	}

	// Verify items pushed was recorded
	if len(metrics.itemsPushed) != 1 {
		t.Fatalf("Expected 1 itemsPushed call, got %d", len(metrics.itemsPushed))
	}
	if metrics.itemsPushed[0].jobID != testJobID {
		t.Errorf("JobID = %s, want testJobID", metrics.itemsPushed[0].jobID)
	}
	if metrics.itemsPushed[0].count != 3 {
		t.Errorf("Count = %d, want 3", metrics.itemsPushed[0].count)
	}
}

func TestInstrumentedQueuePushError(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	_ = innerQueue.Close() // Close to force errors
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()
	items := []WorkItem{{ID: "item-1"}}

	err := q.Push(ctx, testJobID, items)
	if err != ErrQueueClosed {
		t.Fatalf("Push() error = %v, want ErrQueueClosed", err)
	}

	// Verify failure was recorded
	if len(metrics.operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(metrics.operations))
	}

	op := metrics.operations[0]
	if op.success {
		t.Error("Success = true, want false")
	}

	// Items pushed should not be recorded on error
	if len(metrics.itemsPushed) != 0 {
		t.Errorf("Expected 0 itemsPushed calls, got %d", len(metrics.itemsPushed))
	}
}

func TestInstrumentedQueuePop(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()
	items := []WorkItem{{ID: "item-1"}}
	_ = innerQueue.Push(ctx, testJobID, items)

	// Clear metrics from push
	metrics.operations = nil
	metrics.itemsPushed = nil

	item, err := q.Pop(ctx, testJobID)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if item.ID != "item-1" {
		t.Errorf("Item.ID = %s, want item-1", item.ID)
	}

	// Verify operation was recorded
	if len(metrics.operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(metrics.operations))
	}

	op := metrics.operations[0]
	if op.operation != OpPop {
		t.Errorf("Operation = %s, want %s", op.operation, OpPop)
	}
	if !op.success {
		t.Error("Success = false, want true")
	}

	// Verify status change was recorded
	if len(metrics.statusChanges) != 1 {
		t.Fatalf("Expected 1 status change, got %d", len(metrics.statusChanges))
	}

	sc := metrics.statusChanges[0]
	if sc.jobID != testJobID {
		t.Errorf("JobID = %s, want testJobID", sc.jobID)
	}
	if sc.oldStatus != ItemStatusPending {
		t.Errorf("OldStatus = %s, want %s", sc.oldStatus, ItemStatusPending)
	}
	if sc.newStatus != ItemStatusProcessing {
		t.Errorf("NewStatus = %s, want %s", sc.newStatus, ItemStatusProcessing)
	}
}

func TestInstrumentedQueuePopEmpty(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()

	_, err := q.Pop(ctx, testJobID)
	if err != ErrQueueEmpty {
		t.Fatalf("Pop() error = %v, want ErrQueueEmpty", err)
	}

	// Verify operation was recorded as success (ErrQueueEmpty is expected)
	if len(metrics.operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(metrics.operations))
	}

	op := metrics.operations[0]
	if !op.success {
		t.Error("Success = false, want true (ErrQueueEmpty is expected behavior)")
	}

	// No status change should be recorded
	if len(metrics.statusChanges) != 0 {
		t.Errorf("Expected 0 status changes, got %d", len(metrics.statusChanges))
	}
}

func TestInstrumentedQueueAck(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()
	items := []WorkItem{{ID: "item-1"}}
	_ = innerQueue.Push(ctx, testJobID, items)
	item, _ := innerQueue.Pop(ctx, testJobID)

	// Clear metrics
	metrics.operations = nil
	metrics.statusChanges = nil

	result := []byte(`{"success": true}`)
	err := q.Ack(ctx, testJobID, item.ID, result)
	if err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	// Verify operation was recorded
	if len(metrics.operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(metrics.operations))
	}

	op := metrics.operations[0]
	if op.operation != OpAck {
		t.Errorf("Operation = %s, want %s", op.operation, OpAck)
	}
	if !op.success {
		t.Error("Success = false, want true")
	}

	// Verify status change was recorded
	if len(metrics.statusChanges) != 1 {
		t.Fatalf("Expected 1 status change, got %d", len(metrics.statusChanges))
	}

	sc := metrics.statusChanges[0]
	if sc.oldStatus != ItemStatusProcessing {
		t.Errorf("OldStatus = %s, want %s", sc.oldStatus, ItemStatusProcessing)
	}
	if sc.newStatus != ItemStatusCompleted {
		t.Errorf("NewStatus = %s, want %s", sc.newStatus, ItemStatusCompleted)
	}
}

func TestInstrumentedQueueAckError(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()

	// Ack on nonexistent item
	err := q.Ack(ctx, testJobID, "nonexistent", nil)
	if err != ErrJobNotFound {
		t.Fatalf("Ack() error = %v, want ErrJobNotFound", err)
	}

	// Verify failure was recorded
	if len(metrics.operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(metrics.operations))
	}

	op := metrics.operations[0]
	if op.success {
		t.Error("Success = true, want false")
	}

	// No status change should be recorded on error
	if len(metrics.statusChanges) != 0 {
		t.Errorf("Expected 0 status changes, got %d", len(metrics.statusChanges))
	}
}

func TestInstrumentedQueueNack(t *testing.T) {
	innerQueue := NewMemoryQueue(Options{MaxRetries: 3})
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()
	items := []WorkItem{{ID: "item-1"}}
	_ = innerQueue.Push(ctx, testJobID, items)
	item, _ := innerQueue.Pop(ctx, testJobID)

	// Clear metrics
	metrics.operations = nil
	metrics.statusChanges = nil

	testErr := errors.New("temporary failure")
	err := q.Nack(ctx, testJobID, item.ID, testErr)
	if err != nil {
		t.Fatalf("Nack() error = %v", err)
	}

	// Verify operation was recorded
	if len(metrics.operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(metrics.operations))
	}

	op := metrics.operations[0]
	if op.operation != OpNack {
		t.Errorf("Operation = %s, want %s", op.operation, OpNack)
	}
	if !op.success {
		t.Error("Success = false, want true")
	}

	// Verify retry was recorded
	if len(metrics.retries) != 1 {
		t.Fatalf("Expected 1 retry, got %d", len(metrics.retries))
	}
	if metrics.retries[0] != testJobID {
		t.Errorf("Retry jobID = %s, want testJobID", metrics.retries[0])
	}

	// Verify status change was recorded
	if len(metrics.statusChanges) != 1 {
		t.Fatalf("Expected 1 status change, got %d", len(metrics.statusChanges))
	}

	sc := metrics.statusChanges[0]
	if sc.oldStatus != ItemStatusProcessing {
		t.Errorf("OldStatus = %s, want %s", sc.oldStatus, ItemStatusProcessing)
	}
}

func TestInstrumentedQueueNackError(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()

	// Nack on nonexistent item
	err := q.Nack(ctx, testJobID, "nonexistent", nil)
	if err != ErrJobNotFound {
		t.Fatalf("Nack() error = %v, want ErrJobNotFound", err)
	}

	// Verify failure was recorded
	if len(metrics.operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(metrics.operations))
	}

	op := metrics.operations[0]
	if op.success {
		t.Error("Success = true, want false")
	}

	// No retry or status change should be recorded on error
	if len(metrics.retries) != 0 {
		t.Errorf("Expected 0 retries, got %d", len(metrics.retries))
	}
	if len(metrics.statusChanges) != 0 {
		t.Errorf("Expected 0 status changes, got %d", len(metrics.statusChanges))
	}
}

func TestInstrumentedQueueProgress(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()
	items := []WorkItem{{ID: "item-1"}, {ID: "item-2"}}
	_ = innerQueue.Push(ctx, testJobID, items)

	progress, err := q.Progress(ctx, testJobID)
	if err != nil {
		t.Fatalf("Progress() error = %v", err)
	}
	if progress.Total != 2 {
		t.Errorf("Progress.Total = %d, want 2", progress.Total)
	}

	// Progress should not record any operations (read-only)
	if len(metrics.operations) != 0 {
		t.Errorf("Expected 0 operations for Progress, got %d", len(metrics.operations))
	}
}

func TestInstrumentedQueueClose(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()
	_ = innerQueue.Push(ctx, testJobID, []WorkItem{{ID: "item-1"}})

	err := q.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Verify inner queue is closed
	_, err = q.Pop(ctx, testJobID)
	if err != ErrQueueClosed {
		t.Errorf("Pop() after Close() error = %v, want ErrQueueClosed", err)
	}
}

func TestInstrumentedQueueFullWorkflow(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()

	// Push items
	items := []WorkItem{{ID: "item-1"}, {ID: "item-2"}}
	_ = q.Push(ctx, testJobID, items)

	// Pop and ack first item
	item1, _ := q.Pop(ctx, testJobID)
	_ = q.Ack(ctx, testJobID, item1.ID, nil)

	// Pop and nack second item
	item2, _ := q.Pop(ctx, testJobID)
	_ = q.Nack(ctx, testJobID, item2.ID, errors.New("fail"))

	// Verify all operations were recorded
	expectedOps := map[string]int{
		OpPush: 1,
		OpPop:  2,
		OpAck:  1,
		OpNack: 1,
	}

	opCounts := make(map[string]int)
	for _, op := range metrics.operations {
		opCounts[op.operation]++
	}

	for opName, expected := range expectedOps {
		if opCounts[opName] != expected {
			t.Errorf("Operation %s count = %d, want %d", opName, opCounts[opName], expected)
		}
	}

	// Verify items pushed
	if len(metrics.itemsPushed) != 1 || metrics.itemsPushed[0].count != 2 {
		t.Error("Items pushed not recorded correctly")
	}

	// Verify retries
	if len(metrics.retries) != 1 {
		t.Errorf("Retries count = %d, want 1", len(metrics.retries))
	}
}

func TestNoOpQueueMetrics(t *testing.T) {
	// Verify NoOpQueueMetrics can be used without panicking
	m := &NoOpQueueMetrics{}

	// These should all be no-ops
	m.RecordOperation(OpPush, 0.001, true)
	m.RecordItemStatusChange(testJobID, ItemStatusPending, ItemStatusProcessing)
	m.RecordItemsPushed(testJobID, 5)
	m.RecordRetry(testJobID)
	m.IncrementActiveJobs()
	m.DecrementActiveJobs()
}

func TestInstrumentedQueueWithNoOpMetrics(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	metrics := &NoOpQueueMetrics{}
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()

	// Verify queue works normally with no-op metrics
	items := []WorkItem{{ID: "item-1"}}
	if err := q.Push(ctx, testJobID, items); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	item, err := q.Pop(ctx, testJobID)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if item.ID != "item-1" {
		t.Errorf("Item.ID = %s, want item-1", item.ID)
	}

	if err := q.Ack(ctx, testJobID, item.ID, nil); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	progress, err := q.Progress(ctx, testJobID)
	if err != nil {
		t.Fatalf("Progress() error = %v", err)
	}
	if progress.Completed != 1 {
		t.Errorf("Completed = %d, want 1", progress.Completed)
	}
}

func TestInstrumentedQueueGetCompletedItems(t *testing.T) {
	innerQueue := NewMemoryQueueWithDefaults()
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()
	items := []WorkItem{{ID: "item-1"}, {ID: "item-2"}}
	_ = innerQueue.Push(ctx, testJobID, items)

	// Complete one item
	item, _ := innerQueue.Pop(ctx, testJobID)
	_ = innerQueue.Ack(ctx, testJobID, item.ID, []byte(`{"result": "success"}`))

	completed, err := q.GetCompletedItems(ctx, testJobID)
	if err != nil {
		t.Fatalf("GetCompletedItems() error = %v", err)
	}
	if len(completed) != 1 {
		t.Errorf("GetCompletedItems() returned %d items, want 1", len(completed))
	}

	// Read-only operations should not record metrics
	if len(metrics.operations) != 0 {
		t.Errorf("Expected 0 operations, got %d", len(metrics.operations))
	}
}

func TestInstrumentedQueueGetFailedItems(t *testing.T) {
	innerQueue := NewMemoryQueue(Options{MaxRetries: 1})
	metrics := newMockMetrics()
	q := NewInstrumentedQueue(innerQueue, metrics)

	ctx := context.Background()
	items := []WorkItem{{ID: "item-1"}}
	_ = innerQueue.Push(ctx, testJobID, items)

	// Fail one item
	item, _ := innerQueue.Pop(ctx, testJobID)
	_ = innerQueue.Nack(ctx, testJobID, item.ID, errors.New("test error"))

	failed, err := q.GetFailedItems(ctx, testJobID)
	if err != nil {
		t.Fatalf("GetFailedItems() error = %v", err)
	}
	if len(failed) != 1 {
		t.Errorf("GetFailedItems() returned %d items, want 1", len(failed))
	}

	// Read-only operations should not record metrics
	if len(metrics.operations) != 0 {
		t.Errorf("Expected 0 operations, got %d", len(metrics.operations))
	}
}
