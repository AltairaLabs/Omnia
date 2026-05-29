/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsolidationRunStore_LastRunMissingThenMarked(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := freshPgxPool(t)
	s := NewConsolidationRunStore(pool)
	ctx := context.Background()

	// No row yet -> ok=false.
	_, ok, err := s.LastRun(ctx, "research", "uid-1", "staleObservations")
	require.NoError(t, err)
	assert.False(t, ok, "expected no row before first MarkRun")

	// Mark a run, then read it back (truncate to microseconds — Postgres
	// TIMESTAMPTZ resolution).
	at := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, s.MarkRun(ctx, "research", "uid-1", "staleObservations", at))

	got, ok, err := s.LastRun(ctx, "research", "uid-1", "staleObservations")
	require.NoError(t, err)
	require.True(t, ok)
	assert.WithinDuration(t, at, got, time.Microsecond)
}

func TestConsolidationRunStore_MarkRunUpsertsInPlace(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := freshPgxPool(t)
	s := NewConsolidationRunStore(pool)
	ctx := context.Background()

	t1 := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	t2 := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, s.MarkRun(ctx, "p", "ws", "crossScopeCandidates", t1))
	require.NoError(t, s.MarkRun(ctx, "p", "ws", "crossScopeCandidates", t2))

	got, ok, err := s.LastRun(ctx, "p", "ws", "crossScopeCandidates")
	require.NoError(t, err)
	require.True(t, ok)
	assert.WithinDuration(t, t2, got, time.Microsecond, "second MarkRun should overwrite the first")

	// Different axis is independent.
	_, ok, err = s.LastRun(ctx, "p", "ws", "staleObservations")
	require.NoError(t, err)
	assert.False(t, ok)
}
