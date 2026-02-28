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

package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	pkgprovider "github.com/altairalabs/omnia/pkg/provider"
)

// GetAgentRuntime fetches an AgentRuntime CRD by name and namespace.
func GetAgentRuntime(
	ctx context.Context, c client.Client, name, namespace string,
) (*omniav1alpha1.AgentRuntime, error) {
	ar := &omniav1alpha1.AgentRuntime{}
	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, ar); err != nil {
		return nil, fmt.Errorf("get AgentRuntime %s: %w", key, err)
	}
	return ar, nil
}

// GetProvider fetches a Provider CRD by ref, defaulting namespace.
func GetProvider(
	ctx context.Context, c client.Client, ref omniav1alpha1.ProviderRef, defaultNamespace string,
) (*omniav1alpha1.Provider, error) {
	ns := defaultNamespace
	if ref.Namespace != nil {
		ns = *ref.Namespace
	}

	p := &omniav1alpha1.Provider{}
	key := types.NamespacedName{Name: ref.Name, Namespace: ns}
	if err := c.Get(ctx, key, p); err != nil {
		return nil, fmt.Errorf("get Provider %s: %w", key, err)
	}
	return p, nil
}

// GetProviderSecret fetches the Secret referenced by a Provider's SecretRef.
func GetProviderSecret(ctx context.Context, c client.Client, provider *omniav1alpha1.Provider) (*corev1.Secret, error) {
	if provider.Spec.SecretRef == nil {
		return nil, fmt.Errorf("provider %s/%s has no secretRef", provider.Namespace, provider.Name)
	}

	return GetSecret(ctx, c, provider.Spec.SecretRef.Name, provider.Namespace)
}

// GetSecret fetches a Secret by name and namespace.
func GetSecret(ctx context.Context, c client.Client, name, namespace string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("get Secret %s: %w", key, err)
	}
	return secret, nil
}

// EffectiveSecretRef returns the effective SecretKeyRef for a provider,
// preferring credential.secretRef over the legacy secretRef field.
func EffectiveSecretRef(provider *omniav1alpha1.Provider) *omniav1alpha1.SecretKeyRef {
	if provider.Spec.Credential != nil && provider.Spec.Credential.SecretRef != nil {
		return provider.Spec.Credential.SecretRef
	}
	return provider.Spec.SecretRef
}

// DetermineSecretKey returns the key within the Secret to read the API key from.
// If the SecretKeyRef has an explicit key, use it. Otherwise, fall back to the
// provider-appropriate env var name (e.g., ANTHROPIC_API_KEY), then "api-key".
func DetermineSecretKey(ref *omniav1alpha1.SecretKeyRef, providerType omniav1alpha1.ProviderType) string {
	if ref.Key != nil {
		return *ref.Key
	}
	if envName := pkgprovider.APIKeyEnvVarName(string(providerType)); envName != "" {
		return envName
	}
	return "api-key"
}
