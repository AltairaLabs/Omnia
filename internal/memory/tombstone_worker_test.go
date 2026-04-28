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

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTombstoneWorker_RunOnce_DeletesAcrossWorkspaces seeds long
// chains in two workspaces and proves a single RunOnce pass trims
// both. Per-workspace iteration is the unit of work — one bad
// workspace doesn't block the others.
func TestTombstoneWorker_RunOnce_DeletesAcrossWorkspaces(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	for _, ws := range []string{testWorkspace1, testWorkspace2} {
		scope := testScope(ws)
		mem := &Memory{Type: "fact", Content: "v1", Confidence: 0.9, Scope: scope}
		require.NoError(t, store.Save(ctx, mem))
		for i := 0; i < 25; i++ {
			_, err := store.AppendObservationToEntity(ctx, mem.ID,
				&Memory{Type: "fact", Content: "vN", Confidence: 0.9, Scope: scope})
			require.NoError(t, err)
		}
		_, err := store.pool.Exec(ctx, `
			UPDATE memory_observations
			SET observed_at = now() - interval '40 days'
			WHERE entity_id = $1`, mem.ID)
		require.NoError(t, err)
	}

	worker := NewTombstoneWorker(store, TombstoneWorkerOptions{
		Interval:           0, // RunOnce only
		WorkspaceIDs:       []string{testWorkspace1, testWorkspace2},
		MinAge:             30 * 24 * time.Hour,
		MinInactiveCount:   10,
		KeepRecentInactive: 5,
	}, logr.Discard())

	require.NoError(t, worker.RunOnce(ctx))

	// Each workspace had 25 inactive; keep 5 → 20 deleted per ws.
	for _, ws := range []string{testWorkspace1, testWorkspace2} {
		var n int
		require.NoError(t, store.pool.QueryRow(ctx, `
			SELECT count(*) FROM memory_observations o
			JOIN memory_entities e ON e.id = o.entity_id
			WHERE e.workspace_id = $1`, ws).Scan(&n))
		// 1 active + 5 kept inactive = 6 per workspace.
		assert.Equal(t, 6, n, "workspace %s", ws)
	}
}

// TestTombstoneWorker_RunDisabled proves Run exits immediately
// without scheduling the ticker when interval is non-positive.
// Important so binaries can wire Run unconditionally.
func TestTombstoneWorker_RunDisabled(t *testing.T) {
	store := newStore(t)
	worker := NewTombstoneWorker(store, TombstoneWorkerOptions{
		Interval: 0,
	}, logr.Discard())

	done := make(chan struct{})
	go func() {
		worker.Run(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return when interval is non-positive")
	}
}

// TestTombstoneWorker_RunOnce_NoWorkspaces proves a pass with no
// workspaces is a clean no-op rather than an error.
func TestTombstoneWorker_RunOnce_NoWorkspaces(t *testing.T) {
	store := newStore(t)
	worker := NewTombstoneWorker(store, TombstoneWorkerOptions{}, logr.Discard())
	require.NoError(t, worker.RunOnce(context.Background()))
}

// TestTombstoneWorker_RunFiresAndExitsOnCancel exercises the live
// Run path: starting a worker against a workspace with eligible
// chains drains them on the ticker, then returns when ctx is
// cancelled.
func TestTombstoneWorker_RunFiresAndExitsOnCancel(t *testing.T) {
	store := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scope := testScope(testWorkspace1)

	mem := &Memory{Type: "fact", Content: "v1", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, mem))
	for i := 0; i < 25; i++ {
		_, err := store.AppendObservationToEntity(ctx, mem.ID,
			&Memory{Type: "fact", Content: "vN", Confidence: 0.9, Scope: scope})
		require.NoError(t, err)
	}
	_, err := store.pool.Exec(ctx, `
		UPDATE memory_observations
		SET observed_at = now() - interval '40 days'
		WHERE entity_id = $1`, mem.ID)
	require.NoError(t, err)

	worker := NewTombstoneWorker(store, TombstoneWorkerOptions{
		Interval:           50 * time.Millisecond,
		WorkspaceIDs:       []string{testWorkspace1},
		MinAge:             30 * 24 * time.Hour,
		MinInactiveCount:   10,
		KeepRecentInactive: 5,
	}, logr.Discard())

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		var n int
		_ = store.pool.QueryRow(context.Background(),
			`SELECT count(*) FROM memory_observations o
			 JOIN memory_entities e ON e.id = o.entity_id
			 WHERE e.workspace_id = $1`, testWorkspace1).Scan(&n)
		return n == 6 // 1 active + 5 kept inactive
	}, 2*time.Second, 25*time.Millisecond, "worker should drain tombstones on tick")

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit after ctx cancel")
	}
}

// TestTombstoneWorker_RunOnce_DiscovererError surfaces the error
// from the workspace discoverer so an operator alerting hook can
// fire on persistent failures.
func TestTombstoneWorker_RunOnce_DiscovererError(t *testing.T) {
	store := newStore(t)
	worker := NewTombstoneWorker(store, TombstoneWorkerOptions{
		WorkspaceDiscoverer: func(_ context.Context) ([]string, error) {
			return nil, assert.AnError
		},
	}, logr.Discard())
	err := worker.RunOnce(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discover workspaces")
}
