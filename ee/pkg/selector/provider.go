/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package selector

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// ProviderResult contains a matched Provider and its resolved configuration.
type ProviderResult struct {
	// Provider is the matched Provider CRD.
	Provider *corev1alpha1.Provider
	// Group is the group name this provider was selected for.
	Group string
}

// ResolveProviderOverrides resolves provider overrides for an ArenaJob.
// It returns a map of group name -> list of matching Providers.
// Groups without overrides return nil (meaning use config defaults).
func ResolveProviderOverrides(
	ctx context.Context,
	c client.Client,
	namespace string,
	overrides map[string]omniav1alpha1.ProviderGroupSelector,
) (map[string][]*corev1alpha1.Provider, error) {
	if len(overrides) == 0 {
		return nil, nil
	}

	result := make(map[string][]*corev1alpha1.Provider)

	for groupName, selector := range overrides {
		providers, err := SelectProviders(ctx, c, namespace, &selector.Selector)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve providers for group %q: %w", groupName, err)
		}
		result[groupName] = providers
	}

	return result, nil
}

// SelectProviders returns all Provider CRDs in the namespace that match the label selector.
func SelectProviders(
	ctx context.Context,
	c client.Client,
	namespace string,
	selector *metav1.LabelSelector,
) ([]*corev1alpha1.Provider, error) {
	opts, err := ListOptions(selector, namespace)
	if err != nil {
		return nil, fmt.Errorf("invalid provider selector: %w", err)
	}

	providerList := &corev1alpha1.ProviderList{}
	if err := c.List(ctx, providerList, opts...); err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	// Convert to pointer slice for easier manipulation
	providers := make([]*corev1alpha1.Provider, len(providerList.Items))
	for i := range providerList.Items {
		providers[i] = &providerList.Items[i]
	}

	return providers, nil
}

// GetProvidersForGroup returns providers for a specific group, checking:
// 1. Explicit group override
// 2. Wildcard "*" override
// 3. Returns nil if no override (use config defaults)
func GetProvidersForGroup(
	ctx context.Context,
	c client.Client,
	namespace string,
	groupName string,
	overrides map[string]omniav1alpha1.ProviderGroupSelector,
) ([]*corev1alpha1.Provider, bool, error) {
	if len(overrides) == 0 {
		return nil, false, nil
	}

	// Check for explicit group override
	if selector, ok := overrides[groupName]; ok {
		providers, err := SelectProviders(ctx, c, namespace, &selector.Selector)
		if err != nil {
			return nil, false, err
		}
		return providers, true, nil
	}

	// Check for wildcard override
	if selector, ok := overrides["*"]; ok {
		providers, err := SelectProviders(ctx, c, namespace, &selector.Selector)
		if err != nil {
			return nil, false, err
		}
		return providers, true, nil
	}

	// No override for this group
	return nil, false, nil
}
