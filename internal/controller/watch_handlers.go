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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// findAgentRuntimesForProvider returns reconcile requests for all AgentRuntimes
// that reference the given Provider.
//
// When a field index is available (production, via SetupIndexers), the list is
// scoped by index. Otherwise falls back to list-all + local filter (envtest).
func (r *AgentRuntimeReconciler) findAgentRuntimesForProvider(ctx context.Context, obj client.Object) []reconcile.Request {
	provider := obj.(*omniav1alpha1.Provider)
	log := logf.FromContext(ctx).WithValues("provider", provider.Name, "namespace", provider.Namespace)

	key := provider.Namespace + "/" + provider.Name

	// Try indexed list first; fall back to unscoped list if no index is registered.
	var agentRuntimes omniav1alpha1.AgentRuntimeList
	if err := r.List(ctx, &agentRuntimes, client.MatchingFields{IndexAgentRuntimeByProvider: key}); err != nil {
		// MatchingFields fails with a raw client (no index). Fall back to list+filter.
		if err2 := r.List(ctx, &agentRuntimes); err2 != nil {
			log.Error(err2, "failed to list AgentRuntimes for Provider watch")
			return nil
		}
		return r.filterAgentRuntimesByProvider(&agentRuntimes, key, log)
	}

	requests := make([]reconcile.Request, 0, len(agentRuntimes.Items))
	for _, ar := range agentRuntimes.Items {
		log.Info("enqueueing AgentRuntime for Provider change", "agentruntime", ar.Name)
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      ar.Name,
				Namespace: ar.Namespace,
			},
		})
	}
	return requests
}

// filterAgentRuntimesByProvider filters a list of AgentRuntimes to those that
// reference the given provider key ("namespace/name").
func (r *AgentRuntimeReconciler) filterAgentRuntimesByProvider(list *omniav1alpha1.AgentRuntimeList, key string, log logr.Logger) []reconcile.Request {
	var requests []reconcile.Request
	for _, ar := range list.Items {
		refs := extractProviderRefs(&ar)
		for _, ref := range refs {
			if ref == key {
				log.Info("enqueueing AgentRuntime for Provider change", "agentruntime", ar.Name)
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      ar.Name,
						Namespace: ar.Namespace,
					},
				})
				break
			}
		}
	}
	return requests
}

// findAgentRuntimesForPromptPack returns reconcile requests for all AgentRuntimes
// that reference the given PromptPack.
//
// When a field index is available (production, via SetupIndexers), the list is
// scoped by index. Otherwise falls back to list-all + local filter (envtest).
func (r *AgentRuntimeReconciler) findAgentRuntimesForPromptPack(ctx context.Context, obj client.Object) []reconcile.Request {
	promptPack := obj.(*omniav1alpha1.PromptPack)
	log := logf.FromContext(ctx).WithValues("promptpack", promptPack.Name, "namespace", promptPack.Namespace)

	// Try indexed list first; fall back to unscoped list if no index is registered.
	var agentRuntimes omniav1alpha1.AgentRuntimeList
	if err := r.List(ctx, &agentRuntimes,
		client.InNamespace(promptPack.Namespace),
		client.MatchingFields{IndexAgentRuntimeByPromptPack: promptPack.Name},
	); err != nil {
		// MatchingFields fails with a raw client (no index). Fall back to list+filter.
		if err2 := r.List(ctx, &agentRuntimes, client.InNamespace(promptPack.Namespace)); err2 != nil {
			log.Error(err2, "failed to list AgentRuntimes for PromptPack watch")
			return nil
		}
		return r.filterAgentRuntimesByPromptPack(&agentRuntimes, promptPack.Name, log)
	}

	requests := make([]reconcile.Request, 0, len(agentRuntimes.Items))
	for _, ar := range agentRuntimes.Items {
		log.Info("enqueueing AgentRuntime for PromptPack change", "agentruntime", ar.Name)
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      ar.Name,
				Namespace: ar.Namespace,
			},
		})
	}
	return requests
}

// filterAgentRuntimesByPromptPack filters a list of AgentRuntimes to those that
// reference the given PromptPack name.
func (r *AgentRuntimeReconciler) filterAgentRuntimesByPromptPack(list *omniav1alpha1.AgentRuntimeList, name string, log logr.Logger) []reconcile.Request {
	var requests []reconcile.Request
	for _, ar := range list.Items {
		if ar.Spec.PromptPackRef.Name == name {
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

	// Find AgentRuntimes in the same namespace that reference these providers or use the secret directly.
	// Cross-namespace provider references exist but secrets are namespace-scoped, so agents
	// in other namespaces will be reconciled when their own Provider watch fires.
	var agentRuntimes omniav1alpha1.AgentRuntimeList
	if err := r.List(ctx, &agentRuntimes, client.InNamespace(secret.Namespace)); err != nil {
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

		// Check spec.providers list entries
		for _, np := range ar.Spec.Providers {
			refNS := ar.Namespace
			if np.ProviderRef.Namespace != nil {
				refNS = *np.ProviderRef.Namespace
			}
			if refNS == secret.Namespace && providersUsingSecret[np.ProviderRef.Name] {
				log.Info("enqueueing AgentRuntime for Secret change (via providers list)", "agentruntime", ar.Name)
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      ar.Name,
						Namespace: ar.Namespace,
					},
				})
				seen[key] = true
				break
			}
		}
		if seen[key] {
			continue
		}
	}

	return requests
}
