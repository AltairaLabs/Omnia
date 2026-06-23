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
	"fmt"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// reconcileStorage checks and updates workspace storage status.
// PVC creation is handled lazily by Arena controllers when Arena CRDs are created.
// This function only tracks the status of existing PVCs and handles cleanup.
func (r *WorkspaceReconciler) reconcileStorage(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	namespaceName := workspace.Spec.Namespace.Name
	// Use namespace name (not workspace name) so ArenaJob can derive PVC name from namespace
	pvcName := fmt.Sprintf("workspace-%s-content", namespaceName)

	// Check if storage is enabled (defaults to false if not specified for backward compat)
	storageEnabled := workspace.Spec.Storage != nil &&
		(workspace.Spec.Storage.Enabled == nil || *workspace.Spec.Storage.Enabled)

	if !storageEnabled {
		return r.deleteStoragePVCIfExists(ctx, workspace, pvcName, namespaceName)
	}

	// Storage is enabled - check if PVC exists and update status
	// PVC will be created lazily by Arena controllers when needed
	return r.updateStorageStatusIfPVCExists(ctx, workspace, pvcName, namespaceName)
}

// deleteStoragePVCIfExists deletes the workspace storage PVC if it exists and is managed by us.
func (r *WorkspaceReconciler) deleteStoragePVCIfExists(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	pvcName, namespaceName string,
) error {
	log := logf.FromContext(ctx)
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: namespaceName}, pvc)
	if err == nil {
		if pvc.Labels[labelWorkspace] == workspace.Name && pvc.Labels[labelWorkspaceManaged] == labelValueTrue {
			if err := r.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete PVC %s: %w", pvcName, err)
			}
			log.Info("Deleted PVC (storage disabled)", "name", pvcName)
		}
	}
	workspace.Status.Storage = nil
	return nil
}

// updateStorageStatusIfPVCExists updates the workspace status with PVC information if the PVC exists.
// If the PVC doesn't exist yet, it sets the status to indicate it will be created on-demand.
func (r *WorkspaceReconciler) updateStorageStatusIfPVCExists(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	pvcName, namespaceName string,
) error {
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: namespaceName}, pvc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// PVC doesn't exist yet - will be created on-demand by Arena controllers
			workspace.Status.Storage = &omniav1alpha1.WorkspaceStorageStatus{
				PVCName:   pvcName,
				Phase:     "Pending",
				MountPath: fmt.Sprintf("/workspace-content/%s/%s", workspace.Name, namespaceName),
			}
			return nil
		}
		return fmt.Errorf("failed to get PVC status: %w", err)
	}

	capacity := ""
	if pvc.Status.Capacity != nil {
		if storageQty, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
			capacity = storageQty.String()
		}
	}

	workspace.Status.Storage = &omniav1alpha1.WorkspaceStorageStatus{
		PVCName:   pvcName,
		Phase:     string(pvc.Status.Phase),
		Capacity:  capacity,
		MountPath: fmt.Sprintf("/workspace-content/%s/%s", workspace.Name, namespaceName),
	}

	return nil
}
