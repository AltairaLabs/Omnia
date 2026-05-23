/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"

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

// --- fakes ---

type mockLockStore struct{ acquired bool }

func (m *mockLockStore) TryLock(context.Context, string, string) (bool, func(), error) {
	if !m.acquired {
		return false, func() {}, nil
	}
	return true, func() {}, nil
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
