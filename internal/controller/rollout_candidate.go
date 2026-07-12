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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const candidateSuffix = "-canary"

// candidateDeploymentName returns the Deployment name for the canary candidate.
func candidateDeploymentName(agentName string) string {
	return agentName + candidateSuffix
}

// candidateOverrideResult holds the resolved spec for the candidate.
// Fields that the candidate does not override retain the stable spec values.
type candidateOverrideResult struct {
	PromptPackRef   omniav1alpha1.PromptPackRef
	Providers       []omniav1alpha1.NamedProviderRef
	ToolRegistryRef *omniav1alpha1.ToolRegistryRef
}

// applyCandidateOverrides resolves the effective spec for the candidate
// by layering candidate overrides on top of the stable spec.
func applyCandidateOverrides(ar *omniav1alpha1.AgentRuntime) candidateOverrideResult {
	result := candidateOverrideResult{
		PromptPackRef:   ar.Spec.PromptPackRef,
		Providers:       ar.Spec.Providers,
		ToolRegistryRef: ar.Spec.ToolRegistryRef,
	}

	if ar.Spec.Rollout == nil || ar.Spec.Rollout.Candidate == nil {
		return result
	}
	c := ar.Spec.Rollout.Candidate

	if c.PromptPackRef != nil {
		result.PromptPackRef = *c.PromptPackRef.DeepCopy()
	}
	if len(c.ProviderRefs) > 0 {
		result.Providers = c.ProviderRefs
	}
	if c.ToolRegistryRef != nil {
		result.ToolRegistryRef = c.ToolRegistryRef
	}

	return result
}

// reconcileCandidateDeployment creates or updates the candidate Deployment.
// It builds the same spec as the stable Deployment but applies candidate
// overrides and sets the track label to "canary".
func (r *AgentRuntimeReconciler) reconcileCandidateDeployment(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	providers map[string]*omniav1alpha1.Provider,
) (*appsv1.Deployment, error) {
	log := logf.FromContext(ctx)
	deployName := candidateDeploymentName(ar.Name)

	configHash := r.getConfigHash(ctx, providers, promptPack, toolRegistry)
	resolvedClients, _ := r.resolveA2AClients(ctx, log, ar)

	overrides := applyCandidateOverrides(ar)

	// Resolve the candidate's PromptPack content independently of the stable
	// pack. The candidate's ref can differ from stable's on packName, version,
	// or track — reusing the stable promptPack whenever the packName happened
	// to match would silently skip a version/track change, defeating the
	// rollout, so always resolve fresh via label+version/track.
	candidatePromptPack, err := r.resolvePromptPack(ctx, ar.Namespace, overrides.PromptPackRef)
	if err != nil {
		return nil, fmt.Errorf("resolve candidate PromptPack %q: %w", overrides.PromptPackRef.Name, err)
	}

	// Deliver the candidate's provider refs to its pods via a mounted CM so the
	// runtime resolves the candidate's providers, not the shared stable spec
	// (#1468). Reconciled before the Deployment so the volume reference resolves.
	if err := r.reconcileCanaryOverrideConfigMap(ctx, ar); err != nil {
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: ar.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		if err := controllerutil.SetControllerReference(ar, deployment, r.Scheme); err != nil {
			return err
		}

		// Capture replicas before the builder overwrites them: while a
		// replica-weighted rollout is active, reconcileReplicaWeighting owns the
		// candidate's .spec.replicas (the weighted split), so the builder must
		// not reset it to the canonical total each reconcile.
		liveReplicas := deployment.Spec.Replicas

		// Build a modified copy of the AgentRuntime with candidate overrides
		// so the candidate Deployment runs the overridden config, not stable.
		candidateAR := ar.DeepCopy()
		candidateAR.Spec.PromptPackRef = overrides.PromptPackRef
		if len(overrides.Providers) > 0 {
			candidateAR.Spec.Providers = overrides.Providers
		}
		if overrides.ToolRegistryRef != nil {
			candidateAR.Spec.ToolRegistryRef = overrides.ToolRegistryRef
		}

		// Build the standard deployment spec using candidate overrides, mounting
		// the candidate's resolved PromptPack content.
		r.buildDeploymentSpec(ctx, deployment, candidateAR, candidatePromptPack, toolRegistry, configHash, resolvedClients)

		// Override the track label to "canary" on both selector and pod template
		// so candidate pods are disjoint from stable pods.
		if deployment.Spec.Selector == nil {
			deployment.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{}}
		}
		deployment.Spec.Selector.MatchLabels[labelOmniaTrack] = "canary"
		if deployment.Spec.Template.Labels == nil {
			deployment.Spec.Template.Labels = make(map[string]string)
		}
		deployment.Spec.Template.Labels[labelOmniaTrack] = "canary"

		// Tag candidate-served sessions variant=candidate. The base builder set
		// OMNIA_VARIANT=stable; override it here so variant-gated RolloutAnalysis
		// works in replica-weighted mode, where no routing layer sets the
		// x-omnia-variant header (#1449).
		setCandidateVariantEnv(deployment)

		// Mount the candidate's provider-ref override into the candidate pods.
		mountCanaryOverride(deployment, ar.Name)

		r.preserveWeightedReplicas(ctx, ar, deployment, liveReplicas)
		return nil
	})
	if err != nil {
		return nil, err
	}

	log.V(1).Info("candidate deployment reconciled", "name", deployName, "operation", string(result))
	return deployment, nil
}

// deleteCandidateDeployment removes the candidate Deployment if it exists.
// Returns nil when the Deployment is already gone (NotFound).
func (r *AgentRuntimeReconciler) deleteCandidateDeployment(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) error {
	log := logf.FromContext(ctx)
	deployName := candidateDeploymentName(ar.Name)

	deployment := &appsv1.Deployment{}
	key := types.NamespacedName{Name: deployName, Namespace: ar.Namespace}

	if err := r.Get(ctx, key, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			// Deployment already gone; still ensure the override CM is cleaned up.
			return r.deleteCanaryOverrideConfigMap(ctx, ar)
		}
		return err
	}

	log.V(1).Info("deleting candidate deployment", "name", deployName)
	if err := r.Delete(ctx, deployment); err != nil {
		return err
	}
	return r.deleteCanaryOverrideConfigMap(ctx, ar)
}

// setCandidateVariantEnv overrides the facade container's OMNIA_VARIANT env to
// the candidate variant. The base deployment builder sets it to variantStable;
// the candidate clone flips it to variantCandidate so candidate-served sessions
// are recorded variant=candidate even in replica-weighted mode, where there is
// no routing layer to set the x-omnia-variant header (#1449).
func setCandidateVariantEnv(deployment *appsv1.Deployment) {
	containers := deployment.Spec.Template.Spec.Containers
	for i := range containers {
		if containers[i].Name != FacadeContainerName {
			continue
		}
		for j := range containers[i].Env {
			if containers[i].Env[j].Name == envFacadeVariant {
				containers[i].Env[j].Value = variantCandidate
				return
			}
		}
		// Defensive: builder always sets it, but append if somehow absent.
		containers[i].Env = append(containers[i].Env, corev1.EnvVar{
			Name:  envFacadeVariant,
			Value: variantCandidate,
		})
		return
	}
}
