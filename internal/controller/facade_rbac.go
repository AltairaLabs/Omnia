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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const facadeServiceAccountSuffix = "-facade"

// facadeServiceAccountName returns the operator-managed ServiceAccount name for
// the facade (the <name>-facade SA the operator creates).
func facadeServiceAccountName(agentRuntime *omniav1alpha1.AgentRuntime) string {
	return agentRuntime.Name + facadeServiceAccountSuffix
}

// The SA the facade pod actually runs as is resolved by
// r.effectiveFacadeServiceAccountName (see workspace_runtime_identity.go): the
// agent's own podOverrides SA, else the Workspace runtime-default SA, else the
// operator-created <name>-facade SA. RBAC must target that SA — otherwise an
// overridden/inherited pod SA (e.g. an Azure Workload Identity SA) never
// receives the workspace-reader binding, service discovery's `list workspaces`
// is denied, and the facade silently falls back to the in-memory session store
// with no recording (issue #1223).

// reconcileFacadeRBAC creates the ServiceAccount, Role, and RoleBinding for facade CRD reading.
func (r *AgentRuntimeReconciler) reconcileFacadeRBAC(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	if err := r.reconcileServiceAccount(ctx, agentRuntime); err != nil {
		return fmt.Errorf("reconcile ServiceAccount: %w", err)
	}
	if err := r.reconcileRole(ctx, agentRuntime); err != nil {
		return fmt.Errorf("reconcile Role: %w", err)
	}
	if err := r.reconcileRoleBinding(ctx, agentRuntime); err != nil {
		return fmt.Errorf("reconcile RoleBinding: %w", err)
	}
	if err := r.reconcileWorkspaceReaderBinding(ctx, agentRuntime); err != nil {
		return fmt.Errorf("reconcile workspace reader ClusterRoleBinding: %w", err)
	}
	return nil
}

// reconcileWorkspaceReaderBinding creates a ClusterRoleBinding granting the facade SA
// read access to Workspace CRDs (cluster-scoped) for service URL resolution.
// The clusterrolebindings RBAC marker lives in the canonical block above
// AgentRuntimeReconciler.reconcileReferences in agentruntime_controller.go;
// markers on individual reconcile helpers are not picked up by controller-gen.
func (r *AgentRuntimeReconciler) reconcileWorkspaceReaderBinding(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	if r.AgentWorkspaceReaderClusterRole == "" {
		// Not configured — skip (e.g. local dev, tests)
		return nil
	}

	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s-workspace-reader", agentRuntime.Namespace, agentRuntime.Name)

	// Bind the reader scoped to this agent's own workspace rather than the
	// cluster-wide one (#1875). No workspace means no binding, rather than one
	// pointing at a role that will never exist.
	wsName, _ := r.resolveWorkspaceForNamespace(agentRuntime.Namespace)
	if wsName == "" {
		log.V(1).Info("skipping workspace-reader ClusterRoleBinding",
			"reason", "no Workspace owns this namespace",
			"namespace", agentRuntime.Namespace)
		return nil
	}
	roleName := WorkspaceReaderClusterRoleName(wsName)

	if err := deleteStaleRoleRefBinding(ctx, r.Client, name, roleName); err != nil {
		return err
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, crb, func() error {
		crb.Labels = map[string]string{
			labelAppName:      labelValueOmniaAgent,
			labelAppInstance:  agentRuntime.Name,
			labelAppManagedBy: labelValueOmniaOperator,
			// Intentionally the NAMESPACE, not the workspace name — this label
			// is consumed as a namespace and is tested that way.
			"omnia.altairalabs.ai/workspace-reader-for": agentRuntime.Namespace,
		}
		crb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     kindClusterRole,
			Name:     roleName,
		}
		crb.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      r.effectiveFacadeServiceAccountName(agentRuntime),
				Namespace: agentRuntime.Namespace,
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("ClusterRoleBinding reconciled", "name", name, "result", result)
	return nil
}

// reconcileServiceAccount creates/updates the facade ServiceAccount.
func (r *AgentRuntimeReconciler) reconcileServiceAccount(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	log := logf.FromContext(ctx)
	saName := facadeServiceAccountName(agentRuntime)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: agentRuntime.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		if err := controllerutil.SetControllerReference(agentRuntime, sa, r.Scheme); err != nil {
			return err
		}
		sa.Labels = map[string]string{
			labelAppName:      labelValueOmniaAgent,
			labelAppInstance:  agentRuntime.Name,
			labelAppManagedBy: labelValueOmniaOperator,
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("ServiceAccount reconciled", "name", saName, "result", result)
	return nil
}

// facadeSecretVerbs returns the Secret verbs the facade Role needs. oidc
// reads a single named Secret (get), but externalAuth.clientKeys backs its
// validator with a label-selected Secret List at startup — which needs
// list/watch, or the facade crash-loops on RBAC once client-key auth is set
// (#1591). RBAC can't scope a list by label, so this widens to a namespace-wide
// secrets read, granted only when client-key auth is actually configured.
func facadeSecretVerbs(agentRuntime *omniav1alpha1.AgentRuntime) []string {
	if ea := agentRuntime.Spec.ExternalAuth; ea != nil && ea.ClientKeys != nil {
		return []string{verbGet, verbList, "watch"}
	}
	return []string{verbGet}
}

// reconcileRole creates/updates the facade Role with read access to CRDs and secrets.
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolpolicies,verbs=get;list;watch
func (r *AgentRuntimeReconciler) reconcileRole(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	log := logf.FromContext(ctx)
	roleName := facadeServiceAccountName(agentRuntime)

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: agentRuntime.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		if err := controllerutil.SetControllerReference(agentRuntime, role, r.Scheme); err != nil {
			return err
		}
		role.Labels = map[string]string{
			labelAppName:      labelValueOmniaAgent,
			labelAppInstance:  agentRuntime.Name,
			labelAppManagedBy: labelValueOmniaOperator,
		}
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"omnia.altairalabs.ai"},
				// toolregistries removed: the runtime's only GET was vestigial
				// (it consumed just the lookup key it supplied) and 403'd on
				// cross-namespace refs, silently disabling registry-scoped
				// ToolPolicies. Registry provenance now comes from Config.
				Resources: []string{"agentruntimes", "providers"},
				Verbs:     []string{"get"},
			},
			{
				APIGroups: []string{"omnia.altairalabs.ai"},
				Resources: []string{"agentruntimes/status"},
				Verbs:     []string{"get", "patch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     facadeSecretVerbs(agentRuntime),
			},
			{
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
				Verbs:     []string{"get"},
			},
			{
				APIGroups: []string{"omnia.altairalabs.ai"},
				Resources: []string{"toolpolicies"},
				Verbs:     []string{"get", "list", "watch"},
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("Role reconciled", "name", roleName, "result", result)
	return nil
}

// reconcileRoleBinding creates/updates the facade RoleBinding.
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
func (r *AgentRuntimeReconciler) reconcileRoleBinding(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	log := logf.FromContext(ctx)
	name := facadeServiceAccountName(agentRuntime)

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: agentRuntime.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
		if err := controllerutil.SetControllerReference(agentRuntime, rb, r.Scheme); err != nil {
			return err
		}
		rb.Labels = map[string]string{
			labelAppName:      labelValueOmniaAgent,
			labelAppInstance:  agentRuntime.Name,
			labelAppManagedBy: labelValueOmniaOperator,
		}
		rb.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     name,
		}
		rb.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      r.effectiveFacadeServiceAccountName(agentRuntime),
				Namespace: agentRuntime.Namespace,
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("RoleBinding reconciled", "name", name, "result", result)
	return nil
}
