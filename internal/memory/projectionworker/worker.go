/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package projectionworker

import (
	"context"
	"fmt"
	"time"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
	"github.com/altairalabs/omnia/internal/memory/consolidation"
	"github.com/go-logr/logr"
)

// RenderStore is the projection capability the worker needs (satisfied by
// *memory.PostgresMemoryStore).
type RenderStore interface {
	memory.ProjectionStore
}

// WorkerOptions configures the projection pre-render worker.
type WorkerOptions struct {
	Store      RenderStore
	Policies   consolidation.PolicyLister
	Workspaces consolidation.WorkspaceLister
	Lock       consolidation.LockStore
	Interval   time.Duration
	Metrics    *Metrics
	Now        func() time.Time
	Log        logr.Logger
}

// Worker pre-renders the workspace-wide Memory Galaxy projection for every
// MemoryPolicy with spec.projection.enabled, coordinated across replicas via
// a per-workspace advisory lock.
type Worker struct{ opts WorkerOptions }

// NewWorker constructs a Worker. A nil Now defaults to time.Now.
func NewWorker(opts WorkerOptions) *Worker {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Worker{opts: opts}
}

// Run does an immediate RunOnce, then ticks every Interval. Empty Interval
// disables the loop (but the initial pass still runs).
func (w *Worker) Run(ctx context.Context) {
	if err := w.RunOnce(ctx); err != nil {
		w.opts.Log.Error(err, "projection initial render pass failed")
	}
	if w.opts.Interval <= 0 {
		w.opts.Log.Info("projection worker: no interval, ran initial pass only")
		return
	}
	ticker := time.NewTicker(w.opts.Interval)
	defer ticker.Stop()
	w.opts.Log.Info("projection worker started", "interval", w.opts.Interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				w.opts.Log.Error(err, "projection render pass failed")
			}
		}
	}
}

// RunOnce renders every enabled workspace once.
func (w *Worker) RunOnce(ctx context.Context) error {
	policies, err := w.opts.Policies.List(ctx)
	if err != nil {
		return fmt.Errorf("list policies: %w", err)
	}
	for _, p := range policies {
		if p.Spec.Projection == nil || !p.Spec.Projection.Enabled {
			continue
		}
		w.runPolicy(ctx, p)
	}
	return nil
}

func (w *Worker) runPolicy(ctx context.Context, p memoryv1.MemoryPolicy) {
	wss, err := w.opts.Workspaces.ForPolicy(ctx, p.Name)
	if err != nil {
		w.opts.Log.Error(err, "list workspaces", "policy", p.Name)
		return
	}
	for _, ws := range wss {
		w.runWorkspace(ctx, p, ws)
	}
}

func (w *Worker) runWorkspace(ctx context.Context, p memoryv1.MemoryPolicy, ws consolidation.Workspace) {
	scope := map[string]string{memory.ScopeWorkspaceID: ws.UID}
	// Cheap pre-filter (no lock) so a steady-state poll over many workspaces
	// doesn't contend on the advisory lock when nothing has changed.
	if !w.needsRender(ctx, p, ws, scope) {
		return
	}
	w.renderLocked(ctx, p, ws, scope)
}

// needsRender reports whether scope requires a (re)render right now, reading
// the live fingerprint + stored layout and applying shouldRender. Logs and
// returns false on any store/parse error (the next tick retries).
func (w *Worker) needsRender(ctx context.Context, p memoryv1.MemoryPolicy, ws consolidation.Workspace, scope map[string]string) bool {
	live, err := w.opts.Store.ProjectionFingerprint(ctx, scope)
	if err != nil {
		w.opts.Log.Error(err, "fingerprint", "workspace", ws.UID)
		return false
	}
	if live == "" {
		return false // no memories
	}
	stored, err := w.opts.Store.LoadProjection(ctx, memory.ProjectionScopeKey(scope))
	if err != nil {
		w.opts.Log.Error(err, "load projection", "workspace", ws.UID)
		return false
	}
	render, err := shouldRender(stored, live, *p.Spec.Projection, w.opts.Now())
	if err != nil {
		w.opts.Log.Error(err, "shouldRender", "workspace", ws.UID, "policy", p.Name)
		return false
	}
	return render
}

// renderLocked acquires the per-workspace advisory lock and renders, recording
// metrics. It re-checks needsRender INSIDE the lock: a peer replica may have
// rendered between our pre-filter and acquiring the lock, so without this
// re-check two replicas could both render the same change (a wasted t-SNE pass).
func (w *Worker) renderLocked(ctx context.Context, p memoryv1.MemoryPolicy, ws consolidation.Workspace, scope map[string]string) {
	acquired, release, err := w.opts.Lock.TryLock(ctx, ws.UID, "projection")
	if err != nil {
		w.opts.Log.Error(err, "lock", "workspace", ws.UID)
		return
	}
	if !acquired {
		w.opts.Log.V(1).Info("projection skipped", "reason", "lock_held", "workspace", ws.UID)
		return
	}
	defer release()

	// Authoritative re-check under the lock — guarantees exactly one replica
	// renders each change.
	if !w.needsRender(ctx, p, ws, scope) {
		w.opts.Log.V(1).Info("projection skipped", "reason", "already_rendered", "workspace", ws.UID)
		return
	}

	start := w.opts.Now()
	if _, _, err := memory.Render(ctx, w.opts.Store, scope); err != nil {
		w.opts.Metrics.RendersTotal.WithLabelValues(ws.UID, p.Name, "error").Inc()
		w.opts.Log.Error(err, "render", "workspace", ws.UID, "policy", p.Name)
		return
	}
	w.opts.Metrics.RendersTotal.WithLabelValues(ws.UID, p.Name, "ok").Inc()
	w.opts.Metrics.RenderSeconds.WithLabelValues(ws.UID, p.Name).Observe(w.opts.Now().Sub(start).Seconds())
	w.opts.Log.V(1).Info("projection rendered", "workspace", ws.UID, "policy", p.Name)
}
