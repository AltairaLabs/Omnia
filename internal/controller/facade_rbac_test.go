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

// TestReconcileRole_IncludesToolPoliciesReadAccess is the P2.3a wiring guard:
// the policy-broker sidecar runs under the facade's ServiceAccount and watches
// ToolPolicy CRDs in the agent's namespace to build CEL decisions. Without
// get/list/watch on toolpolicies here, the broker RBAC-denies its watch and
// silently never enforces any policy.
func TestReconcileRole_IncludesToolPoliciesReadAccess(t *testing.T) {
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

	var verbs []string
	for _, rule := range role.Rules {
		for _, res := range rule.Resources {
			if res == "toolpolicies" {
				verbs = rule.Verbs
			}
		}
	}
	assert.ElementsMatch(t, []string{"get", "list", "watch"}, verbs,
		"facade Role must grant get/list/watch on toolpolicies for the policy-broker sidecar")
}

// TestReconcileRole_ExcludesToolRegistriesReadAccess asserts the runtime's
// agent Role no longer grants toolregistries. The runtime's only GET was
// vestigial and 403'd on cross-namespace refs; registry provenance now comes
// from Config (#1874). The grant would silently disable registry-scoped
// ToolPolicies via the fail-open it enabled.
func TestReconcileRole_ExcludesToolRegistriesReadAccess(t *testing.T) {
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
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}
	require.NoError(t, r.reconcileRole(context.Background(), ar))

	role := &rbacv1.Role{}
	require.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "test-agent-facade",
		Namespace: "test-ns",
	}, role))

	for _, rule := range role.Rules {
		for _, res := range rule.Resources {
			assert.NotEqual(t, "toolregistries", res,
				"facade Role must not grant toolregistries — the read that justified it was removed")
		}
	}
}

// reconcileRoleAndGetSecretVerbs reconciles the facade Role for ar and returns
// the verbs granted on core/secrets.
func reconcileRoleAndGetSecretVerbs(t *testing.T, ar *omniav1alpha1.AgentRuntime) []string {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}
	require.NoError(t, r.reconcileRole(context.Background(), ar))

	role := &rbacv1.Role{}
	require.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{
		Name: ar.Name + "-facade", Namespace: ar.Namespace,
	}, role))
	for _, rule := range role.Rules {
		for _, res := range rule.Resources {
			if res == "secrets" {
				return rule.Verbs
			}
		}
	}
	return nil
}

// TestReconcileRole_SecretsGetOnlyWithoutClientKeys: with no externalAuth.clientKeys,
// the facade only Gets a named Secret (oidc) — no list/watch.
func TestReconcileRole_SecretsGetOnlyWithoutClientKeys(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "no-clientkeys", Namespace: "test-ns", UID: "u1"},
	}
	verbs := reconcileRoleAndGetSecretVerbs(t, ar)
	assert.ElementsMatch(t, []string{"get"}, verbs,
		"without clientKeys auth the facade should only get a named Secret")
}

// TestReconcileRole_SecretsListWatchWithClientKeys is the #1591 regression: when
// externalAuth.clientKeys is set, the client-key store Lists Secrets by label, so the
// Role must grant list+watch (not just get) — else the facade crash-loops on
// RBAC once client-key auth is enabled.
func TestReconcileRole_SecretsListWatchWithClientKeys(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "with-clientkeys", Namespace: "test-ns", UID: "u2"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				ClientKeys: &omniav1alpha1.ClientKeysAuth{},
			},
		},
	}
	verbs := reconcileRoleAndGetSecretVerbs(t, ar)
	assert.ElementsMatch(t, []string{"get", "list", "watch"}, verbs,
		"clientKeys auth needs list+watch on Secrets for the label-selected store")
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

	// A Workspace owning the namespace is a precondition: the binding targets a
	// per-workspace role, so with no workspace there is nothing to bind (#1875).
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(testNsWorkspace()).Build()

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
	// Binds the reader scoped to this agent's own workspace, not the
	// cluster-wide one (#1875).
	assert.Equal(t, "omnia-workspace-test-ws-reader", crb.RoleRef.Name)
	assert.Len(t, crb.Subjects, 1)
	assert.Equal(t, "ServiceAccount", crb.Subjects[0].Kind)
	assert.Equal(t, "test-agent-facade", crb.Subjects[0].Name)
	assert.Equal(t, "test-ns", crb.Subjects[0].Namespace)
	// This label intentionally holds the NAMESPACE, not the workspace name.
	assert.Equal(t, "test-ns", crb.Labels["omnia.altairalabs.ai/workspace-reader-for"])
}

