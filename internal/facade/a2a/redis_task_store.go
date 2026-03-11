/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/go-logr/logr"
	"github.com/redis/go-redis/v9"

	"github.com/AltairaLabs/PromptKit/runtime/a2a"
	a2aserver "github.com/AltairaLabs/PromptKit/server/a2a"
)

// Redis key prefixes for A2A task storage.
const (
	taskKeyPrefix    = "a2a:task:"
	contextKeyPrefix = "a2a:ctx:"
	eventKeyPrefix   = "a2a:events:"
)

// Redis hash field names for task storage.
const (
	fieldData = "data"
)

// RedisTaskStore implements a2aserver.TaskStore backed by Redis.
// Tasks are stored as JSON in Redis hashes with TTL-based expiration.
// Context-to-task mappings use Redis sets for efficient listing.
type RedisTaskStore struct {
	client  redis.UniversalClient
	taskTTL time.Duration
	log     logr.Logger
}

// RedisTaskStoreConfig holds configuration for creating a RedisTaskStore.
type RedisTaskStoreConfig struct {
	// Client is the Redis client to use.
	Client redis.UniversalClient

	// TaskTTL is the TTL applied to task keys. Tasks in terminal states
	// are automatically expired by Redis after this duration.
	TaskTTL time.Duration

	// Log is the logger.
	Log logr.Logger
}

// NewRedisTaskStore creates a new Redis-backed task store.
func NewRedisTaskStore(cfg RedisTaskStoreConfig) *RedisTaskStore {
	return &RedisTaskStore{
		client:  cfg.Client,
		taskTTL: cfg.TaskTTL,
		log:     cfg.Log,
	}
}

// taskKey returns the Redis key for a task.
func taskKey(taskID string) string {
	return taskKeyPrefix + taskID
}

// contextKey returns the Redis key for a context's task set.
func contextKey(contextID string) string {
	return contextKeyPrefix + contextID
}

// eventKey returns the Redis pub/sub channel for task events.
func eventKey(taskID string) string {
	return eventKeyPrefix + taskID
}

// Create initializes a new task in the submitted state.
func (s *RedisTaskStore) Create(taskID, contextID string) (*a2a.Task, error) {
	ctx := context.Background()
	key := taskKey(taskID)

	// Check if task already exists.
	exists, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis task exists check: %w", err)
	}
	if exists > 0 {
		return nil, a2aserver.ErrTaskAlreadyExists
	}

	now := time.Now().UTC()
	task := &a2a.Task{
		ID:        taskID,
		ContextID: contextID,
		Status: a2a.TaskStatus{
			State:     a2a.TaskStateSubmitted,
			Timestamp: &now,
		},
	}

	if err := s.setTask(ctx, task); err != nil {
		return nil, err
	}

	// Add task ID to context set.
	ctxKey := contextKey(contextID)
	if err := s.client.SAdd(ctx, ctxKey, taskID).Err(); err != nil {
		return nil, fmt.Errorf("redis context set add: %w", err)
	}
	// Reset TTL on context set.
	s.client.Expire(ctx, ctxKey, s.taskTTL)

	return task, nil
}

// Get retrieves a task by ID.
func (s *RedisTaskStore) Get(taskID string) (*a2a.Task, error) {
	ctx := context.Background()
	return s.getTask(ctx, taskID)
}

// SetState transitions the task to a new state with an optional status message.
func (s *RedisTaskStore) SetState(taskID string, state a2a.TaskState, msg *a2a.Message) error {
	ctx := context.Background()

	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return err
	}

	current := task.Status.State

	if isTerminalState(current) {
		return fmt.Errorf("%w: cannot transition from terminal state %q", a2aserver.ErrTaskTerminal, current)
	}

	if !isValidTransition(current, state) {
		return fmt.Errorf("%w: %q → %q", a2aserver.ErrInvalidTransition, current, state)
	}

	now := time.Now().UTC()
	task.Status = a2a.TaskStatus{
		State:     state,
		Message:   msg,
		Timestamp: &now,
	}

	if err := s.setTask(ctx, task); err != nil {
		return err
	}

	// Publish state change event for SSE fan-out.
	s.publishEvent(ctx, taskID, state)

	return nil
}

// AddArtifacts appends artifacts to a task.
func (s *RedisTaskStore) AddArtifacts(taskID string, artifacts []a2a.Artifact) error {
	ctx := context.Background()

	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return err
	}

	task.Artifacts = append(task.Artifacts, artifacts...)
	return s.setTask(ctx, task)
}

// Cancel transitions the task to the canceled state from any non-terminal state.
func (s *RedisTaskStore) Cancel(taskID string) error {
	ctx := context.Background()

	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return err
	}

	if isTerminalState(task.Status.State) {
		return fmt.Errorf("%w: cannot cancel task in terminal state %q", a2aserver.ErrTaskTerminal, task.Status.State)
	}

	now := time.Now().UTC()
	task.Status = a2a.TaskStatus{
		State:     a2a.TaskStateCanceled,
		Timestamp: &now,
	}

	if err := s.setTask(ctx, task); err != nil {
		return err
	}

	// Publish cancel event.
	s.publishEvent(ctx, taskID, a2a.TaskStateCanceled)

	return nil
}

