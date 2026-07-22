/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func devSessionWorkspaceScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1alpha1.AddToScheme(s))
	return s
}

// The dev console exits if it cannot resolve the session-api URL, and it can
// only resolve it with a workspace name. The name must therefore be available
// even when the service group's status is not ready yet — that gap is exactly
// what the console's retry loop waits out, and without the name the loop cannot
// run at all (#1875).
func TestResolveWorkspaceNameForNamespace_AvailableBeforeServiceGroupIsReady(t *testing.T) {
	s := devSessionWorkspaceScheme(t)

	// Workspace "demo" owns namespace "omnia-demo". status.Services is empty:
	// the service group has not published a session URL yet.
	ws := &corev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "demo"},
		Spec: corev1alpha1.WorkspaceSpec{
			DisplayName: "Demo",
			Namespace:   corev1alpha1.NamespaceConfig{Name: "omnia-demo"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ws).Build()
	r := &ArenaDevSessionReconciler{Client: c, Scheme: s}

	assert.Empty(t, r.resolveSessionURLForWorkspace(context.Background(), "omnia-demo"),
		"precondition: the session URL is not resolvable yet")
	assert.Equal(t, "demo", r.resolveWorkspaceNameForNamespace(context.Background(), "omnia-demo"),
		"the workspace name must still resolve, or the console cannot retry")
}

// The lookup is by the namespace a Workspace owns, and returns the workspace
// NAME. Passing the workspace name as if it were a namespace finds nothing.
func TestResolveWorkspaceNameForNamespace_NamespaceIsNotTheWorkspaceName(t *testing.T) {
	s := devSessionWorkspaceScheme(t)

	ws := &corev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "demo"},
		Spec: corev1alpha1.WorkspaceSpec{
			Namespace: corev1alpha1.NamespaceConfig{Name: "omnia-demo"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ws).Build()
	r := &ArenaDevSessionReconciler{Client: c, Scheme: s}

	assert.Equal(t, "demo", r.resolveWorkspaceNameForNamespace(context.Background(), "omnia-demo"))
	assert.Empty(t, r.resolveWorkspaceNameForNamespace(context.Background(), "demo"))
}

func TestResolveWorkspaceNameForNamespace_NoOwningWorkspace(t *testing.T) {
	s := devSessionWorkspaceScheme(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	r := &ArenaDevSessionReconciler{Client: c, Scheme: s}

	assert.Empty(t, r.resolveWorkspaceNameForNamespace(context.Background(), "omnia-demo"))
}
