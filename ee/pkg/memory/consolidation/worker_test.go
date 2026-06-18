/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestWorker_SkipsAxisWithoutFunctionRef(t *testing.T) {
	fakeStore := &MockStore{}
	w := NewWorker(WorkerOptions{
		Store:           fakeStore,
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: oneStaleOnlyPolicy()},
		PreFilterRunner: &mockPreFilterRunner{},
		Log:             testr.New(t),
		Now:             func() time.Time { return time.Unix(0, 0) },
	})

	called := map[PreFilterAxis]int{}
	w.callFunction = func(_ context.Context, ax PreFilterAxis, _ memoryv1.MemoryFunctionRef, _ FunctionInput) ([]Action, error) {
		called[ax]++
		return nil, nil
	}

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	// Even when the runner returns no buckets, the function should not
	// be called (we skip empty pre-filter results to save tokens).
	if called[AxisStaleObservations] != 0 {
		t.Errorf("staleObservations called %d times; runner returned empty buckets so we should skip", called[AxisStaleObservations])
	}
	if called[AxisCrossScopeCandidates] != 0 {
		t.Errorf("crossScopeCandidates should be skipped (no functionRef)")
	}
}

func TestWorker_PassesPreFilterBucketsToFunction(t *testing.T) {
	runner := &mockPreFilterRunner{
		stale: []Bucket{{Key: "kind=preference;name=units",
			Entries: []BucketEntry{{ID: testObsID, Mutability: MutabilityMutable}},
		}},
	}
	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: oneStaleOnlyPolicy()},
		PreFilterRunner: runner,
		Log:             testr.New(t),
	})
	var received FunctionInput
	w.callFunction = func(_ context.Context, _ PreFilterAxis, _ memoryv1.MemoryFunctionRef, in FunctionInput) ([]Action, error) {
		received = in
		return nil, nil
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(received.Buckets) != 1 || received.Buckets[0].Key != "kind=preference;name=units" {
		t.Errorf("buckets not passed to function: %+v", received.Buckets)
	}
}

func TestWorker_SkipsTickWhenLockUnavailable(t *testing.T) {
	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: false},
		Policies:        fakePolicyLister{policies: oneStaleOnlyPolicy()},
		PreFilterRunner: &mockPreFilterRunner{},
		Log:             testr.New(t),
	})
	called := false
	w.callFunction = func(context.Context, PreFilterAxis, memoryv1.MemoryFunctionRef, FunctionInput) ([]Action, error) {
		called = true
		return nil, nil
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("function should not be called when lock unavailable")
	}
}

func TestWorker_PoliciesWithoutConsolidationAreSkipped(t *testing.T) {
	plainPolicy := memoryv1.MemoryPolicy{}
	plainPolicy.Name = "ws-no-consolidation"
	// no Spec.Consolidation set

	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: []memoryv1.MemoryPolicy{plainPolicy}},
		PreFilterRunner: &mockPreFilterRunner{},
		Log:             testr.New(t),
	})
	called := false
	w.callFunction = func(context.Context, PreFilterAxis, memoryv1.MemoryFunctionRef, FunctionInput) ([]Action, error) {
		called = true
		return nil, nil
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("policy without Consolidation should be skipped")
	}
}

// fakeRunTracker is an in-memory RunTracker for gating tests.
type fakeRunTracker struct {
	last   map[string]time.Time // key: policy|ws|axis
	marked map[string]time.Time // records every MarkRun
}

func newFakeRunTracker() *fakeRunTracker {
	return &fakeRunTracker{last: map[string]time.Time{}, marked: map[string]time.Time{}}
}

func rtKey(policy, ws, axis string) string { return policy + "|" + ws + "|" + axis }

func (f *fakeRunTracker) LastRun(_ context.Context, policy, ws, axis string) (time.Time, bool, error) {
	t, ok := f.last[rtKey(policy, ws, axis)]
	return t, ok, nil
}

func (f *fakeRunTracker) MarkRun(_ context.Context, policy, ws, axis string, at time.Time) error {
	f.last[rtKey(policy, ws, axis)] = at
	f.marked[rtKey(policy, ws, axis)] = at
	return nil
}

