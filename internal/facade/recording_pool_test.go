/*
Copyright 2026.

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

package facade

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordingPool_SubmitAndClose(t *testing.T) {
	pool := NewRecordingPool(4, 100, logr.Discard())

	var count atomic.Int32
	for range 50 {
		pool.Submit(func() {
			count.Add(1)
		})
	}

	pool.Close()
	assert.Equal(t, int32(50), count.Load(), "all tasks should be executed")
}

func TestRecordingPool_DropWhenFull(t *testing.T) {
	// Create a pool with 1 worker and queue of 1, then block the worker
	pool := NewRecordingPool(1, 1, logr.Discard())

	blocker := make(chan struct{})
	pool.Submit(func() {
		<-blocker // Block the single worker
	})

	// Wait for the worker to pick up the blocking task
	time.Sleep(50 * time.Millisecond)

	// Fill the queue
	pool.Submit(func() {})

	// This should be dropped (queue full, worker busy)
	var dropped atomic.Bool
	dropped.Store(true)
	pool.Submit(func() {
		dropped.Store(false) // If this runs, it wasn't dropped
	})

	// Unblock and close
	close(blocker)
	pool.Close()

	// The dropped task should not have run
	assert.True(t, dropped.Load(), "task should have been dropped when queue was full")
}

func TestRecordingPool_DefaultValues(t *testing.T) {
	pool := NewRecordingPool(0, 0, logr.Discard())
	require.NotNil(t, pool)
	assert.Equal(t, DefaultRecordingQueueSize, cap(pool.queue))
	pool.Close()
}
