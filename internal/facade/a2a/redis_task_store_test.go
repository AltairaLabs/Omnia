/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package a2a

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/a2a"
	a2aserver "github.com/AltairaLabs/PromptKit/server/a2a"
)

func newTestRedisStore(t *testing.T) *RedisTaskStore {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	store := NewRedisTaskStore(RedisTaskStoreConfig{
		Client:  client,
		TaskTTL: 1 * time.Hour,
		Log:     logr.Discard(),
	})
	return store
}

func TestRedisTaskStore_Create(t *testing.T) {
	store := newTestRedisStore(t)

	task, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)
	assert.Equal(t, "task-1", task.ID)
	assert.Equal(t, "ctx-1", task.ContextID)
	assert.Equal(t, a2a.TaskStateSubmitted, task.Status.State)
	assert.NotNil(t, task.Status.Timestamp)
}

func TestRedisTaskStore_CreateDuplicate(t *testing.T) {
	store := newTestRedisStore(t)

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)

	_, err = store.Create("task-1", "ctx-1")
	assert.ErrorIs(t, err, a2aserver.ErrTaskAlreadyExists)
}

func TestRedisTaskStore_Get(t *testing.T) {
	store := newTestRedisStore(t)

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)

	task, err := store.Get("task-1")
	require.NoError(t, err)
	assert.Equal(t, "task-1", task.ID)
	assert.Equal(t, "ctx-1", task.ContextID)
}

func TestRedisTaskStore_GetNotFound(t *testing.T) {
	store := newTestRedisStore(t)

	_, err := store.Get("nonexistent")
	assert.ErrorIs(t, err, a2aserver.ErrTaskNotFound)
}

func TestRedisTaskStore_SetState(t *testing.T) {
	store := newTestRedisStore(t)

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)

	err = store.SetState("task-1", a2a.TaskStateWorking, nil)
	require.NoError(t, err)

	task, err := store.Get("task-1")
	require.NoError(t, err)
	assert.Equal(t, a2a.TaskStateWorking, task.Status.State)
}

func TestRedisTaskStore_SetStateWithMessage(t *testing.T) {
	store := newTestRedisStore(t)

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)

	err = store.SetState("task-1", a2a.TaskStateWorking, nil)
	require.NoError(t, err)

	text := "processing complete"
	msg := &a2a.Message{
		Role:  a2a.RoleAgent,
		Parts: []a2a.Part{{Text: &text}},
	}
	err = store.SetState("task-1", a2a.TaskStateCompleted, msg)
	require.NoError(t, err)

	task, err := store.Get("task-1")
	require.NoError(t, err)
	assert.Equal(t, a2a.TaskStateCompleted, task.Status.State)
	require.NotNil(t, task.Status.Message)
	assert.Equal(t, a2a.RoleAgent, task.Status.Message.Role)
}

func TestRedisTaskStore_SetStateInvalidTransition(t *testing.T) {
	store := newTestRedisStore(t)

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)

	// submitted -> completed is not valid (must go through working)
	err = store.SetState("task-1", a2a.TaskStateCompleted, nil)
	assert.ErrorIs(t, err, a2aserver.ErrInvalidTransition)
}

func TestRedisTaskStore_SetStateTerminal(t *testing.T) {
	store := newTestRedisStore(t)

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)

	require.NoError(t, store.SetState("task-1", a2a.TaskStateWorking, nil))
	require.NoError(t, store.SetState("task-1", a2a.TaskStateCompleted, nil))

	// Cannot transition from terminal state.
	err = store.SetState("task-1", a2a.TaskStateWorking, nil)
	assert.ErrorIs(t, err, a2aserver.ErrTaskTerminal)
}

func TestRedisTaskStore_SetStateNotFound(t *testing.T) {
	store := newTestRedisStore(t)

	err := store.SetState("nonexistent", a2a.TaskStateWorking, nil)
	assert.ErrorIs(t, err, a2aserver.ErrTaskNotFound)
}

