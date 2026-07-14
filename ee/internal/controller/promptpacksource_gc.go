/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// gcOldVersions garbage-collects Superseded PromptPack version-objects beyond
// the source's history limit. It is a stub here; the retention logic
// (referenced + min-age guard) is implemented in Task 4 of #1840.
func (r *PromptPackSourceReconciler) gcOldVersions(_ context.Context, _ *omniav1alpha1.PromptPackSource) error {
	return nil
}
