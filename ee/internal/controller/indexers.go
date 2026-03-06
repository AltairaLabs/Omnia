/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// Field index paths used by ee watch handlers to scope list operations.
const (
	// IndexArenaJobBySourceRef indexes ArenaJobs by the ArenaSource name they reference.
	IndexArenaJobBySourceRef = ".spec.sourceRef.name"
)

// SetupIndexers registers field indexers required by ee watch handlers.
// Must be called before controllers start.
func SetupIndexers(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&omniav1alpha1.ArenaJob{},
		IndexArenaJobBySourceRef,
		extractSourceRef,
	)
}

// extractSourceRef returns the ArenaSource name referenced by an ArenaJob.
func extractSourceRef(obj client.Object) []string {
	job := obj.(*omniav1alpha1.ArenaJob)
	if job.Spec.SourceRef.Name == "" {
		return nil
	}
	return []string{job.Spec.SourceRef.Name}
}
