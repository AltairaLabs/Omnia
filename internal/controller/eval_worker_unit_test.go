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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

	dep := r.buildEvalWorkerDeployment(context.Background(), "ns", "default", agent.Spec.Evals.PodOverrides)
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
	dep := r.buildEvalWorkerDeployment(context.Background(), "ns", "default", nil)
	require.Empty(t, dep.Spec.Template.Spec.ServiceAccountName, "no overrides, default SA")
}

func TestEvalWorkerName_PerGroup(t *testing.T) {
	require.Equal(t, "arena-eval-worker-default", evalWorkerName("default"))
	require.Equal(t, "arena-eval-worker-prod", evalWorkerName("prod"))
}

func keysOf(m map[string]*omniav1alpha1.PodOverrides) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestServiceGroupsNeedingEvalWorker(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	langChain := &omniav1alpha1.FrameworkConfig{Type: omniav1alpha1.FrameworkTypeLangChain}
	agentA := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ServiceGroup: "default",
			Framework:    langChain,
			Evals:        &omniav1alpha1.EvalConfig{Enabled: true},
		},
	}
	agentB := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ServiceGroup: "prod",
			Framework:    langChain,
			Evals:        &omniav1alpha1.EvalConfig{Enabled: true},
		},
	}
	agentC := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ServiceGroup: "pk",
			Framework:    &omniav1alpha1.FrameworkConfig{Type: omniav1alpha1.FrameworkTypePromptKit},
			Evals:        &omniav1alpha1.EvalConfig{Enabled: true},
		},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(agentA, agentB, agentC).Build(),
		Scheme: scheme,
	}

	needed, err := r.serviceGroupsNeedingEvalWorker(context.Background(), "ns")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"default", "prod"}, keysOf(needed))
}

func envValue(env []corev1.EnvVar, name string) string {
	for _, e := range env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

func findEnv(env []corev1.EnvVar, name string) (corev1.EnvVar, bool) {
	for _, e := range env {
		if e.Name == name {
			return e, true
		}
	}
	return corev1.EnvVar{}, false
}

func TestEvalWorkerEnv_GroupRedisLiteral(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: "ns"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name:  "default",
					Redis: &omniav1alpha1.RedisConfig{URL: "redis://group.example.com:6379/0"},
				},
			},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: []omniav1alpha1.ServiceGroupStatus{
				{Name: "default", SessionURL: "http://session-ws-default.ns:8080", Ready: true},
			},
		},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ws).Build(),
		Scheme:          scheme,
		RedisURL:        "redis://operator-default:6379/0",
		SessionRedisURL: "redis://operator-session:6379/0",
	}

	env := r.buildEvalWorkerEnvVars(context.Background(), "ns", "default")
	require.Equal(t, "redis://group.example.com:6379/0", envValue(env, "REDIS_URL"))
}

func TestEvalWorkerEnv_GroupRedisExistingSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: "ns"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name: "default",
					Redis: &omniav1alpha1.RedisConfig{
						ExistingSecret: &omniav1alpha1.RedisSecretRef{Name: "grp-redis", Key: "url"},
					},
				},
			},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: []omniav1alpha1.ServiceGroupStatus{
				{Name: "default", SessionURL: "http://session-ws-default.ns:8080", Ready: true},
			},
		},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ws).Build(),
		Scheme: scheme,
	}

	env := r.buildEvalWorkerEnvVars(context.Background(), "ns", "default")
	redisEnv, ok := findEnv(env, "REDIS_URL")
	require.True(t, ok, "REDIS_URL env must be set")
	require.Empty(t, redisEnv.Value, "secret-sourced REDIS_URL must not set Value")
	require.NotNil(t, redisEnv.ValueFrom)
	require.NotNil(t, redisEnv.ValueFrom.SecretKeyRef)
	require.Equal(t, "grp-redis", redisEnv.ValueFrom.SecretKeyRef.Name)
	require.Equal(t, "url", redisEnv.ValueFrom.SecretKeyRef.Key)
}

func TestEvalWorkerEnv_FallbackToSessionRedisDefault(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	// No Workspace objects: findServiceGroup returns false, so the eval-worker
	// must fall back to the operator default. SessionRedisURL takes precedence
	// over the legacy RedisURL.
	r := &AgentRuntimeReconciler{
		Client:          fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:          scheme,
		RedisURL:        "redis://operator-default:6379/0",
		SessionRedisURL: "redis://operator-session:6379/0",
	}

	env := r.buildEvalWorkerEnvVars(context.Background(), "ns", "default")
	require.Equal(t, "redis://operator-session:6379/0", envValue(env, "REDIS_URL"))
}

func TestReconcileEvalWorker_PerGroup_CreatesAndCleansUp(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	agentDefault := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ServiceGroup: "default",
			Framework:    &omniav1alpha1.FrameworkConfig{Type: omniav1alpha1.FrameworkTypeLangChain},
			Evals:        &omniav1alpha1.EvalConfig{Enabled: true},
		},
	}
	staleDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      evalWorkerName("gone"),
			Namespace: "ns",
			Labels: map[string]string{
				labelAppName:      labelValueEvalWorker,
				labelAppManagedBy: labelValueOmniaOperator,
				labelServiceGroup: "gone",
			},
		},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(agentDefault, staleDep).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.reconcileEvalWorker(context.Background(), agentDefault))

	// The needed worker exists.
	got := &appsv1.Deployment{}
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: evalWorkerName("default"), Namespace: "ns"}, got))

	// The stale worker was cleaned up.
	err := r.Get(context.Background(),
		types.NamespacedName{Name: evalWorkerName("gone"), Namespace: "ns"}, &appsv1.Deployment{})
	require.True(t, apierrors.IsNotFound(err))
}
