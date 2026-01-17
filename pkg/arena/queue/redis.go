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
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis key prefixes and patterns.
const (
	keyPrefix        = "arena:"
	jobKeyPrefix     = keyPrefix + "job:"
	itemKeyPrefix    = keyPrefix + "item:"
	pendingKeySuffix = ":pending"
	processingKey    = ":processing"
	completedKey     = ":completed"
	failedKey        = ":failed"
	metaKey          = ":meta"
)

// RedisQueue implements WorkQueue using Redis for distributed queue operations.
// It is suitable for production multi-worker deployments with horizontal scaling.
type RedisQueue struct {
	client *redis.Client
	opts   Options
	mu     sync.RWMutex
	closed bool
}

// RedisOptions contains Redis-specific configuration options.
type RedisOptions struct {
	// Addr is the Redis server address (host:port).
	Addr string

	// Password is the Redis password (optional).
	Password string

	// DB is the Redis database number.
	DB int

	// Options contains common queue options.
	Options Options
}

// NewRedisQueue creates a new Redis-backed work queue.
func NewRedisQueue(redisOpts RedisOptions) (*RedisQueue, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     redisOpts.Addr,
		Password: redisOpts.Password,
		DB:       redisOpts.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	opts := redisOpts.Options
	if opts.VisibilityTimeout == 0 {
		opts.VisibilityTimeout = DefaultOptions().VisibilityTimeout
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = DefaultOptions().MaxRetries
	}

	return &RedisQueue{
		client: client,
		opts:   opts,
	}, nil
}

// NewRedisQueueFromClient creates a new Redis-backed work queue from an existing client.
// This is useful for testing or when you want to share a Redis connection pool.
func NewRedisQueueFromClient(client *redis.Client, opts Options) *RedisQueue {
	if opts.VisibilityTimeout == 0 {
		opts.VisibilityTimeout = DefaultOptions().VisibilityTimeout
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = DefaultOptions().MaxRetries
	}

	return &RedisQueue{
		client: client,
		opts:   opts,
	}
}

// Push adds work items to the queue for the specified job.
func (q *RedisQueue) Push(ctx context.Context, jobID string, items []WorkItem) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return ErrQueueClosed
	}
	q.mu.RUnlock()

	if len(items) == 0 {
		return nil
	}

	pipe := q.client.Pipeline()
	now := time.Now()
	pendingKey := q.pendingKey(jobID)

	for i := range items {
		item := items[i]
		item.JobID = jobID
		item.Status = ItemStatusPending
		item.CreatedAt = now
		if item.MaxAttempts == 0 {
			item.MaxAttempts = q.opts.MaxRetries
		}

		// Serialize and store the item
		itemData, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("failed to marshal work item: %w", err)
		}

		// Store item data
		pipe.Set(ctx, q.itemKey(item.ID), itemData, 0)

		// Add to pending queue (LPUSH for FIFO with RPOP)
		pipe.LPush(ctx, pendingKey, item.ID)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to push items to Redis: %w", err)
	}

	return nil
}

// Pop retrieves the next available work item for the specified job.
func (q *RedisQueue) Pop(ctx context.Context, jobID string) (*WorkItem, error) {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return nil, ErrQueueClosed
	}
	q.mu.RUnlock()

	pendingKey := q.pendingKey(jobID)
	processingKey := q.processingKey(jobID)

	// Pop from pending queue (RPOP for FIFO)
	itemID, err := q.client.RPopLPush(ctx, pendingKey, processingKey).Result()
	if err == redis.Nil {
		return nil, ErrQueueEmpty
	}
	if err != nil {
		return nil, fmt.Errorf("failed to pop from queue: %w", err)
	}

	// Get and update the item
	item, err := q.getItem(ctx, itemID)
	if err != nil {
		// Item data missing, remove from processing and return error
		q.client.LRem(ctx, processingKey, 1, itemID)
		return nil, fmt.Errorf("failed to get item data: %w", err)
	}

	// Update item status
	now := time.Now()
	item.Status = ItemStatusProcessing
	item.StartedAt = &now
	item.Attempt++

	// Save updated item
	if err := q.saveItem(ctx, item); err != nil {
		return nil, fmt.Errorf("failed to update item: %w", err)
	}

	// Track processing start time with visibility timeout
	score := float64(now.Add(q.opts.VisibilityTimeout).UnixNano())
	q.client.ZAdd(ctx, q.processingZSetKey(jobID), redis.Z{
		Score:  score,
		Member: itemID,
	})

	// Update job start time if this is the first item
	q.client.HSetNX(ctx, q.metaKey(jobID), "startedAt", now.UnixNano())

	return item, nil
}

