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

	// IndexAgentRuntimeByPromptPack indexes AgentRuntimes by the logical
	// PromptPack packName they reference (ar.Spec.PromptPackRef.Name).
	// Since #1837, a PromptPack's metadata.name is a deterministic pp-<hash>
	// distinct from its spec.packName, so the reverse lookup (watch_handlers.go)
	// MUST query this index with promptPack.Spec.PackName, never promptPack.Name.
	IndexAgentRuntimeByPromptPack = ".spec.promptPackRef"

	// IndexAgentRuntimeByToolRegistry indexes AgentRuntimes by the ToolRegistry they reference.
	// Values are "namespace/name" of the referenced ToolRegistry (refs may be cross-namespace).
	IndexAgentRuntimeByToolRegistry = ".spec.toolRegistryRef"
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

	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&omniav1alpha1.AgentRuntime{},
		IndexAgentRuntimeByPromptPack,
		extractPromptPackRef,
	); err != nil {
		return err
	}

	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&omniav1alpha1.AgentRuntime{},
		IndexAgentRuntimeByToolRegistry,
		extractToolRegistryRef,
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

// extractPromptPackRef returns the logical PromptPack packName referenced by
// an AgentRuntime (ar.Spec.PromptPackRef.Name). This is already the logical
// name, not a PromptPack object's metadata.name — no change needed here for
// #1837, the reverse query side (watch_handlers.go) is what must key on
// promptPack.Spec.PackName instead of promptPack.Name.
func extractPromptPackRef(obj client.Object) []string {
	ar := obj.(*omniav1alpha1.AgentRuntime)
	if ar.Spec.PromptPackRef.Name == "" {
		return nil
	}
	return []string{ar.Spec.PromptPackRef.Name}
}

// extractToolRegistryRef returns the "namespace/name" key for the ToolRegistry
// referenced by an AgentRuntime, or nil if it references none.
func extractToolRegistryRef(obj client.Object) []string {
	ar := obj.(*omniav1alpha1.AgentRuntime)
	if ar.Spec.ToolRegistryRef == nil || ar.Spec.ToolRegistryRef.Name == "" {
		return nil
	}
	return []string{toolRegistryRefKey(ar.Spec.ToolRegistryRef, ar.Namespace)}
}

// toolRegistryRefKey builds a "namespace/name" key from a ToolRegistryRef,
// defaulting to the AgentRuntime's namespace when the ref omits one.
func toolRegistryRefKey(ref *omniav1alpha1.ToolRegistryRef, defaultNS string) string {
	ns := defaultNS
	if ref.Namespace != nil {
		ns = *ref.Namespace
	}
	return ns + "/" + ref.Name
}
