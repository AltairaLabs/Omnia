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
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
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

	// defaultItemTTL is the default TTL for queue items stored in Redis.
	// Items older than this are automatically expired to prevent memory leaks.
	defaultItemTTL = 24 * time.Hour

	// sscanCount is the count hint for SScan iteration.
	sscanCount = 100

	// getItemsBatchSize is the batch size for pipelined GET calls.
	getItemsBatchSize = 100
)

// RedisQueue implements WorkQueue using Redis for distributed queue operations.
// It is suitable for production multi-worker deployments with horizontal scaling.
type RedisQueue struct {
	client  *redis.Client
	opts    Options
	itemTTL time.Duration
	mu      sync.RWMutex
	closed  bool
}

// RedisOptions contains Redis-specific configuration options.
type RedisOptions struct {
	// Addr is the Redis server address (host:port).
	Addr string

	// Password is the Redis password (optional).
	Password string

	// DB is the Redis database number.
	DB int

	// ItemTTL is the TTL for queue items stored in Redis.
	// Defaults to 24 hours if zero.
	ItemTTL time.Duration

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
	// Instrument Redis client for OTel tracing.
	if err := redisotel.InstrumentTracing(client); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to instrument redis tracing: %w", err)
	}

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

	itemTTL := redisOpts.ItemTTL
	if itemTTL == 0 {
		itemTTL = defaultItemTTL
	}

	return &RedisQueue{
		client:  client,
		opts:    opts,
		itemTTL: itemTTL,
	}, nil
}

// NewRedisQueueFromClient creates a new Redis-backed work queue from an existing client.
// This is useful for testing or when you want to share a Redis connection pool.
func NewRedisQueueFromClient(client *redis.Client, opts Options) *RedisQueue {
	return NewRedisQueueFromClientWithTTL(client, opts, defaultItemTTL)
}

