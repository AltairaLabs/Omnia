/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package queue

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

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
