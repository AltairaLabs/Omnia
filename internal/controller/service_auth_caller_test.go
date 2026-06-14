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

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newCallerReconciler builds an AgentRuntimeReconciler wired with a fake client
// and the given service-auth config, for exercising caller-token injection.
func newCallerReconciler(auth ServiceAuthConfig) *AgentRuntimeReconciler {
	scheme := authTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	return &AgentRuntimeReconciler{
		Client:          cl,
		Scheme:          scheme,
		EvalWorkerImage: "ghcr.io/altairalabs/omnia-eval-worker:test",
		ServiceAuth:     auth,
	}
}

func TestEvalWorkerDeployment_CallerToken_Disabled(t *testing.T) {
	r := newCallerReconciler(ServiceAuthConfig{Enabled: false})
	dep := r.buildEvalWorkerDeployment(context.Background(), "acme-ns", "default", nil)

	assert.False(t, hasVolume(dep.Spec.Template.Spec.Volumes, sessionAuthTokenVolumeName))
	for _, c := range dep.Spec.Template.Spec.Containers {
		assert.NotContains(t, envMap(c.Env), envSessionAPITokenPath)
	}
}

func TestEvalWorkerDeployment_CallerToken_Enabled(t *testing.T) {
	r := newCallerReconciler(ServiceAuthConfig{
		Enabled:                true,
		Audience:               testAudience,
		TokenExpirationSeconds: 3600,
	})
	dep := r.buildEvalWorkerDeployment(context.Background(), "acme-ns", "default", nil)
	spec := dep.Spec.Template.Spec

	require.True(t, hasVolume(spec.Volumes, sessionAuthTokenVolumeName))
	for _, c := range spec.Containers {
		assert.True(t, hasMount(c.VolumeMounts, sessionAuthTokenVolumeName), c.Name)
		assert.Equal(t, sessionAuthTokenPath(), envMap(c.Env)[envSessionAPITokenPath], c.Name)
	}
}

// TestApplyCallerToken_FacadePodShape mirrors the facade pod-spec mutation in
// buildDeploymentSpec: the projected token volume is added once and every
// container receives the mount + path env. This covers the same helper the
// facade deployment uses without standing up the full envtest harness.
func TestApplyCallerToken_FacadePodShape(t *testing.T) {
	auth := ServiceAuthConfig{Enabled: true, Audience: testAudience, TokenExpirationSeconds: 3600}
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: FacadeContainerName}, {Name: "runtime"}},
	}
	auth.applyCallerToken(spec)

	require.Len(t, spec.Volumes, 1)
	assert.Equal(t, sessionAuthTokenVolumeName, spec.Volumes[0].Name)
	for _, c := range spec.Containers {
		assert.True(t, hasMount(c.VolumeMounts, sessionAuthTokenVolumeName), c.Name)
		assert.Equal(t, sessionAuthTokenPath(), envMap(c.Env)[envSessionAPITokenPath], c.Name)
	}
}
