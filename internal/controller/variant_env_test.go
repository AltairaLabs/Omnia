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
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const variantTestAgent = "test-agent"

// The stable Deployment's facade container must carry OMNIA_VARIANT=stable so
// the facade records variant=stable when no x-omnia-variant header is set
// (replica-weighted mode) (#1449).
func TestFacadeEnvVars_SetsStableVariant(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = variantTestAgent

	_, ok := findEnv(r.buildFacadeEnvVars(ar), envFacadeVariant)
	assert.True(t, ok, "buildFacadeEnvVars must set %s", envFacadeVariant)
	assert.Equal(t, variantStable, envValue(r.buildFacadeEnvVars(ar), envFacadeVariant))
}

func TestA2AEnvVars_SetsStableVariant(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = variantTestAgent
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeA2A}}

	_, ok := findEnv(r.buildA2AEnvVars(ar, nil, nil), envFacadeVariant)
	assert.True(t, ok, "buildA2AEnvVars must set %s", envFacadeVariant)
	assert.Equal(t, variantStable, envValue(r.buildA2AEnvVars(ar, nil, nil), envFacadeVariant))
}

// setCandidateVariantEnv must flip the facade container's variant to candidate
// (and leave non-facade containers untouched).
func TestSetCandidateVariantEnv(t *testing.T) {
	dep := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							// Non-facade container first, so setCandidateVariantEnv
							// must skip it before reaching the facade container.
							Name: "runtime",
							Env:  []corev1.EnvVar{{Name: envFacadeVariant, Value: variantStable}},
						},
						{
							Name: FacadeContainerName,
							Env:  []corev1.EnvVar{{Name: envFacadeVariant, Value: variantStable}},
						},
					},
				},
			},
		},
	}

	setCandidateVariantEnv(dep)

	containers := dep.Spec.Template.Spec.Containers
	assert.Equal(t, variantCandidate, envValue(containers[1].Env, envFacadeVariant),
		"facade container variant must be flipped to candidate")

	// Non-facade container must be left untouched.
	assert.Equal(t, variantStable, envValue(containers[0].Env, envFacadeVariant),
		"non-facade container must not be modified")
}

func TestSetCandidateVariantEnv_AppendsWhenAbsent(t *testing.T) {
	dep := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: FacadeContainerName, Env: nil},
					},
				},
			},
		},
	}

	setCandidateVariantEnv(dep)

	_, ok := findEnv(dep.Spec.Template.Spec.Containers[0].Env, envFacadeVariant)
	assert.True(t, ok, "variant env must be appended when absent")
	assert.Equal(t, variantCandidate, envValue(dep.Spec.Template.Spec.Containers[0].Env, envFacadeVariant))
}
