/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import (
	"fmt"

	coreconsol "github.com/altairalabs/omnia/internal/memory/consolidation"
)

// PreFilterOptions is defined in internal/memory/consolidation and
// re-exported here as a type alias so worker.go and its tests compile
// unchanged.
type PreFilterOptions = coreconsol.PreFilterOptions

// BuildStaleObservationsQuery, BuildCrossScopeCandidatesQuery, and
// BuildEntityDuplicateCandidatesQuery have moved to
// internal/memory/consolidation. They are accessible as
// coreconsol.Build*Query from the postgres adapter.

// ValidatePreFilterOptions returns a descriptive error if required
// fields are missing for the given axis. Pure validation; safe to call
// before any DB query.
func ValidatePreFilterOptions(axis PreFilterAxis, o PreFilterOptions) error {
	if o.WorkspaceID == "" {
		return fmt.Errorf("preFilter %s: WorkspaceID required", axis)
	}
	switch axis {
	case AxisStaleObservations:
		if o.OlderThan.IsZero() || o.MinGroupSize <= 0 {
			return fmt.Errorf("preFilter %s: OlderThan + MinGroupSize required", axis)
		}
	case AxisCrossScopeCandidates:
		if o.MinDistinctUsers <= 0 {
			return fmt.Errorf("preFilter %s: MinDistinctUsers required", axis)
		}
	case AxisEntityDuplicateCandidates:
		if o.SimilarityFloor <= 0 {
			return fmt.Errorf("preFilter %s: SimilarityFloor required", axis)
		}
	}
	if o.MaxBucketsPerPass <= 0 {
		return fmt.Errorf("preFilter %s: MaxBucketsPerPass must be > 0", axis)
	}
	return nil
}
