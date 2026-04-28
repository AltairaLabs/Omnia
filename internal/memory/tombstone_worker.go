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
	"fmt"
	"time"

	"github.com/go-logr/logr"
)

// TombstoneWorkerOptions configures the background tombstone GC.
//
// The worker runs RunTombstoneGC per workspace on each tick. The
// per-workspace iteration mirrors the compaction worker so a noisy
// workspace can't burn the whole pass on one chain — each workspace
// gets its own bounded delete and the next one runs even if the
// previous failed.
type TombstoneWorkerOptions struct {
	// Interval between passes. A few hours is sensible — the worker
	// is meant to bound storage over time, not block writes. Zero
	// disables the worker.
	Interval time.Duration
	// WorkspaceIDs the worker scans each tick. If non-empty the
	// worker uses this fixed list and WorkspaceDiscoverer is ignored.
	// Intended for tests and operators who want to pin GC to a
	// subset of workspaces.
	WorkspaceIDs []string
	// WorkspaceDiscoverer is called at each tick when WorkspaceIDs is
	// empty. Typical wiring is (*PostgresMemoryStore).ListWorkspaceIDs
	// so newly-seen workspaces get GC'd without a service restart.
	WorkspaceDiscoverer func(context.Context) ([]string, error)
	// MinAge / MinInactiveCount / KeepRecentInactive forwarded to
	// every per-workspace RunTombstoneGC call. Zero-valued fields
	// fall through to the store-level defaults.
	MinAge             time.Duration
	MinInactiveCount   int
	KeepRecentInactive int
}

// TombstoneWorker periodically hard-deletes the older inactive
// observations on long supersession chains. Modelled after
// CompactionWorker so the operational surface (Run / RunOnce /
// log shape) is familiar to anyone who's already wired that.
type TombstoneWorker struct {
	store *PostgresMemoryStore
	opts  TombstoneWorkerOptions
	log   logr.Logger
}

// NewTombstoneWorker constructs a worker. Interval and workspace
// resolution gate whether Run actually fires; ops can construct
// unconditionally and let configuration decide.
func NewTombstoneWorker(store *PostgresMemoryStore, opts TombstoneWorkerOptions, log logr.Logger) *TombstoneWorker {
	return &TombstoneWorker{
		store: store,
		opts:  opts,
		log:   log.WithName("tombstone-worker"),
	}
}

// Run blocks until ctx is cancelled, running RunOnce at each tick.
// A non-positive interval disables the worker — it logs the reason
// and returns immediately so binaries can wire it unconditionally.
func (w *TombstoneWorker) Run(ctx context.Context) {
	if w.opts.Interval <= 0 {
		w.log.Info("tombstone worker disabled", "reason", "interval not set")
		return
	}
	MarkWorkerRunning(WorkerNameTombstoneGC)
	defer MarkWorkerStopped(WorkerNameTombstoneGC)

	ticker := time.NewTicker(w.opts.Interval)
	defer ticker.Stop()

	w.log.Info("tombstone worker started",
		"interval", w.opts.Interval,
		"minAge", w.opts.MinAge,
		"minInactiveCount", w.opts.MinInactiveCount,
		"keepRecent", w.opts.KeepRecentInactive,
	)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("tombstone worker stopped")
			return
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				w.log.Error(err, "tombstone pass failed")
			}
		}
	}
}

// RunOnce performs a single GC pass across every configured /
// discovered workspace. Per-workspace failures are logged but do
// not abort the pass — one bad workspace shouldn't block the rest.
// Returns the first error seen.
func (w *TombstoneWorker) RunOnce(ctx context.Context) error {
	workspaces, err := w.resolveWorkspaces(ctx)
	if err != nil {
		return err
	}
	if len(workspaces) == 0 {
		return nil
	}

	var firstErr error
	var totalDeleted int64
	for _, ws := range workspaces {
		deleted, err := w.store.RunTombstoneGC(ctx, TombstoneGCOptions{
			WorkspaceID:        ws,
			MinAge:             w.opts.MinAge,
			MinInactiveCount:   w.opts.MinInactiveCount,
			KeepRecentInactive: w.opts.KeepRecentInactive,
		})
		if err != nil {
			w.log.Error(err, "tombstone gc workspace failed", "workspace", ws)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		totalDeleted += deleted
	}
	w.log.V(1).Info("tombstone pass complete",
		"workspaces", len(workspaces),
		"deleted", totalDeleted,
	)
	return firstErr
}

// resolveWorkspaces returns the workspace IDs the worker should scan
// this pass. Static WorkspaceIDs wins; otherwise WorkspaceDiscoverer
// is consulted so new workspaces don't require a service restart.
func (w *TombstoneWorker) resolveWorkspaces(ctx context.Context) ([]string, error) {
	if len(w.opts.WorkspaceIDs) > 0 {
		return w.opts.WorkspaceIDs, nil
	}
	if w.opts.WorkspaceDiscoverer == nil {
		return nil, nil
	}
	ids, err := w.opts.WorkspaceDiscoverer(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover workspaces: %w", err)
	}
	return ids, nil
}
