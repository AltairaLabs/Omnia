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
	"fmt"

	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// workspaceContentVolumeName is the volume + mount name that exposes the
// workspace content PVC to the runtime container, mirroring the arena
// worker convention.
const (
	workspaceContentVolumeName = "workspace-content"
	workspaceContentMountPath  = "/workspace-content"
)

// workspaceContentPVCName returns the per-namespace workspace content PVC
// name, matching the ee arena convention so a single PVC backs both kinds
// of workload.
func workspaceContentPVCName(namespace string) string {
	return fmt.Sprintf("workspace-%s-content", namespace)
}

func (r *AgentRuntimeReconciler) buildVolumes(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
) []corev1.Volume {
	var volumes []corev1.Volume

	// Mount PromptPack ConfigMap if source type is configmap
	if promptPack.Spec.Source.Type == omniav1alpha1.PromptPackSourceTypeConfigMap &&
		promptPack.Spec.Source.ConfigMapRef != nil {
		volumes = append(volumes, corev1.Volume{
			Name: promptpackConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: *promptPack.Spec.Source.ConfigMapRef,
				},
			},
		})
	}

	// Mount tools ConfigMap if ToolRegistry is present
	if toolRegistry != nil {
		volumes = append(volumes, corev1.Volume{
			Name: toolsConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: agentRuntime.Name + ToolsConfigMapSuffix,
					},
				},
			},
		})
	}

	// Mount the workspace content PVC when configured. Read-only — the
	// runtime only reads skills from this volume; writes happen via the
	// PromptPack reconciler in the operator pod.
	if r.WorkspaceContentPath != "" {
		volumes = append(volumes, corev1.Volume{
			Name: workspaceContentVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: workspaceContentPVCName(agentRuntime.Namespace),
					ReadOnly:  true,
				},
			},
		})
	}

	// Add user-specified volumes for media files, mock configs, etc.
	if agentRuntime.Spec.Runtime != nil && len(agentRuntime.Spec.Runtime.Volumes) > 0 {
		volumes = append(volumes, agentRuntime.Spec.Runtime.Volumes...)
	}

	return volumes
}

// buildFacadeVolumeMounts creates volume mounts for the facade container.
// The facade only needs the promptpack-config volume (for dual-protocol A2A access);
// it does not need tools-config or user-specified runtime volumes.
func (r *AgentRuntimeReconciler) buildFacadeVolumeMounts(
	promptPack *omniav1alpha1.PromptPack,
) []corev1.VolumeMount {
	var volumeMounts []corev1.VolumeMount

	if promptPack.Spec.Source.Type == omniav1alpha1.PromptPackSourceTypeConfigMap &&
		promptPack.Spec.Source.ConfigMapRef != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      promptpackConfigVolumeName,
			MountPath: PromptPackMountPath,
			ReadOnly:  true,
		})
	}

	return volumeMounts
}

// buildRuntimeVolumeMounts creates volume mounts for the runtime container.
func (r *AgentRuntimeReconciler) buildRuntimeVolumeMounts(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
) []corev1.VolumeMount {
	var volumeMounts []corev1.VolumeMount

	// Mount PromptPack ConfigMap
	if promptPack.Spec.Source.Type == omniav1alpha1.PromptPackSourceTypeConfigMap &&
		promptPack.Spec.Source.ConfigMapRef != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      promptpackConfigVolumeName,
			MountPath: PromptPackMountPath,
			ReadOnly:  true,
		})
	}

	// Mount tools ConfigMap if ToolRegistry is present
	if toolRegistry != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      toolsConfigVolumeName,
			MountPath: ToolsMountPath,
			ReadOnly:  true,
		})
	}

	// Mount the workspace content PVC into the runtime container so it can
	// read the skill manifest emitted by the PromptPack reconciler.
	if r.WorkspaceContentPath != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      workspaceContentVolumeName,
			MountPath: workspaceContentMountPath,
			ReadOnly:  true,
		})
	}

	// Add user-specified volume mounts for media files, mock configs, etc.
	if agentRuntime.Spec.Runtime != nil && len(agentRuntime.Spec.Runtime.VolumeMounts) > 0 {
		volumeMounts = append(volumeMounts, agentRuntime.Spec.Runtime.VolumeMounts...)
	}

	return volumeMounts
}

// skillManifestPath returns the workspace-content path the runtime container
// should read for its PromptPack skill manifest. Returns "" when skills are
// disabled (WorkspaceContentPath unset on the reconciler).
//
// The returned path is relative to the runtime container's mount point —
// the PVC subtree below /workspace-content/ already encodes workspace and
// namespace, so the manifest lives at .../manifests/<pack>.json.
func (r *AgentRuntimeReconciler) skillManifestPath(promptPackName string) string {
	if r.WorkspaceContentPath == "" {
		return ""
	}
	return fmt.Sprintf("%s/manifests/%s.json", workspaceContentMountPath, promptPackName)
}
