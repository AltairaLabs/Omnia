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
	"time"

	"github.com/redis/go-redis/v9"
)

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

	// Get items that have exceeded visibility timeout. ZRangeArgs with
	// ByScore is the non-deprecated equivalent of ZRANGEBYSCORE (Redis 6.2+).
	itemIDs, err := q.client.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     q.processingZSetKey(jobID),
		Start:   "-inf",
		Stop:    fmt.Sprintf("%f", maxScore),
		ByScore: true,
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
