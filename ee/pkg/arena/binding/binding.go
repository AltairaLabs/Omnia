/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package binding provides annotation-based credential resolution for arena providers.
// Arena projects imported from the dashboard contain binding annotations on provider YAML files
// that link them to Provider CRDs. This package resolves those annotations against a registry
// of available providers and injects credentials into the arena config.
package binding

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/altairalabs/omnia/ee/pkg/arena/overrides"
	"gopkg.in/yaml.v3"
)

const (
	// AnnotationProviderName is the annotation key for the Provider CRD name.
	AnnotationProviderName = "omnia.altairalabs.ai/provider-name"
	// AnnotationProviderNamespace is the annotation key for the Provider CRD namespace.
	AnnotationProviderNamespace = "omnia.altairalabs.ai/provider-namespace"
)

// ProviderBinding represents a binding between an arena provider YAML file
// and a Provider CRD, extracted from annotations.
type ProviderBinding struct {
	// ProviderID is the arena provider ID (from spec.id in the YAML file).
	ProviderID string
	// SourceName is the Provider CRD name from the annotation.
	SourceName string
	// SourceNamespace is the Provider CRD namespace from the annotation.
	SourceNamespace string
}

// ParseProviderAnnotations reads provider YAML files referenced by the config
// and extracts binding annotations. Files that are unreadable or unparseable
// are skipped gracefully.
func ParseProviderAnnotations(cfg *config.Config, configPath string) ([]ProviderBinding, error) {
	if len(cfg.Providers) == 0 {
		return nil, nil
	}

	configDir := config.ResolveConfigDir(cfg, configPath)
	bindings := make([]ProviderBinding, 0, len(cfg.Providers))

	for _, ref := range cfg.Providers {
		if ref.File == "" {
			continue
		}

		filePath := filepath.Join(configDir, ref.File)
		data, err := os.ReadFile(filePath)
		if err != nil {
			// Skip unreadable files gracefully
			continue
		}

		var providerCfg config.ProviderConfig
		if err := yaml.Unmarshal(data, &providerCfg); err != nil {
			// Skip unparseable files
			continue
		}

		name := providerCfg.Metadata.Annotations[AnnotationProviderName]
		namespace := providerCfg.Metadata.Annotations[AnnotationProviderNamespace]
		if name == "" || namespace == "" {
			// No binding annotations present
			continue
		}

		bindings = append(bindings, ProviderBinding{
			ProviderID:      providerCfg.Spec.ID,
			SourceName:      name,
			SourceNamespace: namespace,
		})
	}

	return bindings, nil
}

// ApplyBindings resolves binding annotations against the registry and injects
// credentials into providers that don't already have them. Returns the number
// of providers that were successfully bound.
func ApplyBindings(
	cfg *config.Config, bindings []ProviderBinding,
	registry map[string]overrides.ProviderOverride, verbose bool,
) int {
	if len(bindings) == 0 || len(registry) == 0 {
		return 0
	}

	boundCount := 0
	for _, b := range bindings {
		provider := cfg.LoadedProviders[b.ProviderID]
		if provider == nil {
			continue
		}

		if hasCredentials(provider) {
			if verbose {
				fmt.Printf("  Binding skip: %s already has credentials\n", b.ProviderID)
			}
			continue
		}

		key := b.SourceNamespace + "/" + b.SourceName
		override, ok := registry[key]
		if !ok {
			if verbose {
				fmt.Printf("  Binding miss: %s -> %s not found in registry\n", b.ProviderID, key)
			}
			continue
		}

		injectCredentials(provider, &override)
		boundCount++
		if verbose {
			fmt.Printf("  Binding match: %s -> %s\n", b.ProviderID, key)
		}
	}

	return boundCount
}

// ApplyNameMatching tries to match providers that still lack credentials by
// comparing their provider ID against the "{namespace}-{name}" format used by
// the dashboard's import converter. Returns the number of matched providers.
func ApplyNameMatching(cfg *config.Config, registry map[string]overrides.ProviderOverride, verbose bool) int {
	if len(registry) == 0 || len(cfg.LoadedProviders) == 0 {
		return 0
	}

	matchedCount := 0
	for id, provider := range cfg.LoadedProviders {
		if hasCredentials(provider) {
			continue
		}
		if !requiresCredentials(provider.Type) {
			continue
		}

		// Try matching provider ID against "{namespace}-{name}" from registry keys
		for key, override := range registry {
			parts := strings.SplitN(key, "/", 2)
			if len(parts) != 2 {
				continue
			}
			namespace, name := parts[0], parts[1]
			expectedID := namespace + "-" + name
			if id == expectedID {
				injectCredentials(provider, &override)
				matchedCount++
				if verbose {
					fmt.Printf("  Name match: %s -> %s\n", id, key)
				}
				break
			}
		}
	}

	return matchedCount
}

// hasCredentials checks whether a provider already has credentials configured.
func hasCredentials(p *config.Provider) bool {
	if p.Credential == nil {
		return false
	}
	return p.Credential.APIKey != "" ||
		p.Credential.CredentialEnv != "" ||
		p.Credential.CredentialFile != ""
}

// requiresCredentials returns false for provider types that don't need credentials.
func requiresCredentials(providerType string) bool {
	switch providerType {
	case "mock", "ollama":
		return false
	default:
		return true
	}
}

// injectCredentials copies credential and platform config from an override into a provider.
func injectCredentials(provider *config.Provider, override *overrides.ProviderOverride) {
	if override.SecretEnvVar != "" {
		provider.Credential = &config.CredentialConfig{
			CredentialEnv: override.SecretEnvVar,
		}
	} else if override.CredentialFile != "" {
		provider.Credential = &config.CredentialConfig{
			CredentialFile: override.CredentialFile,
		}
	}

	// Inject platform config if the provider lacks it
	if provider.Platform == nil && override.Platform != nil {
		provider.Platform = &config.PlatformConfig{
			Type:     override.Platform.Type,
			Region:   override.Platform.Region,
			Project:  override.Platform.Project,
			Endpoint: override.Platform.Endpoint,
		}
	}
}
