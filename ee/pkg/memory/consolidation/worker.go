/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// LockStore abstracts the per-workspace Postgres advisory lock used
// to deduplicate work across memory-api replicas.
type LockStore interface {
	// TryLock attempts to acquire a non-blocking advisory lock keyed by
	// (workspaceID, trigger). Returns true and a release function on
	// success; false (and a no-op release) when another replica holds
	// the lock.
	TryLock(ctx context.Context, workspaceID, trigger string) (bool, func(), error)
}

// PolicyLister returns the MemoryPolicy CRs the worker should process.
type PolicyLister interface {
	List(ctx context.Context) ([]memoryv1.MemoryPolicy, error)
}

// PreFilterRunner executes the SQL pre-filters and decodes the result
// rows into Buckets. One method per axis so each implementation maps
// to one SQL builder in prefilter.go.
type PreFilterRunner interface {
	RunStaleObservations(ctx context.Context, opts PreFilterOptions) ([]Bucket, error)
	RunCrossScopeCandidates(ctx context.Context, opts PreFilterOptions) ([]Bucket, error)
	RunEntityDuplicateCandidates(ctx context.Context, opts PreFilterOptions) ([]Bucket, error)
}

// RunTracker persists the last-run timestamp per (policy, workspace, axis)
// so the worker can honour per-axis cron schedules across restarts. Nil
// disables gating — axes run on every tick (used by unit tests).
type RunTracker interface {
	LastRun(ctx context.Context, policyName, workspaceID, axis string) (time.Time, bool, error)
	MarkRun(ctx context.Context, policyName, workspaceID, axis string, at time.Time) error
}

// WorkerOptions configures the consolidation worker.
type WorkerOptions struct {
	Store           Store
	LockStore       LockStore
	Policies        PolicyLister
	Workspaces      WorkspaceLister
	PreFilterRunner PreFilterRunner
	Auditor         Auditor
	Client          *Client
	Metrics         *Metrics
	Interval        time.Duration
	// RunTracker persists per-(policy, workspace, axis) last-run times so
	// per-axis cron schedules are honoured and survive restarts. Nil
	// disables gating (axes run every tick) — used by unit tests.
	RunTracker RunTracker
	Log        logr.Logger
	// PIIRedactor is passed to the per-axis Validator so the PII gate
	// has something to call. Nil disables the gate (tests, OSS builds
	// without EE deps).
	PIIRedactor PIIRedactor
	// LivenessMark / LivenessUnmark instrument the worker's running
	// gauge. Set from memory.MarkWorkerRunning / MarkWorkerStopped in
	// the memory-api wiring; nil-safe for tests.
	LivenessMark   func()
	LivenessUnmark func()
	// Now is injectable for tests; production wiring leaves it nil
	// (defaults to time.Now).
	Now func() time.Time
}

// Worker orchestrates consolidation across workspaces on a cron tick.
type Worker struct {
	opts         WorkerOptions
	applier      *Applier
	callFunction func(ctx context.Context, axis PreFilterAxis, ref memoryv1.MemoryFunctionRef, in FunctionInput) ([]Action, error)
}

// consolidationCronParser parses the standard 5-field cron expressions used
// by MemoryConsolidationConfig.Schedule and per-axis overrides. Matches the
// parser used by the retention worker (internal/memory/retention.go).
var consolidationCronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// axisDue reports whether an axis whose previous run was at `last` is due
// again at `now`, per the given cron schedule.
func axisDue(schedule string, last, now time.Time) (bool, error) {
	sched, err := consolidationCronParser.Parse(schedule)
	if err != nil {
		return false, err
	}
	return !sched.Next(last).After(now), nil
}

// NewWorker constructs a Worker. callFunction defaults to a thin
// wrapper around opts.Client.Call; tests override it.
func NewWorker(opts WorkerOptions) *Worker {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	w := &Worker{
		opts:    opts,
		applier: NewApplierWithAudit(opts.Store, opts.Auditor),
	}
	w.callFunction = w.defaultCallFunction
	return w
}

// Run blocks until ctx is cancelled, running RunOnce per tick.
// The liveness gauge is flipped on after the disabled-fast-path so a
// never-started worker stays at 0 (mirrors the CompactionWorker
// pattern; required by the worker-liveness alert rule).
func (w *Worker) Run(ctx context.Context) {
	if w.opts.Interval <= 0 {
		w.opts.Log.Info("consolidation worker disabled", "reason", "interval not set")
		return
	}
	if w.opts.LivenessMark != nil {
		w.opts.LivenessMark()
	}
	if w.opts.LivenessUnmark != nil {
		defer w.opts.LivenessUnmark()
	}
	ticker := time.NewTicker(w.opts.Interval)
	defer ticker.Stop()
	w.opts.Log.Info("consolidation worker started", "interval", w.opts.Interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				w.opts.Log.Error(err, "consolidation pass failed")
			}
		}
	}
}

