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
	// 3 args: workspace, older_than, limit. MinGroupSize is now applied
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

func TestValidatePreFilterOptions(t *testing.T) {
	cases := []struct {
		name    string
		axis    PreFilterAxis
		opts    PreFilterOptions
		wantErr bool
	}{
		{
			"stale: ok",
			AxisStaleObservations,
			PreFilterOptions{WorkspaceID: "w", OlderThan: time.Now(), MinGroupSize: 5, MaxBucketsPerPass: 100},
			false,
		},
		{
			"stale: missing OlderThan",
			AxisStaleObservations,
			PreFilterOptions{WorkspaceID: "w", MinGroupSize: 5, MaxBucketsPerPass: 100},
			true,
		},
		{
			"stale: missing MinGroupSize",
			AxisStaleObservations,
			PreFilterOptions{WorkspaceID: "w", OlderThan: time.Now(), MaxBucketsPerPass: 100},
			true,
		},
		{
			"cross-scope: ok",
			AxisCrossScopeCandidates,
			PreFilterOptions{WorkspaceID: "w", MinDistinctUsers: 5, MaxBucketsPerPass: 100},
			false,
		},
		{
			"cross-scope: missing MinDistinctUsers",
			AxisCrossScopeCandidates,
			PreFilterOptions{WorkspaceID: "w", MaxBucketsPerPass: 100},
			true,
		},
		{
			"entity-dupe: ok",
			AxisEntityDuplicateCandidates,
			PreFilterOptions{WorkspaceID: "w", SimilarityFloor: 0.85, MaxBucketsPerPass: 100},
			false,
		},
		{
			"missing WorkspaceID",
			AxisStaleObservations,
			PreFilterOptions{OlderThan: time.Now(), MinGroupSize: 5, MaxBucketsPerPass: 100},
			true,
		},
		{
			"missing MaxBucketsPerPass",
			AxisStaleObservations,
			PreFilterOptions{WorkspaceID: "w", OlderThan: time.Now(), MinGroupSize: 5},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePreFilterOptions(tc.axis, tc.opts)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
