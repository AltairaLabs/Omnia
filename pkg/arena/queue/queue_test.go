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
	"encoding/json"
	"testing"
	"time"
)

func TestWorkItemSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	item := WorkItem{
		ID:          "item-1",
		JobID:       "job-1",
		ScenarioID:  "scenario-1",
		ProviderID:  "provider-1",
		BundleURL:   "http://example.com/bundle.tar.gz",
		Config:      []byte(`{"key":"value"}`),
		Status:      ItemStatusPending,
		Attempt:     1,
		MaxAttempts: 3,
		CreatedAt:   now,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal WorkItem: %v", err)
	}

	var decoded WorkItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal WorkItem: %v", err)
	}

	if decoded.ID != item.ID {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID, item.ID)
	}
	if decoded.JobID != item.JobID {
		t.Errorf("JobID mismatch: got %s, want %s", decoded.JobID, item.JobID)
	}
	if decoded.ScenarioID != item.ScenarioID {
		t.Errorf("ScenarioID mismatch: got %s, want %s", decoded.ScenarioID, item.ScenarioID)
	}
	if decoded.Status != item.Status {
		t.Errorf("Status mismatch: got %s, want %s", decoded.Status, item.Status)
	}
}

func TestJobProgressIsComplete(t *testing.T) {
	tests := []struct {
		name     string
		progress JobProgress
		want     bool
	}{
		{
			name: "all completed",
			progress: JobProgress{
				Total:      10,
				Pending:    0,
				Processing: 0,
				Completed:  8,
				Failed:     2,
			},
			want: true,
		},
		{
			name: "still pending",
			progress: JobProgress{
				Total:      10,
				Pending:    5,
				Processing: 0,
				Completed:  5,
				Failed:     0,
			},
			want: false,
		},
		{
			name: "still processing",
			progress: JobProgress{
				Total:      10,
				Pending:    0,
				Processing: 2,
				Completed:  8,
				Failed:     0,
			},
			want: false,
		},
		{
			name: "empty job",
			progress: JobProgress{
				Total:      0,
				Pending:    0,
				Processing: 0,
				Completed:  0,
				Failed:     0,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.progress.IsComplete(); got != tt.want {
				t.Errorf("IsComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJobProgressSuccessRate(t *testing.T) {
	tests := []struct {
		name     string
		progress JobProgress
		want     float64
	}{
		{
			name: "all successful",
			progress: JobProgress{
				Completed: 10,
				Failed:    0,
			},
			want: 100.0,
		},
		{
			name: "all failed",
			progress: JobProgress{
				Completed: 0,
				Failed:    10,
			},
			want: 0.0,
		},
		{
			name: "50% success",
			progress: JobProgress{
				Completed: 5,
				Failed:    5,
			},
			want: 50.0,
		},
		{
			name: "no finished items",
			progress: JobProgress{
				Completed: 0,
				Failed:    0,
			},
			want: 0.0,
		},
		{
			name: "80% success",
			progress: JobProgress{
				Completed: 8,
				Failed:    2,
			},
			want: 80.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.progress.SuccessRate(); got != tt.want {
				t.Errorf("SuccessRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.VisibilityTimeout != 5*time.Minute {
		t.Errorf("VisibilityTimeout = %v, want %v", opts.VisibilityTimeout, 5*time.Minute)
	}
	if opts.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want %d", opts.MaxRetries, 3)
	}
}

func TestItemStatusConstants(t *testing.T) {
	// Verify status constants have expected values
	if ItemStatusPending != "pending" {
		t.Errorf("ItemStatusPending = %s, want pending", ItemStatusPending)
	}
	if ItemStatusProcessing != "processing" {
		t.Errorf("ItemStatusProcessing = %s, want processing", ItemStatusProcessing)
	}
	if ItemStatusCompleted != "completed" {
		t.Errorf("ItemStatusCompleted = %s, want completed", ItemStatusCompleted)
	}
	if ItemStatusFailed != "failed" {
		t.Errorf("ItemStatusFailed = %s, want failed", ItemStatusFailed)
	}
}

// mockQueue is a minimal implementation to verify interface compliance.
type mockQueue struct {
	closed bool
}

func (m *mockQueue) Push(_ context.Context, _ string, _ []WorkItem) error {
	if m.closed {
		return ErrQueueClosed
	}
	return nil
}

func (m *mockQueue) Pop(_ context.Context, _ string) (*WorkItem, error) {
	if m.closed {
		return nil, ErrQueueClosed
	}
	return nil, ErrQueueEmpty
}

func (m *mockQueue) Ack(_ context.Context, _, _ string, _ []byte) error {
	if m.closed {
		return ErrQueueClosed
	}
	return nil
}

func (m *mockQueue) Nack(_ context.Context, _, _ string, _ error) error {
	if m.closed {
		return ErrQueueClosed
	}
	return nil
}

func (m *mockQueue) Progress(_ context.Context, _ string) (*JobProgress, error) {
	if m.closed {
		return nil, ErrQueueClosed
	}
	return nil, ErrJobNotFound
}

func (m *mockQueue) Close() error {
	m.closed = true
	return nil
}

func (m *mockQueue) GetCompletedItems(_ context.Context, _ string) ([]*WorkItem, error) {
	if m.closed {
		return nil, ErrQueueClosed
	}
	return nil, ErrJobNotFound
}

func (m *mockQueue) GetFailedItems(_ context.Context, _ string) ([]*WorkItem, error) {
	if m.closed {
		return nil, ErrQueueClosed
	}
	return nil, ErrJobNotFound
}

// Compile-time check that mockQueue implements WorkQueue.
var _ WorkQueue = (*mockQueue)(nil)

func TestMockQueueImplementsInterface(t *testing.T) {
	var q WorkQueue = &mockQueue{}
	ctx := context.Background()

	// Test basic operations
	if err := q.Push(ctx, "job-1", nil); err != nil {
		t.Errorf("Push() error = %v", err)
	}

	if _, err := q.Pop(ctx, "job-1"); err != ErrQueueEmpty {
		t.Errorf("Pop() error = %v, want ErrQueueEmpty", err)
	}

	if _, err := q.Progress(ctx, "job-1"); err != ErrJobNotFound {
		t.Errorf("Progress() error = %v, want ErrJobNotFound", err)
	}

	// Test after close
	if err := q.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if err := q.Push(ctx, "job-1", nil); err != ErrQueueClosed {
		t.Errorf("Push() after close error = %v, want ErrQueueClosed", err)
	}
}
