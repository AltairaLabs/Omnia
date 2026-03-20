/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
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

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

const arenaWorkerRBACName = "arena-worker"

// reconcileWorkerRBAC creates the ServiceAccount, Role, and RoleBinding for arena worker
// CRD reads in the ArenaJob's namespace. Resources are owned by the ArenaJob and will be
// garbage collected when the job is deleted.
func (r *ArenaJobReconciler) reconcileWorkerRBAC(
	ctx context.Context,
	arenaJob *omniav1alpha1.ArenaJob,
) (string, error) {
	saName := arenaWorkerRBACName

	if err := r.reconcileWorkerServiceAccount(ctx, arenaJob, saName); err != nil {
		return "", fmt.Errorf("reconcile worker ServiceAccount: %w", err)
	}
	if err := r.reconcileWorkerRole(ctx, arenaJob, saName); err != nil {
		return "", fmt.Errorf("reconcile worker Role: %w", err)
	}
	if err := r.reconcileWorkerRoleBinding(ctx, arenaJob, saName); err != nil {
		return "", fmt.Errorf("reconcile worker RoleBinding: %w", err)
	}

	return saName, nil
}

// reconcileWorkerServiceAccount creates/updates the arena worker ServiceAccount.
func (r *ArenaJobReconciler) reconcileWorkerServiceAccount(
	ctx context.Context,
	arenaJob *omniav1alpha1.ArenaJob,
	name string,
) error {
	log := logf.FromContext(ctx)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: arenaJob.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		if err := controllerutil.SetOwnerReference(arenaJob, sa, r.Scheme); err != nil {
			return err
		}
		sa.Labels = map[string]string{
			"app.kubernetes.io/name":       "arena-worker",
			"app.kubernetes.io/managed-by": "omnia-operator",
			"omnia.altairalabs.ai/job":     arenaJob.Name,
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("worker ServiceAccount reconciled", "name", name, "result", result)
	return nil
}

// reconcileWorkerRole creates/updates the Role granting read access to Omnia CRDs.
func (r *ArenaJobReconciler) reconcileWorkerRole(
	ctx context.Context,
	arenaJob *omniav1alpha1.ArenaJob,
	name string,
) error {
	log := logf.FromContext(ctx)

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: arenaJob.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		if err := controllerutil.SetOwnerReference(arenaJob, role, r.Scheme); err != nil {
			return err
		}
		role.Labels = map[string]string{
			"app.kubernetes.io/name":       "arena-worker",
			"app.kubernetes.io/managed-by": "omnia-operator",
			"omnia.altairalabs.ai/job":     arenaJob.Name,
		}
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"omnia.altairalabs.ai"},
				Resources: []string{"providers", "agentruntimes", "toolregistries", "arenajobs"},
				Verbs:     []string{"get"},
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("worker Role reconciled", "name", name, "result", result)
	return nil
}

// reconcileWorkerRoleBinding creates/updates the RoleBinding for the worker ServiceAccount.
func (r *ArenaJobReconciler) reconcileWorkerRoleBinding(
	ctx context.Context,
	arenaJob *omniav1alpha1.ArenaJob,
	name string,
) error {
	log := logf.FromContext(ctx)

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: arenaJob.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
		if err := controllerutil.SetOwnerReference(arenaJob, rb, r.Scheme); err != nil {
			return err
		}
		rb.Labels = map[string]string{
			"app.kubernetes.io/name":       "arena-worker",
			"app.kubernetes.io/managed-by": "omnia-operator",
			"omnia.altairalabs.ai/job":     arenaJob.Name,
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
				Namespace: arenaJob.Namespace,
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("worker RoleBinding reconciled", "name", name, "result", result)
	return nil
}
