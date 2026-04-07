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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestCandidateDeploymentName(t *testing.T) {
	assert.Equal(t, "customer-support-canary", candidateDeploymentName("customer-support"))
	assert.Equal(t, "my-agent-canary", candidateDeploymentName("my-agent"))
}

func TestApplyCandidateOverrides_PromptPackVersion(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}

	result := applyCandidateOverrides(ar)
	assert.Equal(t, "v2", *result.PromptPackRef.Version)
	assert.Equal(t, "support-pack", result.PromptPackRef.Name)
	// Providers unchanged.
	assert.Equal(t, "claude-provider", result.Providers[0].ProviderRef.Name)
}

func TestApplyCandidateOverrides_ProviderRefs(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			ProviderRefs: []omniav1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: omniav1alpha1.ProviderRef{Name: "openai-provider"},
				},
			},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}

	result := applyCandidateOverrides(ar)
	assert.Equal(t, "openai-provider", result.Providers[0].ProviderRef.Name)
	// PromptPack version unchanged.
	assert.Equal(t, "v1", *result.PromptPackRef.Version)
}

func TestApplyCandidateOverrides_ToolRegistryRef(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "tools-v2"},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}

	result := applyCandidateOverrides(ar)
	assert.Equal(t, "tools-v2", result.ToolRegistryRef.Name)
}

func TestApplyCandidateOverrides_NoOverrides(t *testing.T) {
	ar := newRolloutTestAR()
	// No rollout — returns original spec values.
	result := applyCandidateOverrides(ar)
	assert.Equal(t, "v1", *result.PromptPackRef.Version)
	assert.Equal(t, "claude-provider", result.Providers[0].ProviderRef.Name)
	assert.Equal(t, "tools-v1", result.ToolRegistryRef.Name)
}

func TestApplyCandidateOverrides_EmptyCandidate(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{},
		Steps:     []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}

	result := applyCandidateOverrides(ar)
	assert.Equal(t, "v1", *result.PromptPackRef.Version)
	assert.Equal(t, "claude-provider", result.Providers[0].ProviderRef.Name)
	assert.Equal(t, "tools-v1", result.ToolRegistryRef.Name)
}

func TestReconcileCandidateDeployment_CreatesWithCanaryLabel(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeWebSocket
	ar.Spec.PromptPackRef.Name = "test-pack"
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}

	pp := newTestPromptPack()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	deploy, err := r.reconcileCandidateDeployment(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, deploy)

	assert.Equal(t, "customer-support-canary", deploy.Name)
	assert.Equal(t, "canary", deploy.Spec.Template.Labels[labelOmniaTrack])
}

func TestDeleteCandidateDeployment_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))

	ar := newRolloutTestAR()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Should not error when deployment doesn't exist.
	err := r.deleteCandidateDeployment(context.Background(), ar)
	assert.NoError(t, err)
}

func TestDeleteCandidateDeployment_DeletesExisting(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))

	ar := newRolloutTestAR()

	existing := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      candidateDeploymentName(ar.Name),
			Namespace: ar.Namespace,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	err := r.deleteCandidateDeployment(context.Background(), ar)
	require.NoError(t, err)

	// Verify it's gone.
	got := &appsv1.Deployment{}
	key := types.NamespacedName{Name: candidateDeploymentName(ar.Name), Namespace: ar.Namespace}
	err = fakeClient.Get(context.Background(), key, got)
	assert.True(t, err != nil, "expected NotFound after delete")
}
