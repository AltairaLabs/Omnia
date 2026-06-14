/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package projectionworker

import (
	"context"
	"testing"
	"time"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
	"github.com/altairalabs/omnia/internal/memory/consolidation"
	"github.com/go-logr/logr/testr"
)

type fakePolicyLister struct{ policies []memoryv1.MemoryPolicy }

func (f fakePolicyLister) List(context.Context) ([]memoryv1.MemoryPolicy, error) {
	return f.policies, nil
}

type fakeWorkspaceLister struct {
	byPolicy map[string][]consolidation.Workspace
}

func (f fakeWorkspaceLister) ForPolicy(_ context.Context, p string) ([]consolidation.Workspace, error) {
	return f.byPolicy[p], nil
}

type fakeLock struct{ ok bool }

func (f fakeLock) TryLock(context.Context, string, string) (bool, func(), error) {
	return f.ok, func() {}, nil
}

// fakeRenderStore records renders + drives fingerprints.
type fakeRenderStore struct {
	fingerprint string
	stored      *memory.StoredProjection
	rendered    []string // scopeKeys rendered
}

func (f *fakeRenderStore) ProjectionFingerprint(context.Context, map[string]string) (string, error) {
	return f.fingerprint, nil
}

func (f *fakeRenderStore) LoadProjectionInputs(context.Context, map[string]string) ([]memory.ProjectionInput, error) {
	return []memory.ProjectionInput{{EntityID: "e1", Content: "x", Tier: "institutional"}}, nil
}

func (f *fakeRenderStore) LoadProjection(context.Context, string) (*memory.StoredProjection, error) {
	return f.stored, nil
}

func (f *fakeRenderStore) SaveProjection(_ context.Context, key, _, _, _, _ string, _ []memory.ProjectionPoint) error {
	f.rendered = append(f.rendered, key)
	return nil
}

func projectionPolicy(name string, enabled bool) memoryv1.MemoryPolicy {
	p := memoryv1.MemoryPolicy{}
	p.Name = name
	if enabled {
		p.Spec.Projection = &memoryv1.MemoryProjectionConfig{Enabled: true}
	}
	return p
}

func TestRunOnce_RendersEnabledWorkspaceColdScope(t *testing.T) {
	store := &fakeRenderStore{fingerprint: "5:1", stored: nil} // cold
	w := NewWorker(WorkerOptions{
		Store:      store,
		Policies:   fakePolicyLister{policies: []memoryv1.MemoryPolicy{projectionPolicy("p1", true)}},
		Workspaces: fakeWorkspaceLister{byPolicy: map[string][]consolidation.Workspace{"p1": {{Name: "w", UID: "ws-uid"}}}},
		Lock:       fakeLock{ok: true},
		Metrics:    NewMetrics(),
		Now:        func() time.Time { return time.Unix(0, 0) },
		Log:        testr.New(t),
	})
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.rendered) != 1 || store.rendered[0] != "ws-uid||" {
		t.Errorf("rendered = %v, want [ws-uid||]", store.rendered)
	}
}

func TestRunOnce_SkipsDisabledPolicy(t *testing.T) {
	store := &fakeRenderStore{fingerprint: "5:1"}
	w := NewWorker(WorkerOptions{
		Store:      store,
		Policies:   fakePolicyLister{policies: []memoryv1.MemoryPolicy{projectionPolicy("p1", false)}},
		Workspaces: fakeWorkspaceLister{byPolicy: map[string][]consolidation.Workspace{"p1": {{Name: "w", UID: "ws-uid"}}}},
		Lock:       fakeLock{ok: true},
		Metrics:    NewMetrics(), Now: func() time.Time { return time.Unix(0, 0) }, Log: testr.New(t),
	})
	_ = w.RunOnce(context.Background())
	if len(store.rendered) != 0 {
		t.Errorf("disabled policy rendered: %v", store.rendered)
	}
}

func TestRunOnce_SkipsWhenLockHeld(t *testing.T) {
	store := &fakeRenderStore{fingerprint: "5:1"}
	w := NewWorker(WorkerOptions{
		Store:      store,
		Policies:   fakePolicyLister{policies: []memoryv1.MemoryPolicy{projectionPolicy("p1", true)}},
		Workspaces: fakeWorkspaceLister{byPolicy: map[string][]consolidation.Workspace{"p1": {{Name: "w", UID: "ws-uid"}}}},
		Lock:       fakeLock{ok: false}, // held by another pod
		Metrics:    NewMetrics(), Now: func() time.Time { return time.Unix(0, 0) }, Log: testr.New(t),
	})
	_ = w.RunOnce(context.Background())
	if len(store.rendered) != 0 {
		t.Errorf("rendered despite lock held: %v", store.rendered)
	}
}