// NewRedisQueueFromClientWithTTL creates a new Redis-backed work queue with a custom item TTL.
func NewRedisQueueFromClientWithTTL(client *redis.Client, opts Options, itemTTL time.Duration) *RedisQueue {
	if opts.VisibilityTimeout == 0 {
		opts.VisibilityTimeout = DefaultOptions().VisibilityTimeout
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = DefaultOptions().MaxRetries
	}
	if itemTTL == 0 {
		itemTTL = defaultItemTTL
	}

	return &RedisQueue{
		client:  client,
		opts:    opts,
		itemTTL: itemTTL,
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

	// Store job metadata so Progress() can identify the job exists
	metaKey := q.metaKey(jobID)
	pipe.HSet(ctx, metaKey, map[string]interface{}{
		"totalItems": len(items),
		"createdAt":  now.Format(time.RFC3339),
	})
	pipe.Expire(ctx, metaKey, q.itemTTL)

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

		// Store item data with TTL
		pipe.Set(ctx, q.itemKey(item.ID), itemData, q.itemTTL)

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
	processingZKey := q.processingZSetKey(jobID)
	score := float64(now.Add(q.opts.VisibilityTimeout).UnixNano())
	q.client.ZAdd(ctx, processingZKey, redis.Z{
		Score:  score,
		Member: itemID,
	})
	q.client.Expire(ctx, processingZKey, q.itemTTL)

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

	// Add to completed set with TTL
	completedSetKey := q.completedKey(jobID)
	q.client.SAdd(ctx, completedSetKey, itemID)
	q.client.Expire(ctx, completedSetKey, q.itemTTL)

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

		// Add to failed set with TTL
		failedSetKey := q.failedKey(jobID)
		q.client.SAdd(ctx, failedSetKey, itemID)
		q.client.Expire(ctx, failedSetKey, q.itemTTL)
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
	return q.client.Set(ctx, q.itemKey(item.ID), data, q.itemTTL).Err()
}

func (q *RedisQueue) findLatestCompletionTime(ctx context.Context, jobID string) time.Time {
	var latestCompletion time.Time

	// Check completed items using cursor-based iteration
	q.scanSetForLatestCompletion(ctx, q.completedKey(jobID), &latestCompletion)

	// Check failed items using cursor-based iteration
	q.scanSetForLatestCompletion(ctx, q.failedKey(jobID), &latestCompletion)

	return latestCompletion
}

// scanSetForLatestCompletion iterates a set with SScan and finds the latest completion time.
func (q *RedisQueue) scanSetForLatestCompletion(ctx context.Context, setKey string, latest *time.Time) {
	var cursor uint64
	for {
		ids, nextCursor, err := q.client.SScan(ctx, setKey, cursor, "", sscanCount).Result()
		if err != nil {
			return
		}

		// Batch GET for this chunk of IDs
		q.updateLatestFromIDs(ctx, ids, latest)

		cursor = nextCursor
		if cursor == 0 {
			return
		}
	}
}

// updateLatestFromIDs uses a pipeline to batch-GET items and update the latest completion time.
func (q *RedisQueue) updateLatestFromIDs(ctx context.Context, ids []string, latest *time.Time) {
	if len(ids) == 0 {
		return
	}

	pipe := q.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(ids))
	for i, itemID := range ids {
		cmds[i] = pipe.Get(ctx, q.itemKey(itemID))
	}
	_, _ = pipe.Exec(ctx)

	for _, cmd := range cmds {
		data, err := cmd.Bytes()
		if err != nil {
			continue
		}
		var item WorkItem
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		if item.CompletedAt != nil && item.CompletedAt.After(*latest) {
			*latest = *item.CompletedAt
		}
	}
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

// GetCompletedItems returns all completed work items for a job.
func (q *RedisQueue) GetCompletedItems(ctx context.Context, jobID string) ([]*WorkItem, error) {
	return q.getItemsFromSet(ctx, jobID, q.completedKey(jobID), "completed")
}

// GetFailedItems returns all failed work items for a job.
func (q *RedisQueue) GetFailedItems(ctx context.Context, jobID string) ([]*WorkItem, error) {
	return q.getItemsFromSet(ctx, jobID, q.failedKey(jobID), "failed")
}

// getItemsFromSet retrieves all work items from a Redis set for a job.
func (q *RedisQueue) getItemsFromSet(ctx context.Context, jobID, setKey, itemType string) ([]*WorkItem, error) {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return nil, ErrQueueClosed
	}
	q.mu.RUnlock()

	// Use SScan to iterate the set in chunks instead of loading all at once
	var allItems []*WorkItem
	var cursor uint64
	firstIteration := true

	for {
		ids, nextCursor, err := q.client.SScan(ctx, setKey, cursor, "", sscanCount).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan %s items: %w", itemType, err)
		}

		if firstIteration && len(ids) == 0 && nextCursor == 0 {
			// Set is empty or doesn't exist — check if job exists
			if err := q.checkJobExists(ctx, jobID); err != nil {
				return nil, err
			}
			return []*WorkItem{}, nil
		}
		firstIteration = false

		// Batch GET for this chunk of IDs
		items := q.batchGetItems(ctx, ids)
		allItems = append(allItems, items...)

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return allItems, nil
}

// checkJobExists verifies that a job exists by checking for any job-related keys.
func (q *RedisQueue) checkJobExists(ctx context.Context, jobID string) error {
	pipe := q.client.Pipeline()
	metaExists := pipe.Exists(ctx, q.metaKey(jobID))
	pendingExists := pipe.Exists(ctx, q.pendingKey(jobID))
	processingExists := pipe.Exists(ctx, q.processingKey(jobID))
	completedExists := pipe.Exists(ctx, q.completedKey(jobID))
	failedExists := pipe.Exists(ctx, q.failedKey(jobID))

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to check job existence: %w", err)
	}

	if metaExists.Val() == 0 && pendingExists.Val() == 0 && processingExists.Val() == 0 &&
		completedExists.Val() == 0 && failedExists.Val() == 0 {
		return ErrJobNotFound
	}
	return nil
}

// batchGetItems uses a pipeline to batch-GET items by their IDs.
func (q *RedisQueue) batchGetItems(ctx context.Context, ids []string) []*WorkItem {
	if len(ids) == 0 {
		return nil
	}

	var allItems []*WorkItem

	// Process in batches
	for i := 0; i < len(ids); i += getItemsBatchSize {
		end := i + getItemsBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		pipe := q.client.Pipeline()
		cmds := make([]*redis.StringCmd, len(batch))
		for j, itemID := range batch {
			cmds[j] = pipe.Get(ctx, q.itemKey(itemID))
		}
		_, _ = pipe.Exec(ctx)

		for _, cmd := range cmds {
			data, err := cmd.Bytes()
			if err != nil {
				continue
			}
			var item WorkItem
			if err := json.Unmarshal(data, &item); err != nil {
				continue
			}
			allItems = append(allItems, &item)
		}
	}

	return allItems
}

