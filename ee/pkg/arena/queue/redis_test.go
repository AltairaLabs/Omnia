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
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestRedisClient returns a Redis client for testing.
// It skips the test if Redis is not available.
// It also cleans up any stale arena keys from previous test runs.
func getTestRedisClient(t *testing.T) *redis.Client {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available at %s: %v", addr, err)
	}

	// Clean up stale keys from previous test runs
	keys, err := client.Keys(ctx, "arena:*").Result()
	if err == nil && len(keys) > 0 {
		_ = client.Del(ctx, keys...)
	}

	return client
}

// cleanupRedisKeys removes all test keys from Redis.
func cleanupRedisKeys(t *testing.T, client *redis.Client) {
	ctx := context.Background()
	keys, err := client.Keys(ctx, "arena:*").Result()
	if err != nil {
		t.Logf("Warning: failed to get keys for cleanup: %v", err)
		return
	}
	if len(keys) > 0 {
		_ = client.Del(ctx, keys...)
	}
}

func TestRedisQueue_Push(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())

	ctx := context.Background()
	jobID := "test-job-push"

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-2", ScenarioID: "scenario-2", ProviderID: "provider-1"},
	}

	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	// Verify items were added to pending queue
	pendingLen, err := client.LLen(ctx, q.pendingKey(jobID)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(2), pendingLen)

	// Verify item data was stored
	item1, err := q.getItem(ctx, "item-1")
	require.NoError(t, err)
	assert.Equal(t, "item-1", item1.ID)
	assert.Equal(t, jobID, item1.JobID)
	assert.Equal(t, ItemStatusPending, item1.Status)
	assert.Equal(t, 3, item1.MaxAttempts) // Default
}

func TestRedisQueue_Pop(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-pop"

	// Push items
	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-2", ScenarioID: "scenario-2", ProviderID: "provider-1"},
	}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	// Pop first item
	item, err := q.Pop(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, "item-1", item.ID)
	assert.Equal(t, ItemStatusProcessing, item.Status)
	assert.Equal(t, 1, item.Attempt)
	assert.NotNil(t, item.StartedAt)

	// Pop second item
	item, err = q.Pop(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, "item-2", item.ID)

	// Pop from empty queue
	_, err = q.Pop(ctx, jobID)
	assert.Equal(t, ErrQueueEmpty, err)
}