func TestWorker_FirstSightingAnchorsWithoutRunning(t *testing.T) {
	tracker := newFakeRunTracker()
	now := time.Date(2026, 5, 29, 14, 0, 0, 0, time.UTC)
	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: oneStaleOnlyPolicy()},
		PreFilterRunner: &mockPreFilterRunner{stale: []Bucket{{Key: "k", Entries: []BucketEntry{{ID: testObsID, Mutability: MutabilityMutable}}}}},
		RunTracker:      tracker,
		Log:             testr.New(t),
		Now:             func() time.Time { return now },
	})
	called := false
	w.callFunction = func(context.Context, PreFilterAxis, memoryv1.MemoryFunctionRef, FunctionInput) ([]Action, error) {
		called = true
		return nil, nil
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("first sighting must anchor without running the axis")
	}
	if _, ok := tracker.marked[rtKey(testWorkspaceID, testWorkspaceID, "staleObservations")]; !ok {
		t.Error("first sighting must record an anchor via MarkRun")
	}
}

func TestWorker_RunsAxisWhenDue(t *testing.T) {
	tracker := newFakeRunTracker()
	now := time.Date(2026, 5, 29, 14, 0, 0, 0, time.UTC)
	// Last ran 25h ago; default schedule "0 2 * * *" => next after last is
	// well before now, so it is due.
	tracker.last[rtKey(testWorkspaceID, testWorkspaceID, "staleObservations")] = now.Add(-25 * time.Hour)
	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: oneStaleOnlyPolicy()},
		PreFilterRunner: &mockPreFilterRunner{stale: []Bucket{{Key: "k", Entries: []BucketEntry{{ID: testObsID, Mutability: MutabilityMutable}}}}},
		RunTracker:      tracker,
		Log:             testr.New(t),
		Now:             func() time.Time { return now },
	})
	called := 0
	w.callFunction = func(context.Context, PreFilterAxis, memoryv1.MemoryFunctionRef, FunctionInput) ([]Action, error) {
		called++
		return nil, nil
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Errorf("expected axis to run once when due, ran %d", called)
	}
	if got := tracker.marked[rtKey(testWorkspaceID, testWorkspaceID, "staleObservations")]; !got.Equal(now) {
		t.Errorf("expected MarkRun(now) after running, got %v", got)
	}
}

func TestWorker_SkipsAxisWhenNotDue(t *testing.T) {
	tracker := newFakeRunTracker()
	now := time.Date(2026, 5, 29, 14, 0, 0, 0, time.UTC)
	// Ran 1 minute ago; default daily schedule => not due again yet.
	ranAt := now.Add(-time.Minute)
	tracker.last[rtKey(testWorkspaceID, testWorkspaceID, "staleObservations")] = ranAt
	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: oneStaleOnlyPolicy()},
		PreFilterRunner: &mockPreFilterRunner{stale: []Bucket{{Key: "k", Entries: []BucketEntry{{ID: testObsID, Mutability: MutabilityMutable}}}}},
		RunTracker:      tracker,
		Log:             testr.New(t),
		Now:             func() time.Time { return now },
	})
	called := false
	w.callFunction = func(context.Context, PreFilterAxis, memoryv1.MemoryFunctionRef, FunctionInput) ([]Action, error) {
		called = true
		return nil, nil
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("axis ran while not due")
	}
	// last_ran_at must be unchanged when not due.
	if got := tracker.last[rtKey(testWorkspaceID, testWorkspaceID, "staleObservations")]; !got.Equal(ranAt) {
		t.Errorf("last_ran_at changed on a not-due tick: %v", got)
	}
}

func TestWorker_MarksRunEvenWhenAxisErrors(t *testing.T) {
	tracker := newFakeRunTracker()
	now := time.Date(2026, 5, 29, 14, 0, 0, 0, time.UTC)
	tracker.last[rtKey(testWorkspaceID, testWorkspaceID, "staleObservations")] = now.Add(-25 * time.Hour)
	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: oneStaleOnlyPolicy()},
		PreFilterRunner: &mockPreFilterRunner{stale: []Bucket{{Key: "k", Entries: []BucketEntry{{ID: testObsID, Mutability: MutabilityMutable}}}}},
		RunTracker:      tracker,
		Log:             testr.New(t),
		Now:             func() time.Time { return now },
	})
	w.callFunction = func(context.Context, PreFilterAxis, memoryv1.MemoryFunctionRef, FunctionInput) ([]Action, error) {
		return nil, errFakeStoreFailure // axis fails
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := tracker.marked[rtKey(testWorkspaceID, testWorkspaceID, "staleObservations")]; !got.Equal(now) {
		t.Errorf("mark-on-attempt: expected MarkRun(now) even on axis error, got %v", got)
	}
}