// Redis key constants for accumulator stats.
const (
	statsKeySuffix          = ":stats"
	statsScenarioKeyInfix   = ":stats:scenario:"
	statsProviderKeyInfix   = ":stats:provider:"
	statsFieldTotal         = "total"
	statsFieldPassed        = "passed"
	statsFieldFailed        = "failed"
	statsFieldTotalDuration = "totalDurationMs"
	statsFieldTotalTokens   = "totalTokens"
	statsFieldTotalCost     = "totalCost"
)

func (q *RedisQueue) statsKey(jobID string) string {
	return jobKeyPrefix + jobID + statsKeySuffix
}

func (q *RedisQueue) statsScenarioKey(jobID, scenarioID string) string {
	return jobKeyPrefix + jobID + statsScenarioKeyInfix + scenarioID
}

func (q *RedisQueue) statsProviderKey(jobID, providerID string) string {
	return jobKeyPrefix + jobID + statsProviderKeyInfix + providerID
}

// CompleteItem acknowledges a work item and updates accumulators atomically.
func (q *RedisQueue) CompleteItem(ctx context.Context, jobID string, itemID string, result *ItemResult) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return ErrQueueClosed
	}
	q.mu.RUnlock()

	// Remove from processing zset
	removed, err := q.client.ZRem(ctx, q.processingZSetKey(jobID), itemID).Result()
	if err != nil {
		return fmt.Errorf("failed to remove from processing: %w", err)
	}
	if removed == 0 {
		return ErrItemNotFound
	}

	// Remove from processing list
	q.client.LRem(ctx, q.processingKey(jobID), 1, itemID)

	// Get the item for scenarioID/providerID
	item, err := q.getItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}

	// Marshal result JSON and update item
	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal result: %w", marshalErr)
	}

	now := time.Now()
	item.Status = ItemStatusCompleted
	item.CompletedAt = &now
	item.Result = resultJSON

	// Check idempotency: skip stats increment if this item was already counted
	// (e.g., due to re-enqueue after partial failure or duplicate processing).
	alreadyCounted, markErr := q.markStatsCounted(ctx, jobID, itemID)
	if markErr != nil {
		// Non-fatal: if we can't check, increment anyway to avoid lost stats.
		// This is the pre-fix behavior and only triggers on Redis errors.
		alreadyCounted = false
	}

	// Build and execute the accumulator pipeline
	pipe := q.client.Pipeline()
	q.saveItemPipe(ctx, pipe, item)
	q.addToCompletedSetPipe(ctx, pipe, jobID, itemID)
	q.incrementStatsPipe(ctx, pipe, jobID, item, result, alreadyCounted)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute complete pipeline: %w", err)
	}

	return nil
}

// FailItem marks an item as terminally failed and updates failure accumulators.
func (q *RedisQueue) FailItem(ctx context.Context, jobID string, itemID string, failErr error) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return ErrQueueClosed
	}
	q.mu.RUnlock()

	// Remove from processing zset
	removed, err := q.client.ZRem(ctx, q.processingZSetKey(jobID), itemID).Result()
	if err != nil {
		return fmt.Errorf("failed to remove from processing: %w", err)
	}
	if removed == 0 {
		return ErrItemNotFound
	}

	// Remove from processing list
	q.client.LRem(ctx, q.processingKey(jobID), 1, itemID)

	// Get the item for scenarioID/providerID
	item, err := q.getItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}

	now := time.Now()
	item.Status = ItemStatusFailed
	item.CompletedAt = &now
	if failErr != nil {
		item.Error = failErr.Error()
	}

	alreadyCounted, markErr := q.markStatsCounted(ctx, jobID, itemID)
	if markErr != nil {
		alreadyCounted = false
	}

	// Build and execute the failure pipeline
	pipe := q.client.Pipeline()
	q.saveItemPipe(ctx, pipe, item)
	q.addToFailedSetPipe(ctx, pipe, jobID, itemID)
	if !alreadyCounted {
		q.incrementFailureStatsPipe(ctx, pipe, jobID, item)
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute fail pipeline: %w", err)
	}

	return nil
}

