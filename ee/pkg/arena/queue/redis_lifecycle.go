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
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

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

	// Pop from pending queue (RPOP for FIFO). LMove(RIGHT, LEFT) is the
	// non-deprecated equivalent of RPOPLPUSH (Redis 6.2+).
	itemID, err := q.client.LMove(ctx, pendingKey, processingKey, "RIGHT", "LEFT").Result()
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
		if errors.Is(err, ErrItemNotFound) {
			return nil, ErrItemNotFound
		}
		return nil, fmt.Errorf("failed to get item data: %w", err)
	}

	// Update item status
	now := time.Now()
	item.Status = ItemStatusProcessing
	item.StartedAt = &now
	item.Attempt++

	// Save updated item
	// Batch write-side bookkeeping into one pipeline to reduce round trips.
	processingZKey := q.processingZSetKey(jobID)
	score := float64(now.Add(q.opts.VisibilityTimeout).UnixNano())
	pipe := q.client.Pipeline()
	q.saveItemPipe(ctx, pipe, item)
	pipe.ZAdd(ctx, processingZKey, redis.Z{
		Score:  score,
		Member: itemID,
	})
	pipe.Expire(ctx, processingZKey, q.itemTTL)
	// Update job start time if this is the first item.
	pipe.HSetNX(ctx, q.metaKey(jobID), "startedAt", now.UnixNano())
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update item: %w", err)
	}

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
	if status == statusPass {
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
