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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Field index paths used by watch handlers to scope list operations.
const (
	// IndexAgentRuntimeByProvider indexes AgentRuntimes by the provider names they reference.
	// Values are "namespace/name" of referenced Provider resources.
	IndexAgentRuntimeByProvider = ".spec.providerRefs"

	// IndexAgentRuntimeByPromptPack indexes AgentRuntimes by the PromptPack name they reference.
	IndexAgentRuntimeByPromptPack = ".spec.promptPackRef"
)

// SetupIndexers registers field indexers required by watch handlers.
// Must be called before controllers start.
func SetupIndexers(ctx context.Context, mgr manager.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&omniav1alpha1.AgentRuntime{},
		IndexAgentRuntimeByProvider,
		extractProviderRefs,
	); err != nil {
		return err
	}

	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&omniav1alpha1.AgentRuntime{},
		IndexAgentRuntimeByPromptPack,
		extractPromptPackRef,
	)
}

// extractProviderRefs returns the "namespace/name" keys for all Provider references
// on an AgentRuntime from spec.providers[].providerRef.
func extractProviderRefs(obj client.Object) []string {
	ar := obj.(*omniav1alpha1.AgentRuntime)
	var refs []string
	seen := make(map[string]bool)

	for _, np := range ar.Spec.Providers {
		key := providerRefKey(np.ProviderRef, ar.Namespace)
		if !seen[key] {
			refs = append(refs, key)
			seen[key] = true
		}
	}

	return refs
}

// providerRefKey builds a "namespace/name" key from a ProviderRef.
func providerRefKey(ref omniav1alpha1.ProviderRef, defaultNS string) string {
	ns := defaultNS
	if ref.Namespace != nil {
		ns = *ref.Namespace
	}
	return ns + "/" + ref.Name
}

// extractPromptPackRef returns the PromptPack name referenced by an AgentRuntime.
func extractPromptPackRef(obj client.Object) []string {
	ar := obj.(*omniav1alpha1.AgentRuntime)
	if ar.Spec.PromptPackRef.Name == "" {
		return nil
	}
	return []string{ar.Spec.PromptPackRef.Name}
}
