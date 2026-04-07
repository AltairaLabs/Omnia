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

	appsv1 "k8s.io/api/apps/v1"
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

	if c.PromptPackVersion != nil {
		result.PromptPackRef.Version = c.PromptPackVersion
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

	secretHash := r.getSecretHash(ctx, ar, providers)
	resolvedClients, _ := r.resolveA2AClients(ctx, log, ar)

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

		// Build the standard deployment spec (sets track="stable").
		r.buildDeploymentSpec(ctx, deployment, ar, promptPack, toolRegistry, secretHash, resolvedClients)

		// Override the track label to "canary" on pod template.
		if deployment.Spec.Template.Labels == nil {
			deployment.Spec.Template.Labels = make(map[string]string)
		}
		deployment.Spec.Template.Labels[labelOmniaTrack] = "canary"

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
			return nil
		}
		return err
	}

	log.V(1).Info("deleting candidate deployment", "name", deployName)
	return r.Delete(ctx, deployment)
}
