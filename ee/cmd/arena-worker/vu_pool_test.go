/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

const testJobID = "test-job"

// pushTestItems is a helper that pushes n work items to the queue.
func pushTestItems(t *testing.T, q *queue.MemoryQueue, n int) {
	t.Helper()
	items := make([]queue.WorkItem, n)
	for i := range n {
		items[i] = queue.WorkItem{
			ID:          fmt.Sprintf("item-%d", i),
			ScenarioID:  "scenario-1",
			ProviderID:  "provider-1",
			MaxAttempts: 1,
		}
	}
	require.NoError(t, q.Push(context.Background(), testJobID, items))
}

// newTestMetrics creates WorkerMetrics with an isolated registry for tests.
func newTestMetrics() *WorkerMetrics {
	return newWorkerMetricsWithRegisterer(prometheus.NewRegistry())
}

// concurrencyTracker provides a reusable execute function that tracks
// peak concurrency and total items processed.
type concurrencyTracker struct {
	processed         atomic.Int32
	maxConcurrent     atomic.Int32
	currentConcurrent atomic.Int32
	workDuration      time.Duration
}

func (ct *concurrencyTracker) execute(_ context.Context, _ *queue.WorkItem) (*ExecutionResult, error) {
	cur := ct.currentConcurrent.Add(1)
	for {
		old := ct.maxConcurrent.Load()
		if cur <= old || ct.maxConcurrent.CompareAndSwap(old, cur) {
			break
		}
	}
	time.Sleep(ct.workDuration)
	ct.currentConcurrent.Add(-1)
	ct.processed.Add(1)
	return &ExecutionResult{Status: statusPass, DurationMs: float64(ct.workDuration.Milliseconds())}, nil
}

func TestVUPool_SingleVU(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	defer func() { require.NoError(t, q.Close()) }()

	pushTestItems(t, q, 5)

	var processed atomic.Int32
	pool := NewVUPool(VUPoolConfig{
		Size:         1,
		Concurrency:  0,
		Queue:        q,
		JobID:        testJobID,
		Log:          testr.New(t),
		Metrics:      newTestMetrics(),
		PollInterval: 10 * time.Millisecond,
		Execute: func(_ context.Context, _ *queue.WorkItem) (*ExecutionResult, error) {
			processed.Add(1)
			return &ExecutionResult{Status: statusPass, DurationMs: 1}, nil
		},
	})

	err := pool.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(5), processed.Load())
}

func TestVUPool_MultipleVUs(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	defer func() { require.NoError(t, q.Close()) }()

	pushTestItems(t, q, 10)

	ct := &concurrencyTracker{workDuration: 50 * time.Millisecond}
	pool := NewVUPool(VUPoolConfig{
		Size:         3,
		Concurrency:  0,
		Queue:        q,
		JobID:        testJobID,
		Log:          testr.New(t),
		Metrics:      newTestMetrics(),
		PollInterval: 10 * time.Millisecond,
		Execute:      ct.execute,
	})

	err := pool.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(10), ct.processed.Load())
	assert.Greater(t, ct.maxConcurrent.Load(), int32(1), "expected concurrent execution with 3 VUs")
}

func TestVUPool_GracefulShutdown(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	defer func() { require.NoError(t, q.Close()) }()

	pushTestItems(t, q, 20)

	var processed atomic.Int32
	startedCh := make(chan struct{})

	pool := NewVUPool(VUPoolConfig{
		Size:         2,
		Concurrency:  0,
		Queue:        q,
		JobID:        testJobID,
		Log:          testr.New(t),
		Metrics:      newTestMetrics(),
		PollInterval: 10 * time.Millisecond,
		Execute: func(_ context.Context, _ *queue.WorkItem) (*ExecutionResult, error) {
			select {
			case startedCh <- struct{}{}:
			default:
			}
			time.Sleep(100 * time.Millisecond)
			processed.Add(1)
			return &ExecutionResult{Status: statusPass, DurationMs: 100}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- pool.Run(ctx)
	}()

	// Wait for at least one item to start processing, then cancel.
	<-startedCh
	cancel()

	err := <-done
	require.NoError(t, err)

	p := processed.Load()
	assert.Greater(t, p, int32(0), "expected at least one item processed before shutdown")
	assert.Less(t, p, int32(20), "expected shutdown to stop before processing all items")
}

func TestVUPool_ConcurrencyLimit(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	defer func() { require.NoError(t, q.Close()) }()

	pushTestItems(t, q, 6)

	ct := &concurrencyTracker{workDuration: 80 * time.Millisecond}
	pool := NewVUPool(VUPoolConfig{
		Size:         4,
		Concurrency:  2,
		Queue:        q,
		JobID:        testJobID,
		Log:          testr.New(t),
		Metrics:      newTestMetrics(),
		PollInterval: 10 * time.Millisecond,
		Execute:      ct.execute,
	})

	err := pool.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(6), ct.processed.Load())
	assert.LessOrEqual(t, ct.maxConcurrent.Load(), int32(2),
		"concurrency limit should cap simultaneous processing")
}

func TestVUPool_ExecuteError(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	defer func() { require.NoError(t, q.Close()) }()

	pushTestItems(t, q, 3)

	var processed atomic.Int32
	pool := NewVUPool(VUPoolConfig{
		Size:         2,
		Concurrency:  0,
		Queue:        q,
		JobID:        testJobID,
		Log:          testr.New(t),
		Metrics:      newTestMetrics(),
		PollInterval: 10 * time.Millisecond,
		Execute: func(_ context.Context, item *queue.WorkItem) (*ExecutionResult, error) {
			processed.Add(1)
			if item.ID == "item-1" {
				return nil, fmt.Errorf("simulated failure")
			}
			return &ExecutionResult{Status: statusPass, DurationMs: 1}, nil
		},
	})

	err := pool.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(3), processed.Load())

	failed, fErr := q.GetFailedItems(context.Background(), testJobID)
	require.NoError(t, fErr)
	assert.Len(t, failed, 1)
	assert.Equal(t, "item-1", failed[0].ID)
}

func TestVUPool_SizeDefaultsToOne(t *testing.T) {
	pool := NewVUPool(VUPoolConfig{
		Size: 0,
	})
	assert.Equal(t, 1, pool.size)
}

func TestVUPool_EmptyQueue(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	defer func() { require.NoError(t, q.Close()) }()

	require.NoError(t, q.Push(context.Background(), testJobID, []queue.WorkItem{}))

	pool := NewVUPool(VUPoolConfig{
		Size:         2,
		Concurrency:  0,
		Queue:        q,
		JobID:        testJobID,
		Log:          testr.New(t),
		Metrics:      newTestMetrics(),
		PollInterval: 10 * time.Millisecond,
		Execute: func(_ context.Context, _ *queue.WorkItem) (*ExecutionResult, error) {
			t.Fatal("execute should not be called on empty queue")
			return nil, nil
		},
	})

	err := pool.Run(context.Background())
	require.NoError(t, err)
}
