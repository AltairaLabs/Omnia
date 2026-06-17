/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package projectionworker

import (
	"context"
	"errors"
	"testing"
	"time"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/memory/consolidation"
	"github.com/altairalabs/omnia/internal/memory"
	"github.com/go-logr/logr/testr"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

const (
	testPolicyName = "p1"
	testWSUID      = "ws-uid"
	testFP5        = "5:1"
	testScopeKey   = testWSUID + "||" // ProjectionScopeKey for {workspace: ws-uid}
)

func fixedNow() time.Time { return time.Unix(0, 0) }

type fakePolicyLister struct {
	policies []memoryv1.MemoryPolicy
	err      error
}

func (f fakePolicyLister) List(context.Context) ([]memoryv1.MemoryPolicy, error) {
	return f.policies, f.err
}

type fakeWorkspaceLister struct {
	byPolicy map[string][]consolidation.Workspace
	err      error
}

func (f fakeWorkspaceLister) ForPolicy(_ context.Context, p string) ([]consolidation.Workspace, error) {
	return f.byPolicy[p], f.err
}

type fakeLock struct {
	ok  bool
	err error
}

func (f fakeLock) TryLock(context.Context, string, string) (bool, func(), error) {
	return f.ok, func() {}, f.err
}

// fakeRenderStore records renders + drives fingerprints. The *Err fields inject
// failures so the worker's error branches are exercised; onSave fires after each
// successful SaveProjection (used to drive the ticker loop deterministically).
type fakeRenderStore struct {
	fingerprint string
	stored      *memory.StoredProjection
	rendered    []string // scopeKeys successfully rendered
	fpErr       error    // ProjectionFingerprint failure
	loadErr     error    // LoadProjection failure
	saveErr     error    // SaveProjection failure (render error path)
	onSave      func()
}

func (f *fakeRenderStore) ProjectionFingerprint(context.Context, map[string]string) (string, error) {
	return f.fingerprint, f.fpErr
}

func (f *fakeRenderStore) LoadProjectionInputs(context.Context, map[string]string) ([]memory.ProjectionInput, error) {
	return []memory.ProjectionInput{{EntityID: "e1", Content: "x", Tier: "institutional"}}, nil
}

func (f *fakeRenderStore) LoadProjection(context.Context, string) (*memory.StoredProjection, error) {
	return f.stored, f.loadErr
}

func (f *fakeRenderStore) SaveProjection(_ context.Context, key, _, _, _, _ string, _ []memory.ProjectionPoint) error {
	if f.saveErr != nil {
		return f.saveErr // not recorded — render failed
	}
	f.rendered = append(f.rendered, key)
	if f.onSave != nil {
		f.onSave()
	}
	return nil
}

func projectionPolicy(enabled bool) memoryv1.MemoryPolicy {
	p := memoryv1.MemoryPolicy{}
	p.Name = testPolicyName
	if enabled {
		p.Spec.Projection = &memoryv1.MemoryProjectionConfig{Enabled: true}
	}
	return p
}

// oneWorkspace maps the test policy to a single workspace UID.
func oneWorkspace() fakeWorkspaceLister {
	return fakeWorkspaceLister{byPolicy: map[string][]consolidation.Workspace{
		testPolicyName: {{Name: "w", UID: testWSUID}},
	}}
}

// workerOpts builds WorkerOptions for the single enabled-policy / one-workspace
// case, overridable by the caller before constructing the worker.
func workerOpts(t *testing.T, store RenderStore, m *Metrics) WorkerOptions {
	t.Helper()
	return WorkerOptions{
		Store:      store,
		Policies:   fakePolicyLister{policies: []memoryv1.MemoryPolicy{projectionPolicy(true)}},
		Workspaces: oneWorkspace(),
		Lock:       fakeLock{ok: true},
		Metrics:    m,
		Now:        fixedNow,
		Log:        testr.New(t),
	}
}

func TestRunOnce_RendersEnabledWorkspaceColdScope(t *testing.T) {
	store := &fakeRenderStore{fingerprint: testFP5, stored: nil} // cold
	// Omit Now to also cover NewWorker's time.Now default — a cold scope
	// renders regardless of the clock.
	opts := workerOpts(t, store, NewMetrics())
	opts.Now = nil
	w := NewWorker(opts)
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.rendered) != 1 || store.rendered[0] != testScopeKey {
		t.Errorf("rendered = %v, want [%s]", store.rendered, testScopeKey)
	}
}

