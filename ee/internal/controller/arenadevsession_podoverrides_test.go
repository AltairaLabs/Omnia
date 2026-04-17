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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func newDevSessionDeploymentFixture() *appsv1.Deployment {
	return &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": "arena-dev-console"},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "default",
					Containers: []corev1.Container{{
						Name: "arena-dev-console",
					}},
				},
			},
		},
	}
}

func TestApplyDevSessionPodOverrides_Nil(t *testing.T) {
	dep := newDevSessionDeploymentFixture()
	session := &eev1alpha1.ArenaDevSession{}
	applyDevSessionPodOverrides(dep, session)
	require.Equal(t, "default", dep.Spec.Template.Spec.ServiceAccountName, "nil PodOverrides must not mutate")
}

func TestApplyDevSessionPodOverrides_AllFields(t *testing.T) {
	dep := newDevSessionDeploymentFixture()
	session := &eev1alpha1.ArenaDevSession{
		Spec: eev1alpha1.ArenaDevSessionSpec{
			PodOverrides: &corev1alpha1.PodOverrides{
				ServiceAccountName: "dev-sa",
				NodeSelector:       map[string]string{"team": "arena"},
				ExtraEnv:           []corev1.EnvVar{{Name: "DEV_FLAG", Value: "true"}},
			},
		},
	}
	applyDevSessionPodOverrides(dep, session)

	spec := dep.Spec.Template.Spec
	require.Equal(t, "dev-sa", spec.ServiceAccountName)
	require.Equal(t, "arena", spec.NodeSelector["team"])

	c := spec.Containers[0]
	hasDevFlag := false
	for _, e := range c.Env {
		if e.Name == "DEV_FLAG" {
			hasDevFlag = true
		}
	}
	require.True(t, hasDevFlag, "extraEnv DEV_FLAG must be applied")
}
