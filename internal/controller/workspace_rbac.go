/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// WorkspaceReaderClusterRoleName returns the name of the per-workspace
// ClusterRole granting get on exactly one Workspace. Derived from the workspace
// NAME (e.g. "demo"), not the namespace that workspace owns ("omnia-demo").
func WorkspaceReaderClusterRoleName(workspaceName string) string {
	return fmt.Sprintf("omnia-workspace-%s-reader", workspaceName)
}

// deleteStaleRoleRefBinding removes a ClusterRoleBinding whose roleRef points
// somewhere other than roleName, so the caller's CreateOrUpdate can recreate it.
//
// roleRef is immutable: the API server rejects an update that changes it. Every
// binding created before #1875 points at the cluster-wide reader, so without
// this an upgrade fails reconcile forever rather than repointing.
func deleteStaleRoleRefBinding(ctx context.Context, c client.Client, name, roleName string) error {
	existing := &rbacv1.ClusterRoleBinding{}
	err := c.Get(ctx, client.ObjectKey{Name: name}, existing)
	switch {
	case apierrors.IsNotFound(err):
		return nil
	case err != nil:
		return fmt.Errorf("get workspace-reader binding %q: %w", name, err)
	case existing.RoleRef.Name == roleName:
		return nil
	}
	if err := c.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete stale workspace-reader binding %q: %w", name, err)
	}
	return nil
}

// reconcileWorkspaceReaderClusterRole ensures a ClusterRole granting get on
// this Workspace alone. Agent pods and eval-workers bind it instead of the
// cluster-wide reader, so a pod in workspace A can no longer enumerate the
// configuration of B, C and D (#1875).
//
// The cluster-wide agent-workspace-reader ClusterRole is gone (#1899).
// session-api and memory-api service pods bind this same per-workspace
// get-only role too — their remaining cluster-wide need (the EE memory
// consolidation worker's MemoryPolicy list) is served by a separate,
// enterprise-gated reader role
// (charts/omnia/templates/memory-enterprise-reader-clusterrole.yaml).
//
// The grant is deliberately get-only. RBAC resourceNames cannot restrict
// collection verbs — there is no way to express "list just one" — and that is
// fine here, because agent config is read once at boot and nothing watches it.
//
// Workspace is cluster-scoped, so it can own a ClusterRole and the role is
// garbage-collected with the workspace. A namespaced owner such as AgentRuntime
// could not own a cluster-scoped object, which is why this lives on the
// Workspace reconciler rather than alongside the bindings.
//
// The clusterroles RBAC marker lives in the canonical block above
// WorkspaceReconciler.Reconcile in workspace_controller.go; markers on
// individual reconcile helpers are not picked up by controller-gen.
func (r *WorkspaceReconciler) reconcileWorkspaceReaderClusterRole(
	ctx context.Context, workspace *omniav1alpha1.Workspace,
) error {
	log := logf.FromContext(ctx)

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: WorkspaceReaderClusterRoleName(workspace.Name)},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, cr, func() error {
		if err := controllerutil.SetControllerReference(workspace, cr, r.Scheme); err != nil {
			return err
		}
		cr.Labels = map[string]string{
			labelAppName:      labelValueOmniaAgent,
			labelAppManagedBy: labelValueOmniaOperator,
			labelWorkspace:    workspace.Name,
		}
		cr.Rules = []rbacv1.PolicyRule{{
			APIGroups:     []string{omniaAPIGroup},
			Resources:     []string{"workspaces"},
			Verbs:         []string{verbGet},
			ResourceNames: []string{workspace.Name},
		}}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reconcile workspace reader ClusterRole: %w", err)
	}

	log.V(1).Info("workspace reader ClusterRole reconciled",
		"name", cr.Name, "workspace", workspace.Name, "result", result)
	return nil
}
