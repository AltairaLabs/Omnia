/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	goredis "github.com/redis/go-redis/v9"
)

// Stream key and MAXLEN constants for Redis Streams memory event publishing.
const (
	memoryStreamKeyPrefix       = "omnia:memory-events:"
	memoryStreamMaxLen    int64 = 10000
	memoryPublishTimeout        = 2 * time.Second
)

// Metric name constants for memory event publishing.
const (
	metricMemoryEventsPublished      = "omnia_memory_api_events_published_total"
	metricMemoryEventPublishDuration = "omnia_memory_api_event_publish_duration_seconds"
)

// MemoryEvent represents a lightweight event published to Redis Streams when a
// memory is created or deleted.
type MemoryEvent struct {
	EventType   string `json:"eventType"`
	MemoryID    string `json:"memoryId"`
	WorkspaceID string `json:"workspaceId"`
	UserID      string `json:"userId,omitempty"`
	AgentID     string `json:"agentId,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Purpose     string `json:"purpose,omitempty"`
	Timestamp   string `json:"timestamp"`
	Traceparent string `json:"traceparent,omitempty"`
}

// MemoryEventPublisher publishes memory lifecycle events for downstream consumers.
type MemoryEventPublisher interface {
	PublishMemoryEvent(ctx context.Context, event MemoryEvent) error
	Close() error
}

// memoryPublishMetrics holds Prometheus metrics for the memory event publisher.
type memoryPublishMetrics struct {
	published *prometheus.CounterVec
	duration  prometheus.Histogram
}

// newMemoryPublishMetrics registers memory publish metrics against the given
// Prometheus registerer. Pass nil to use the default global registry via promauto.
func newMemoryPublishMetrics(reg prometheus.Registerer) *memoryPublishMetrics {
	factory := promauto.With(reg)
	return &memoryPublishMetrics{
		published: factory.NewCounterVec(prometheus.CounterOpts{
			Name: metricMemoryEventsPublished,
			Help: "Redis stream memory event publish attempts by status",
		}, []string{"status"}),
		duration: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    metricMemoryEventPublishDuration,
			Help:    "Time to publish a memory event to Redis Streams",
			Buckets: []float64{0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		}),
	}
}

// RedisMemoryEventPublisher publishes memory events to Redis Streams.
type RedisMemoryEventPublisher struct {
	client  goredis.UniversalClient
	log     logr.Logger
	metrics *memoryPublishMetrics
}

// NewRedisMemoryEventPublisher creates a new RedisMemoryEventPublisher.
// The caller retains ownership of the Redis client; Close is a no-op.
func NewRedisMemoryEventPublisher(client goredis.UniversalClient, log logr.Logger) *RedisMemoryEventPublisher {
	return &RedisMemoryEventPublisher{
		client:  client,
		log:     log.WithName("memory-event-publisher"),
		metrics: newMemoryPublishMetrics(nil),
	}
}

// PublishMemoryEvent publishes a memory event to the workspace-scoped Redis Stream.
func (p *RedisMemoryEventPublisher) PublishMemoryEvent(ctx context.Context, event MemoryEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal memory event: %w", err)
	}

	streamKey := memoryStreamKeyPrefix + event.WorkspaceID

	pubCtx, cancel := context.WithTimeout(ctx, memoryPublishTimeout)
	defer cancel()

	start := time.Now()
	pubErr := p.client.XAdd(pubCtx, &goredis.XAddArgs{
		Stream: streamKey,
		MaxLen: memoryStreamMaxLen,
		Approx: true,
		Values: map[string]interface{}{
			"payload": string(payload),
		},
	}).Err()

	duration := time.Since(start).Seconds()
	p.metrics.duration.Observe(duration)
	if pubErr != nil {
		p.metrics.published.WithLabelValues("error").Inc()
	} else {
		p.metrics.published.WithLabelValues("success").Inc()
	}

	return pubErr
}

// Close is a no-op because the publisher does not own the Redis client.
func (p *RedisMemoryEventPublisher) Close() error {
	return nil
}

// MemoryStreamKey returns the Redis Stream key for the given workspace.
func MemoryStreamKey(workspaceID string) string {
	return memoryStreamKeyPrefix + workspaceID
}
