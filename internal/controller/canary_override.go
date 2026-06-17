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
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// deleteCanaryOverrideConfigMap removes the candidate's override ConfigMap as
// part of candidate teardown (promotion finish, rollback, idle cleanup). A
// no-op when already gone. The CM is owner-ref'd so it is also GC'd with the
// AgentRuntime, but deleting it here keeps a finished rollout from leaving a
// stale override behind.
func (r *AgentRuntimeReconciler) deleteCanaryOverrideConfigMap(ctx context.Context, ar *omniav1alpha1.AgentRuntime) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ar.Name + CanaryConfigMapSuffix,
			Namespace: ar.Namespace,
		},
	}
	if err := r.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete canary override ConfigMap: %w", err)
	}
	return nil
}

// canaryOverride is the wire contract written into the candidate pod's mounted
// override ConfigMap and read by the runtime (internal/runtime.CanaryOverride).
// Keep the JSON field names in sync with that type — the round-trip is guarded
// by a contract test. It carries provider *refs*; the runtime resolves the
// referenced Provider CRDs + secrets live, exactly as it does for stable.
type canaryOverride struct {
	ProviderRefs []omniav1alpha1.NamedProviderRef `json:"providerRefs,omitempty"`
}

// reconcileCanaryOverrideConfigMap creates/updates the <agent>-canary-config
// ConfigMap holding the candidate's effective provider refs. Mounted only into
// candidate pods so their runtime resolves the candidate's providers instead of
// self-reading the shared stable spec (#1468). Owner-ref'd for GC with the
// AgentRuntime.
func (r *AgentRuntimeReconciler) reconcileCanaryOverrideConfigMap(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) error {
	overrides := applyCandidateOverrides(ar)
	data, err := json.Marshal(canaryOverride{ProviderRefs: overrides.Providers})
	if err != nil {
		return fmt.Errorf("marshal canary override: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ar.Name + CanaryConfigMapSuffix,
			Namespace: ar.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if err := controllerutil.SetControllerReference(ar, cm, r.Scheme); err != nil {
			return err
		}
		cm.Data = map[string]string{CanaryOverrideFileName: string(data)}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reconcile canary override ConfigMap: %w", err)
	}
	return nil
}

// mountCanaryOverride adds the canary override volume to the pod and mounts it
// (read-only) into every container at CanaryOverrideMountPath, matching the
// existing pack-mounted-in-all-containers convention. Applied to the candidate
// Deployment only; stable pods never get the volume and fall back to the live
// spec.
func mountCanaryOverride(deployment *appsv1.Deployment, agentName string) {
	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: canaryOverrideVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: agentName + CanaryConfigMapSuffix},
			},
		},
	})
	mount := corev1.VolumeMount{
		Name:      canaryOverrideVolumeName,
		MountPath: CanaryOverrideMountPath,
		ReadOnly:  true,
	}
	for i := range deployment.Spec.Template.Spec.Containers {
		deployment.Spec.Template.Spec.Containers[i].VolumeMounts = append(
			deployment.Spec.Template.Spec.Containers[i].VolumeMounts, mount)
	}
}
