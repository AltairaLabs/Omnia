/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package podoverrides

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestApplyPod_Nil(t *testing.T) {
	spec := &corev1.PodSpec{ServiceAccountName: "default"}
	meta := &metav1.ObjectMeta{Labels: map[string]string{"a": "b"}}
	ApplyPod(spec, meta, nil)
	if spec.ServiceAccountName != "default" {
		t.Fatalf("nil overrides must not mutate spec")
	}
	if meta.Labels["a"] != "b" {
		t.Fatalf("nil overrides must not mutate meta")
	}
}

func TestApplyPod_NilSpec(t *testing.T) {
	ApplyPod(nil, &metav1.ObjectMeta{}, &omniav1alpha1.PodOverrides{ServiceAccountName: "x"})
}

func TestApplyPod_ServiceAccountReplaces(t *testing.T) {
	spec := &corev1.PodSpec{ServiceAccountName: "default"}
	ApplyPod(spec, &metav1.ObjectMeta{}, &omniav1alpha1.PodOverrides{
		ServiceAccountName: "csi-sa",
	})
	if spec.ServiceAccountName != "csi-sa" {
		t.Fatalf("want csi-sa, got %s", spec.ServiceAccountName)
	}
}

func TestApplyPod_EmptySANoReplace(t *testing.T) {
	spec := &corev1.PodSpec{ServiceAccountName: "default"}
	ApplyPod(spec, &metav1.ObjectMeta{}, &omniav1alpha1.PodOverrides{})
	if spec.ServiceAccountName != "default" {
		t.Fatalf("empty SA must not replace existing")
	}
}

func TestApplyPod_LabelsOperatorWins(t *testing.T) {
	meta := &metav1.ObjectMeta{Labels: map[string]string{"app.kubernetes.io/name": "operator-set"}}
	ApplyPod(&corev1.PodSpec{}, meta, &omniav1alpha1.PodOverrides{
		Labels: map[string]string{"app.kubernetes.io/name": "user-attempt", "custom": "ok"},
	})
	if meta.Labels["app.kubernetes.io/name"] != "operator-set" {
		t.Fatalf("operator-set label must win, got %s", meta.Labels["app.kubernetes.io/name"])
	}
	if meta.Labels["custom"] != "ok" {
		t.Fatalf("non-conflicting user label must be added, got %q", meta.Labels["custom"])
	}
}

func TestApplyPod_LabelsIntoEmptyMeta(t *testing.T) {
	meta := &metav1.ObjectMeta{}
	ApplyPod(&corev1.PodSpec{}, meta, &omniav1alpha1.PodOverrides{
		Labels: map[string]string{"foo": "bar"},
	})
	if meta.Labels["foo"] != "bar" {
		t.Fatalf("labels must be added when meta starts empty")
	}
}

func TestApplyPod_AnnotationsUserWins(t *testing.T) {
	meta := &metav1.ObjectMeta{Annotations: map[string]string{"sidecar.istio.io/inject": "true"}}
	ApplyPod(&corev1.PodSpec{}, meta, &omniav1alpha1.PodOverrides{
		Annotations: map[string]string{"sidecar.istio.io/inject": "false"},
	})
	if meta.Annotations["sidecar.istio.io/inject"] != "false" {
		t.Fatalf("user annotation must win, got %q", meta.Annotations["sidecar.istio.io/inject"])
	}
}

func TestApplyPod_NodeSelectorMerge(t *testing.T) {
	spec := &corev1.PodSpec{NodeSelector: map[string]string{"zone": "us-east-1a"}}
	ApplyPod(spec, &metav1.ObjectMeta{}, &omniav1alpha1.PodOverrides{
		NodeSelector: map[string]string{"zone": "us-west-2a", "gpu": "a100"},
	})
	if spec.NodeSelector["zone"] != "us-west-2a" {
		t.Fatalf("user nodeSelector must win on key collision")
	}
	if spec.NodeSelector["gpu"] != "a100" {
		t.Fatalf("new nodeSelector key must be added")
	}
}

func TestApplyPod_TolerationsAppend(t *testing.T) {
	spec := &corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: "existing"}}}
	ApplyPod(spec, &metav1.ObjectMeta{}, &omniav1alpha1.PodOverrides{
		Tolerations: []corev1.Toleration{{Key: "gpu"}},
	})
	if len(spec.Tolerations) != 2 || spec.Tolerations[1].Key != "gpu" {
		t.Fatalf("tolerations must be appended, got %+v", spec.Tolerations)
	}
}

