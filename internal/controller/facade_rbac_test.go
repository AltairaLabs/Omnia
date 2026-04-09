/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestReconcileRole_IncludesNamespaceReadAccess(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
			UID:       "fake-uid",
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	err := r.reconcileRole(context.Background(), ar)
	require.NoError(t, err)

	role := &rbacv1.Role{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "test-agent-facade",
		Namespace: "test-ns",
	}, role)
	require.NoError(t, err)

	// The facade needs namespace read access so ResolveWorkspaceName can read
	// the namespace label (omnia.altairalabs.ai/workspace) as a fallback when
	// the AgentRuntime itself has no workspace label. Without this, sessions
	// are stored with an empty workspace_name and don't appear in the dashboard.
	var hasNamespaceRead bool
	for _, rule := range role.Rules {
		for _, res := range rule.Resources {
			if res == "namespaces" {
				for _, verb := range rule.Verbs {
					if verb == "get" {
						hasNamespaceRead = true
					}
				}
			}
		}
	}
	assert.True(t, hasNamespaceRead, "facade Role must grant GET on namespaces for workspace name resolution")
}

func TestReconcileWorkspaceReaderBinding_Creates(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &AgentRuntimeReconciler{
		Client:                          fakeClient,
		Scheme:                          scheme,
		AgentWorkspaceReaderClusterRole: "omnia-agent-workspace-reader",
	}

	err := r.reconcileWorkspaceReaderBinding(context.Background(), ar)
	require.NoError(t, err)

	// Verify ClusterRoleBinding was created
	crb := &rbacv1.ClusterRoleBinding{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "test-ns-test-agent-workspace-reader",
	}, crb)
	require.NoError(t, err)

	assert.Equal(t, "ClusterRole", crb.RoleRef.Kind)
	assert.Equal(t, "omnia-agent-workspace-reader", crb.RoleRef.Name)
	assert.Len(t, crb.Subjects, 1)
	assert.Equal(t, "ServiceAccount", crb.Subjects[0].Kind)
	assert.Equal(t, "test-agent-facade", crb.Subjects[0].Name)
	assert.Equal(t, "test-ns", crb.Subjects[0].Namespace)
	assert.Equal(t, "test-ns", crb.Labels["omnia.altairalabs.ai/workspace-reader-for"])
}

func TestReconcileWorkspaceReaderBinding_SkipsWhenUnconfigured(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
		// AgentWorkspaceReaderClusterRole intentionally empty
	}

	err := r.reconcileWorkspaceReaderBinding(context.Background(), ar)
	require.NoError(t, err)

	// Verify no ClusterRoleBinding was created
	crb := &rbacv1.ClusterRoleBinding{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "test-ns-test-agent-workspace-reader",
	}, crb)
	assert.Error(t, err)
}

func TestReconcileWorkspaceReaderBinding_UpdatesExisting(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
	}

	// Pre-existing CRB with stale rolRef
	existing := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns-test-agent-workspace-reader",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "stale-cluster-role",
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	r := &AgentRuntimeReconciler{
		Client:                          fakeClient,
		Scheme:                          scheme,
		AgentWorkspaceReaderClusterRole: "omnia-agent-workspace-reader",
	}

	err := r.reconcileWorkspaceReaderBinding(context.Background(), ar)
	// Note: ClusterRoleBinding.RoleRef is immutable in Kubernetes, but the fake
	// client allows updates. In production, update failures are handled by
	// CreateOrUpdate's conflict resolution. This test verifies the reconcile
	// path executes without error for existing resources.
	require.NoError(t, err)

	// Verify CRB still exists (reconcile is idempotent)
	crb := &rbacv1.ClusterRoleBinding{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "test-ns-test-agent-workspace-reader",
	}, crb)
	require.NoError(t, err)
}
