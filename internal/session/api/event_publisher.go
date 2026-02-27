/*
Copyright 2026 Altaira Labs.

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

// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	goredis "github.com/redis/go-redis/v9"
)

// Stream key and MAXLEN constants for Redis Streams event publishing.
const (
	streamKeyPrefix       = "omnia:eval-events:"
	streamMaxLen    int64 = 10000
	publishTimeout        = 2 * time.Second
)

// SessionEvent represents a lightweight event published to Redis Streams.
type SessionEvent struct {
	EventType         string `json:"eventType"`
	SessionID         string `json:"sessionId"`
	AgentName         string `json:"agentName"`
	Namespace         string `json:"namespace"`
	MessageID         string `json:"messageId,omitempty"`
	MessageRole       string `json:"messageRole,omitempty"`
	PromptPackName    string `json:"promptPackName,omitempty"`
	PromptPackVersion string `json:"promptPackVersion,omitempty"`
	Timestamp         string `json:"timestamp"`
}

// EventPublisher publishes session events for downstream consumers.
type EventPublisher interface {
	PublishMessageEvent(ctx context.Context, event SessionEvent) error
	Close() error
}

// RedisEventPublisher publishes events to Redis Streams.
type RedisEventPublisher struct {
	client goredis.UniversalClient
	log    logr.Logger
}

// NewRedisEventPublisher creates a new RedisEventPublisher.
// The caller retains ownership of the Redis client; Close is a no-op.
func NewRedisEventPublisher(client goredis.UniversalClient, log logr.Logger) *RedisEventPublisher {
	return &RedisEventPublisher{
		client: client,
		log:    log.WithName("event-publisher"),
	}
}

// PublishMessageEvent publishes a session event to the namespace-scoped Redis Stream.
func (p *RedisEventPublisher) PublishMessageEvent(ctx context.Context, event SessionEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	streamKey := streamKeyPrefix + event.Namespace

	pubCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	return p.client.XAdd(pubCtx, &goredis.XAddArgs{
		Stream: streamKey,
		MaxLen: streamMaxLen,
		Approx: true,
		Values: map[string]interface{}{
			"payload": string(payload),
		},
	}).Err()
}

// Close is a no-op because the publisher does not own the Redis client.
func (p *RedisEventPublisher) Close() error {
	return nil
}

// streamKey returns the Redis Stream key for the given namespace.
func StreamKey(namespace string) string {
	return streamKeyPrefix + namespace
}
