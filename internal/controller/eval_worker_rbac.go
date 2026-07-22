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

// labelWorkspaceReaderFor scopes a Workspace-reader ClusterRoleBinding to the
// namespace whose ServiceAccount it grants. Two namespaces can both have a
// service group named "default", so cleanup must filter CRBs by this label to
// avoid deleting another namespace's binding.
const labelWorkspaceReaderFor = "omnia.altairalabs.ai/workspace-reader-for"

const (
	rbacAPIGroup       = "rbac.authorization.k8s.io"
	omniaAPIGroup      = "omnia.altairalabs.ai"
	kindServiceAccount = "ServiceAccount"
	kindClusterRole    = "ClusterRole"
	kindRole           = "Role"
	verbGet            = "get"
	verbList           = "list"
)

// evalWorkerRBACLabels is the label set carried by every operator-managed
// eval-worker RBAC object. It matches the Deployment's selector labels so
// cleanupEvalWorkers can find the RBAC alongside the Deployment.
func evalWorkerRBACLabels(serviceGroup string) map[string]string {
	return map[string]string{
		labelAppName:      labelValueEvalWorker,
		labelAppInstance:  evalWorkerName(serviceGroup),
		labelAppManagedBy: labelValueOmniaOperator,
		labelServiceGroup: serviceGroup,
	}
}

// evalWorkerServiceAccountName returns the ServiceAccount the eval-worker pod
// runs as: the podOverrides override when set (e.g. a shared workload-identity
// SA so the worker's llm_judge can mint a keyless cloud token), otherwise the
// operator-managed default arena-eval-worker-<group>. RBAC subjects must target
// this SA so an overridden (WI) SA still receives the worker's read grants.
func evalWorkerServiceAccountName(serviceGroup string, po *omniav1alpha1.PodOverrides) string {
	if po != nil && po.ServiceAccountName != "" {
		return po.ServiceAccountName
	}
	return evalWorkerName(serviceGroup)
}

// ensureEvalWorkerRBAC creates or updates the per-service-group eval-worker
// ServiceAccount, Role, RoleBinding, and (when a workspace-reader ClusterRole is
// configured) a ClusterRoleBinding for cluster-scoped Workspace reads.
//
// All objects are OWNER-LESS: a per-group worker outlives any single
// AgentRuntime, so owning the RBAC by one agent would garbage-collect it
// prematurely. The objects share the eval-worker label set and are cleaned up
// explicitly in cleanupEvalWorkers, mirroring the Deployment lifecycle.
//
// When podOverrides points the worker at an external ServiceAccount, the
// operator does not create/own that SA (it is user/chart-managed) but still
// binds the Role + workspace-reader ClusterRole to it.
func (r *AgentRuntimeReconciler) ensureEvalWorkerRBAC(
	ctx context.Context,
	namespace, serviceGroup string,
	podOverrides *omniav1alpha1.PodOverrides,
) error {
	saName := evalWorkerServiceAccountName(serviceGroup, podOverrides)
	// Only manage the default SA; an overridden SA is owned elsewhere.
	if saName == evalWorkerName(serviceGroup) {
		if err := r.ensureEvalWorkerServiceAccount(ctx, namespace, serviceGroup); err != nil {
			return fmt.Errorf("ensure eval worker ServiceAccount: %w", err)
		}
	}
	if err := r.ensureEvalWorkerRole(ctx, namespace, serviceGroup); err != nil {
		return fmt.Errorf("ensure eval worker Role: %w", err)
	}
	if err := r.ensureEvalWorkerRoleBinding(ctx, namespace, serviceGroup, saName); err != nil {
		return fmt.Errorf("ensure eval worker RoleBinding: %w", err)
	}
	if err := r.ensureEvalWorkerWorkspaceReaderBinding(ctx, namespace, serviceGroup, saName); err != nil {
		return fmt.Errorf("ensure eval worker workspace reader ClusterRoleBinding: %w", err)
	}
	return nil
}