// GetStats returns the current accumulator statistics for a job.
func (q *RedisQueue) GetStats(ctx context.Context, jobID string) (*JobStats, error) {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return nil, ErrQueueClosed
	}
	q.mu.RUnlock()

	stats := &JobStats{
		ByScenario: make(map[string]*GroupStats),
		ByProvider: make(map[string]*GroupStats),
	}

	// Read main stats hash
	mainStats, err := q.client.HGetAll(ctx, q.statsKey(jobID)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	parseStatsHash(mainStats, stats)

	// Scan for scenario sub-keys
	q.scanGroupStats(ctx, jobID, statsScenarioKeyInfix, stats.ByScenario)

	// Scan for provider sub-keys
	q.scanGroupStats(ctx, jobID, statsProviderKeyInfix, stats.ByProvider)

	return stats, nil
}

// scanGroupStats scans for group stat keys matching the pattern and populates the map.
func (q *RedisQueue) scanGroupStats(
	ctx context.Context, jobID string, infix string, target map[string]*GroupStats,
) {
	pattern := jobKeyPrefix + jobID + infix + "*"
	prefixLen := len(jobKeyPrefix + jobID + infix)
	var cursor uint64
	for {
		keys, nextCursor, err := q.client.Scan(ctx, cursor, pattern, sscanCount).Result()
		if err != nil {
			return
		}
		for _, key := range keys {
			groupID := key[prefixLen:]
			data, hErr := q.client.HGetAll(ctx, key).Result()
			if hErr != nil {
				continue
			}
			gs := &GroupStats{}
			parseGroupStatsHash(data, gs)
			target[groupID] = gs
		}
		cursor = nextCursor
		if cursor == 0 {
			return
		}
	}
}

// statsCountedKey returns the Redis key for the set of item IDs that have been counted in stats.
func (q *RedisQueue) statsCountedKey(jobID string) string {
	return jobKeyPrefix + jobID + ":stats:counted"
}

// incrementStatsPipe adds accumulator increment commands to a pipeline for a completed item.
// Idempotent: only increments if the item has not already been counted.
// The caller must call markStatsCounted first and pass alreadyCounted=true to skip.
func (q *RedisQueue) incrementStatsPipe(
	ctx context.Context, pipe redis.Pipeliner, jobID string, item *WorkItem, result *ItemResult,
	alreadyCounted bool,
) {
	if alreadyCounted {
		return
	}

	mainKey := q.statsKey(jobID)
	tokens := extractTokens(result.Metrics)
	cost := extractCost(result.Metrics)

	q.incrStatsFields(ctx, pipe, mainKey, result.Status, result.DurationMs, tokens, cost)

	if item.ScenarioID != "" {
		scenKey := q.statsScenarioKey(jobID, item.ScenarioID)
		q.incrStatsFields(ctx, pipe, scenKey, result.Status, result.DurationMs, tokens, cost)
	}

	if item.ProviderID != "" {
		provKey := q.statsProviderKey(jobID, item.ProviderID)
		q.incrStatsFields(ctx, pipe, provKey, result.Status, result.DurationMs, tokens, cost)
	}
}

// markStatsCounted atomically adds the item ID to the stats-counted set.
// Returns true if the item was already counted (SADD returned 0).
func (q *RedisQueue) markStatsCounted(ctx context.Context, jobID, itemID string) (bool, error) {
	countedKey := q.statsCountedKey(jobID)
	added, err := q.client.SAdd(ctx, countedKey, itemID).Result()
	if err != nil {
		return false, err
	}
	q.client.Expire(ctx, countedKey, q.itemTTL)
	return added == 0, nil
}

