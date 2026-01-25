// Copyright (c) 2025 Altaira Labs
// SPDX-License-Identifier: FSL-1.1-Apache-2.0

// Package workspace provides utilities for workspace-related operations in enterprise features.
package workspace

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	// LabelWorkspace identifies the workspace that owns a resource.
	LabelWorkspace = "omnia.altairalabs.ai/workspace"
	// LabelWorkspaceManaged indicates a resource is managed by the workspace controller.
	LabelWorkspaceManaged = "omnia.altairalabs.ai/workspace-managed"
	// LabelValueTrue is the string "true" used for label values.
	LabelValueTrue = "true"
)

// StorageManager handles workspace storage operations for Arena features.
type StorageManager struct {
	Client              client.Client
	DefaultStorageClass string
}

// NewStorageManager creates a new StorageManager.
func NewStorageManager(c client.Client, defaultStorageClass string) *StorageManager {
	return &StorageManager{
		Client:              c,
		DefaultStorageClass: defaultStorageClass,
	}
}

// EnsureWorkspacePVC creates the workspace shared PVC if storage is enabled and it doesn't exist.
// This is an idempotent operation - it will not fail if the PVC already exists.
// Returns the PVC name if successful, or an error if storage is not enabled or creation fails.
func (m *StorageManager) EnsureWorkspacePVC(ctx context.Context, workspaceName string) (string, error) {
	log := logf.FromContext(ctx)

	// Get the workspace to read storage config
	workspace := &omniav1alpha1.Workspace{}
	if err := m.Client.Get(ctx, client.ObjectKey{Name: workspaceName}, workspace); err != nil {
		return "", fmt.Errorf("failed to get workspace %s: %w", workspaceName, err)
	}

	namespaceName := workspace.Spec.Namespace.Name
	pvcName := fmt.Sprintf("workspace-%s-content", namespaceName)

	// Check if storage is enabled
	if !m.isStorageEnabled(workspace) {
		return "", fmt.Errorf("workspace storage is not enabled for workspace %s", workspaceName)
	}

	// Parse storage configuration
	quantity, accessModes, err := m.parseStorageConfig(workspace.Spec.Storage)
	if err != nil {
		return "", fmt.Errorf("failed to parse storage config: %w", err)
	}

	// Create or update the PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespaceName,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, m.Client, pvc, func() error {
		return m.mutatePVC(pvc, workspace, quantity, accessModes)
	})
	if err != nil {
		return "", fmt.Errorf("failed to create/update PVC %s: %w", pvcName, err)
	}

	if result != controllerutil.OperationResultNone {
		log.Info("Workspace PVC reconciled", "name", pvcName, "namespace", namespaceName, "result", result)
	}

	return pvcName, nil
}

// GetWorkspacePVCName returns the PVC name for a workspace if storage is enabled.
// Returns empty string and nil error if storage is not enabled.
// Returns empty string and error if workspace lookup fails.
func (m *StorageManager) GetWorkspacePVCName(ctx context.Context, workspaceName string) (string, error) {
	workspace := &omniav1alpha1.Workspace{}
	if err := m.Client.Get(ctx, client.ObjectKey{Name: workspaceName}, workspace); err != nil {
		return "", fmt.Errorf("failed to get workspace %s: %w", workspaceName, err)
	}

	if !m.isStorageEnabled(workspace) {
		return "", nil
	}

	return fmt.Sprintf("workspace-%s-content", workspace.Spec.Namespace.Name), nil
}

// PVCExists checks if the workspace PVC exists.
func (m *StorageManager) PVCExists(ctx context.Context, workspaceName string) (bool, error) {
	workspace := &omniav1alpha1.Workspace{}
	if err := m.Client.Get(ctx, client.ObjectKey{Name: workspaceName}, workspace); err != nil {
		return false, fmt.Errorf("failed to get workspace %s: %w", workspaceName, err)
	}

	namespaceName := workspace.Spec.Namespace.Name
	pvcName := fmt.Sprintf("workspace-%s-content", namespaceName)

	pvc := &corev1.PersistentVolumeClaim{}
	err := m.Client.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: namespaceName}, pvc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// isStorageEnabled checks if storage is enabled for the workspace.
func (m *StorageManager) isStorageEnabled(workspace *omniav1alpha1.Workspace) bool {
	return workspace.Spec.Storage != nil &&
		(workspace.Spec.Storage.Enabled == nil || *workspace.Spec.Storage.Enabled)
}

// parseStorageConfig parses the storage configuration and returns quantity and access modes.
func (m *StorageManager) parseStorageConfig(
	config *omniav1alpha1.WorkspaceStorageConfig,
) (resource.Quantity, []corev1.PersistentVolumeAccessMode, error) {
	storageSize := "10Gi"
	if config.Size != "" {
		storageSize = config.Size
	}

	accessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
	if len(config.AccessModes) > 0 {
		accessModes = make([]corev1.PersistentVolumeAccessMode, 0, len(config.AccessModes))
		for _, mode := range config.AccessModes {
			accessModes = append(accessModes, corev1.PersistentVolumeAccessMode(mode))
		}
	}

	quantity, err := resource.ParseQuantity(storageSize)
	if err != nil {
		return resource.Quantity{}, nil, fmt.Errorf("invalid storage size %s: %w", storageSize, err)
	}

	return quantity, accessModes, nil
}

// mutatePVC sets or updates the PVC spec and labels.
func (m *StorageManager) mutatePVC(
	pvc *corev1.PersistentVolumeClaim,
	workspace *omniav1alpha1.Workspace,
	quantity resource.Quantity,
	accessModes []corev1.PersistentVolumeAccessMode,
) error {
	storageConfig := workspace.Spec.Storage

	if pvc.CreationTimestamp.IsZero() {
		// New PVC - set all fields
		pvc.Labels = map[string]string{
			LabelWorkspace:        workspace.Name,
			LabelWorkspaceManaged: LabelValueTrue,
		}
		pvc.Spec = corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: quantity,
				},
			},
		}
		// Use explicit storage class from workspace spec, or fall back to controller default
		storageClass := storageConfig.StorageClass
		if storageClass == "" {
			storageClass = m.DefaultStorageClass
		}
		if storageClass != "" {
			pvc.Spec.StorageClassName = &storageClass
		}
	} else {
		// Existing PVC - only update labels (PVC spec is immutable)
		if pvc.Labels == nil {
			pvc.Labels = make(map[string]string)
		}
		pvc.Labels[LabelWorkspace] = workspace.Name
		pvc.Labels[LabelWorkspaceManaged] = LabelValueTrue
	}
	return nil
}