func TestRunOnce_SkipsDisabledPolicy(t *testing.T) {
	store := &fakeRenderStore{fingerprint: testFP5}
	opts := workerOpts(t, store, NewMetrics())
	opts.Policies = fakePolicyLister{policies: []memoryv1.MemoryPolicy{projectionPolicy(false)}}
	w := NewWorker(opts)
	_ = w.RunOnce(context.Background())
	if len(store.rendered) != 0 {
		t.Errorf("disabled policy rendered: %v", store.rendered)
	}
}

func TestRunOnce_SkipsWhenLockHeld(t *testing.T) {
	store := &fakeRenderStore{fingerprint: testFP5}
	opts := workerOpts(t, store, NewMetrics())
	opts.Lock = fakeLock{ok: false} // held by another pod
	w := NewWorker(opts)
	_ = w.RunOnce(context.Background())
	if len(store.rendered) != 0 {
		t.Errorf("rendered despite lock held: %v", store.rendered)
	}
}

// TestRunOnce_SkipsUnchangedFingerprint proves the steady-state poll: a stored
// layout whose fingerprint matches live is NOT re-rendered (shouldRender=false).
func TestRunOnce_SkipsUnchangedFingerprint(t *testing.T) {
	store := &fakeRenderStore{
		fingerprint: testFP5,
		stored:      &memory.StoredProjection{Fingerprint: testFP5},
	}
	w := NewWorker(workerOpts(t, store, NewMetrics()))
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.rendered) != 0 {
		t.Errorf("re-rendered an unchanged scope: %v", store.rendered)
	}
}

// TestRunOnce_SkipsEmptyScope proves a workspace with no memories (empty
// fingerprint) is skipped without touching the lock or rendering.
func TestRunOnce_SkipsEmptyScope(t *testing.T) {
	store := &fakeRenderStore{fingerprint: ""} // no memories
	w := NewWorker(workerOpts(t, store, NewMetrics()))
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.rendered) != 0 {
		t.Errorf("rendered an empty scope: %v", store.rendered)
	}
}

// TestRunOnce_RecordsErrorMetricOnRenderFailure proves the render error branch:
// SaveProjection fails, nothing is recorded, and the error counter increments.
func TestRunOnce_RecordsErrorMetricOnRenderFailure(t *testing.T) {
	store := &fakeRenderStore{fingerprint: testFP5, saveErr: errors.New("save boom")}
	m := NewMetrics()
	w := NewWorker(workerOpts(t, store, m))
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.rendered) != 0 {
		t.Errorf("recorded a render despite SaveProjection error: %v", store.rendered)
	}
	if got := testutil.ToFloat64(m.RendersTotal.WithLabelValues(testWSUID, testPolicyName, "error")); got != 1 {
		t.Errorf("error render counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.RendersTotal.WithLabelValues(testWSUID, testPolicyName, "ok")); got != 0 {
		t.Errorf("ok render counter = %v, want 0", got)
	}
}

// TestRunOnce_ReturnsErrorWhenPolicyListFails proves RunOnce surfaces a
// policy-list failure (the worker's only hard error).
func TestRunOnce_ReturnsErrorWhenPolicyListFails(t *testing.T) {
	store := &fakeRenderStore{fingerprint: testFP5}
	opts := workerOpts(t, store, NewMetrics())
	opts.Policies = fakePolicyLister{err: errors.New("list boom")}
	w := NewWorker(opts)
	if err := w.RunOnce(context.Background()); err == nil {
		t.Fatal("expected error when policy list fails")
	}
}

// TestRunOnce_SkipsWhenWorkspaceListFails proves a workspace-list failure is
// logged-and-skipped (not fatal — other policies still process).
func TestRunOnce_SkipsWhenWorkspaceListFails(t *testing.T) {
	store := &fakeRenderStore{fingerprint: testFP5}
	opts := workerOpts(t, store, NewMetrics())
	opts.Workspaces = fakeWorkspaceLister{err: errors.New("ws boom")}
	w := NewWorker(opts)
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.rendered) != 0 {
		t.Errorf("rendered despite workspace-list error: %v", store.rendered)
	}
}

