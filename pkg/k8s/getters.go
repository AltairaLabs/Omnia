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

	secret := &corev1.Secret{}
	key := types.NamespacedName{
		Name:      provider.Spec.SecretRef.Name,
		Namespace: provider.Namespace,
	}
	if err := c.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("get Secret %s: %w", key, err)
	}
	return secret, nil
}
