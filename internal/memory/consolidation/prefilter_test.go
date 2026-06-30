/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import (
	"strings"
	"testing"
	"time"
)

const testWorkspaceID = "ws-1"

func TestStaleObservationsExcludesNonMutable(t *testing.T) {
	q, args := BuildStaleObservationsQuery(PreFilterOptions{
		WorkspaceID:       testWorkspaceID,
		OlderThan:         time.Now().Add(-720 * time.Hour),
		MinGroupSize:      5,
		MaxBucketsPerPass: 100,
		MaxPerBucket:      50,
	})
	if !strings.Contains(q, "mutability = 'mutable'") {
		t.Errorf("query does not filter mutability:\n%s", q)
	}
	if !strings.Contains(q, "source_type != 'regulated'") {
		t.Errorf("query does not exclude regulated source_type:\n%s", q)
	}
	if !strings.Contains(q, "superseded_by IS NULL") {
		t.Errorf("query does not exclude already-superseded rows:\n%s", q)
	}
	// 3 args: workspace, older_than, limit. MinGroupSize is applied
	// in Go (the adapter groups by user/agent/kind/name and filters small
	// groups out) so the SQL doesn't carry it.
	if len(args) != 3 {
		t.Errorf("expected 3 args (workspace, older_than, limit), got %d", len(args))
	}
}

func TestCrossScopeCandidatesGroupsByKindName(t *testing.T) {
	q, args := BuildCrossScopeCandidatesQuery(PreFilterOptions{
		WorkspaceID:       testWorkspaceID,
		MinDistinctUsers:  5,
		MaxBucketsPerPass: 100,
	})
	if !strings.Contains(q, "GROUP BY") || !strings.Contains(q, "kind") {
		t.Errorf("cross-scope query missing GROUP BY kind:\n%s", q)
	}
	if !strings.Contains(q, "COUNT(DISTINCT") {
		t.Errorf("cross-scope query missing COUNT(DISTINCT user):\n%s", q)
	}
	if !strings.Contains(q, "virtual_user_id IS NOT NULL") {
		t.Errorf("cross-scope query missing user-row filter:\n%s", q)
	}
	if len(args) != 3 {
		t.Errorf("expected 3 args, got %d", len(args))
	}
}

func TestEntityDuplicateCandidatesUsesEmbedding(t *testing.T) {
	q, args := BuildEntityDuplicateCandidatesQuery(PreFilterOptions{
		WorkspaceID:       testWorkspaceID,
		SimilarityFloor:   0.85,
		MaxBucketsPerPass: 100,
	})
	if !strings.Contains(q, "embedding") || !strings.Contains(q, "<=>") {
		t.Errorf("entity-dupe query missing embedding distance operator:\n%s", q)
	}
	if !strings.Contains(q, "e1.id < e2.id") {
		t.Errorf("entity-dupe query missing self-join dedup (e1.id < e2.id):\n%s", q)
	}
	if len(args) != 3 {
		t.Errorf("expected 3 args, got %d", len(args))
	}
}
