/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetCondition sets a status condition on the given conditions slice.
func SetCondition(conditions *[]metav1.Condition, generation int64, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: generation,
		Reason:             reason,
		Message:            message,
	})
}

// GetWorkspaceForNamespace looks up the workspace name from a namespace's labels.
// Returns the namespace name as fallback if the workspace label is not found or
// if the client is nil.
func GetWorkspaceForNamespace(ctx context.Context, c client.Reader, namespace string) string {
	if c == nil {
		return namespace
	}
	ns := &corev1.Namespace{}
	if err := c.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		// Fallback to namespace name if we can't look it up
		return namespace
	}
	if wsName, ok := ns.Labels[labelWorkspace]; ok && wsName != "" {
		return wsName
	}
	// Fallback to namespace name
	return namespace
}
