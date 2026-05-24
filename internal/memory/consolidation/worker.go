/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

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

// WorkerOptions configures the consolidation worker.
type WorkerOptions struct {
	Store           Store
	LockStore       LockStore
	Policies        PolicyLister
	PreFilterRunner PreFilterRunner
	Auditor         Auditor
	Client          *Client
	Metrics         *Metrics
	Interval        time.Duration
	Log             logr.Logger
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
	acquired, release, err := w.opts.LockStore.TryLock(ctx, p.Name, "consolidation")
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	if !acquired {
		w.opts.Log.V(1).Info("consolidation tick skipped",
			"reason", "lock_unavailable",
			"workspace", p.Name)
		return nil
	}
	defer release()

	refs := p.Spec.Consolidation.FunctionRefs
	gates := p.ResolvedSafetyGates()

	const msgAxisFailed = "axis failed"

	if refs.StaleObservations != nil {
		if err := w.runAxis(ctx, AxisStaleObservations, *refs.StaleObservations, p, gates); err != nil {
			w.opts.Log.Error(err, msgAxisFailed,
				"axis", AxisStaleObservations, "workspace", p.Name)
		}
	}
	if refs.CrossScopeCandidates != nil {
		if err := w.runAxis(ctx, AxisCrossScopeCandidates, *refs.CrossScopeCandidates, p, gates); err != nil {
			w.opts.Log.Error(err, msgAxisFailed,
				"axis", AxisCrossScopeCandidates, "workspace", p.Name)
		}
	}
	if refs.EntityDuplicateCandidates != nil {
		if err := w.runAxis(ctx, AxisEntityDuplicateCandidates, *refs.EntityDuplicateCandidates, p, gates); err != nil {
			w.opts.Log.Error(err, msgAxisFailed,
				"axis", AxisEntityDuplicateCandidates, "workspace", p.Name)
		}
	}
	return nil
}

func (w *Worker) runAxis(ctx context.Context, axis PreFilterAxis, ref memoryv1.MemoryFunctionRef, p memoryv1.MemoryPolicy, gates memoryv1.MemoryConsolidationSafetyGates) error {
	start := w.opts.Now()
	status := "ok"
	defer func() {
		if w.opts.Metrics != nil {
			w.opts.Metrics.PassesTotal.WithLabelValues(p.Name, ref.Name, status).Inc()
			w.opts.Metrics.PassDurationSeconds.WithLabelValues(p.Name, ref.Name).
				Observe(w.opts.Now().Sub(start).Seconds())
		}
	}()

	buckets, err := w.runPreFilter(ctx, axis, p)
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
		WorkspaceID: p.Name,
		Buckets:     buckets,
		Gates: ResolvedGates{
			MinDistinctUserCount: gates.MinDistinctUserCount,
			RequirePIIRedaction:  gates.RequirePIIRedaction,
		},
	}
	fnStart := w.opts.Now()
	actions, err := w.callFunction(ctx, axis, ref, input)
	if w.opts.Metrics != nil {
		w.opts.Metrics.FunctionCallDurationSeconds.WithLabelValues(p.Name, ref.Name).
			Observe(w.opts.Now().Sub(fnStart).Seconds())
	}
	if err != nil {
		status = "function_error"
		return fmt.Errorf("call function %s/%s: %w", ref.Namespace, ref.Name, err)
	}
	v := NewValidator(ValidatorOptions{WorkspaceID: p.Name, Gates: gates})
	results := v.Validate(actions, w.buildValidationContext(buckets))
	w.recordActionMetrics(p.Name, ref.Name, results)
	if err := w.applier.Apply(ctx, ApplyContext{
		WorkspaceID: p.Name,
		RunID:       fmt.Sprintf("%s-%d", p.Name, w.opts.Now().Unix()),
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
func (w *Worker) recordActionMetrics(workspace, function string, results []Result) {
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
			workspace, function, string(r.Action.Kind()), outcome, tier,
		).Inc()
	}
}

// runPreFilter dispatches to the matching PreFilterRunner method.
func (w *Worker) runPreFilter(ctx context.Context, axis PreFilterAxis, p memoryv1.MemoryPolicy) ([]Bucket, error) {
	if w.opts.PreFilterRunner == nil {
		return nil, fmt.Errorf("no PreFilterRunner configured")
	}
	opts := w.buildPreFilterOptions(p)
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
func (w *Worker) buildPreFilterOptions(p memoryv1.MemoryPolicy) PreFilterOptions {
	opts := PreFilterOptions{
		WorkspaceID:       p.Name,
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
	return w.opts.Client.Call(ctx, ref.Name, in)
}
