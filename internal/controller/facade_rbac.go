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

// facadeServiceAccountName returns the ServiceAccount name for the facade.
func facadeServiceAccountName(agentRuntime *omniav1alpha1.AgentRuntime) string {
	return agentRuntime.Name + facadeServiceAccountSuffix
}

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

// reconcileRole creates/updates the facade Role with read access to CRDs and secrets.
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
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
				Resources: []string{"agentruntimes", "providers"},
				Verbs:     []string{"get"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get"},
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
				Name:      name,
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