// TestReconcileWorkspaceReaderBinding_FollowsPodOverrideServiceAccount is the
// regression guard for #1223: spec.podOverrides.serviceAccountName replaces the
// pod SA, so the workspace-reader binding must target THAT SA (not
// <name>-facade) — otherwise the effective pod SA can't list workspaces,
// service discovery is denied, and the facade silently falls back to the
// in-memory session store with no recording.
func TestReconcileWorkspaceReaderBinding_FollowsPodOverrideServiceAccount(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PodOverrides: &omniav1alpha1.PodOverrides{
				ServiceAccountName: "omnia-runtime-wi",
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(testNsWorkspace()).Build()

	r := &AgentRuntimeReconciler{
		Client:                          fakeClient,
		Scheme:                          scheme,
		AgentWorkspaceReaderClusterRole: "omnia-agent-workspace-reader",
	}

	require.NoError(t, r.reconcileWorkspaceReaderBinding(context.Background(), ar))

	crb := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "test-ns-test-agent-workspace-reader",
	}, crb))
	require.Len(t, crb.Subjects, 1)
	assert.Equal(t, "omnia-runtime-wi", crb.Subjects[0].Name,
		"workspace-reader must bind the overridden pod SA, not <name>-facade")
	assert.Equal(t, "test-ns", crb.Subjects[0].Namespace)
}

// TestReconcileWorkspaceReaderBinding_SubjectFlipsOnOverrideChange covers the
// operational migration path: an agent first reconciled with the default SA,
// then updated to set podOverrides.serviceAccountName, must have its
// workspace-reader subject MOVED to the override SA with no stale subject left
// (the exact transition when migrating an existing agent onto a WI SA).
func TestReconcileWorkspaceReaderBinding_SubjectFlipsOnOverrideChange(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(testNsWorkspace()).Build()
	r := &AgentRuntimeReconciler{
		Client:                          fakeClient,
		Scheme:                          scheme,
		AgentWorkspaceReaderClusterRole: "omnia-agent-workspace-reader",
	}

	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "test-ns"},
	}
	// First reconcile: default <name>-facade SA.
	require.NoError(t, r.reconcileWorkspaceReaderBinding(context.Background(), ar))

	// User migrates the agent onto a workload-identity SA and we reconcile again.
	ar.Spec.PodOverrides = &omniav1alpha1.PodOverrides{ServiceAccountName: "omnia-runtime-wi"}
	require.NoError(t, r.reconcileWorkspaceReaderBinding(context.Background(), ar))

	crb := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "test-ns-test-agent-workspace-reader",
	}, crb))
	require.Len(t, crb.Subjects, 1, "no stale subject should linger after the override change")
	assert.Equal(t, "omnia-runtime-wi", crb.Subjects[0].Name)
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

// TestReconcileRoleBinding_FollowsPodOverrideServiceAccount guards the
// namespaced facade RoleBinding (CRD/secret read) the same way as the
// workspace-reader ClusterRoleBinding: its subject must be the effective pod SA
// when spec.podOverrides.serviceAccountName is set (#1223).
func TestReconcileRoleBinding_FollowsPodOverrideServiceAccount(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PodOverrides: &omniav1alpha1.PodOverrides{
				ServiceAccountName: "omnia-runtime-wi",
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}
	require.NoError(t, r.reconcileRoleBinding(context.Background(), ar))

	rb := &rbacv1.RoleBinding{}
	require.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "test-agent-facade",
		Namespace: "test-ns",
	}, rb))
	// RoleBinding + Role keep the operator-managed name; only the subject moves.
	assert.Equal(t, "Role", rb.RoleRef.Kind)
	assert.Equal(t, "test-agent-facade", rb.RoleRef.Name)
	require.Len(t, rb.Subjects, 1)
	assert.Equal(t, "omnia-runtime-wi", rb.Subjects[0].Name,
		"facade RoleBinding must bind the overridden pod SA")
	assert.Equal(t, "test-ns", rb.Subjects[0].Namespace)
}
