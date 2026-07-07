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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// getConfigHash calculates a hash of the external config the agent pod depends
// on but that does NOT live in the pod spec: provider config + secrets, and the
// PromptPack / ToolRegistry the runtime loads from mounted ConfigMaps. It is set
// as the pod-template config-hash annotation so the Deployment rolls when any of
// them changes. Without the pack/registry inputs, editing a ToolRegistry or
// PromptPack would leave the pod running stale tool/prompt config — the change
// silently no-ops until something else restarts the pod.
func (r *AgentRuntimeReconciler) getConfigHash(
	ctx context.Context,
	providers map[string]*omniav1alpha1.Provider,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
) string {
	hasher := sha256.New()

	// PromptPack / ToolRegistry: Generation increments on every spec change, so
	// a hash over it rolls the pod when the pack or the tool set changes. The
	// operator generates the tools ConfigMap from the ToolRegistry spec, so the
	// registry Generation transitively covers the runtime's tools.yaml too.
	if promptPack != nil {
		hashField(hasher, "promptPack.name", promptPack.Name)
		hashField(hasher, "promptPack.generation", fmt.Sprintf("%d", promptPack.Generation))
	}
	if toolRegistry != nil {
		hashField(hasher, "toolRegistry.name", toolRegistry.Name)
		hashField(hasher, "toolRegistry.generation", fmt.Sprintf("%d", toolRegistry.Generation))
	}

	if len(providers) == 0 {
		if promptPack == nil && toolRegistry == nil {
			return ""
		}
		return finishHash(hasher)
	}

	// Include all providers in sorted key order for determinism
	providerNames := make([]string, 0, len(providers))
	for name := range providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)

	for _, name := range providerNames {
		provider := providers[name]
		// Hash provider identity and spec fields
		hashField(hasher, "name", name)
		hashField(hasher, "type", string(provider.Spec.Type))
		hashField(hasher, "model", provider.Spec.Model)
		hashField(hasher, "baseURL", provider.Spec.BaseURL)

		// Hash defaults
		hashProviderDefaults(hasher, provider.Spec.Defaults)

		// Hash pricing
		hashProviderPricing(hasher, provider.Spec.Pricing)

		// Hash secret data
		if ref := effectiveSecretRef(provider); ref != nil {
			r.hashSecretData(ctx, hasher, ref.Name, provider.Namespace)
		}
	}

	return finishHash(hasher)
}

// finishHash renders the hasher's digest as the first 16 hex chars.
func finishHash(hasher hash.Hash) string {
	hashStr := hex.EncodeToString(hasher.Sum(nil))
	if len(hashStr) > 16 {
		hashStr = hashStr[:16]
	}
	return hashStr
}

// hashField writes a key-value pair to the hasher with null-byte delimiters.
func hashField(hasher hash.Hash, key, value string) {
	hasher.Write([]byte(key))
	hasher.Write([]byte{0})
	hasher.Write([]byte(value))
	hasher.Write([]byte{0})
}

// hashProviderDefaults writes provider defaults fields to the hasher.
func hashProviderDefaults(hasher hash.Hash, defaults *omniav1alpha1.ProviderDefaults) {
	if defaults == nil {
		return
	}
	if defaults.Temperature != nil {
		hashField(hasher, "defaults.temperature", *defaults.Temperature)
	}
	if defaults.TopP != nil {
		hashField(hasher, "defaults.topP", *defaults.TopP)
	}
	if defaults.MaxTokens != nil {
		hashField(hasher, "defaults.maxTokens", fmt.Sprintf("%d", *defaults.MaxTokens))
	}
	if defaults.ContextWindow != nil {
		hashField(hasher, "defaults.contextWindow", fmt.Sprintf("%d", *defaults.ContextWindow))
	}
}

// hashProviderPricing writes provider pricing fields to the hasher.
func hashProviderPricing(hasher hash.Hash, pricing *omniav1alpha1.ProviderPricing) {
	if pricing == nil {
		return
	}
	if pricing.InputCostPer1K != nil {
		hashField(hasher, "pricing.inputCostPer1K", *pricing.InputCostPer1K)
	}
	if pricing.OutputCostPer1K != nil {
		hashField(hasher, "pricing.outputCostPer1K", *pricing.OutputCostPer1K)
	}
	if pricing.CachedCostPer1K != nil {
		hashField(hasher, "pricing.cachedCostPer1K", *pricing.CachedCostPer1K)
	}
}

// hashSecretData reads a secret and writes its data to the hasher in deterministic order.
func (r *AgentRuntimeReconciler) hashSecretData(ctx context.Context, hasher hash.Hash, secretName, namespace string) {
	log := logf.FromContext(ctx)
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Name: secretName, Namespace: namespace}
	if err := r.Get(ctx, secretKey, secret); err == nil {
		keys := make([]string, 0, len(secret.Data))
		for k := range secret.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			hasher.Write([]byte(k))
			hasher.Write(secret.Data[k])
		}
		log.V(1).Info("Included secret in hash", "secret", secretKey.String())
	} else {
		log.V(1).Info("Could not get secret for hash", "secret", secretKey.String(), "error", err)
	}
}
