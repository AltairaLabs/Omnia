/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"errors"
	"testing"
)

func TestRender_ComputesAndPersists(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	// seed a few institutional memories in testWorkspace1
	for i := 0; i < 5; i++ {
		must(t, store.SaveInstitutional(ctx, &Memory{Type: projTypePolicy,
			Content: "policy fact number " + string(rune('a'+i)), Confidence: 0.9,
			Scope: map[string]string{ScopeWorkspaceID: testWorkspace1}}))
	}
	scope := map[string]string{ScopeWorkspaceID: testWorkspace1}

	res, computedAt, err := Render(ctx, store, scope)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if res.Total != 5 || len(res.Points) != 5 {
		t.Fatalf("total/points = %d/%d, want 5/5", res.Total, len(res.Points))
	}
	if computedAt.IsZero() {
		t.Error("computedAt not set")
	}
	// Persisted: LoadProjection returns the layout with a fingerprint.
	sp, err := store.LoadProjection(ctx, ProjectionScopeKey(scope))
	if err != nil || sp == nil {
		t.Fatalf("LoadProjection after Render: sp=%v err=%v", sp, err)
	}
	if len(sp.Layout) != 5 {
		t.Errorf("stored layout = %d points, want 5", len(sp.Layout))
	}
}

// fakeProjectionStore is a ProjectionStore whose calls fail on demand, so
// Render's error branches can be exercised without a database.
type fakeProjectionStore struct {
	fpErr, inputsErr, loadErr, saveErr error
}

func (f fakeProjectionStore) ProjectionFingerprint(context.Context, map[string]string) (string, error) {
	return "1:1", f.fpErr
}

func (f fakeProjectionStore) LoadProjectionInputs(context.Context, map[string]string) ([]ProjectionInput, error) {
	return []ProjectionInput{{EntityID: "e1", Content: "x", Tier: string(TierInstitutional)}}, f.inputsErr
}

func (f fakeProjectionStore) LoadProjection(context.Context, string) (*StoredProjection, error) {
	return nil, f.loadErr
}

func (f fakeProjectionStore) SaveProjection(context.Context, string, string, string, string, string, []ProjectionPoint) error {
	return f.saveErr
}

func TestRender_PropagatesStoreErrors(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]fakeProjectionStore{
		"fingerprint": {fpErr: boom},
		"inputs":      {inputsErr: boom},
		"load":        {loadErr: boom},
		"save":        {saveErr: boom},
	}
	scope := map[string]string{ScopeWorkspaceID: testWorkspace1}
	for name, store := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Render(context.Background(), store, scope); err == nil {
				t.Errorf("Render: expected error from %s stage", name)
			}
		})
	}
}
