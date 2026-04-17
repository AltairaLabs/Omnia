/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newEvalWorkerTestReconciler() *AgentRuntimeReconciler {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	return &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
}

func TestBuildEvalWorkerDeployment_PodOverrides(t *testing.T) {
	r := newEvalWorkerTestReconciler()
	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Evals: &omniav1alpha1.EvalConfig{
				Enabled: true,
				PodOverrides: &omniav1alpha1.PodOverrides{
					ServiceAccountName: "eval-sa",
					NodeSelector:       map[string]string{"workload": "batch"},
					ExtraEnv:           []corev1.EnvVar{{Name: "JUDGE_API_KEY_FILE", Value: "/mnt/kv/key"}},
					ExtraVolumes:       []corev1.Volume{{Name: "kv"}},
					ExtraVolumeMounts:  []corev1.VolumeMount{{Name: "kv", MountPath: "/mnt/kv"}},
				},
			},
		},
	}

	dep := r.buildEvalWorkerDeployment(context.Background(), agent)
	spec := dep.Spec.Template.Spec

	require.Equal(t, "eval-sa", spec.ServiceAccountName)
	require.Equal(t, "batch", spec.NodeSelector["workload"])
	require.NotEmpty(t, spec.Volumes)
	require.Equal(t, "kv", spec.Volumes[0].Name)

	c := spec.Containers[0]
	hasEnv := false
	for _, e := range c.Env {
		if e.Name == "JUDGE_API_KEY_FILE" {
			hasEnv = true
		}
	}
	require.True(t, hasEnv, "extraEnv must be applied on eval-worker container")
	require.NotEmpty(t, c.VolumeMounts)
	require.Equal(t, "kv", c.VolumeMounts[0].Name)
}

func TestBuildEvalWorkerDeployment_NoOverrides(t *testing.T) {
	r := newEvalWorkerTestReconciler()
	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
	}
	dep := r.buildEvalWorkerDeployment(context.Background(), agent)
	require.Empty(t, dep.Spec.Template.Spec.ServiceAccountName, "no overrides, default SA")
}
