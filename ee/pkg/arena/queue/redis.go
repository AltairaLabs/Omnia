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
	// URL is the full Redis connection URL (redis:// or rediss://).
	// host/port/auth/TLS/db-index all encoded per RFC 7595; parsed via
	// redis.ParseURL. Required.
	URL string

	// ItemTTL is the TTL for queue items stored in Redis.
	// Defaults to 24 hours if zero.
	ItemTTL time.Duration

	// Options contains common queue options.
	Options Options
}

// NewRedisQueue creates a new Redis-backed work queue.
func NewRedisQueue(redisOpts RedisOptions) (*RedisQueue, error) {
	clientOpts, err := redis.ParseURL(redisOpts.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}
	client := redis.NewClient(clientOpts)
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

// Close releases resources and marks the queue as closed.
func (q *RedisQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	return q.client.Close()
}

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

	// statusPass is the ItemResult status value indicating a passing outcome.
	statusPass = "pass"
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

// statsCountedKey returns the Redis key for the set of item IDs that have been counted in stats.
func (q *RedisQueue) statsCountedKey(jobID string) string {
	return jobKeyPrefix + jobID + ":stats:counted"
}

// saveItemPipe adds a SET command to a pipeline for saving a work item.
func (q *RedisQueue) saveItemPipe(ctx context.Context, pipe redis.Pipeliner, item *WorkItem) {
	data, err := json.Marshal(item)
	if err != nil {
		return
	}
	pipe.Set(ctx, q.itemKey(item.ID), data, q.itemTTL)
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
