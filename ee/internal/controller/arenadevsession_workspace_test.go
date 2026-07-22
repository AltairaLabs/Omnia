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
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
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

// The dev console resolves its own session-api URL, so the controller must not
// inject one. The fixture publishes a session URL so the old code WOULD have
// injected it — that is what makes this test meaningful.
func TestReconcileDeployment_DoesNotInjectSessionURL(t *testing.T) {
	s := devSessionWorkspaceScheme(t)
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, omniav1alpha1.AddToScheme(s))

	ws := &corev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "demo"},
		Spec: corev1alpha1.WorkspaceSpec{
			Namespace: corev1alpha1.NamespaceConfig{Name: "omnia-demo"},
		},
		Status: corev1alpha1.WorkspaceStatus{
			Services: []corev1alpha1.ServiceGroupStatus{{
				Name:       "default",
				SessionURL: "http://session.omnia-demo.svc:8080",
				Ready:      true,
			}},
		},
	}
	session := &omniav1alpha1.ArenaDevSession{
		ObjectMeta: metav1.ObjectMeta{Name: "dev", Namespace: "omnia-demo"},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ws, session).Build()
	r := &ArenaDevSessionReconciler{Client: c, Scheme: s}

	require.NoError(t, r.reconcileDeployment(context.Background(), session))

	dep := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Name: r.resourceName(session), Namespace: "omnia-demo"}, dep))

	var sawWorkspaceName bool
	for _, ev := range dep.Spec.Template.Spec.Containers[0].Env {
		if ev.Name == "SESSION_API_URL" {
			t.Fatal("controller still injects SESSION_API_URL")
		}
		if ev.Name == "OMNIA_WORKSPACE_NAME" {
			sawWorkspaceName = true
		}
	}
	require.True(t, sawWorkspaceName,
		"the console cannot resolve anything without its workspace name")
}