func TestApplyPod_AffinityReplace(t *testing.T) {
	spec := &corev1.PodSpec{Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}}}
	custom := &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{}}
	ApplyPod(spec, &metav1.ObjectMeta{}, &omniav1alpha1.PodOverrides{Affinity: custom})
	if spec.Affinity.PodAntiAffinity == nil {
		t.Fatal("user affinity must replace operator-default")
	}
}

func TestApplyPod_PriorityClass(t *testing.T) {
	spec := &corev1.PodSpec{}
	ApplyPod(spec, &metav1.ObjectMeta{}, &omniav1alpha1.PodOverrides{PriorityClassName: "critical"})
	if spec.PriorityClassName != "critical" {
		t.Fatalf("priorityClass must be set")
	}
}

func TestApplyPod_TopologySpreadAppend(t *testing.T) {
	spec := &corev1.PodSpec{TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{MaxSkew: 1}}}
	ApplyPod(spec, &metav1.ObjectMeta{}, &omniav1alpha1.PodOverrides{
		TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{MaxSkew: 2}},
	})
	if len(spec.TopologySpreadConstraints) != 2 {
		t.Fatalf("topology spread must be appended, got %d", len(spec.TopologySpreadConstraints))
	}
}

func TestApplyPod_ImagePullSecretsAppend(t *testing.T) {
	spec := &corev1.PodSpec{ImagePullSecrets: []corev1.LocalObjectReference{{Name: "a"}}}
	ApplyPod(spec, &metav1.ObjectMeta{}, &omniav1alpha1.PodOverrides{
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: "b"}},
	})
	if len(spec.ImagePullSecrets) != 2 || spec.ImagePullSecrets[1].Name != "b" {
		t.Fatalf("imagePullSecrets must be appended, got %+v", spec.ImagePullSecrets)
	}
}

func TestApplyPod_ExtraVolumesAppend(t *testing.T) {
	spec := &corev1.PodSpec{Volumes: []corev1.Volume{{Name: "existing"}}}
	ApplyPod(spec, &metav1.ObjectMeta{}, &omniav1alpha1.PodOverrides{
		ExtraVolumes: []corev1.Volume{{Name: "kv"}},
	})
	if len(spec.Volumes) != 2 || spec.Volumes[1].Name != "kv" {
		t.Fatalf("volumes must be appended, got %+v", spec.Volumes)
	}
}

func TestApplyContainer_Nil(t *testing.T) {
	c := &corev1.Container{Env: []corev1.EnvVar{{Name: "A"}}}
	ApplyContainer(c, nil)
	if len(c.Env) != 1 {
		t.Fatalf("nil overrides must not mutate container")
	}
}

func TestApplyContainer_NilContainer(t *testing.T) {
	ApplyContainer(nil, &omniav1alpha1.PodOverrides{ExtraEnv: []corev1.EnvVar{{Name: "X"}}})
}

func TestApplyContainer_EnvAppend(t *testing.T) {
	c := &corev1.Container{Env: []corev1.EnvVar{{Name: "A", Value: "1"}}}
	ApplyContainer(c, &omniav1alpha1.PodOverrides{
		ExtraEnv: []corev1.EnvVar{{Name: "B", Value: "2"}},
	})
	if len(c.Env) != 2 || c.Env[1].Name != "B" {
		t.Fatalf("extraEnv must be appended, got %+v", c.Env)
	}
}

func TestApplyContainer_EnvFromAppend(t *testing.T) {
	c := &corev1.Container{}
	ApplyContainer(c, &omniav1alpha1.PodOverrides{
		ExtraEnvFrom: []corev1.EnvFromSource{{Prefix: "KV_"}},
	})
	if len(c.EnvFrom) != 1 || c.EnvFrom[0].Prefix != "KV_" {
		t.Fatalf("extraEnvFrom must be appended, got %+v", c.EnvFrom)
	}
}

func TestApplyContainer_VolumeMountsAppend(t *testing.T) {
	c := &corev1.Container{VolumeMounts: []corev1.VolumeMount{{Name: "tmp"}}}
	ApplyContainer(c, &omniav1alpha1.PodOverrides{
		ExtraVolumeMounts: []corev1.VolumeMount{{Name: "kv", MountPath: "/mnt/kv"}},
	})
	if len(c.VolumeMounts) != 2 || c.VolumeMounts[1].Name != "kv" {
		t.Fatalf("extraVolumeMounts must be appended, got %+v", c.VolumeMounts)
	}
}