// --- fakes ---

type mockLockStore struct {
	acquired     bool
	acquiredKeys []string // (workspaceUID, trigger) pairs joined with ":"
}

func (m *mockLockStore) TryLock(_ context.Context, workspaceUID, trigger string) (bool, func(), error) {
	m.acquiredKeys = append(m.acquiredKeys, workspaceUID+":"+trigger)
	if !m.acquired {
		return false, func() {}, nil
	}
	return true, func() {}, nil
}

// fakeWorkspaceLister returns a fixed set of workspaces per policy. Tests
// that exercise the legacy single-workspace shape can use the
// singletonWorkspaceLister helper.
type fakeWorkspaceLister struct {
	forPolicy map[string][]Workspace
}

func (f *fakeWorkspaceLister) ForPolicy(_ context.Context, policy string) ([]Workspace, error) {
	return f.forPolicy[policy], nil
}

type fakePolicyLister struct{ policies []memoryv1.MemoryPolicy }

func (f fakePolicyLister) List(context.Context) ([]memoryv1.MemoryPolicy, error) {
	return f.policies, nil
}

type mockPreFilterRunner struct {
	stale       []Bucket
	crossScope  []Bucket
	entityDupes []Bucket
}

func (m *mockPreFilterRunner) RunStaleObservations(context.Context, PreFilterOptions) ([]Bucket, error) {
	return m.stale, nil
}
func (m *mockPreFilterRunner) RunCrossScopeCandidates(context.Context, PreFilterOptions) ([]Bucket, error) {
	return m.crossScope, nil
}
func (m *mockPreFilterRunner) RunEntityDuplicateCandidates(context.Context, PreFilterOptions) ([]Bucket, error) {
	return m.entityDupes, nil
}

func oneStaleOnlyPolicy() []memoryv1.MemoryPolicy {
	p := memoryv1.MemoryPolicy{}
	p.Name = testWorkspaceID
	p.Spec.Consolidation = &memoryv1.MemoryConsolidationConfig{
		FunctionRefs: memoryv1.MemoryConsolidationFunctionRefs{
			StaleObservations: &memoryv1.MemoryFunctionRef{Name: "safe-default-summarizer"},
		},
	}
	return []memoryv1.MemoryPolicy{p}
}

func TestWorker_AllThreeAxesWhenConfigured(t *testing.T) {
	runner := &mockPreFilterRunner{
		stale:       []Bucket{{Key: "k1", Entries: []BucketEntry{{ID: testObsID, Mutability: MutabilityMutable}}}},
		crossScope:  []Bucket{{Key: "k2", Entries: []BucketEntry{{ID: "obs-2", Mutability: MutabilityMutable}}}},
		entityDupes: []Bucket{{Key: "k3", Entries: []BucketEntry{{ID: "ent-1", Mutability: MutabilityMutable}}}},
	}
	p := memoryv1.MemoryPolicy{}
	p.Name = testWorkspaceID
	p.Spec.Consolidation = &memoryv1.MemoryConsolidationConfig{
		FunctionRefs: memoryv1.MemoryConsolidationFunctionRefs{
			StaleObservations:         &memoryv1.MemoryFunctionRef{Name: "stale-pack"},
			CrossScopeCandidates:      &memoryv1.MemoryFunctionRef{Name: "cross-pack"},
			EntityDuplicateCandidates: &memoryv1.MemoryFunctionRef{Name: "dupe-pack"},
		},
		CandidateLimits: &memoryv1.MemoryConsolidationCandidateLimits{
			MaxBucketsPerPass: 50,
			MaxPerBucket:      25,
		},
	}
	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: []memoryv1.MemoryPolicy{p}},
		PreFilterRunner: runner,
		Log:             testr.New(t),
	})
	called := map[PreFilterAxis]int{}
	w.callFunction = func(_ context.Context, ax PreFilterAxis, _ memoryv1.MemoryFunctionRef, _ FunctionInput) ([]Action, error) {
		called[ax]++
		return nil, nil
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called[AxisStaleObservations] != 1 || called[AxisCrossScopeCandidates] != 1 || called[AxisEntityDuplicateCandidates] != 1 {
		t.Errorf("expected each axis called once, got %+v", called)
	}
}