// TestRunOnce_SkipsOnFingerprintError covers needsRender's fingerprint-error
// branch.
func TestRunOnce_SkipsOnFingerprintError(t *testing.T) {
	store := &fakeRenderStore{fingerprint: testFP5, fpErr: errors.New("fp boom")}
	w := NewWorker(workerOpts(t, store, NewMetrics()))
	_ = w.RunOnce(context.Background())
	if len(store.rendered) != 0 {
		t.Errorf("rendered despite fingerprint error: %v", store.rendered)
	}
}

// TestRunOnce_SkipsOnLoadProjectionError covers needsRender's load-error branch.
func TestRunOnce_SkipsOnLoadProjectionError(t *testing.T) {
	store := &fakeRenderStore{fingerprint: testFP5, loadErr: errors.New("load boom")}
	w := NewWorker(workerOpts(t, store, NewMetrics()))
	_ = w.RunOnce(context.Background())
	if len(store.rendered) != 0 {
		t.Errorf("rendered despite load error: %v", store.rendered)
	}
}

// TestRunOnce_SkipsOnInvalidCronSchedule covers shouldRender's cron-parse-error
// branch (surfaced through needsRender): a changed fingerprint plus a malformed
// schedule must not render.
func TestRunOnce_SkipsOnInvalidCronSchedule(t *testing.T) {
	store := &fakeRenderStore{
		fingerprint: "6:2",
		stored:      &memory.StoredProjection{Fingerprint: testFP5}, // changed → gates apply
	}
	p := projectionPolicy(true)
	p.Spec.Projection.Schedule = "not-a-cron"
	opts := workerOpts(t, store, NewMetrics())
	opts.Policies = fakePolicyLister{policies: []memoryv1.MemoryPolicy{p}}
	w := NewWorker(opts)
	_ = w.RunOnce(context.Background())
	if len(store.rendered) != 0 {
		t.Errorf("rendered despite invalid cron schedule: %v", store.rendered)
	}
}

// TestRunOnce_SkipsOnLockError covers renderLocked's TryLock-error branch.
func TestRunOnce_SkipsOnLockError(t *testing.T) {
	store := &fakeRenderStore{fingerprint: testFP5} // cold → passes needsRender pre-filter
	opts := workerOpts(t, store, NewMetrics())
	opts.Lock = fakeLock{ok: false, err: errors.New("lock boom")}
	w := NewWorker(opts)
	_ = w.RunOnce(context.Background())
	if len(store.rendered) != 0 {
		t.Errorf("rendered despite lock error: %v", store.rendered)
	}
}

// TestRun_NoIntervalRunsInitialPassOnly proves Run does one immediate pass then
// returns when Interval is unset (the loop is disabled).
func TestRun_NoIntervalRunsInitialPassOnly(t *testing.T) {
	store := &fakeRenderStore{fingerprint: testFP5}
	opts := workerOpts(t, store, NewMetrics())
	opts.Interval = 0
	w := NewWorker(opts)
	w.Run(context.Background()) // returns after the initial pass
	if len(store.rendered) != 1 {
		t.Errorf("initial pass rendered %d, want 1", len(store.rendered))
	}
}

// TestRun_LogsInitialPassError proves Run tolerates (logs) an initial-pass error
// and still returns when the loop is disabled.
func TestRun_LogsInitialPassError(t *testing.T) {
	opts := workerOpts(t, &fakeRenderStore{fingerprint: testFP5}, NewMetrics())
	opts.Policies = fakePolicyLister{err: errors.New("list boom")}
	opts.Interval = 0
	w := NewWorker(opts)
	w.Run(context.Background()) // must not panic or hang
}

// TestRun_TicksThenStopsOnContextCancel covers the ticker loop and the ctx.Done
// return: the store cancels the context after the first ticked render so the
// loop body (case <-ticker.C) and the cancel path both execute.
func TestRun_TicksThenStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var saves int
	store := &fakeRenderStore{fingerprint: testFP5}
	store.onSave = func() {
		saves++
		if saves >= 2 { // initial pass + one ticker tick
			cancel()
		}
	}
	opts := workerOpts(t, store, NewMetrics())
	opts.Interval = time.Millisecond
	w := NewWorker(opts)
	w.Run(ctx) // blocks until ctx is cancelled by the second render
	if saves < 2 {
		t.Errorf("ticker did not fire: saves=%d, want >=2", saves)
	}
}
