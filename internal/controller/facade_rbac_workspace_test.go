package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Agent lives in namespace "omnia-demo", which is owned by workspace "demo".
func workspaceReaderAgent() *omniav1alpha1.AgentRuntime {
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "omnia-demo"
	return ar
}

func workspaceReaderBindingName(ar *omniav1alpha1.AgentRuntime) string {
	return fmt.Sprintf("%s-%s-workspace-reader", ar.Namespace, ar.Name)
}

// testNsWorkspace is the Workspace owning the "test-ns" namespace used by the
// older facade RBAC tests. Named "test-ws" so the two identifiers stay visibly
// distinct (#1875).
func testNsWorkspace() *omniav1alpha1.Workspace {
	return &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ws", UID: "test-ws-uid"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Test",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "test-ns"},
		},
	}
}

// The agent binds the per-workspace reader, not the cluster-wide one. That is
// the whole point of #1875: a pod in workspace A must not be able to enumerate
// B, C and D.
func TestReconcileWorkspaceReaderBinding_BindsThePerWorkspaceRole(t *testing.T) {
	s := workspaceRBACScheme(t)
	ar := workspaceReaderAgent()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(demoWorkspace(), ar).Build()
	r := &AgentRuntimeReconciler{
		Client:                     c,
		Scheme:                     s,
		WorkspaceReaderRBACEnabled: true,
	}

	require.NoError(t, r.reconcileWorkspaceReaderBinding(context.Background(), ar))

	var crb rbacv1.ClusterRoleBinding
	require.NoError(t, c.Get(context.Background(),
		client.ObjectKey{Name: workspaceReaderBindingName(ar)}, &crb))

	assert.Equal(t, WorkspaceReaderClusterRoleName("demo"), crb.RoleRef.Name)
	assert.NotEqual(t, "omnia-agent-workspace-reader", crb.RoleRef.Name,
		"agents must no longer bind the cluster-wide reader")
}

// ClusterRoleBinding.roleRef is immutable. On upgrade the binding already
// exists pointing at the cluster-wide role, so the reconciler must delete and
// recreate it — a plain update is rejected by the API server and every
// reconcile would fail (#1875).
func TestReconcileWorkspaceReaderBinding_RepointsAnExistingBinding(t *testing.T) {
	s := workspaceRBACScheme(t)
	ar := workspaceReaderAgent()
	existing := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: workspaceReaderBindingName(ar)},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     kindClusterRole,
			Name:     "omnia-agent-workspace-reader", // the pre-upgrade wide role
		},
		Subjects: []rbacv1.Subject{{
			Kind: kindServiceAccount, Name: "old-sa", Namespace: ar.Namespace,
		}},
	}
	c := fake.NewClientBuilder().WithScheme(s).
		WithObjects(demoWorkspace(), ar, existing).Build()
	r := &AgentRuntimeReconciler{
		Client:                     c,
		Scheme:                     s,
		WorkspaceReaderRBACEnabled: true,
	}

	require.NoError(t, r.reconcileWorkspaceReaderBinding(context.Background(), ar))

	var crb rbacv1.ClusterRoleBinding
	require.NoError(t, c.Get(context.Background(),
		client.ObjectKey{Name: workspaceReaderBindingName(ar)}, &crb))
	assert.Equal(t, WorkspaceReaderClusterRoleName("demo"), crb.RoleRef.Name)
}

// A namespace with no owning Workspace gets no binding at all, rather than one
// pointing at a role that will never exist.
func TestReconcileWorkspaceReaderBinding_SkipsWhenNamespaceHasNoWorkspace(t *testing.T) {
	s := workspaceRBACScheme(t)
	ar := workspaceReaderAgent()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ar).Build() // no Workspace
	r := &AgentRuntimeReconciler{
		Client:                     c,
		Scheme:                     s,
		WorkspaceReaderRBACEnabled: true,
	}

	require.NoError(t, r.reconcileWorkspaceReaderBinding(context.Background(), ar))

	var crb rbacv1.ClusterRoleBinding
	err := c.Get(context.Background(),
		client.ObjectKey{Name: workspaceReaderBindingName(ar)}, &crb)
	assert.Error(t, err, "no workspace means no binding")
}