// ensureEvalWorkerServiceAccount creates/updates the eval-worker ServiceAccount.
func (r *AgentRuntimeReconciler) ensureEvalWorkerServiceAccount(
	ctx context.Context,
	namespace, serviceGroup string,
) error {
	log := logf.FromContext(ctx)
	name := evalWorkerName(serviceGroup)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		sa.Labels = evalWorkerRBACLabels(serviceGroup)
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("eval worker ServiceAccount reconciled",
		"name", name, "namespace", namespace, "serviceGroup", serviceGroup, "result", result)
	return nil
}

// ensureEvalWorkerRole creates/updates the eval-worker Role granting namespaced
// read access to the resources the worker resolves (PromptPack eval-def
// ConfigMaps, provider API-key Secrets, AgentRuntimes, Providers).
func (r *AgentRuntimeReconciler) ensureEvalWorkerRole(
	ctx context.Context,
	namespace, serviceGroup string,
) error {
	log := logf.FromContext(ctx)
	name := evalWorkerName(serviceGroup)

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		role.Labels = evalWorkerRBACLabels(serviceGroup)
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps", "secrets"},
				Verbs:     []string{verbGet, verbList},
			},
			{
				APIGroups: []string{omniaAPIGroup},
				Resources: []string{"agentruntimes", "providers", "promptpacks"},
				Verbs:     []string{verbGet, verbList},
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("eval worker Role reconciled",
		"name", name, "namespace", namespace, "serviceGroup", serviceGroup, "result", result)
	return nil
}

// ensureEvalWorkerRoleBinding creates/updates the RoleBinding tying the
// eval-worker ServiceAccount to its Role.
func (r *AgentRuntimeReconciler) ensureEvalWorkerRoleBinding(
	ctx context.Context,
	namespace, serviceGroup, saName string,
) error {
	log := logf.FromContext(ctx)
	name := evalWorkerName(serviceGroup)

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
		rb.Labels = evalWorkerRBACLabels(serviceGroup)
		rb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     kindRole,
			Name:     name,
		}
		rb.Subjects = []rbacv1.Subject{
			{
				Kind:      kindServiceAccount,
				Name:      saName,
				Namespace: namespace,
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("eval worker RoleBinding reconciled",
		"name", name, "namespace", namespace, "serviceGroup", serviceGroup, "result", result)
	return nil
}

// ensureEvalWorkerWorkspaceReaderBinding creates/updates a ClusterRoleBinding
// granting the eval-worker ServiceAccount read access to cluster-scoped
// Workspace CRDs (session-api URL fallback). Skipped when no workspace-reader
// ClusterRole is configured (e.g. local dev, tests), mirroring the facade.
func (r *AgentRuntimeReconciler) ensureEvalWorkerWorkspaceReaderBinding(
	ctx context.Context,
	namespace, serviceGroup, saName string,
) error {
	if r.AgentWorkspaceReaderClusterRole == "" {
		return nil
	}

	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s-workspace-reader", namespace, evalWorkerName(serviceGroup))

	// Scoped to this eval-worker's own workspace rather than the cluster-wide
	// reader (#1875). An unresolved workspace skips this reconcile rather than
	// deleting an existing binding — see the note in facade_rbac.go for why a
	// transient lookup failure must not tear down valid RBAC.
	wsName, _ := r.resolveWorkspaceForNamespace(namespace)
	if wsName == "" {
		log.V(1).Info("skipping eval worker workspace-reader ClusterRoleBinding",
			"reason", "no Workspace owns this namespace", "namespace", namespace)
		return nil
	}
	roleName := WorkspaceReaderClusterRoleName(wsName)

	if err := deleteStaleRoleRefBinding(ctx, r.Client, name, roleName); err != nil {
		return err
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, crb, func() error {
		labels := evalWorkerRBACLabels(serviceGroup)
		labels[labelWorkspaceReaderFor] = namespace
		crb.Labels = labels
		crb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     kindClusterRole,
			Name:     roleName,
		}
		crb.Subjects = []rbacv1.Subject{
			{
				Kind:      kindServiceAccount,
				Name:      saName,
				Namespace: namespace,
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("eval worker ClusterRoleBinding reconciled",
		"name", name, "namespace", namespace, "serviceGroup", serviceGroup, "result", result)
	return nil
}