func TestWorker_DefaultCallFunctionErrorsWithoutClient(t *testing.T) {
	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: oneStaleOnlyPolicy()},
		PreFilterRunner: &mockPreFilterRunner{stale: []Bucket{{Key: "k1", Entries: []BucketEntry{{ID: testObsID, Mutability: MutabilityMutable}}}}},
		Log:             testr.New(t),
		// Client nil — defaultCallFunction returns an error
	})
	// w.callFunction NOT overridden, so the default path runs.
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err) // worker swallows axis errors; RunOnce returns first POLICY error, not axis
	}
}

func TestWorker_RunDisabledWithZeroInterval(t *testing.T) {
	w := NewWorker(WorkerOptions{
		Log: testr.New(t),
		// Interval: 0 — should log and exit immediately
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
		// good — exited immediately
	case <-time.After(time.Second):
		t.Fatal("Run did not return for disabled worker (Interval=0)")
	}
}

func TestWorker_RunHonoursContextCancellation(t *testing.T) {
	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: nil},
		PreFilterRunner: &mockPreFilterRunner{},
		Interval:        10 * time.Millisecond,
		Log:             testr.New(t),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	// Let one tick fire, then cancel.
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
		// good — exited on cancel
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestWorker_RecordsMetricsOnSuccessfulPass(t *testing.T) {
	runner := &mockPreFilterRunner{
		stale: []Bucket{{
			Key: "k1",
			Entries: []BucketEntry{{
				ID: testObsID, Mutability: MutabilityMutable,
				Scope: Scope{WorkspaceID: testWorkspaceID, UserID: testUserID},
			}},
		}},
	}
	metrics := NewMetrics()
	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: oneStaleOnlyPolicy()},
		PreFilterRunner: runner,
		Metrics:         metrics,
		Log:             testr.New(t),
	})
	w.callFunction = func(context.Context, PreFilterAxis, memoryv1.MemoryFunctionRef, FunctionInput) ([]Action, error) {
		return []Action{
			CreateSummaryAction{
				FromIDs: []string{testObsID},
				Scope:   Scope{WorkspaceID: testWorkspaceID, UserID: testUserID},
				Content: "summary",
			},
		}, nil
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	// One pass should be counted with status=ok. Labels are (workspaceUID,
	// policyName, function, status); legacy tests use the policy name as
	// both the policy-name and workspace-UID since the singletonWorkspaceLister
	// fallback wires UID=Name.
	if got := testCounterValue(t, metrics.PassesTotal, testWorkspaceID, testWorkspaceID, "safe-default-summarizer", "ok"); got != 1 {
		t.Errorf("PassesTotal ok = %v, want 1", got)
	}
	// One create_summary action applied.
	if got := testCounterValue(t, metrics.ActionsTotal,
		testWorkspaceID, testWorkspaceID, "safe-default-summarizer", "create_summary", OutcomeApplied, ""); got != 1 {
		t.Errorf("ActionsTotal create_summary applied = %v, want 1", got)
	}
}