func TestRedisQueue_Ack(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-ack"

	// Push and pop an item
	items := []WorkItem{{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"}}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	item, err := q.Pop(ctx, jobID)
	require.NoError(t, err)

	// Ack the item
	result := []byte(`{"success": true}`)
	err = q.Ack(ctx, jobID, item.ID, result)
	require.NoError(t, err)

	// Verify item is in completed set
	isMember, err := client.SIsMember(ctx, q.completedKey(jobID), item.ID).Result()
	require.NoError(t, err)
	assert.True(t, isMember)

	// Verify item data was updated
	updatedItem, err := q.getItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ItemStatusCompleted, updatedItem.Status)
	assert.NotNil(t, updatedItem.CompletedAt)
	assert.Equal(t, result, updatedItem.Result)
}

func TestRedisQueue_Nack_Retry(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, Options{MaxRetries: 3})
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-nack-retry"

	// Push and pop an item
	items := []WorkItem{{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"}}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	item, err := q.Pop(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 1, item.Attempt)

	// Nack the item (should retry)
	err = q.Nack(ctx, jobID, item.ID, errors.New("temporary error"))
	require.NoError(t, err)

	// Verify item is back in pending queue
	pendingLen, err := client.LLen(ctx, q.pendingKey(jobID)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), pendingLen)

	// Pop again
	item, err = q.Pop(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 2, item.Attempt)
}

func TestRedisQueue_Nack_MaxRetries(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, Options{MaxRetries: 2})
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-nack-max"

	// Push and pop an item
	items := []WorkItem{{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"}}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	// Exhaust retries
	for range 2 {
		item, err := q.Pop(ctx, jobID)
		require.NoError(t, err)
		err = q.Nack(ctx, jobID, item.ID, errors.New("error"))
		require.NoError(t, err)
	}

	// Verify item is in failed set
	isMember, err := client.SIsMember(ctx, q.failedKey(jobID), "item-1").Result()
	require.NoError(t, err)
	assert.True(t, isMember)

	// Verify queue is empty
	_, err = q.Pop(ctx, jobID)
	assert.Equal(t, ErrQueueEmpty, err)
}

func TestRedisQueue_Progress(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-progress"

	// Push items
	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-2", ScenarioID: "scenario-2", ProviderID: "provider-1"},
		{ID: "item-3", ScenarioID: "scenario-3", ProviderID: "provider-1"},
	}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	// Check initial progress
	progress, err := q.Progress(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 3, progress.Total)
	assert.Equal(t, 3, progress.Pending)
	assert.Equal(t, 0, progress.Processing)
	assert.Equal(t, 0, progress.Completed)
	assert.Equal(t, 0, progress.Failed)
	assert.Nil(t, progress.StartedAt)

	// Pop one item
	item, err := q.Pop(ctx, jobID)
	require.NoError(t, err)

	progress, err = q.Progress(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 2, progress.Pending)
	assert.Equal(t, 1, progress.Processing)
	assert.NotNil(t, progress.StartedAt)

	// Ack the item
	err = q.Ack(ctx, jobID, item.ID, nil)
	require.NoError(t, err)

	progress, err = q.Progress(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 2, progress.Pending)
	assert.Equal(t, 0, progress.Processing)
	assert.Equal(t, 1, progress.Completed)
}

func TestRedisQueue_Progress_JobNotFound(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()

	_, err := q.Progress(ctx, "nonexistent-job")
	assert.Equal(t, ErrJobNotFound, err)
}

func TestRedisQueue_Close(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)

	// Create a new client for this test since we'll close it
	testClient := redis.NewClient(&redis.Options{
		Addr: client.Options().Addr,
	})

	q := NewRedisQueueFromClient(testClient, DefaultOptions())

	ctx := context.Background()
	jobID := "test-job-close"

	// Push items
	items := []WorkItem{{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"}}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	// Close the queue
	err = q.Close()
	require.NoError(t, err)

	// Operations should fail
	_, err = q.Pop(ctx, jobID)
	assert.Equal(t, ErrQueueClosed, err)

	err = q.Push(ctx, jobID, items)
	assert.Equal(t, ErrQueueClosed, err)
}

func TestRedisQueue_RequeueTimedOutItems(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	// Use a very short visibility timeout for testing
	q := NewRedisQueueFromClient(client, Options{
		VisibilityTimeout: 100 * time.Millisecond,
		MaxRetries:        3,
	})
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-timeout"

	// Push and pop an item
	items := []WorkItem{{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"}}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	_, err = q.Pop(ctx, jobID)
	require.NoError(t, err)

	// Queue should be empty
	_, err = q.Pop(ctx, jobID)
	assert.Equal(t, ErrQueueEmpty, err)

	// Wait for visibility timeout
	time.Sleep(150 * time.Millisecond)

	// Requeue timed out items
	requeued, err := q.RequeueTimedOutItems(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 1, requeued)

	// Item should be back in queue
	item, err := q.Pop(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, "item-1", item.ID)
}

func TestRedisQueue_FIFO(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-fifo"

	// Push items in order
	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-2", ScenarioID: "scenario-2", ProviderID: "provider-1"},
		{ID: "item-3", ScenarioID: "scenario-3", ProviderID: "provider-1"},
	}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	// Pop should return in FIFO order
	item1, _ := q.Pop(ctx, jobID)
	item2, _ := q.Pop(ctx, jobID)
	item3, _ := q.Pop(ctx, jobID)

	assert.Equal(t, "item-1", item1.ID)
	assert.Equal(t, "item-2", item2.ID)
	assert.Equal(t, "item-3", item3.ID)
}

func TestRedisQueue_ConcurrentPush(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-concurrent"

	// Push items concurrently from multiple goroutines
	done := make(chan bool, 10)
	for i := range 10 {
		go func(n int) {
			items := []WorkItem{
				{ID: "item-" + string(rune('a'+n)), ScenarioID: "scenario", ProviderID: "provider"},
			}
			err := q.Push(ctx, jobID, items)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}

	// Verify all items were added
	progress, err := q.Progress(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 10, progress.Total)
}

func TestNewRedisQueue(t *testing.T) {
	// Test successful connection
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	// First check if Redis is available
	testClient := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := testClient.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available at %s: %v", addr, err)
	}
	_ = testClient.Close()

	q, err := NewRedisQueue(RedisOptions{
		Addr: addr,
		Options: Options{
			VisibilityTimeout: 30 * time.Second,
			MaxRetries:        5,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, q)
	defer func() { _ = q.Close() }()

	// Verify options were set
	assert.Equal(t, 5, q.opts.MaxRetries)
	assert.Equal(t, 30*time.Second, q.opts.VisibilityTimeout)
}

func TestNewRedisQueue_ConnectionError(t *testing.T) {
	// Test connection to non-existent Redis
	q, err := NewRedisQueue(RedisOptions{
		Addr: "localhost:59999", // Invalid port
	})
	require.Error(t, err)
	require.Nil(t, q)
	assert.Contains(t, err.Error(), "failed to connect to Redis")
}

func TestRedisQueue_Progress_Complete(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-complete"

	// Push items
	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-2", ScenarioID: "scenario-2", ProviderID: "provider-1"},
	}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	// Pop and ack all items
	item1, err := q.Pop(ctx, jobID)
	require.NoError(t, err)
	err = q.Ack(ctx, jobID, item1.ID, []byte("result1"))
	require.NoError(t, err)

	item2, err := q.Pop(ctx, jobID)
	require.NoError(t, err)
	err = q.Ack(ctx, jobID, item2.ID, []byte("result2"))
	require.NoError(t, err)

	// Check progress shows completion
	progress, err := q.Progress(ctx, jobID)
	require.NoError(t, err)
	assert.True(t, progress.IsComplete())
	assert.Equal(t, 2, progress.Completed)
	assert.NotNil(t, progress.CompletedAt)
}

func TestRedisQueue_Progress_WithFailed(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, Options{MaxRetries: 1})
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-with-failed"

	// Push items
	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-2", ScenarioID: "scenario-2", ProviderID: "provider-1"},
	}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	// Pop and ack first item
	item1, err := q.Pop(ctx, jobID)
	require.NoError(t, err)
	err = q.Ack(ctx, jobID, item1.ID, nil)
	require.NoError(t, err)

	// Pop and fail second item (exhaust retries)
	item2, err := q.Pop(ctx, jobID)
	require.NoError(t, err)
	err = q.Nack(ctx, jobID, item2.ID, errors.New("permanent error"))
	require.NoError(t, err)

	// Check progress shows completion with failure
	progress, err := q.Progress(ctx, jobID)
	require.NoError(t, err)
	assert.True(t, progress.IsComplete())
	assert.Equal(t, 1, progress.Completed)
	assert.Equal(t, 1, progress.Failed)
	assert.NotNil(t, progress.CompletedAt)
}

func TestRedisQueue_Ack_ItemNotFound(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-ack-notfound"

	// Try to ack a non-existent item
	err := q.Ack(ctx, jobID, "nonexistent-item", nil)
	assert.Equal(t, ErrItemNotFound, err)
}

func TestRedisQueue_Nack_ItemNotFound(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-nack-notfound"

	// Try to nack a non-existent item
	err := q.Nack(ctx, jobID, "nonexistent-item", errors.New("error"))
	assert.Equal(t, ErrItemNotFound, err)
}

func TestRedisQueue_Push_Empty(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-empty-push"

	// Push empty slice should succeed without error
	err := q.Push(ctx, jobID, []WorkItem{})
	require.NoError(t, err)
}

func TestRedisQueue_GetCompletedItems(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-completed-items"

	// Push and complete some items
	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-2", ScenarioID: "scenario-2", ProviderID: "provider-1"},
		{ID: "item-3", ScenarioID: "scenario-3", ProviderID: "provider-1"},
	}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	// Complete 2 items
	item1, _ := q.Pop(ctx, jobID)
	item2, _ := q.Pop(ctx, jobID)
	_ = q.Ack(ctx, jobID, item1.ID, []byte(`{"result": "success"}`))
	_ = q.Ack(ctx, jobID, item2.ID, []byte(`{"result": "ok"}`))

	// Get completed items
	completed, err := q.GetCompletedItems(ctx, jobID)
	require.NoError(t, err)
	assert.Len(t, completed, 2)

	// Verify results are preserved
	for _, item := range completed {
		assert.Equal(t, ItemStatusCompleted, item.Status)
		assert.NotNil(t, item.Result)
	}
}

func TestRedisQueue_GetCompletedItems_Empty(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-completed-empty"

	// Push but don't complete any items
	items := []WorkItem{{ID: "item-1"}}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	// Get completed items should return empty slice
	completed, err := q.GetCompletedItems(ctx, jobID)
	require.NoError(t, err)
	assert.Empty(t, completed)
}

func TestRedisQueue_GetCompletedItems_JobNotFound(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()

	_, err := q.GetCompletedItems(ctx, "nonexistent-job")
	assert.Equal(t, ErrJobNotFound, err)
}

func TestRedisQueue_GetFailedItems(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, Options{MaxRetries: 1})
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	jobID := "test-job-failed-items"

	// Push and fail some items
	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-2", ScenarioID: "scenario-2", ProviderID: "provider-1"},
	}
	err := q.Push(ctx, jobID, items)
	require.NoError(t, err)

	// Fail 2 items (max retries = 1, so first nack fails them)
	item1, _ := q.Pop(ctx, jobID)
	item2, _ := q.Pop(ctx, jobID)
	_ = q.Nack(ctx, jobID, item1.ID, errors.New("error 1"))
	_ = q.Nack(ctx, jobID, item2.ID, errors.New("error 2"))

	// Get failed items
	failed, err := q.GetFailedItems(ctx, jobID)
	require.NoError(t, err)
	assert.Len(t, failed, 2)

	// Verify error is preserved
	for _, item := range failed {
		assert.Equal(t, ItemStatusFailed, item.Status)
		assert.NotEmpty(t, item.Error)
	}
}

func TestRedisQueue_GetFailedItems_JobNotFound(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	defer func() { _ = q.Close() }()

	ctx := context.Background()

	_, err := q.GetFailedItems(ctx, "nonexistent-job")
	assert.Equal(t, ErrJobNotFound, err)
}

func TestRedisQueue_GetItems_Closed(t *testing.T) {
	client := getTestRedisClient(t)
	defer cleanupRedisKeys(t, client)
	defer func() { _ = client.Close() }()

	q := NewRedisQueueFromClient(client, DefaultOptions())
	_ = q.Close()

	ctx := context.Background()

	_, err := q.GetCompletedItems(ctx, "job-1")
	assert.Equal(t, ErrQueueClosed, err)

	_, err = q.GetFailedItems(ctx, "job-1")
	assert.Equal(t, ErrQueueClosed, err)
}
