/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package providers provides utilities for discovering and validating
// LLM provider credentials in Kubernetes environments.
package providers

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProviderCredentials maps provider types to their required environment variables.
// Keys are PromptKit provider type names (the wire protocol). Platform-hosted
// providers (claude on bedrock, gemini on vertex, openai on azure) use platform
// auth via spec.platform/spec.auth, not these env vars.
var ProviderCredentials = map[string][]string{
	"claude":   {"ANTHROPIC_API_KEY"},
	"openai":   {"OPENAI_API_KEY"},
	"gemini":   {"GOOGLE_API_KEY", "GEMINI_API_KEY"},
	"vllm":     {}, // Auth via custom headers; no standard env var
	"voyageai": {"VOYAGE_API_KEY"},
	"groq":     {"GROQ_API_KEY"},
	"together": {"TOGETHER_API_KEY"},
	"ollama":   {}, // No API key required for local Ollama
	"mock":     {}, // Mock provider doesn't need credentials
}

// SecretRef contains information about where to find credentials.
type SecretRef struct {
	EnvVar     string
	SecretName string
	Key        string
}

// Discovery provides methods for discovering available LLM providers
// based on credential availability.
type Discovery struct {
	client    client.Client
	namespace string
}

// NewDiscovery creates a new Discovery service.
func NewDiscovery(c client.Client, namespace string) *Discovery {
	return &Discovery{
		client:    c,
		namespace: namespace,
	}
}

// DiscoverAvailableProviders checks which providers from the config have
// available credentials in the namespace and returns their IDs.
func (d *Discovery) DiscoverAvailableProviders(ctx context.Context, cfg *arenaconfig.Config) ([]string, error) {
	available := []string{}

	for providerID, provider := range cfg.LoadedProviders {
		envVars := GetAPIKeyEnvVars(provider.Type)

		// Skip providers that don't require credentials
		if len(envVars) == 0 {
			available = append(available, providerID)
			continue
		}

		// Check if any of the required env vars have corresponding secrets
		for _, envVar := range envVars {
			secretName := EnvVarToSecretName(envVar)
			if d.secretExists(ctx, secretName) {
				available = append(available, providerID)
				break
			}
		}
	}

	return available, nil
}

// GetMissingCredentials returns a list of providers that are missing credentials.
func (d *Discovery) GetMissingCredentials(ctx context.Context, cfg *arenaconfig.Config) (map[string][]string, error) {
	missing := make(map[string][]string)

	for providerID, provider := range cfg.LoadedProviders {
		envVars := GetAPIKeyEnvVars(provider.Type)
		if len(envVars) == 0 {
			continue
		}

		hasCredential := false
		for _, envVar := range envVars {
			secretName := EnvVarToSecretName(envVar)
			if d.secretExists(ctx, secretName) {
				hasCredential = true
				break
			}
		}

		if !hasCredential {
			missing[providerID] = envVars
		}
	}

	return missing, nil
}

// secretExists checks if a secret with the given name exists in the namespace.
func (d *Discovery) secretExists(ctx context.Context, secretName string) bool {
	if d.client == nil {
		return false
	}

	secret := &corev1.Secret{}
	err := d.client.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: d.namespace,
	}, secret)

	return err == nil
}

// GetAPIKeyEnvVars returns the environment variable names for a provider type.
// Returns nil for providers that don't require credentials.
func GetAPIKeyEnvVars(providerType string) []string {
	providerType = strings.ToLower(providerType)
	if envVars, ok := ProviderCredentials[providerType]; ok {
		return envVars
	}
	// Unknown provider type - assume it needs a standard API key
	return []string{strings.ToUpper(providerType) + "_API_KEY"}
}

// EnvVarToSecretName converts an environment variable name to a Kubernetes secret name.
// E.g., "OPENAI_API_KEY" -> "openai-api-key"
func EnvVarToSecretName(envVar string) string {
	return strings.ToLower(strings.ReplaceAll(envVar, "_", "-"))
}

// SecretNameToEnvVar converts a Kubernetes secret name to an environment variable name.
// E.g., "openai-api-key" -> "OPENAI_API_KEY"
func SecretNameToEnvVar(secretName string) string {
	return strings.ToUpper(strings.ReplaceAll(secretName, "-", "_"))
}

// GetSecretRefsForProvider returns the secret references for a provider.
func GetSecretRefsForProvider(providerType string) []SecretRef {
	envVars := GetAPIKeyEnvVars(providerType)
	refs := make([]SecretRef, 0, len(envVars))

	for _, envVar := range envVars {
		refs = append(refs, SecretRef{
			EnvVar:     envVar,
			SecretName: EnvVarToSecretName(envVar),
			Key:        "value", // Standard key name for the secret value
		})
	}

	return refs
}

// ValidateProviderCredentials checks if credentials are available for a provider.
// It first checks the provider's explicit Credential config (APIKey, CredentialEnv,
// CredentialFile), then falls back to the legacy default env var lookup.
func ValidateProviderCredentials(cfg *arenaconfig.Config, providerID string) error {
	provider := cfg.LoadedProviders[providerID]
	if provider == nil {
		return fmt.Errorf("provider %s not found in config", providerID)
	}

	// If the provider has an explicit credential config, validate that. A
	// conclusive result (done) short-circuits; otherwise fall through to the
	// legacy default-env-var check below.
	if provider.Credential != nil {
		if done, err := validateExplicitCredential(providerID, provider.Credential); done {
			return err
		}
	}

	// Legacy fallback: check default env vars for this provider type
	envVars := GetAPIKeyEnvVars(provider.Type)
	if len(envVars) == 0 {
		// Provider doesn't require credentials
		return nil
	}
	if anyEnvSet(envVars) {
		return nil
	}

	return fmt.Errorf("missing credentials for provider %s (type: %s, required env: %v)",
		providerID, provider.Type, envVars)
}

// validateExplicitCredential validates an explicit credential config. It returns
// done=true when the config is conclusive (err nil means valid, non-nil means
// invalid), or done=false when no explicit source is set and the caller should
// fall back to the default env vars.
func validateExplicitCredential(providerID string, cred *config.CredentialConfig) (bool, error) {
	switch {
	case cred.APIKey != "":
		return true, nil
	case cred.CredentialEnv != "":
		if os.Getenv(cred.CredentialEnv) != "" {
			return true, nil
		}
		return true, fmt.Errorf("missing credentials for provider %s: env var %s is not set",
			providerID, cred.CredentialEnv)
	case cred.CredentialFile != "":
		if _, err := os.Stat(cred.CredentialFile); err == nil {
			return true, nil
		}
		return true, fmt.Errorf("missing credentials for provider %s: credential file %s not found",
			providerID, cred.CredentialFile)
	default:
		return false, nil
	}
}

// anyEnvSet reports whether any of the given environment variables is set.
func anyEnvSet(envVars []string) bool {
	for _, envVar := range envVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}
	return false
}

// ValidateProviderCredentialsByType checks if the required environment variables
// are set for a given provider type (without needing the full config).
func ValidateProviderCredentialsByType(providerType string) error {
	envVars := GetAPIKeyEnvVars(providerType)
	if len(envVars) == 0 {
		return nil
	}
	if anyEnvSet(envVars) {
		return nil
	}

	return fmt.Errorf("missing credentials for provider type %s (required env: %v)",
		providerType, envVars)
}
