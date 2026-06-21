/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"
)

// seedUnembeddedEntities saves n single-observation entities (distinct users so
// each is its own entity) in workspace ws, none embedded. Returns nothing —
// callers re-query via Find/Count.
func seedUnembeddedEntities(t *testing.T, store *PostgresMemoryStore, ws string, contents ...string) {
	t.Helper()
	ctx := context.Background()
	for i, c := range contents {
		must(t, store.Save(ctx, &Memory{
			Type: "profile", Content: c, Confidence: 0.9,
			Scope: map[string]string{ScopeWorkspaceID: ws, ScopeUserID: "u-" + string(rune('a'+i))},
		}))
	}
}

// embedByContent embeds the latest observation whose content matches, using the
// given model. Returns true if a row was embedded.
func embedByContent(t *testing.T, store *PostgresMemoryStore, content, model string) bool {
	t.Helper()
	ctx := context.Background()
	missing, err := store.FindObservationsMissingEmbedding(ctx, "", 100)
	must(t, err)
	vec := oneHotFloat(0, 1536)
	for _, m := range missing {
		if m.Content == content {
			must(t, store.UpdateObservationEmbedding(ctx, m.ObservationID, vec, model))
			return true
		}
	}
	return false
}

func TestEmbeddingCoverage_LatestObservationFraction(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// Empty workspace → 0/0 (no NaN, the collector guards on total>0).
	total, embedded, err := store.EmbeddingCoverage(ctx, testWorkspace2)
	must(t, err)
	if total != 0 || embedded != 0 {
		t.Fatalf("empty workspace coverage = %d/%d, want 0/0", embedded, total)
	}

	// 4 entities, embed 3 of them.
	seedUnembeddedEntities(t, store, testWorkspace1, "fa", "fb", "fc", "fd")
	for _, c := range []string{"fa", "fb", "fc"} {
		if !embedByContent(t, store, c, "test-embed") {
			t.Fatalf("failed to embed %q", c)
		}
	}

	total, embedded, err = store.EmbeddingCoverage(ctx, testWorkspace1)
	must(t, err)
	if total != 4 || embedded != 3 {
		t.Errorf("coverage = %d/%d, want 3/4", embedded, total)
	}
}

func TestCountObservationsMissingEmbedding_ScopedAndModelAware(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// 3 entities in ws1, 1 in ws2 (must NOT leak into ws1's backlog).
	seedUnembeddedEntities(t, store, testWorkspace1, "fa", "fb", "fc")
	seedUnembeddedEntities(t, store, testWorkspace2, "other")

	n, err := store.CountObservationsMissingEmbedding(ctx, testWorkspace1, "new-model")
	must(t, err)
	if n != 3 {
		t.Errorf("initial ws1 backlog = %d, want 3 (ws2 excluded)", n)
	}

	// Embed one with the CURRENT model → drains from the backlog.
	if !embedByContent(t, store, "fa", "new-model") {
		t.Fatal("failed to embed fa")
	}
	n, err = store.CountObservationsMissingEmbedding(ctx, testWorkspace1, "new-model")
	must(t, err)
	if n != 2 {
		t.Errorf("backlog after embedding 1 = %d, want 2", n)
	}

	// Embed another with an OLD model → still backlog for the current model.
	if !embedByContent(t, store, "fb", "old-model") {
		t.Fatal("failed to embed fb")
	}
	n, err = store.CountObservationsMissingEmbedding(ctx, testWorkspace1, "new-model")
	must(t, err)
	if n != 2 {
		t.Errorf("backlog with a stale-model row = %d, want 2 (fb stale + fc null)", n)
	}
}