func TestRedisTaskStore_AddArtifacts(t *testing.T) {
	store := newTestRedisStore(t)

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)

	text := "hello"
	artifacts := []a2a.Artifact{
		{ArtifactID: "art-1", Parts: []a2a.Part{{Text: &text}}},
	}
	err = store.AddArtifacts("task-1", artifacts)
	require.NoError(t, err)

	task, err := store.Get("task-1")
	require.NoError(t, err)
	require.Len(t, task.Artifacts, 1)
	assert.Equal(t, "art-1", task.Artifacts[0].ArtifactID)
}

func TestRedisTaskStore_AddArtifactsNotFound(t *testing.T) {
	store := newTestRedisStore(t)

	err := store.AddArtifacts("nonexistent", []a2a.Artifact{})
	assert.ErrorIs(t, err, a2aserver.ErrTaskNotFound)
}

func TestRedisTaskStore_Cancel(t *testing.T) {
	store := newTestRedisStore(t)

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)

	err = store.Cancel("task-1")
	require.NoError(t, err)

	task, err := store.Get("task-1")
	require.NoError(t, err)
	assert.Equal(t, a2a.TaskStateCanceled, task.Status.State)
}

func TestRedisTaskStore_CancelNotFound(t *testing.T) {
	store := newTestRedisStore(t)

	err := store.Cancel("nonexistent")
	assert.ErrorIs(t, err, a2aserver.ErrTaskNotFound)
}

func TestRedisTaskStore_CancelTerminal(t *testing.T) {
	store := newTestRedisStore(t)

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)

	require.NoError(t, store.SetState("task-1", a2a.TaskStateWorking, nil))
	require.NoError(t, store.SetState("task-1", a2a.TaskStateCompleted, nil))

	err = store.Cancel("task-1")
	assert.ErrorIs(t, err, a2aserver.ErrTaskTerminal)
}

func TestRedisTaskStore_List(t *testing.T) {
	store := newTestRedisStore(t)

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)
	_, err = store.Create("task-2", "ctx-1")
	require.NoError(t, err)
	_, err = store.Create("task-3", "ctx-2")
	require.NoError(t, err)

	// List tasks for ctx-1.
	tasks, err := store.List("ctx-1", 10, 0)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)

	// Verify sorted by ID.
	assert.Equal(t, "task-1", tasks[0].ID)
	assert.Equal(t, "task-2", tasks[1].ID)
}

func TestRedisTaskStore_ListWithPagination(t *testing.T) {
	store := newTestRedisStore(t)

	for i := 0; i < 5; i++ {
		_, err := store.Create(fmt.Sprintf("task-%d", i), "ctx-1")
		require.NoError(t, err)
	}

	// Offset 2, limit 2.
	tasks, err := store.List("ctx-1", 2, 2)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
	assert.Equal(t, "task-2", tasks[0].ID)
	assert.Equal(t, "task-3", tasks[1].ID)
}

func TestRedisTaskStore_ListEmptyContext(t *testing.T) {
	store := newTestRedisStore(t)

	tasks, err := store.List("", 10, 0)
	require.NoError(t, err)
	assert.Nil(t, tasks)
}

func TestRedisTaskStore_ListNonexistentContext(t *testing.T) {
	store := newTestRedisStore(t)

	tasks, err := store.List("nonexistent", 10, 0)
	require.NoError(t, err)
	assert.Nil(t, tasks)
}

func TestRedisTaskStore_EvictTerminal(t *testing.T) {
	store := newTestRedisStore(t)

	// EvictTerminal is a no-op for Redis (TTL handles it).
	evicted := store.EvictTerminal(time.Now())
	assert.Nil(t, evicted)
}

