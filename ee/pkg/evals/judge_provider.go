/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"errors"
	"fmt"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Error messages used across judge provider resolution.
const (
	errUnknownJudge     = "unknown judge name: %s"
	errProviderLookup   = "failed to look up provider for judge %q: %w"
	errMissingAPIKey    = "provider %q for judge %q has no API key configured"
	errMissingNamespace = "namespace is required when providerRef.namespace is not set"
)

// ProviderLookup resolves a Provider CRD by name and namespace.
type ProviderLookup interface {
	GetProvider(ctx context.Context, name, namespace string) (*ProviderInfo, error)
}

// ProviderInfo holds the resolved fields from a Provider CRD.
type ProviderInfo struct {
	// Type is the provider type (e.g., "openai", "claude", "gemini").
	Type string
	// APIKey is the resolved API key from the Provider's secret reference.
	APIKey string
	// BaseURL is the optional base URL override from the Provider.
	BaseURL string
	// Model is the default model from the Provider, if set.
	Model string
	// ExtraConfig holds additional provider-specific configuration.
	ExtraConfig map[string]string
}

// JudgeConfig holds the resolved LLM configuration for a single judge.
type JudgeConfig struct {
	// ProviderType is the LLM provider type (e.g., "openai", "claude").
	ProviderType string
	// Model is the model identifier (e.g., "gpt-4", "claude-sonnet-4-20250514").
	Model string
	// APIKey is the resolved API key for the provider.
	APIKey string
	// BaseURL is the optional API endpoint override.
	BaseURL string
	// ExtraConfig holds additional provider-specific configuration.
	ExtraConfig map[string]string
}

// judgeEntry stores the unresolved judge mapping for later resolution.
type judgeEntry struct {
	providerRef v1alpha1.ProviderRef
}

// JudgeProvider resolves judge names to LLM provider credentials.
type JudgeProvider struct {
	judges         map[string]judgeEntry
	providerLookup ProviderLookup
	namespace      string
}

// NewJudgeProvider creates a JudgeProvider from named provider references and a provider lookup.
// The namespace is used as the default namespace when providerRef.namespace is not set.
func NewJudgeProvider(
	providers []v1alpha1.NamedProviderRef,
	providerLookup ProviderLookup,
	namespace string,
) *JudgeProvider {
	entries := make(map[string]judgeEntry, len(providers))
	for _, p := range providers {
		entries[p.Name] = judgeEntry{
			providerRef: p.ProviderRef,
		}
	}
	return &JudgeProvider{
		judges:         entries,
		providerLookup: providerLookup,
		namespace:      namespace,
	}
}

// Resolve looks up the provider credentials for the given judge name.
func (jp *JudgeProvider) Resolve(ctx context.Context, judgeName string) (*JudgeConfig, error) {
	entry, ok := jp.judges[judgeName]
	if !ok {
		return nil, fmt.Errorf(errUnknownJudge, judgeName)
	}

	ns := jp.resolveNamespace(entry.providerRef)
	if ns == "" {
		return nil, errors.New(errMissingNamespace)
	}

	info, err := jp.providerLookup.GetProvider(ctx, entry.providerRef.Name, ns)
	if err != nil {
		return nil, fmt.Errorf(errProviderLookup, judgeName, err)
	}

	if info.APIKey == "" {
		return nil, fmt.Errorf(errMissingAPIKey, entry.providerRef.Name, judgeName)
	}

	return buildJudgeConfig(info), nil
}

// resolveNamespace returns the namespace from the provider ref, falling back to the default.
func (jp *JudgeProvider) resolveNamespace(ref v1alpha1.ProviderRef) string {
	if ref.Namespace != nil && *ref.Namespace != "" {
		return *ref.Namespace
	}
	return jp.namespace
}

// buildJudgeConfig constructs a JudgeConfig from the provider info.
func buildJudgeConfig(info *ProviderInfo) *JudgeConfig {
	return &JudgeConfig{
		ProviderType: info.Type,
		Model:        info.Model,
		APIKey:       info.APIKey,
		BaseURL:      info.BaseURL,
		ExtraConfig:  info.ExtraConfig,
	}
}

// JudgeNames returns the names of all configured judges.
func (jp *JudgeProvider) JudgeNames() []string {
	names := make([]string, 0, len(jp.judges))
	for name := range jp.judges {
		names = append(names, name)
	}
	return names
}