// Ack acknowledges successful processing of a work item.
func (q *RedisQueue) Ack(ctx context.Context, jobID string, itemID string, result []byte) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return ErrQueueClosed
	}
	q.mu.RUnlock()

	// Check if item exists in processing
	removed, err := q.client.ZRem(ctx, q.processingZSetKey(jobID), itemID).Result()
	if err != nil {
		return fmt.Errorf("failed to remove from processing: %w", err)
	}
	if removed == 0 {
		return ErrItemNotFound
	}

	// Also remove from processing list
	q.client.LRem(ctx, q.processingKey(jobID), 1, itemID)

	// Get and update the item
	item, err := q.getItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}

	// Mark as completed
	now := time.Now()
	item.Status = ItemStatusCompleted
	item.CompletedAt = &now
	item.Result = result

	// Save updated item
	if err := q.saveItem(ctx, item); err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	// Add to completed set
	q.client.SAdd(ctx, q.completedKey(jobID), itemID)

	return nil
}

// Nack indicates that processing of a work item failed.
func (q *RedisQueue) Nack(ctx context.Context, jobID string, itemID string, errMsg error) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return ErrQueueClosed
	}
	q.mu.RUnlock()

	// Check if item exists in processing
	removed, err := q.client.ZRem(ctx, q.processingZSetKey(jobID), itemID).Result()
	if err != nil {
		return fmt.Errorf("failed to remove from processing: %w", err)
	}
	if removed == 0 {
		return ErrItemNotFound
	}

	// Also remove from processing list
	q.client.LRem(ctx, q.processingKey(jobID), 1, itemID)

	// Get the item
	item, err := q.getItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}

	// Check if we can retry
	if item.Attempt < item.MaxAttempts {
		// Requeue for retry
		item.Status = ItemStatusPending
		item.StartedAt = nil
		if errMsg != nil {
			item.Error = errMsg.Error()
		}

		// Save updated item
		if err := q.saveItem(ctx, item); err != nil {
			return fmt.Errorf("failed to update item: %w", err)
		}

		// Add back to pending queue
		q.client.LPush(ctx, q.pendingKey(jobID), itemID)
	} else {
		// Max retries exceeded, mark as failed
		now := time.Now()
		item.Status = ItemStatusFailed
		item.CompletedAt = &now
		if errMsg != nil {
			item.Error = errMsg.Error()
		}

		// Save updated item
		if err := q.saveItem(ctx, item); err != nil {
			return fmt.Errorf("failed to update item: %w", err)
		}

		// Add to failed set
		q.client.SAdd(ctx, q.failedKey(jobID), itemID)
	}

	return nil
}

// Progress returns the current progress for the specified job.
func (q *RedisQueue) Progress(ctx context.Context, jobID string) (*JobProgress, error) {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return nil, ErrQueueClosed
	}
	q.mu.RUnlock()

	pipe := q.client.Pipeline()

	pendingCmd := pipe.LLen(ctx, q.pendingKey(jobID))
	processingCmd := pipe.ZCard(ctx, q.processingZSetKey(jobID))
	completedCmd := pipe.SCard(ctx, q.completedKey(jobID))
	failedCmd := pipe.SCard(ctx, q.failedKey(jobID))
	metaCmd := pipe.HGetAll(ctx, q.metaKey(jobID))

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get progress: %w", err)
	}

	pending := int(pendingCmd.Val())
	processing := int(processingCmd.Val())
	completed := int(completedCmd.Val())
	failed := int(failedCmd.Val())
	total := pending + processing + completed + failed

	// If no items exist for this job, return job not found
	if total == 0 {
		// Check if the job metadata exists
		if len(metaCmd.Val()) == 0 {
			return nil, ErrJobNotFound
		}
	}

	progress := &JobProgress{
		JobID:      jobID,
		Total:      total,
		Pending:    pending,
		Processing: processing,
		Completed:  completed,
		Failed:     failed,
	}

	// Parse metadata
	meta := metaCmd.Val()
	if startedAtStr, ok := meta["startedAt"]; ok {
		if startedAtNano, err := strconv.ParseInt(startedAtStr, 10, 64); err == nil {
			startedAt := time.Unix(0, startedAtNano)
			progress.StartedAt = &startedAt
		}
	}

	// Set completion time if all items are done
	if progress.IsComplete() && progress.Total > 0 {
		// Find the latest completion time from completed and failed items
		latestCompletion := q.findLatestCompletionTime(ctx, jobID)
		if !latestCompletion.IsZero() {
			progress.CompletedAt = &latestCompletion
		}
	}

	return progress, nil
}

