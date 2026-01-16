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
	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

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
			Name: "promptpack-config",
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

	// Add user-specified volumes for media files, mock configs, etc.
	if agentRuntime.Spec.Runtime != nil && len(agentRuntime.Spec.Runtime.Volumes) > 0 {
		volumes = append(volumes, agentRuntime.Spec.Runtime.Volumes...)
	}

	return volumes
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
			Name:      "promptpack-config",
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

	// Add user-specified volume mounts for media files, mock configs, etc.
	if agentRuntime.Spec.Runtime != nil && len(agentRuntime.Spec.Runtime.VolumeMounts) > 0 {
		volumeMounts = append(volumeMounts, agentRuntime.Spec.Runtime.VolumeMounts...)
	}

	return volumeMounts
}