// incrStatsFields adds HINCRBY/HINCRBYFLOAT commands for a stats hash.
func (q *RedisQueue) incrStatsFields(
	ctx context.Context, pipe redis.Pipeliner, key, status string,
	durationMs float64, tokens int64, cost float64,
) {
	pipe.HIncrBy(ctx, key, statsFieldTotal, 1)
	if status == "pass" {
		pipe.HIncrBy(ctx, key, statsFieldPassed, 1)
	} else {
		pipe.HIncrBy(ctx, key, statsFieldFailed, 1)
	}
	pipe.HIncrByFloat(ctx, key, statsFieldTotalDuration, durationMs)
	pipe.HIncrBy(ctx, key, statsFieldTotalTokens, tokens)
	pipe.HIncrByFloat(ctx, key, statsFieldTotalCost, cost)
	pipe.Expire(ctx, key, q.itemTTL)
}

// incrementFailureStatsPipe adds failure accumulator increment commands to a pipeline.
func (q *RedisQueue) incrementFailureStatsPipe(
	ctx context.Context, pipe redis.Pipeliner, jobID string, item *WorkItem,
) {
	mainKey := q.statsKey(jobID)
	q.incrFailureFields(ctx, pipe, mainKey)

	if item.ScenarioID != "" {
		scenKey := q.statsScenarioKey(jobID, item.ScenarioID)
		q.incrFailureFields(ctx, pipe, scenKey)
	}

	if item.ProviderID != "" {
		provKey := q.statsProviderKey(jobID, item.ProviderID)
		q.incrFailureFields(ctx, pipe, provKey)
	}
}

// incrFailureFields adds HINCRBY commands for failure counters only.
func (q *RedisQueue) incrFailureFields(
	ctx context.Context, pipe redis.Pipeliner, key string,
) {
	pipe.HIncrBy(ctx, key, statsFieldTotal, 1)
	pipe.HIncrBy(ctx, key, statsFieldFailed, 1)
	pipe.Expire(ctx, key, q.itemTTL)
}

// saveItemPipe adds a SET command to a pipeline for saving a work item.
func (q *RedisQueue) saveItemPipe(ctx context.Context, pipe redis.Pipeliner, item *WorkItem) {
	data, err := json.Marshal(item)
	if err != nil {
		return
	}
	pipe.Set(ctx, q.itemKey(item.ID), data, q.itemTTL)
}

// addToCompletedSetPipe adds SADD + EXPIRE commands for the completed set.
func (q *RedisQueue) addToCompletedSetPipe(ctx context.Context, pipe redis.Pipeliner, jobID, itemID string) {
	completedSetKey := q.completedKey(jobID)
	pipe.SAdd(ctx, completedSetKey, itemID)
	pipe.Expire(ctx, completedSetKey, q.itemTTL)
}

// addToFailedSetPipe adds SADD + EXPIRE commands for the failed set.
func (q *RedisQueue) addToFailedSetPipe(ctx context.Context, pipe redis.Pipeliner, jobID, itemID string) {
	failedSetKey := q.failedKey(jobID)
	pipe.SAdd(ctx, failedSetKey, itemID)
	pipe.Expire(ctx, failedSetKey, q.itemTTL)
}

// parseStatsHash parses a Redis hash into a JobStats struct.
func parseStatsHash(data map[string]string, stats *JobStats) {
	stats.Total = parseInt64(data[statsFieldTotal])
	stats.Passed = parseInt64(data[statsFieldPassed])
	stats.Failed = parseInt64(data[statsFieldFailed])
	stats.TotalDurationMs = parseFloat64(data[statsFieldTotalDuration])
	stats.TotalTokens = parseInt64(data[statsFieldTotalTokens])
	stats.TotalCost = parseFloat64(data[statsFieldTotalCost])
}

// parseGroupStatsHash parses a Redis hash into a GroupStats struct.
func parseGroupStatsHash(data map[string]string, gs *GroupStats) {
	gs.Total = parseInt64(data[statsFieldTotal])
	gs.Passed = parseInt64(data[statsFieldPassed])
	gs.Failed = parseInt64(data[statsFieldFailed])
	gs.TotalDurationMs = parseFloat64(data[statsFieldTotalDuration])
	gs.TotalTokens = parseInt64(data[statsFieldTotalTokens])
	gs.TotalCost = parseFloat64(data[statsFieldTotalCost])
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func parseFloat64(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// Ensure RedisQueue implements WorkQueue interface.
var _ WorkQueue = (*RedisQueue)(nil)
