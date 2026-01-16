/*
Copyright 2025.

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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// findAgentRuntimesForProvider returns reconcile requests for all AgentRuntimes
// that reference the given Provider.
func (r *AgentRuntimeReconciler) findAgentRuntimesForProvider(ctx context.Context, obj client.Object) []reconcile.Request {
	provider := obj.(*omniav1alpha1.Provider)
	log := logf.FromContext(ctx).WithValues("provider", provider.Name, "namespace", provider.Namespace)

	// List all AgentRuntimes
	var agentRuntimes omniav1alpha1.AgentRuntimeList
	if err := r.List(ctx, &agentRuntimes); err != nil {
		log.Error(err, "failed to list AgentRuntimes for Provider watch")
		return nil
	}

	var requests []reconcile.Request
	for _, ar := range agentRuntimes.Items {
		// Check if this AgentRuntime references the changed Provider
		if ar.Spec.ProviderRef != nil && ar.Spec.ProviderRef.Name == provider.Name {
			// Check namespace match (same namespace or explicit namespace reference)
			providerNS := provider.Namespace
			arProviderNS := ar.Namespace
			if ar.Spec.ProviderRef.Namespace != nil {
				arProviderNS = *ar.Spec.ProviderRef.Namespace
			}
			if providerNS == arProviderNS {
				log.Info("enqueueing AgentRuntime for Provider change", "agentruntime", ar.Name)
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      ar.Name,
						Namespace: ar.Namespace,
					},
				})
			}
		}
	}
	return requests
}

// findAgentRuntimesForPromptPack returns reconcile requests for all AgentRuntimes
// that reference the given PromptPack.
func (r *AgentRuntimeReconciler) findAgentRuntimesForPromptPack(ctx context.Context, obj client.Object) []reconcile.Request {
	promptPack := obj.(*omniav1alpha1.PromptPack)
	log := logf.FromContext(ctx).WithValues("promptpack", promptPack.Name, "namespace", promptPack.Namespace)

	// List all AgentRuntimes in the same namespace (PromptPack refs don't have namespace field)
	var agentRuntimes omniav1alpha1.AgentRuntimeList
	if err := r.List(ctx, &agentRuntimes, client.InNamespace(promptPack.Namespace)); err != nil {
		log.Error(err, "failed to list AgentRuntimes for PromptPack watch")
		return nil
	}

	var requests []reconcile.Request
	for _, ar := range agentRuntimes.Items {
		// Check if this AgentRuntime references the changed PromptPack
		if ar.Spec.PromptPackRef.Name == promptPack.Name {
			log.Info("enqueueing AgentRuntime for PromptPack change", "agentruntime", ar.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ar.Name,
					Namespace: ar.Namespace,
				},
			})
		}
	}
	return requests
}

// findAgentRuntimesForSecret returns reconcile requests for all AgentRuntimes
// that use the given Secret (via Provider or inline provider config).
// This enables rollouts when credential secrets are updated.
func (r *AgentRuntimeReconciler) findAgentRuntimesForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret := obj.(*corev1.Secret)
	log := logf.FromContext(ctx).WithValues("secret", secret.Name, "namespace", secret.Namespace)

	// Only watch credential secrets (those with our label)
	if secret.Labels["omnia.altairalabs.ai/type"] != "credentials" {
		return nil
	}

	// First, find all Providers that reference this secret
	var providers omniav1alpha1.ProviderList
	if err := r.List(ctx, &providers, client.InNamespace(secret.Namespace)); err != nil {
		log.Error(err, "failed to list Providers for Secret watch")
		return nil
	}

	// Build set of provider names that use this secret
	providersUsingSecret := make(map[string]bool)
	for _, p := range providers.Items {
		if p.Spec.SecretRef != nil && p.Spec.SecretRef.Name == secret.Name {
			providersUsingSecret[p.Name] = true
		}
	}

	// Now find all AgentRuntimes that reference these providers or use the secret directly
	var agentRuntimes omniav1alpha1.AgentRuntimeList
	if err := r.List(ctx, &agentRuntimes); err != nil {
		log.Error(err, "failed to list AgentRuntimes for Secret watch")
		return nil
	}

	var requests []reconcile.Request
	seen := make(map[string]bool) // Avoid duplicates

	for _, ar := range agentRuntimes.Items {
		key := ar.Namespace + "/" + ar.Name
		if seen[key] {
			continue
		}

		// Check if AgentRuntime uses a Provider that references this secret
		if ar.Spec.ProviderRef != nil {
			providerNS := ar.Namespace
			if ar.Spec.ProviderRef.Namespace != nil {
				providerNS = *ar.Spec.ProviderRef.Namespace
			}
			// Only check if provider is in the same namespace as the secret
			if providerNS == secret.Namespace && providersUsingSecret[ar.Spec.ProviderRef.Name] {
				log.Info("enqueueing AgentRuntime for Secret change (via Provider)", "agentruntime", ar.Name)
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      ar.Name,
						Namespace: ar.Namespace,
					},
				})
				seen[key] = true
				continue
			}
		}

		// Check if AgentRuntime uses inline provider with this secret (legacy)
		if ar.Spec.Provider != nil && ar.Spec.Provider.SecretRef != nil &&
			ar.Spec.Provider.SecretRef.Name == secret.Name && ar.Namespace == secret.Namespace {
			log.Info("enqueueing AgentRuntime for Secret change (inline provider)", "agentruntime", ar.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ar.Name,
					Namespace: ar.Namespace,
				},
			})
			seen[key] = true
		}
	}

	return requests
}
