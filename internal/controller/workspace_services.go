/*
Copyright 2026 Altaira Labs.

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

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// setReconcileError sets a failure condition, marks the workspace as error, and updates status.
func (r *WorkspaceReconciler) setReconcileError(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	condType, reason string,
	err error,
	log logr.Logger,
) (ctrl.Result, error) {
	SetCondition(&workspace.Status.Conditions, workspace.Generation, condType, metav1.ConditionFalse,
		reason, err.Error())
	workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
	if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
		log.Error(statusErr, logMsgFailedToUpdateStatus)
	}
	return ctrl.Result{}, err
}

// setServicesReadyCondition updates the ServicesReady condition based on service group statuses.
func setServicesReadyCondition(conditions *[]metav1.Condition, generation int64, services []omniav1alpha1.ServiceGroupStatus) {
	if len(services) == 0 {
		return
	}
	for _, svc := range services {
		if !svc.Ready {
			SetCondition(conditions, generation, ConditionTypeServicesReady, metav1.ConditionFalse,
				"ServicesNotReady", "One or more service groups are not ready")
			return
		}
	}
	SetCondition(conditions, generation, ConditionTypeServicesReady, metav1.ConditionTrue,
		"ServicesReady", "All service groups are ready")
}

// reconcileServices ensures that each service group defined in the workspace
// spec has the correct Deployments, Services, and status entries.
func (r *WorkspaceReconciler) reconcileServices(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)
	namespace := workspace.Spec.Namespace.Name

	statuses := make([]omniav1alpha1.ServiceGroupStatus, 0, len(workspace.Spec.Services))

	for _, sg := range workspace.Spec.Services {
		status, err := r.reconcileServiceGroup(ctx, workspace, namespace, sg)
		if err != nil {
			return fmt.Errorf("service group %s: %w", sg.Name, err)
		}
		statuses = append(statuses, status)
	}

	workspace.Status.Services = statuses

	if err := r.cleanupRemovedServiceGroups(ctx, workspace, namespace); err != nil {
		log.Error(err, "service group cleanup failed")
		return err
	}

	return nil
}

// reconcileServiceGroup handles a single service group, returning its status.
func (r *WorkspaceReconciler) reconcileServiceGroup(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
	sg omniav1alpha1.WorkspaceServiceGroup,
) (omniav1alpha1.ServiceGroupStatus, error) {
	if sg.Mode == omniav1alpha1.ServiceModeExternal {
		return r.reconcileExternalServiceGroup(sg), nil
	}
	return r.reconcileManagedServiceGroup(ctx, workspace, namespace, sg)
}

// reconcileExternalServiceGroup returns a status for an externally-provided service group.
func (r *WorkspaceReconciler) reconcileExternalServiceGroup(sg omniav1alpha1.WorkspaceServiceGroup) omniav1alpha1.ServiceGroupStatus {
	return omniav1alpha1.ServiceGroupStatus{
		Name:       sg.Name,
		SessionURL: sg.External.SessionURL,
		MemoryURL:  sg.External.MemoryURL,
		Ready:      true,
	}
}

// reconcileManagedServiceGroup creates/updates Deployments and Services for a managed group.
func (r *WorkspaceReconciler) reconcileManagedServiceGroup(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
	sg omniav1alpha1.WorkspaceServiceGroup,
) (omniav1alpha1.ServiceGroupStatus, error) {
	sessionDepName := fmt.Sprintf("session-%s-%s", workspace.Name, sg.Name)
	memoryDepName := fmt.Sprintf("memory-%s-%s", workspace.Name, sg.Name)

	// Reconcile session-api deployment and service
	sessionDep := r.ServiceBuilder.BuildSessionDeployment(workspace.Name, namespace, sg)
	if err := r.reconcileManagedDeployment(ctx, workspace, namespace, sessionDep); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("session deployment: %w", err)
	}

	sessionSvc := BuildService(sessionDepName, namespace, "session-api", workspace.Name, sg.Name)
	if err := r.reconcileManagedService(ctx, workspace, namespace, sessionSvc); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("session service: %w", err)
	}

	// Reconcile memory-api deployment and service
	memoryDep := r.ServiceBuilder.BuildMemoryDeployment(workspace.Name, namespace, sg)
	if err := r.reconcileManagedDeployment(ctx, workspace, namespace, memoryDep); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("memory deployment: %w", err)
	}

	memorySvc := BuildService(memoryDepName, namespace, "memory-api", workspace.Name, sg.Name)
	if err := r.reconcileManagedService(ctx, workspace, namespace, memorySvc); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("memory service: %w", err)
	}

	// Check readiness
	sessionReady := r.isDeploymentReady(ctx, sessionDepName, namespace)
	memoryReady := r.isDeploymentReady(ctx, memoryDepName, namespace)

	return omniav1alpha1.ServiceGroupStatus{
		Name:       sg.Name,
		SessionURL: ServiceURL(sessionDepName, namespace),
		MemoryURL:  ServiceURL(memoryDepName, namespace),
		Ready:      sessionReady && memoryReady,
	}, nil
}

// reconcileManagedDeployment creates or updates a Deployment for a managed service group.
func (r *WorkspaceReconciler) reconcileManagedDeployment(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
	desired *appsv1.Deployment,
) error {
	existing := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)

	if apierrors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(workspace, desired, r.Scheme); err != nil {
			return fmt.Errorf("set controller reference: %w", err)
		}
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get deployment %s/%s: %w", namespace, desired.Name, err)
	}

	// Update existing deployment
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

// reconcileManagedService creates or updates a Service for a managed service group.
func (r *WorkspaceReconciler) reconcileManagedService(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
	desired *corev1.Service,
) error {
	existing := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)

	if apierrors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(workspace, desired, r.Scheme); err != nil {
			return fmt.Errorf("set controller reference: %w", err)
		}
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get service %s/%s: %w", namespace, desired.Name, err)
	}

	// Update existing service
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Ports = desired.Spec.Ports
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

// isDeploymentReady returns true if the named deployment has at least one ready replica.
func (r *WorkspaceReconciler) isDeploymentReady(ctx context.Context, name, namespace string) bool {
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, dep); err != nil {
		return false
	}
	return dep.Status.ReadyReplicas > 0
}

// cleanupRemovedServiceGroups deletes Deployments and Services for service groups
// that are no longer present in the workspace spec.
func (r *WorkspaceReconciler) cleanupRemovedServiceGroups(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
) error {
	// Build set of expected managed group names
	expected := make(map[string]bool)
	for _, sg := range workspace.Spec.Services {
		if sg.Mode != omniav1alpha1.ServiceModeExternal {
			expected[sg.Name] = true
		}
	}

	// Clean up deployments
	if err := r.cleanupOrphanedDeployments(ctx, workspace.Name, namespace, expected); err != nil {
		return err
	}

	// Clean up services
	return r.cleanupOrphanedServices(ctx, workspace.Name, namespace, expected)
}

// cleanupOrphanedDeployments deletes Deployments whose service group is not in the expected set.
func (r *WorkspaceReconciler) cleanupOrphanedDeployments(
	ctx context.Context,
	workspaceName, namespace string,
	expected map[string]bool,
) error {
	depList := &appsv1.DeploymentList{}
	if err := r.List(ctx, depList,
		client.InNamespace(namespace),
		client.MatchingLabels{labelAppManagedBy: labelValueOmniaOperator, labelWorkspace: workspaceName},
	); err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	for i := range depList.Items {
		groupName := depList.Items[i].Labels[labelServiceGroup]
		if groupName != "" && !expected[groupName] {
			if err := r.Delete(ctx, &depList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete deployment %s: %w", depList.Items[i].Name, err)
			}
		}
	}
	return nil
}

// cleanupOrphanedServices deletes Services whose service group is not in the expected set.
func (r *WorkspaceReconciler) cleanupOrphanedServices(
	ctx context.Context,
	workspaceName, namespace string,
	expected map[string]bool,
) error {
	svcList := &corev1.ServiceList{}
	if err := r.List(ctx, svcList,
		client.InNamespace(namespace),
		client.MatchingLabels{labelAppManagedBy: labelValueOmniaOperator, labelWorkspace: workspaceName},
	); err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	for i := range svcList.Items {
		groupName := svcList.Items[i].Labels[labelServiceGroup]
		if groupName != "" && !expected[groupName] {
			if err := r.Delete(ctx, &svcList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete service %s: %w", svcList.Items[i].Name, err)
			}
		}
	}
	return nil
}
