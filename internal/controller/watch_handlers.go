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
