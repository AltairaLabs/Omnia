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
)

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
