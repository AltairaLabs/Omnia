/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"
)

const (
	projTestUser   = "17b0187b2d95fca1" // a pseudonymized user id
	projTypePolicy = "policy"
)

func abs64(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func seedProjectionScope(t *testing.T, store *PostgresMemoryStore) (instID, agentID, userID string) {
	t.Helper()
	ctx := context.Background()

	inst := &Memory{Type: projTypePolicy, Content: "refund policy: 30 days", Confidence: 0.9,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1}}
	must(t, store.SaveInstitutional(ctx, inst))

	ag := &Memory{Type: "pattern", Content: "legacy plan hits E_QUOTA", Confidence: 0.7,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeAgentID: testAgent1}}
	must(t, store.SaveAgentScoped(ctx, ag))

	usr := &Memory{Type: "profile", Content: "prefers email contact", Confidence: 0.8,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeUserID: projTestUser}}
	must(t, store.Save(ctx, usr))

	return inst.ID, ag.ID, usr.ID
}

func TestLoadProjectionInputs_OneRowPerEntityWithTiers(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	instID, agentID, userID := seedProjectionScope(t, store)

	got, err := store.LoadProjectionInputs(ctx, map[string]string{ScopeWorkspaceID: testWorkspace1})
	if err != nil {
		t.Fatalf("LoadProjectionInputs: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d inputs, want 3", len(got))
	}
	byID := map[string]ProjectionInput{}
	for _, in := range got {
		byID[in.EntityID] = in
	}
	if byID[instID].Tier != string(TierInstitutional) {
		t.Errorf("inst tier = %q, want institutional", byID[instID].Tier)
	}
	if byID[agentID].Tier != string(TierAgent) {
		t.Errorf("agent tier = %q, want agent", byID[agentID].Tier)
	}
	u := byID[userID]
	if u.Tier != string(TierUser) {
		t.Errorf("user tier = %q, want user", u.Tier)
	}
	if u.User != projTestUser {
		t.Errorf("user pseudonym = %q, want %q", u.User, projTestUser)
	}
	if u.Content != "prefers email contact" {
		t.Errorf("user content = %q", u.Content)
	}
	// No embeddings were written, so all must be nil.
	for _, in := range got {
		if in.Embedding != nil {
			t.Errorf("entity %s has unexpected embedding", in.EntityID)
		}
	}
}

func TestLoadProjectionInputs_SurfacesEmbedding(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	_, _, userID := seedProjectionScope(t, store)

	emb := make([]float32, 1536) // matches EnsureEmbeddingSchema(1536)
	for i := range emb {
		emb[i] = float32(i%7) * 0.1
	}
	must(t, store.UpdateEmbedding(ctx, userID, emb, "test-model"))

	got, err := store.LoadProjectionInputs(ctx, map[string]string{ScopeWorkspaceID: testWorkspace1})
	if err != nil {
		t.Fatalf("LoadProjectionInputs: %v", err)
	}
	var found bool
	for _, in := range got {
		if in.EntityID == userID {
			found = true
			if len(in.Embedding) != 1536 {
				t.Errorf("embedding len = %d, want 1536", len(in.Embedding))
			}
		}
	}
	if !found {
		t.Fatal("embedded entity not returned")
	}
}

func TestProjectionFingerprint_EmptyAndChanges(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	empty, err := store.ProjectionFingerprint(ctx, map[string]string{ScopeWorkspaceID: testWorkspace2})
	if err != nil {
		t.Fatalf("fingerprint empty: %v", err)
	}
	if empty != "" {
		t.Errorf("empty scope fingerprint = %q, want \"\"", empty)
	}

	seedProjectionScope(t, store)
	fp1, err := store.ProjectionFingerprint(ctx, map[string]string{ScopeWorkspaceID: testWorkspace1})
	if err != nil {
		t.Fatalf("fingerprint fp1: %v", err)
	}
	if fp1 == "" {
		t.Fatal("fp1 must be non-empty after seeding")
	}
	must(t, store.Save(ctx, &Memory{Type: "profile", Content: "another", Confidence: 0.5,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeUserID: "other-user"}}))
	fp2, err := store.ProjectionFingerprint(ctx, map[string]string{ScopeWorkspaceID: testWorkspace1})
	if err != nil {
		t.Fatalf("fingerprint fp2: %v", err)
	}
	if fp2 == fp1 {
		t.Errorf("fingerprint did not change after adding a memory: %q", fp2)
	}
}

func TestSaveAndLoadProjection_RoundTrip(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	key := "ws|" + projTestUser + "|"

	none, err := store.LoadProjection(ctx, key)
	if err != nil {
		t.Fatalf("LoadProjection empty: %v", err)
	}
	if none != nil {
		t.Fatal("expected nil for unstored scope")
	}

	const (
		e1 = "11111111-1111-1111-1111-111111111111"
		e2 = "22222222-2222-2222-2222-222222222222"
		e3 = "33333333-3333-3333-3333-333333333333"
	)
	pts := []ProjectionPoint{{EntityID: e1, X: 0.1, Y: 0.2}, {EntityID: e2, X: -0.3, Y: 0.4}}
	must(t, store.SaveProjection(ctx, key, testWorkspace1, "fp1", "tsne", "dense", pts))

	sp, err := store.LoadProjection(ctx, key)
	if err != nil {
		t.Fatalf("LoadProjection: %v", err)
	}
	if sp == nil || len(sp.Layout) != 2 {
		t.Fatalf("layout = %+v, want 2 points", sp)
	}
	if sp.Fingerprint != "fp1" || sp.Model != "tsne" || sp.Basis != "dense" {
		t.Errorf("metadata = %+v", sp)
	}
	// Coords are stored as REAL (float32), so compare approximately.
	if d := sp.Layout[e1]; abs64(d[0]-0.1) > 1e-5 || abs64(d[1]-0.2) > 1e-5 {
		t.Errorf("e1 = %v, want ~[0.1 0.2]", d)
	}

	// Re-save replaces (no duplicate rows / new fingerprint).
	must(t, store.SaveProjection(ctx, key, testWorkspace1, "fp2", "pca", "lexical",
		[]ProjectionPoint{{EntityID: e3, X: 1, Y: 1}}))
	sp2, err := store.LoadProjection(ctx, key)
	if err != nil {
		t.Fatalf("LoadProjection after replace: %v", err)
	}
	if len(sp2.Layout) != 1 || sp2.Fingerprint != "fp2" || sp2.Basis != "lexical" {
		t.Errorf("replace failed: %+v", sp2)
	}
}
