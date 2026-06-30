/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import (
	"testing"
	"time"
)

// Build*Query tests moved to internal/memory/consolidation/prefilter_test.go
// alongside their implementations.

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