// Close releases resources and marks the queue as closed.
func (q *RedisQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	return q.client.Close()
}

// Helper methods for key generation.

func (q *RedisQueue) pendingKey(jobID string) string {
	return jobKeyPrefix + jobID + pendingKeySuffix
}

func (q *RedisQueue) processingKey(jobID string) string {
	return jobKeyPrefix + jobID + processingKey
}

func (q *RedisQueue) processingZSetKey(jobID string) string {
	return jobKeyPrefix + jobID + ":processing_zset"
}

func (q *RedisQueue) completedKey(jobID string) string {
	return jobKeyPrefix + jobID + completedKey
}

func (q *RedisQueue) failedKey(jobID string) string {
	return jobKeyPrefix + jobID + failedKey
}

func (q *RedisQueue) metaKey(jobID string) string {
	return jobKeyPrefix + jobID + metaKey
}

func (q *RedisQueue) itemKey(itemID string) string {
	return itemKeyPrefix + itemID
}

// Helper methods for item operations.

func (q *RedisQueue) getItem(ctx context.Context, itemID string) (*WorkItem, error) {
	data, err := q.client.Get(ctx, q.itemKey(itemID)).Bytes()
	if err == redis.Nil {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}

	var item WorkItem
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, err
	}

	return &item, nil
}

func (q *RedisQueue) saveItem(ctx context.Context, item *WorkItem) error {
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return q.client.Set(ctx, q.itemKey(item.ID), data, 0).Err()
}

func (q *RedisQueue) findLatestCompletionTime(ctx context.Context, jobID string) time.Time {
	var latestCompletion time.Time

	// Check completed items
	completedIDs, _ := q.client.SMembers(ctx, q.completedKey(jobID)).Result()
	for _, itemID := range completedIDs {
		item, err := q.getItem(ctx, itemID)
		if err == nil && item.CompletedAt != nil && item.CompletedAt.After(latestCompletion) {
			latestCompletion = *item.CompletedAt
		}
	}

	// Check failed items
	failedIDs, _ := q.client.SMembers(ctx, q.failedKey(jobID)).Result()
	for _, itemID := range failedIDs {
		item, err := q.getItem(ctx, itemID)
		if err == nil && item.CompletedAt != nil && item.CompletedAt.After(latestCompletion) {
			latestCompletion = *item.CompletedAt
		}
	}

	return latestCompletion
}

// RequeueTimedOutItems moves items that have exceeded their visibility timeout
// back to the pending queue. This should be called periodically by a background
// goroutine to handle workers that crashed or timed out.
func (q *RedisQueue) RequeueTimedOutItems(ctx context.Context, jobID string) (int, error) {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return 0, ErrQueueClosed
	}
	q.mu.RUnlock()

	now := time.Now()
	maxScore := float64(now.UnixNano())

	// Get items that have exceeded visibility timeout
	itemIDs, err := q.client.ZRangeByScore(ctx, q.processingZSetKey(jobID), &redis.ZRangeBy{
		Min: "-inf",
		Max: fmt.Sprintf("%f", maxScore),
	}).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get timed out items: %w", err)
	}

	if len(itemIDs) == 0 {
		return 0, nil
	}

	requeued := 0
	for _, itemID := range itemIDs {
		// Remove from processing
		removed, err := q.client.ZRem(ctx, q.processingZSetKey(jobID), itemID).Result()
		if err != nil || removed == 0 {
			continue
		}
		q.client.LRem(ctx, q.processingKey(jobID), 1, itemID)

		// Get the item
		item, err := q.getItem(ctx, itemID)
		if err != nil {
			continue
		}

		// Reset status and requeue
		item.Status = ItemStatusPending
		item.StartedAt = nil

		if err := q.saveItem(ctx, item); err != nil {
			continue
		}

		q.client.LPush(ctx, q.pendingKey(jobID), itemID)
		requeued++
	}

	return requeued, nil
}

// Ensure RedisQueue implements WorkQueue interface.
var _ WorkQueue = (*RedisQueue)(nil)
