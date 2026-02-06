/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.

Package providers provides utilities for building environment variables
from Provider CRDs for use in arena worker and dev console pods.
*/

package providers

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// buildEnvVarFromRef creates a corev1.EnvVar from a SecretRef and optional provider secret config.
func buildEnvVarFromRef(ref SecretRef, providerSecretRef *corev1alpha1.SecretKeyRef) corev1.EnvVar {
	if providerSecretRef != nil {
		secretKey := ref.Key
		if providerSecretRef.Key != nil && *providerSecretRef.Key != "" {
			secretKey = *providerSecretRef.Key
		}
		return corev1.EnvVar{
			Name: ref.EnvVar,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: providerSecretRef.Name,
					},
					Key:      secretKey,
					Optional: ptr.To(true),
				},
			},
		}
	}
	// Fall back to default secret naming convention
	return corev1.EnvVar{
		Name: ref.EnvVar,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: ref.SecretName,
				},
				Key:      ref.Key,
				Optional: ptr.To(true),
			},
		},
	}
}

// BuildEnvVarsFromProviders builds environment variables for Provider CRDs.
// This extracts credentials from each provider's secretRef and maps them
// to the appropriate environment variable names for PromptKit.
//
// For example, an OpenAI provider with secretRef pointing to "openai-credentials"
// will create an env var OPENAI_API_KEY sourced from that secret.
func BuildEnvVarsFromProviders(providers []*corev1alpha1.Provider) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}
	seen := make(map[string]bool)

	for _, provider := range providers {
		secretRefs := GetSecretRefsForProvider(string(provider.Spec.Type))
		if len(secretRefs) == 0 {
			continue
		}

		for _, ref := range secretRefs {
			if seen[ref.EnvVar] {
				continue
			}
			seen[ref.EnvVar] = true
			envVars = append(envVars, buildEnvVarFromRef(ref, provider.Spec.SecretRef))
		}
	}

	return envVars
}

// FlattenProviderGroups returns a deduplicated flat list of providers from grouped providers.
// Used when you have providers organized by group but need a flat list for env var building.
func FlattenProviderGroups(providersByGroup map[string][]*corev1alpha1.Provider) []*corev1alpha1.Provider {
	if len(providersByGroup) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var allProviders []*corev1alpha1.Provider

	for _, groupProviders := range providersByGroup {
		for _, p := range groupProviders {
			key := p.Namespace + "/" + p.Name
			if !seen[key] {
				seen[key] = true
				allProviders = append(allProviders, p)
			}
		}
	}

	return allProviders
}