func TestRedisTaskStore_TTLExpiry(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	store := NewRedisTaskStore(RedisTaskStoreConfig{
		Client:  client,
		TaskTTL: 10 * time.Second,
		Log:     logr.Discard(),
	})

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)

	// Task should be retrievable.
	_, err = store.Get("task-1")
	require.NoError(t, err)

	// Fast-forward past TTL.
	mr.FastForward(11 * time.Second)

	// Task should be expired.
	_, err = store.Get("task-1")
	assert.ErrorIs(t, err, a2aserver.ErrTaskNotFound)
}

func TestRedisTaskStore_Subscribe(t *testing.T) {
	store := newTestRedisStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := store.Create("task-1", "ctx-1")
	require.NoError(t, err)

	ch := store.Subscribe(ctx, "task-1")

	// Publish a state change.
	err = store.SetState("task-1", a2a.TaskStateWorking, nil)
	require.NoError(t, err)

	// Should receive the event.
	select {
	case state := <-ch:
		assert.Equal(t, a2a.TaskStateWorking, state)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for subscription event")
	}
}

func TestRedisTaskStore_SubscribeCanceled(t *testing.T) {
	store := newTestRedisStore(t)

	ctx, cancel := context.WithCancel(context.Background())

	ch := store.Subscribe(ctx, "task-1")

	// Cancel the subscription.
	cancel()

	// Channel should eventually close.
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestRedisTaskStore_Ping(t *testing.T) {
	store := newTestRedisStore(t)

	err := store.Ping(context.Background())
	assert.NoError(t, err)
}

func TestRedisTaskStore_PingFailure(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	store := NewRedisTaskStore(RedisTaskStoreConfig{
		Client:  client,
		TaskTTL: 1 * time.Hour,
		Log:     logr.Discard(),
	})

	// Close the miniredis server to simulate failure.
	mr.Close()

	err := store.Ping(context.Background())
	assert.Error(t, err)
}

func TestRedisTaskStore_StateTransitions(t *testing.T) {
	tests := []struct {
		name    string
		from    a2a.TaskState
		to      a2a.TaskState
		valid   bool
		setupFn func(store *RedisTaskStore, taskID string) // set up from state
	}{
		{
			name:  "submitted to working",
			from:  a2a.TaskStateSubmitted,
			to:    a2a.TaskStateWorking,
			valid: true,
		},
		{
			name:    "working to completed",
			from:    a2a.TaskStateWorking,
			to:      a2a.TaskStateCompleted,
			valid:   true,
			setupFn: func(s *RedisTaskStore, id string) { _ = s.SetState(id, a2a.TaskStateWorking, nil) },
		},
		{
			name:    "working to failed",
			from:    a2a.TaskStateWorking,
			to:      a2a.TaskStateFailed,
			valid:   true,
			setupFn: func(s *RedisTaskStore, id string) { _ = s.SetState(id, a2a.TaskStateWorking, nil) },
		},
		{
			name:    "working to input_required",
			from:    a2a.TaskStateWorking,
			to:      a2a.TaskStateInputRequired,
			valid:   true,
			setupFn: func(s *RedisTaskStore, id string) { _ = s.SetState(id, a2a.TaskStateWorking, nil) },
		},
		{
			name:  "input_required to working",
			from:  a2a.TaskStateInputRequired,
			to:    a2a.TaskStateWorking,
			valid: true,
			setupFn: func(s *RedisTaskStore, id string) {
				_ = s.SetState(id, a2a.TaskStateWorking, nil)
				_ = s.SetState(id, a2a.TaskStateInputRequired, nil)
			},
		},
		{
			name:  "submitted to completed (invalid)",
			from:  a2a.TaskStateSubmitted,
			to:    a2a.TaskStateCompleted,
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestRedisStore(t)
			taskID := "task-" + tt.name

			_, err := store.Create(taskID, "ctx-1")
			require.NoError(t, err)

			if tt.setupFn != nil {
				tt.setupFn(store, taskID)
			}

			err = store.SetState(taskID, tt.to, nil)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
