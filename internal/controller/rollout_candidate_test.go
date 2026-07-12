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

func TestApplyCandidateOverrides_PromptPackRef(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackRef: &omniav1alpha1.PromptPackRef{Name: testStablePackName, Version: ptr.To("v2")},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}

	result := applyCandidateOverrides(ar)
	assert.Equal(t, "v2", *result.PromptPackRef.Version)
	assert.Equal(t, testStablePackName, result.PromptPackRef.Name)
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
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}}
	ar.Spec.PromptPackRef.Name = "test-pack"
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackRef: &omniav1alpha1.PromptPackRef{Name: "test-pack", Version: ptr.To("v2")},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}

	pp := newTestPromptPack()

	// The candidate resolves its own PromptPack independently (by label +
	// version/track), even though its packName here happens to match stable's
	// — so a real, labeled candidate version must exist in the fake client.
	candidatePP := newTestPromptPack()
	candidatePP.Namespace = ar.Namespace
	candidatePP.Spec.PackName = "test-pack"
	candidatePP.Spec.Version = "v2"
	candidatePP.Labels = map[string]string{LabelPromptPackName: "test-pack"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(candidatePP).
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

// TestReconcileCandidateDeployment_MountsCandidatePromptPack is the regression
// for the bug where a candidate overriding PromptPackRef to a DIFFERENT pack
// still mounted the stable pack's ConfigMap — silently running stable prompts
// under the candidate label, so the rollout could never test a new prompt. The
// candidate Deployment must resolve and mount its OWN pack's ConfigMap.
func TestReconcileCandidateDeployment_MountsCandidatePromptPack(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}}
	ar.Spec.PromptPackRef.Name = "stable-pack"
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackRef: &omniav1alpha1.PromptPackRef{Name: "candidate-pack", Version: ptr.To("1.0.0")},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}

	stablePack := &omniav1alpha1.PromptPack{}
	stablePack.Name = "stable-pack"
	stablePack.Namespace = ar.Namespace
	stablePack.Spec.Source.Type = omniav1alpha1.PromptPackSourceTypeConfigMap
	stablePack.Spec.Source.ConfigMapRef = &corev1.LocalObjectReference{Name: "stable-pack-config"}

	candidatePack := &omniav1alpha1.PromptPack{}
	candidatePack.Name = "candidate-pack"
	candidatePack.Namespace = ar.Namespace
	candidatePack.Spec.PackName = "candidate-pack"
	candidatePack.Spec.Version = "1.0.0"
	candidatePack.Labels = map[string]string{LabelPromptPackName: "candidate-pack"}
	candidatePack.Spec.Source.Type = omniav1alpha1.PromptPackSourceTypeConfigMap
	candidatePack.Spec.Source.ConfigMapRef = &corev1.LocalObjectReference{Name: "candidate-pack-config"}

	// The operator resolves the candidate's pack by label + version/track from the cluster.
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(candidatePack).
		Build()

	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	// Pass the stable pack as the reconciler would; the candidate must NOT use it.
	deploy, err := r.reconcileCandidateDeployment(context.Background(), ar, stablePack, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, deploy)

	var mounted string
	for _, v := range deploy.Spec.Template.Spec.Volumes {
		if v.Name == promptpackConfigVolumeName && v.ConfigMap != nil {
			mounted = v.ConfigMap.Name
		}
	}
	assert.Equal(t, "candidate-pack-config", mounted,
		"candidate must mount its own pack's ConfigMap, not the stable pack's")
}

// TestReconcileCandidateDeployment_ConfigHashUsesCandidatePack is the
// regression for T5: the candidate Deployment's config-hash annotation must
// be computed from the CANDIDATE's own resolved PromptPack (different
// Generation here), not the stable pack passed in as the (intentionally
// unread) stable-pack argument. Before the fix, hashing the stable pack meant
// a candidate-only PromptPack change never rolled the candidate pods.
func TestReconcileCandidateDeployment_ConfigHashUsesCandidatePack(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}}
	ar.Spec.PromptPackRef.Name = "stable-pack"
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackRef: &omniav1alpha1.PromptPackRef{Name: "candidate-pack", Version: ptr.To("1.0.0")},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}

	stablePack := &omniav1alpha1.PromptPack{}
	stablePack.Name = "stable-pack"
	stablePack.Namespace = ar.Namespace
	stablePack.Generation = 1
	stablePack.Spec.Source.Type = omniav1alpha1.PromptPackSourceTypeConfigMap
	stablePack.Spec.Source.ConfigMapRef = &corev1.LocalObjectReference{Name: "stable-pack-config"}

	candidatePack := &omniav1alpha1.PromptPack{}
	candidatePack.Name = "candidate-pack"
	candidatePack.Namespace = ar.Namespace
	candidatePack.Generation = 7 // deliberately different from stablePack's, so the hashes diverge
	candidatePack.Spec.PackName = "candidate-pack"
	candidatePack.Spec.Version = "1.0.0"
	candidatePack.Labels = map[string]string{LabelPromptPackName: "candidate-pack"}
	candidatePack.Spec.Source.Type = omniav1alpha1.PromptPackSourceTypeConfigMap
	candidatePack.Spec.Source.ConfigMapRef = &corev1.LocalObjectReference{Name: "candidate-pack-config"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(candidatePack).
		Build()

	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	// Pass the stable pack as the reconciler would; the candidate's config
	// hash must NOT be derived from it.
	deploy, err := r.reconcileCandidateDeployment(context.Background(), ar, stablePack, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, deploy)

	candidateHash := deploy.Spec.Template.Annotations[annotationConfigHash]
	require.NotEmpty(t, candidateHash, "candidate deployment must carry a config-hash annotation")

	stableHash := r.getConfigHash(context.Background(), nil, stablePack, nil)
	assert.NotEqual(t, stableHash, candidateHash,
		"candidate config-hash must be computed from the candidate's own resolved pack, not the stable pack")
}

func TestDeleteCandidateDeployment_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme)) // candidate teardown also deletes the override CM

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
	require.NoError(t, corev1.AddToScheme(scheme)) // candidate teardown also deletes the override CM

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
