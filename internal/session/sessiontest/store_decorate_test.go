/*
Copyright 2026.

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

package sessiontest

import (
	"context"
	"testing"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	mdTag1        = "tag1"
	mdSourceArena = "source:arena"
)

func TestStore_DecorateSession(t *testing.T) {
	ctx := context.Background()
	m := NewStore()

	_, err := m.EnsureSessionRecord(ctx, session.SessionRecordOptions{
		ID:           "s1",
		AgentName:    "agent",
		Tags:         []string{mdTag1, "tag2"},
		InitialState: map[string]string{"key": "value"},
	})
	require.NoError(t, err)

	// Overlapping tag (tag1) is not duplicated; new tags appended. State is
	// shallow-merged (overlapping key overwritten, new key added).
	err = m.DecorateSession(ctx, "s1", DecorateSessionOptions{
		AddTags:    []string{mdTag1, mdSourceArena, "arena-job:demo"},
		MergeState: map[string]string{"key": "overwritten", "arena.job": "demo"},
	})
	require.NoError(t, err)

	got, err := m.GetSession(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, []string{mdTag1, "tag2", mdSourceArena, "arena-job:demo"}, got.Tags)
	assert.Equal(t, map[string]string{"key": "overwritten", "arena.job": "demo"}, got.State)

	// Idempotent for tags: re-adding existing tags changes nothing.
	require.NoError(t, m.DecorateSession(ctx, "s1", DecorateSessionOptions{
		AddTags: []string{mdSourceArena},
	}))
	got2, err := m.GetSession(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, got.Tags, got2.Tags)

	// RemoveTags drops tags before AddTags are applied.
	require.NoError(t, m.DecorateSession(ctx, "s1", DecorateSessionOptions{
		RemoveTags: []string{mdTag1, mdSourceArena},
		AddTags:    []string{"source:final"},
	}))
	got3, err := m.GetSession(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, []string{"tag2", "arena-job:demo", "source:final"}, got3.Tags)
}

func TestStore_DecorateSession_NotFound(t *testing.T) {
	ctx := context.Background()
	m := NewStore()

	err := m.DecorateSession(ctx, "missing", DecorateSessionOptions{AddTags: []string{"x"}})
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestStore_DecorateSession_Guards(t *testing.T) {
	m := NewStore()

	// Empty session ID is rejected.
	err := m.DecorateSession(context.Background(), "", DecorateSessionOptions{AddTags: []string{"x"}})
	assert.ErrorIs(t, err, ErrInvalidSessionID)

	// A cancelled context is surfaced.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = m.DecorateSession(ctx, "s1", DecorateSessionOptions{AddTags: []string{"x"}})
	assert.ErrorIs(t, err, context.Canceled)
}
