/*
Copyright 2025.

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
	"errors"
	"fmt"
	"time"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

//nolint:gocognit // Deletion logic requires handling many resource types
func (r *WorkspaceReconciler) reconcileDelete(ctx context.Context, workspace *omniav1alpha1.Workspace) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("workspace deletion started")

	namespaceName := workspace.Spec.Namespace.Name
	labels := client.MatchingLabels{labelWorkspace: workspace.Name, labelWorkspaceManaged: labelValueTrue}

	var errs []error

	// Clean up PVCs (only if retention policy is Delete or not specified)
	retentionPolicy := "Delete"
	if workspace.Spec.Storage != nil && workspace.Spec.Storage.RetentionPolicy != "" {
		retentionPolicy = workspace.Spec.Storage.RetentionPolicy
	}
	if retentionPolicy == "Delete" {
		if err := r.deletePVCs(ctx, namespaceName, labels, log); err != nil {
			errs = append(errs, err)
		}
	} else {
		log.Info("retaining PVC", "retentionPolicy", retentionPolicy)
	}

	if err := r.deleteNetworkPolicies(ctx, namespaceName, labels, log); err != nil {
		errs = append(errs, err)
	}
	if err := r.deleteRoleBindings(ctx, namespaceName, labels, log); err != nil {
		errs = append(errs, err)
	}
	if err := r.deleteServiceAccounts(ctx, namespaceName, labels, log); err != nil {
		errs = append(errs, err)
	}
	if err := r.deleteNamespaceIfCreated(ctx, workspace, namespaceName, log); err != nil {
		errs = append(errs, err)
	}

	// Only remove finalizer if ALL deletions succeeded
	if len(errs) > 0 {
		combined := errors.Join(errs...)
		log.Error(combined, "partial cleanup failure, retaining finalizer", "errorCount", len(errs))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, combined
	}

	controllerutil.RemoveFinalizer(workspace, WorkspaceFinalizerName)
	if err := r.Update(ctx, workspace); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// deletePVCs deletes all managed PVCs in the namespace.
//
// Pagination was removed: the controller-runtime cache that backs r.Client
// does not support client.Continue tokens (it holds the entire watched set
// in memory), and attempting paginated listing returned "continue list
// option is not supported by the cache" errors that kept the finalizer
// stuck and blocked namespace cleanup in e2e tests. The cache already has
// everything in memory, so a single List is both correct and cheap.
func (r *WorkspaceReconciler) deletePVCs(ctx context.Context, ns string, labels client.MatchingLabels, log logr.Logger) error {
	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcs, client.InNamespace(ns), labels); err != nil {
		return fmt.Errorf("list PVCs: %w", err)
	}
	var errs []error
	for i := range pvcs.Items {
		if err := r.Delete(ctx, &pvcs.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "delete PVC failed", "name", pvcs.Items[i].Name)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deleteNetworkPolicies deletes all managed NetworkPolicies. See deletePVCs
// comment — pagination removed for the same reason.
func (r *WorkspaceReconciler) deleteNetworkPolicies(ctx context.Context, ns string, labels client.MatchingLabels, log logr.Logger) error {
	list := &networkingv1.NetworkPolicyList{}
	if err := r.List(ctx, list, client.InNamespace(ns), labels); err != nil {
		return fmt.Errorf("list NetworkPolicies: %w", err)
	}
	var errs []error
	for i := range list.Items {
		if err := r.Delete(ctx, &list.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "delete NetworkPolicy failed", "name", list.Items[i].Name)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deleteRoleBindings deletes all managed RoleBindings. See deletePVCs
// comment — pagination removed for the same reason.
func (r *WorkspaceReconciler) deleteRoleBindings(ctx context.Context, ns string, labels client.MatchingLabels, log logr.Logger) error {
	list := &rbacv1.RoleBindingList{}
	if err := r.List(ctx, list, client.InNamespace(ns), labels); err != nil {
		return fmt.Errorf("list RoleBindings: %w", err)
	}
	var errs []error
	for i := range list.Items {
		if err := r.Delete(ctx, &list.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "delete RoleBinding failed", "name", list.Items[i].Name)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deleteServiceAccounts deletes all managed ServiceAccounts. See deletePVCs
// comment — pagination removed for the same reason.
func (r *WorkspaceReconciler) deleteServiceAccounts(ctx context.Context, ns string, labels client.MatchingLabels, log logr.Logger) error {
	list := &corev1.ServiceAccountList{}
	if err := r.List(ctx, list, client.InNamespace(ns), labels); err != nil {
		return fmt.Errorf("list ServiceAccounts: %w", err)
	}
	var errs []error
	for i := range list.Items {
		if err := r.Delete(ctx, &list.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "delete ServiceAccount failed", "name", list.Items[i].Name)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deleteNamespaceIfCreated deletes the workspace namespace only if the controller created it.
func (r *WorkspaceReconciler) deleteNamespaceIfCreated(ctx context.Context, workspace *omniav1alpha1.Workspace, namespaceName string, log logr.Logger) error {
	if workspace.Status.Namespace == nil || !workspace.Status.Namespace.Created {
		return nil
	}
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, client.ObjectKey{Name: namespaceName}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get namespace: %w", err)
	}
	if ns.Labels[labelWorkspace] != workspace.Name {
		return nil
	}
	if err := r.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "delete namespace failed", "namespace", namespaceName)
		return err
	}
	return nil
}