// RunOnce performs a single consolidation pass across all configured
// policies. Per-workspace isolation: one workspace's failure does not
// affect others.
func (w *Worker) RunOnce(ctx context.Context) error {
	policies, err := w.opts.Policies.List(ctx)
	if err != nil {
		return fmt.Errorf("list policies: %w", err)
	}
	var firstErr error
	for _, p := range policies {
		if p.Spec.Consolidation == nil {
			continue
		}
		if err := w.runPolicy(ctx, p); err != nil {
			w.opts.Log.Error(err, "consolidation workspace failed", "workspace", p.Name)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (w *Worker) runPolicy(ctx context.Context, p memoryv1.MemoryPolicy) error {
	workspaces, err := w.listWorkspaces(ctx, p.Name)
	if err != nil {
		return fmt.Errorf("list workspaces for policy %s: %w", p.Name, err)
	}
	var firstErr error
	for _, ws := range workspaces {
		if err := w.runWorkspace(ctx, p, ws); err != nil {
			w.opts.Log.Error(err, "consolidation workspace failed",
				"policy", p.Name, "workspaceUID", ws.UID, "workspaceName", ws.Name)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// listWorkspaces resolves workspaces for a policy. When no WorkspaceLister
// is wired (e.g., legacy tests), falls back to a singleton entry keyed on
// the policy name — preserves the old single-workspace behavior so tests
// that haven't been updated still pass.
func (w *Worker) listWorkspaces(ctx context.Context, policyName string) ([]Workspace, error) {
	if w.opts.Workspaces == nil {
		return []Workspace{{Name: policyName, UID: policyName}}, nil
	}
	return w.opts.Workspaces.ForPolicy(ctx, policyName)
}

func (w *Worker) runWorkspace(ctx context.Context, p memoryv1.MemoryPolicy, ws Workspace) error {
	acquired, release, err := w.opts.LockStore.TryLock(ctx, ws.UID, "consolidation")
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	if !acquired {
		w.opts.Log.V(1).Info("consolidation tick skipped",
			"reason", "lock_unavailable",
			"workspaceUID", ws.UID,
			"policy", p.Name)
		return nil
	}
	defer release()

	// Per-policy wall-clock deadline so a runaway pack/HTTP call can't
	// pin the worker indefinitely. The lock is released by the defer
	// above; the deadline ends the (axis, function) loop early.
	_, wcTimeout := p.ResolvedTimeouts()
	wsCtx, cancel := context.WithTimeout(ctx, wcTimeout)
	defer cancel()

	refs := p.Spec.Consolidation.FunctionRefs
	gates := p.ResolvedSafetyGates()

	axes := []struct {
		axis PreFilterAxis
		ref  *memoryv1.MemoryFunctionRef
	}{
		{AxisStaleObservations, refs.StaleObservations},
		{AxisCrossScopeCandidates, refs.CrossScopeCandidates},
		{AxisEntityDuplicateCandidates, refs.EntityDuplicateCandidates},
	}
	for _, a := range axes {
		if a.ref == nil {
			continue
		}
		w.maybeRunAxis(wsCtx, a.axis, *a.ref, p, ws, gates)
	}
	return nil
}

// maybeRunAxis applies per-axis cron gating (via RunTracker) before
// delegating to runAxis. With no RunTracker wired the axis runs every tick
// (legacy behaviour). First sighting of a (policy, workspace, axis) tuple
// anchors last_ran_at to now and skips, so the first real run lands at the
// next cron occurrence rather than firing the moment a policy/workspace
// appears. last_ran_at is marked after every attempt — success or failure —
// so a broken pack waits for its next cron tick instead of retrying every
// poll.
func (w *Worker) maybeRunAxis(
	ctx context.Context,
	axis PreFilterAxis,
	ref memoryv1.MemoryFunctionRef,
	p memoryv1.MemoryPolicy,
	ws Workspace,
	gates memoryv1.MemoryConsolidationSafetyGates,
) {
	if w.opts.RunTracker == nil {
		w.runAxisLogged(ctx, axis, ref, p, ws, gates)
		return
	}

	now := w.opts.Now()
	axisStr := string(axis)
	last, ok, err := w.opts.RunTracker.LastRun(ctx, p.Name, ws.UID, axisStr)
	if err != nil {
		w.opts.Log.Error(err, "consolidation last-run lookup failed",
			"axis", axis, "workspaceUID", ws.UID, "policy", p.Name)
		return
	}
	if !ok {
		w.markRun(ctx, p.Name, ws.UID, axisStr, now) // anchor, do not run
		w.opts.Log.V(1).Info("consolidation axis anchored",
			"axis", axis, "workspaceUID", ws.UID, "policy", p.Name)
		return
	}

	due, err := axisDue(p.ResolvedSchedule(axisStr), last, now)
	if err != nil {
		w.opts.Log.Error(err, "consolidation invalid cron schedule",
			"axis", axis, "policy", p.Name, "schedule", p.ResolvedSchedule(axisStr))
		return
	}
	if !due {
		w.opts.Log.V(1).Info("consolidation axis not due",
			"axis", axis, "workspaceUID", ws.UID, "policy", p.Name)
		return
	}

	w.runAxisLogged(ctx, axis, ref, p, ws, gates)
	w.markRun(ctx, p.Name, ws.UID, axisStr, now) // mark-on-attempt
}

// runAxisLogged runs an axis and logs (but swallows) its error, matching the
// per-axis isolation the worker had before gating was added.
func (w *Worker) runAxisLogged(
	ctx context.Context,
	axis PreFilterAxis,
	ref memoryv1.MemoryFunctionRef,
	p memoryv1.MemoryPolicy,
	ws Workspace,
	gates memoryv1.MemoryConsolidationSafetyGates,
) {
	if err := w.runAxis(ctx, axis, ref, p, ws, gates); err != nil {
		w.opts.Log.Error(err, "axis failed",
			"axis", axis, "workspaceUID", ws.UID, "policy", p.Name)
	}
}

// markRun records a last-run timestamp, logging (but not propagating) errors.
func (w *Worker) markRun(ctx context.Context, policy, wsUID, axis string, at time.Time) {
	if err := w.opts.RunTracker.MarkRun(ctx, policy, wsUID, axis, at); err != nil {
		w.opts.Log.Error(err, "consolidation mark-run failed",
			"axis", axis, "workspaceUID", wsUID, "policy", policy)
	}
}

func (w *Worker) runAxis(
	ctx context.Context,
	axis PreFilterAxis,
	ref memoryv1.MemoryFunctionRef,
	p memoryv1.MemoryPolicy,
	ws Workspace,
	gates memoryv1.MemoryConsolidationSafetyGates,
) error {
	start := w.opts.Now()
	status := "ok"
	defer func() {
		if w.opts.Metrics != nil {
			w.opts.Metrics.PassesTotal.WithLabelValues(ws.UID, p.Name, ref.Name, status).Inc()
			w.opts.Metrics.PassDurationSeconds.WithLabelValues(ws.UID, p.Name, ref.Name).
				Observe(w.opts.Now().Sub(start).Seconds())
		}
	}()

	buckets, err := w.runPreFilter(ctx, axis, p, ws)
	if err != nil {
		status = "prefilter_error"
		return fmt.Errorf("prefilter %s: %w", axis, err)
	}
	if len(buckets) == 0 {
		status = "empty"
		return nil
	}
	input := FunctionInput{
		Axis:        axis,
		WorkspaceID: ws.UID,
		Buckets:     buckets,
		Gates: ResolvedGates{
			MinDistinctUserCount: gates.MinDistinctUserCount,
			RequirePIIRedaction:  gates.PIIRedactionEnabled(),
		},
	}
	fnStart := w.opts.Now()
	fnTimeout, _ := p.ResolvedTimeouts()
	callCtx, cancelCall := context.WithTimeout(ctx, fnTimeout)
	actions, err := w.callFunction(callCtx, axis, ref, input)
	cancelCall()
	if w.opts.Metrics != nil {
		w.opts.Metrics.FunctionCallDurationSeconds.WithLabelValues(ws.UID, p.Name, ref.Name).
			Observe(w.opts.Now().Sub(fnStart).Seconds())
	}
	if err != nil {
		status = "function_error"
		return fmt.Errorf("call function %s/%s: %w", ref.Namespace, ref.Name, err)
	}
	v := NewValidator(ValidatorOptions{
		WorkspaceID: ws.UID,
		Gates:       gates,
		PIIRedactor: w.opts.PIIRedactor,
	})
	results := v.Validate(actions, w.buildValidationContext(buckets))
	w.recordActionMetrics(ws.UID, p.Name, ref.Name, results)
	if err := w.applier.Apply(ctx, ApplyContext{
		WorkspaceID: ws.UID,
		RunID:       fmt.Sprintf("%s-%d", ws.UID, w.opts.Now().Unix()),
		PackRef:     ref.Name,
		Now:         w.opts.Now(),
	}, results); err != nil {
		status = "apply_error"
		return err
	}
	return nil
}

// recordActionMetrics emits the per-action ActionsTotal counter for
// every validated action result. Tier label is derived from the
// rescope action's destination scope; non-rescope actions get an
// empty tier label.
func (w *Worker) recordActionMetrics(workspaceUID, policy, function string, results []Result) {
	if w.opts.Metrics == nil {
		return
	}
	for _, r := range results {
		outcome := OutcomeApplied
		if !r.Accepted {
			outcome = "rejected_" + r.Reason
		}
		tier := ""
		if rs, ok := r.Action.(RescopeAction); ok {
			tier = string(rs.NewScope.Shape())
		}
		w.opts.Metrics.ActionsTotal.WithLabelValues(
			workspaceUID, policy, function, string(r.Action.Kind()), outcome, tier,
		).Inc()
	}
}

// runPreFilter dispatches to the matching PreFilterRunner method.
func (w *Worker) runPreFilter(ctx context.Context, axis PreFilterAxis, p memoryv1.MemoryPolicy, ws Workspace) ([]Bucket, error) {
	if w.opts.PreFilterRunner == nil {
		return nil, fmt.Errorf("no PreFilterRunner configured")
	}
	opts := w.buildPreFilterOptions(p, ws)
	switch axis {
	case AxisStaleObservations:
		return w.opts.PreFilterRunner.RunStaleObservations(ctx, opts)
	case AxisCrossScopeCandidates:
		return w.opts.PreFilterRunner.RunCrossScopeCandidates(ctx, opts)
	case AxisEntityDuplicateCandidates:
		return w.opts.PreFilterRunner.RunEntityDuplicateCandidates(ctx, opts)
	default:
		return nil, fmt.Errorf("unknown axis: %s", axis)
	}
}

// buildPreFilterOptions assembles PreFilterOptions from the policy's
// configuration with sensible defaults.
func (w *Worker) buildPreFilterOptions(p memoryv1.MemoryPolicy, ws Workspace) PreFilterOptions {
	opts := PreFilterOptions{
		WorkspaceID:       ws.UID,
		MaxBucketsPerPass: 100,
		MaxPerBucket:      50,
		OlderThan:         w.opts.Now().Add(-30 * 24 * time.Hour),
		MinGroupSize:      5,
		MinDistinctUsers:  int(p.ResolvedSafetyGates().MinDistinctUserCount["agentScoped"]),
		SimilarityFloor:   0.85,
	}
	if p.Spec.Consolidation != nil && p.Spec.Consolidation.CandidateLimits != nil {
		if v := p.Spec.Consolidation.CandidateLimits.MaxBucketsPerPass; v > 0 {
			opts.MaxBucketsPerPass = int(v)
		}
		if v := p.Spec.Consolidation.CandidateLimits.MaxPerBucket; v > 0 {
			opts.MaxPerBucket = int(v)
		}
	}
	return opts
}

// buildValidationContext extracts row mutability + bucket stats so the
// validator can apply mutability + k-anonymity gates.
func (w *Worker) buildValidationContext(buckets []Bucket) ValidationContext {
	mut := make(map[string]string)
	scope := make(map[string]Scope)
	distinct := 0
	for _, b := range buckets {
		if d, ok := b.Stats["distinctUsers"].(int); ok && d > distinct {
			distinct = d
		}
		for _, e := range b.Entries {
			mut[e.ID] = e.Mutability
			scope[e.ID] = e.Scope
		}
	}
	return ValidationContext{
		RowMutability:           mut,
		RowScope:                scope,
		BucketDistinctUserCount: distinct,
	}
}

func (w *Worker) defaultCallFunction(ctx context.Context, _ PreFilterAxis, ref memoryv1.MemoryFunctionRef, in FunctionInput) ([]Action, error) {
	if w.opts.Client == nil {
		return nil, fmt.Errorf("no function client configured")
	}
	return w.opts.Client.Call(ctx, ref, in)
}