func TestWorker_RecordsPrefilterErrorStatus(t *testing.T) {
	runner := &failingPreFilterRunner{}
	metrics := NewMetrics()
	w := NewWorker(WorkerOptions{
		Store:           &MockStore{},
		LockStore:       &mockLockStore{acquired: true},
		Policies:        fakePolicyLister{policies: oneStaleOnlyPolicy()},
		PreFilterRunner: runner,
		Metrics:         metrics,
		Log:             testr.New(t),
	})
	w.callFunction = func(context.Context, PreFilterAxis, memoryv1.MemoryFunctionRef, FunctionInput) ([]Action, error) {
		t.Fatal("function should not be called when prefilter fails")
		return nil, nil
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := testCounterValue(t, metrics.PassesTotal, testWorkspaceID, testWorkspaceID, "safe-default-summarizer", "prefilter_error"); got != 1 {
		t.Errorf("PassesTotal prefilter_error = %v, want 1", got)
	}
}

type failingPreFilterRunner struct{}

func (failingPreFilterRunner) RunStaleObservations(context.Context, PreFilterOptions) ([]Bucket, error) {
	return nil, errFakeStoreFailure
}

func (failingPreFilterRunner) RunCrossScopeCandidates(context.Context, PreFilterOptions) ([]Bucket, error) {
	return nil, errFakeStoreFailure
}

func (failingPreFilterRunner) RunEntityDuplicateCandidates(context.Context, PreFilterOptions) ([]Bucket, error) {
	return nil, errFakeStoreFailure
}

// testCounterValue extracts a value from a Prometheus CounterVec for
// the given labels. Returns 0 if the label combination has no recorded
// value (Prometheus auto-initialises to 0 anyway, so the read is safe).
func testCounterValue(t *testing.T, vec interface {
	WithLabelValues(...string) prometheus.Counter
}, labels ...string,
) float64 {
	t.Helper()
	return testutil.ToFloat64(vec.WithLabelValues(labels...))
}

func TestWorker_WithoutPreFilterRunner(t *testing.T) {
	w := NewWorker(WorkerOptions{
		Store:     &MockStore{},
		LockStore: &mockLockStore{acquired: true},
		Policies:  fakePolicyLister{policies: oneStaleOnlyPolicy()},
		Log:       testr.New(t),
		// PreFilterRunner: nil
	})
	// Each axis errors at runPreFilter, but worker logs and continues.
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// TestWorker_PerWorkspaceIteration verifies the worker resolves one
// MemoryPolicy to N workspaces (via WorkspaceLister) and emits one
// FunctionInput.WorkspaceID + one SaveSummary per workspace UID — never
// using the policy name as the workspace identifier.
func TestWorker_PerWorkspaceIteration(t *testing.T) {
	policy := memoryv1.MemoryPolicy{}
	policy.Name = "research"
	policy.Spec.Consolidation = &memoryv1.MemoryConsolidationConfig{
		FunctionRefs: memoryv1.MemoryConsolidationFunctionRefs{
			StaleObservations: &memoryv1.MemoryFunctionRef{Name: "stub", Namespace: "ns"},
		},
	}
	wsList := &fakeWorkspaceLister{forPolicy: map[string][]Workspace{
		"research": {
			{Name: "alpha", UID: "uid-alpha"},
			{Name: "beta", UID: "uid-beta"},
		},
	}}
	lock := &mockLockStore{acquired: true}
	pf := &mockPreFilterRunner{
		stale: []Bucket{{
			Key:     "k",
			Entries: []BucketEntry{{ID: "obs-1", Mutability: MutabilityMutable, Scope: Scope{WorkspaceID: "uid-x"}}},
		}},
	}
	store := &MockStore{}
	w := NewWorker(WorkerOptions{
		Store:           store,
		LockStore:       lock,
		Policies:        fakePolicyLister{policies: []memoryv1.MemoryPolicy{policy}},
		Workspaces:      wsList,
		PreFilterRunner: pf,
		Log:             testr.New(t),
	})

	gotWorkspaceIDs := map[string]bool{}
	w.callFunction = func(_ context.Context, _ PreFilterAxis, _ memoryv1.MemoryFunctionRef, in FunctionInput) ([]Action, error) {
		gotWorkspaceIDs[in.WorkspaceID] = true
		return []Action{CreateSummaryAction{
			FromIDs: []string{"obs-1"},
			Scope:   Scope{WorkspaceID: in.WorkspaceID},
			Content: "summary for " + in.WorkspaceID,
		}}, nil
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if !gotWorkspaceIDs["uid-alpha"] || !gotWorkspaceIDs["uid-beta"] {
		t.Errorf("function called with wrong workspaceIDs: %+v", gotWorkspaceIDs)
	}
	if len(store.Summaries) != 2 {
		t.Fatalf("want 2 summaries (one per workspace), got %d", len(store.Summaries))
	}
	gotSummaryWS := map[string]bool{}
	for _, s := range store.Summaries {
		gotSummaryWS[s.WorkspaceID] = true
	}
	if !gotSummaryWS["uid-alpha"] || !gotSummaryWS["uid-beta"] {
		t.Errorf("summary writes keyed wrong: %+v", store.Summaries)
	}
	// Locks acquired per workspace UID, not per policy name.
	wantLocks := []string{"uid-alpha:consolidation", "uid-beta:consolidation"}
	for _, want := range wantLocks {
		found := false
		for _, got := range lock.acquiredKeys {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing lock for %q (got %v)", want, lock.acquiredKeys)
		}
	}
}