// List returns tasks matching the given contextID with pagination.
// If contextID is empty, this is not efficient for Redis and returns nil.
func (s *RedisTaskStore) List(contextID string, limit, offset int) ([]*a2a.Task, error) {
	ctx := context.Background()

	if contextID == "" {
		// Listing all tasks across all contexts is not efficient with Redis sets.
		// Return empty for now — callers should provide a contextID.
		return nil, nil
	}

	// Get task IDs from context set.
	taskIDs, err := s.client.SMembers(ctx, contextKey(contextID)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis context set members: %w", err)
	}

	// Sort for deterministic pagination.
	sort.Strings(taskIDs)

	// Apply offset.
	if offset >= len(taskIDs) {
		return nil, nil
	}
	taskIDs = taskIDs[offset:]

	// Apply limit.
	if limit > 0 && limit < len(taskIDs) {
		taskIDs = taskIDs[:limit]
	}

	// Fetch each task.
	tasks := make([]*a2a.Task, 0, len(taskIDs))
	for _, id := range taskIDs {
		task, err := s.getTask(ctx, id)
		if err != nil {
			// Task may have expired; skip it.
			s.log.V(1).Info("skipping expired task in list", "taskID", id, "contextID", contextID)
			continue
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// EvictTerminal removes tasks in a terminal state whose last status timestamp
// is older than the given cutoff time. For Redis, TTL handles most eviction
// automatically, but this method handles explicit eviction requests.
func (s *RedisTaskStore) EvictTerminal(olderThan time.Time) []string {
	// Redis TTL handles expiration automatically. This method is a no-op
	// because we set EXPIRE on every task key. The PromptKit server calls
	// this periodically, but Redis native expiry is more efficient.
	//
	// We return nil (no evicted IDs) — the server will clean up its
	// in-memory cancel/broadcaster maps independently.
	return nil
}

// Subscribe returns a channel that receives task state change events via Redis pub/sub.
// The channel is closed when the context is canceled.
func (s *RedisTaskStore) Subscribe(ctx context.Context, taskID string) <-chan a2a.TaskState {
	ch := make(chan a2a.TaskState, 8)

	pubsub := s.client.Subscribe(ctx, eventKey(taskID))

	go func() {
		defer close(ch)
		defer func() { _ = pubsub.Close() }()

		msgCh := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				state := a2a.TaskState(msg.Payload)
				select {
				case ch <- state:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch
}

// Ping checks connectivity to the Redis server.
func (s *RedisTaskStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// Close closes the Redis client connection.
func (s *RedisTaskStore) Close() error {
	return s.client.Close()
}

// setTask serializes and stores a task in Redis with TTL.
func (s *RedisTaskStore) setTask(ctx context.Context, task *a2a.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("redis task marshal: %w", err)
	}

	key := taskKey(task.ID)
	if err := s.client.HSet(ctx, key, fieldData, data).Err(); err != nil {
		return fmt.Errorf("redis task set: %w", err)
	}

	// Reset TTL on every write.
	s.client.Expire(ctx, key, s.taskTTL)

	return nil
}

// getTask retrieves and deserializes a task from Redis.
func (s *RedisTaskStore) getTask(ctx context.Context, taskID string) (*a2a.Task, error) {
	data, err := s.client.HGet(ctx, taskKey(taskID), fieldData).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, a2aserver.ErrTaskNotFound
		}
		return nil, fmt.Errorf("redis task get: %w", err)
	}

	var task a2a.Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("redis task unmarshal: %w", err)
	}

	return &task, nil
}

// publishEvent publishes a task state change to the Redis pub/sub channel.
func (s *RedisTaskStore) publishEvent(ctx context.Context, taskID string, state a2a.TaskState) {
	if err := s.client.Publish(ctx, eventKey(taskID), string(state)).Err(); err != nil {
		s.log.V(1).Info("event publish failed", "taskID", taskID, "state", state, "error", err)
	}
}

// isTerminalState checks if a task state is terminal.
func isTerminalState(state a2a.TaskState) bool {
	switch state {
	case a2a.TaskStateCompleted, a2a.TaskStateFailed, a2a.TaskStateCanceled, a2a.TaskStateRejected:
		return true
	default:
		return false
	}
}

// isValidTransition checks if a state transition is allowed.
func isValidTransition(from, to a2a.TaskState) bool {
	switch from {
	case a2a.TaskStateSubmitted:
		return to == a2a.TaskStateWorking
	case a2a.TaskStateWorking:
		switch to {
		case a2a.TaskStateCompleted, a2a.TaskStateFailed, a2a.TaskStateCanceled,
			a2a.TaskStateInputRequired, a2a.TaskStateAuthRequired, a2a.TaskStateRejected:
			return true
		}
	case a2a.TaskStateInputRequired:
		return to == a2a.TaskStateWorking || to == a2a.TaskStateCanceled
	case a2a.TaskStateAuthRequired:
		return to == a2a.TaskStateWorking || to == a2a.TaskStateCanceled
	}
	return false
}
