/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func newWorkerJobFixture() *batchv1.Job {
	return &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": "arena-worker"},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "arena-worker",
					Containers: []corev1.Container{{
						Name: "worker",
					}},
				},
			},
		},
	}
}

func TestApplyWorkerPodOverrides_Nil(t *testing.T) {
	job := newWorkerJobFixture()
	aj := &eev1alpha1.ArenaJob{}
	applyWorkerPodOverrides(job, aj)
	require.Equal(t, "arena-worker", job.Spec.Template.Spec.ServiceAccountName, "nil Workers must not mutate")
}

func TestApplyWorkerPodOverrides_EmptyOverrides(t *testing.T) {
	job := newWorkerJobFixture()
	aj := &eev1alpha1.ArenaJob{Spec: eev1alpha1.ArenaJobSpec{Workers: &eev1alpha1.WorkerConfig{}}}
	applyWorkerPodOverrides(job, aj)
	require.Equal(t, "arena-worker", job.Spec.Template.Spec.ServiceAccountName, "Workers without PodOverrides must not mutate")
}

func TestApplyWorkerPodOverrides_AllFields(t *testing.T) {
	job := newWorkerJobFixture()
	aj := &eev1alpha1.ArenaJob{
		Spec: eev1alpha1.ArenaJobSpec{
			Workers: &eev1alpha1.WorkerConfig{
				PodOverrides: &corev1alpha1.PodOverrides{
					NodeSelector:      map[string]string{"workload": "batch"},
					ImagePullSecrets:  []corev1.LocalObjectReference{{Name: "regcred"}},
					ExtraVolumes:      []corev1.Volume{{Name: "kv"}},
					ExtraVolumeMounts: []corev1.VolumeMount{{Name: "kv", MountPath: "/mnt/kv"}},
					ExtraEnvFrom: []corev1.EnvFromSource{{
						SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "provider-creds"}},
					}},
				},
			},
		},
	}
	applyWorkerPodOverrides(job, aj)

	spec := job.Spec.Template.Spec
	require.Equal(t, "batch", spec.NodeSelector["workload"])
	require.NotEmpty(t, spec.ImagePullSecrets)
	require.Equal(t, "regcred", spec.ImagePullSecrets[0].Name)
	require.Equal(t, "kv", spec.Volumes[0].Name)

	c := spec.Containers[0]
	require.Equal(t, "kv", c.VolumeMounts[0].Name)
	require.NotEmpty(t, c.EnvFrom)
	require.Equal(t, "provider-creds", c.EnvFrom[0].SecretRef.Name)
}
