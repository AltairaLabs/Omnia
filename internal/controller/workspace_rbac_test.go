package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// workspaceRBACScheme builds a scheme with the Workspace CRD and RBAC types.
func workspaceRBACScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(s))
	require.NoError(t, rbacv1.AddToScheme(s))
	return s
}

// demoWorkspace is named "demo" and owns the namespace "omnia-demo" — the pair
// exists so that any code putting the namespace where the workspace name
// belongs fails here rather than failing closed in a cluster (#1875).
func demoWorkspace() *omniav1alpha1.Workspace {
	return &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", UID: "ws-uid-abc"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Demo",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "omnia-demo"},
		},
	}
}

func TestReconcileWorkspaceReaderClusterRole_ScopesToTheWorkspaceName(t *testing.T) {
	s := workspaceRBACScheme(t)
	ws := demoWorkspace()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ws).Build()
	r := &WorkspaceReconciler{Client: c, Scheme: s}

	require.NoError(t, r.reconcileWorkspaceReaderClusterRole(context.Background(), ws))

	var cr rbacv1.ClusterRole
	require.NoError(t, c.Get(context.Background(),
		client.ObjectKey{Name: WorkspaceReaderClusterRoleName("demo")}, &cr))

	require.Len(t, cr.Rules, 1)
	rule := cr.Rules[0]
	assert.Equal(t, []string{verbGet}, rule.Verbs,
		"resourceNames cannot restrict list/watch, so the grant must be get-only")
	assert.Equal(t, []string{"demo"}, rule.ResourceNames)
	assert.NotContains(t, rule.ResourceNames, "omnia-demo",
		"resourceNames must hold the workspace name, not the namespace it owns")
	assert.Equal(t, []string{"workspaces"}, rule.Resources)
	assert.NotContains(t, rule.Resources, "memorypolicies",
		"agents never read MemoryPolicy; that grant belongs to memory-api alone")
}

// Workspace is cluster-scoped, so it can own a ClusterRole and the role is
// garbage-collected with it. A namespaced owner could not.
func TestReconcileWorkspaceReaderClusterRole_IsOwnedByTheWorkspace(t *testing.T) {
	s := workspaceRBACScheme(t)
	ws := demoWorkspace()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ws).Build()
	r := &WorkspaceReconciler{Client: c, Scheme: s}

	require.NoError(t, r.reconcileWorkspaceReaderClusterRole(context.Background(), ws))

	var cr rbacv1.ClusterRole
	require.NoError(t, c.Get(context.Background(),
		client.ObjectKey{Name: WorkspaceReaderClusterRoleName("demo")}, &cr))

	require.Len(t, cr.OwnerReferences, 1)
	assert.Equal(t, "demo", cr.OwnerReferences[0].Name)
	assert.Equal(t, "Workspace", cr.OwnerReferences[0].Kind)
}

func TestReconcileWorkspaceReaderClusterRole_IsIdempotent(t *testing.T) {
	s := workspaceRBACScheme(t)
	ws := demoWorkspace()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ws).Build()
	r := &WorkspaceReconciler{Client: c, Scheme: s}

	require.NoError(t, r.reconcileWorkspaceReaderClusterRole(context.Background(), ws))
	require.NoError(t, r.reconcileWorkspaceReaderClusterRole(context.Background(), ws))

	var list rbacv1.ClusterRoleList
	require.NoError(t, c.List(context.Background(), &list))
	assert.Len(t, list.Items, 1)
}

func TestWorkspaceReaderClusterRoleName_DerivesFromWorkspaceName(t *testing.T) {
	assert.Equal(t, "omnia-workspace-demo-reader", WorkspaceReaderClusterRoleName("demo"))
}
