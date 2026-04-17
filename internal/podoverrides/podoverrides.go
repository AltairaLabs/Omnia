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

// Package podoverrides applies shared PodOverrides from the API types onto
// operator-built PodSpec / Container values. Kept in its own package so both
// core (internal/controller) and enterprise (ee/internal/controller) reconcilers
// can import it without creating an import cycle (internal/controller already
// depends on ee/api/v1alpha1).
package podoverrides

import (
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ApplyPod applies pod-level fields from the user-supplied PodOverrides
// onto the operator-built PodSpec and pod ObjectMeta. It is nil-safe.
//
// Merge semantics:
//   - ServiceAccountName, Affinity, PriorityClassName: user replaces operator-default when set.
//   - Labels: operator-set keys always win (service selectors depend on them).
//   - Annotations: user values override operator defaults.
//   - NodeSelector: merged by key; user wins on collision.
//   - Tolerations, TopologySpreadConstraints, ImagePullSecrets, ExtraVolumes: appended.
//
// Container-scoped fields (ExtraEnv, ExtraEnvFrom, ExtraVolumeMounts) are handled
// separately by ApplyContainer.
func ApplyPod(spec *corev1.PodSpec, meta *metav1.ObjectMeta, overrides *omniav1alpha1.PodOverrides) {
	if overrides == nil || spec == nil {
		return
	}

	if overrides.ServiceAccountName != "" {
		spec.ServiceAccountName = overrides.ServiceAccountName
	}

	if meta != nil {
		meta.Labels = mergeLabels(meta.Labels, overrides.Labels)
		meta.Annotations = mergeUserWins(meta.Annotations, overrides.Annotations)
	}

	if len(overrides.NodeSelector) > 0 {
		spec.NodeSelector = mergeUserWins(spec.NodeSelector, overrides.NodeSelector)
	}

	if len(overrides.Tolerations) > 0 {
		spec.Tolerations = append(spec.Tolerations, overrides.Tolerations...)
	}

	if overrides.Affinity != nil {
		spec.Affinity = overrides.Affinity.DeepCopy()
	}

	if overrides.PriorityClassName != "" {
		spec.PriorityClassName = overrides.PriorityClassName
	}

	if len(overrides.TopologySpreadConstraints) > 0 {
		spec.TopologySpreadConstraints = append(spec.TopologySpreadConstraints, overrides.TopologySpreadConstraints...)
	}

	if len(overrides.ImagePullSecrets) > 0 {
		spec.ImagePullSecrets = append(spec.ImagePullSecrets, overrides.ImagePullSecrets...)
	}

	if len(overrides.ExtraVolumes) > 0 {
		spec.Volumes = append(spec.Volumes, overrides.ExtraVolumes...)
	}
}

// ApplyContainer appends container-scoped fields from PodOverrides
// (ExtraEnv, ExtraEnvFrom, ExtraVolumeMounts) onto the given container. It is
// nil-safe. Callers MUST skip operator-injected sidecars (e.g. policy-proxy)
// that should not receive user env/mounts.
func ApplyContainer(container *corev1.Container, overrides *omniav1alpha1.PodOverrides) {
	if overrides == nil || container == nil {
		return
	}
	if len(overrides.ExtraEnv) > 0 {
		container.Env = append(container.Env, overrides.ExtraEnv...)
	}
	if len(overrides.ExtraEnvFrom) > 0 {
		container.EnvFrom = append(container.EnvFrom, overrides.ExtraEnvFrom...)
	}
	if len(overrides.ExtraVolumeMounts) > 0 {
		container.VolumeMounts = append(container.VolumeMounts, overrides.ExtraVolumeMounts...)
	}
}

// mergeLabels returns the union of operator-set and user-supplied labels where
// operator-set keys always win (they are load-bearing for Service selectors).
func mergeLabels(operatorSet, user map[string]string) map[string]string {
	if len(user) == 0 {
		return operatorSet
	}
	out := make(map[string]string, len(operatorSet)+len(user))
	maps.Copy(out, user)
	maps.Copy(out, operatorSet)
	return out
}

// mergeUserWins returns the union where user-supplied keys override base keys.
func mergeUserWins(base, user map[string]string) map[string]string {
	if len(user) == 0 {
		return base
	}
	out := make(map[string]string, len(base)+len(user))
	maps.Copy(out, base)
	maps.Copy(out, user)
	return out
}
