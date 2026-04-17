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

package v1alpha1

import corev1 "k8s.io/api/core/v1"

// PodOverrides carries per-workload scheduling, identity, and injection
// overrides applied to every operator-generated Pod. All fields optional.
//
// Pod-level fields (serviceAccountName, labels, annotations, nodeSelector,
// tolerations, affinity, priorityClassName, topologySpreadConstraints,
// imagePullSecrets) are merged into the PodSpec.
//
// Container-level fields (extraEnv, extraEnvFrom, extraVolumeMounts) are
// appended to every non-operator-injected container in the pod. extraVolumes
// are appended to the PodSpec and available to all containers.
//
// Existing per-CRD hooks (e.g. FacadeConfig.ExtraEnv, RuntimeConfig.NodeSelector)
// take precedence; PodOverrides values are merged/concatenated after them.
type PodOverrides struct {
	// serviceAccountName overrides the default service account.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// labels are merged into pod labels. Operator-set labels take precedence
	// on key collision (they are load-bearing for Service selectors).
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations are merged into pod annotations. User values override
	// operator-set defaults on key collision.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// nodeSelector keys are merged with operator-set keys; user values win on collision.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// tolerations are appended to the pod's tolerations.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// affinity replaces the operator-default affinity when set.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// priorityClassName overrides the operator-default priority class when set.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// topologySpreadConstraints are appended to the pod's constraints.
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// imagePullSecrets are appended to the pod's imagePullSecrets.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// extraEnv is appended to every non-operator-injected container's env.
	// +optional
	ExtraEnv []corev1.EnvVar `json:"extraEnv,omitempty"`

	// extraEnvFrom is appended to every non-operator-injected container's envFrom.
	// +optional
	ExtraEnvFrom []corev1.EnvFromSource `json:"extraEnvFrom,omitempty"`

	// extraVolumes are appended to the PodSpec.Volumes.
	// +optional
	ExtraVolumes []corev1.Volume `json:"extraVolumes,omitempty"`

	// extraVolumeMounts are appended to every non-operator-injected container's volumeMounts.
	// +optional
	ExtraVolumeMounts []corev1.VolumeMount `json:"extraVolumeMounts,omitempty"`
}
