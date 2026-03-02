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

package runtime

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
	pkgprovider "github.com/altairalabs/omnia/pkg/provider"
)

// LoadFromCRD loads runtime configuration by reading the AgentRuntime CRD directly.
// It resolves the provider from the CRD, reads the API key secret, and sets the
// corresponding environment variable for the PromptKit SDK.
func LoadFromCRD(ctx context.Context, c client.Client, name, namespace string) (*Config, error) {
	ar, err := k8s.GetAgentRuntime(ctx, c, name, namespace)
	if err != nil {
		return nil, fmt.Errorf("load AgentRuntime CRD: %w", err)
	}

	cfg := &Config{
		AgentName:      name,
		Namespace:      namespace,
		PromptPackPath: getEnvOrDefault(envPromptPackPath, defaultPromptPackPath),
		PromptName:     getEnvOrDefault(envPromptName, defaultPromptName),
		GRPCPort:       defaultGRPCPort,
		HealthPort:     defaultHealthPort,
		SessionTTL:     defaultSessionTTL,
		MediaBasePath:  defaultMediaBasePath,
	}

	// PromptPack info from CRD
	cfg.PromptPackName = ar.Spec.PromptPackRef.Name
	cfg.PromptPackNamespace = namespace

	// Session config from CRD
	loadRuntimeSessionFromCRD(cfg, ar)

	// Media config from CRD
	if ar.Spec.Media != nil && ar.Spec.Media.BasePath != "" {
		cfg.MediaBasePath = ar.Spec.Media.BasePath
	}

	// Eval config from CRD
	cfg.EvalEnabled = ar.Spec.Evals != nil && ar.Spec.Evals.Enabled

	// Provider resolution: providers map → providerRef → inline provider
	if err := loadProviderFromCRD(ctx, c, cfg, ar, namespace); err != nil {
		return nil, err
	}

	// Mock provider annotation (dev/test mode)
	if mock, ok := ar.Annotations["omnia.altairalabs.ai/mock-provider"]; ok && mock == "true" {
		cfg.MockProvider = true
	}

	// Auto-enable mock provider when provider type is "mock"
	if cfg.ProviderType == string(pkgprovider.TypeMock) {
		cfg.MockProvider = true
	}

	// Tools config path from env (mount-path based)
	cfg.ToolsConfigPath = getEnvOrDefault(envToolsConfigPath, defaultToolsConfigPath)

	// Tracing config from env (injected by operator from Helm values)
	cfg.TracingEnabled = os.Getenv(envTracingEnabled) == "true"
	cfg.TracingEndpoint = os.Getenv(envTracingEndpoint)
	cfg.TracingInsecure = os.Getenv(envTracingInsecure) == "true"
	cfg.TracingSampleRate = 1.0

	// Parse env-only overrides (ports, tracing sample rate, etc.)
	if err := cfg.parseEnvironmentOverrides(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadRuntimeSessionFromCRD populates session config from the AgentRuntime CRD.
func loadRuntimeSessionFromCRD(cfg *Config, ar *v1alpha1.AgentRuntime) {
	if ar.Spec.Session == nil {
		cfg.SessionType = defaultSessionType
		return
	}

	cfg.SessionType = string(ar.Spec.Session.Type)

	if ar.Spec.Session.TTL != nil {
		if ttl, err := time.ParseDuration(*ar.Spec.Session.TTL); err == nil {
			cfg.SessionTTL = ttl
		}
	}

	// Session store URL still comes from env (secret-backed)
	cfg.SessionURL = os.Getenv(envSessionURL)
}

// loadProviderFromCRD resolves the provider from the AgentRuntime CRD and sets
// the API key environment variable for the PromptKit SDK.
func loadProviderFromCRD(ctx context.Context, c client.Client, cfg *Config, ar *v1alpha1.AgentRuntime, namespace string) error {
	// 1. Try spec.providers (named providers map)
	if len(ar.Spec.Providers) > 0 {
		return loadFromNamedProviders(ctx, c, cfg, ar.Spec.Providers, namespace)
	}

	// 2. Try spec.providerRef (legacy CRD reference)
	if ar.Spec.ProviderRef != nil {
		return loadFromProviderRef(ctx, c, cfg, *ar.Spec.ProviderRef, namespace)
	}

	// 3. Try spec.provider (legacy inline config — no secret reading needed)
	if ar.Spec.Provider != nil {
		loadFromInlineProvider(cfg, ar.Spec.Provider)
		return nil
	}

	return nil
}

// loadFromNamedProviders resolves the "default" (or first sorted) named provider.
func loadFromNamedProviders(ctx context.Context, c client.Client, cfg *Config, providers []v1alpha1.NamedProviderRef, namespace string) error {
	// Find "default" entry, or use first sorted
	var ref v1alpha1.ProviderRef
	found := false
	for _, np := range providers {
		if np.Name == "default" {
			ref = np.ProviderRef
			found = true
			break
		}
	}
	if !found {
		// Sort by name and use first
		sorted := make([]v1alpha1.NamedProviderRef, len(providers))
		copy(sorted, providers)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
		ref = sorted[0].ProviderRef
	}

	return loadFromProviderRef(ctx, c, cfg, ref, namespace)
}

// loadFromProviderRef loads config from a Provider CRD reference and injects the API key.
func loadFromProviderRef(ctx context.Context, c client.Client, cfg *Config, ref v1alpha1.ProviderRef, namespace string) error {
	provider, err := k8s.GetProvider(ctx, c, ref, namespace)
	if err != nil {
		return fmt.Errorf("resolve provider: %w", err)
	}

	cfg.ProviderType = string(provider.Spec.Type)
	cfg.Model = provider.Spec.Model
	cfg.BaseURL = provider.Spec.BaseURL
	cfg.ProviderRefName = provider.Name
	cfg.ProviderRefNamespace = provider.Namespace

	if provider.Spec.Defaults != nil {
		loadProviderDefaults(cfg, provider.Spec.Defaults)
	}

	// Inject API key from secret
	return injectAPIKey(ctx, c, cfg, provider)
}

// loadProviderDefaults populates config fields from Provider CRD defaults.
func loadProviderDefaults(cfg *Config, defaults *v1alpha1.ProviderDefaults) {
	if defaults.ContextWindow != nil {
		cfg.ContextWindow = int(*defaults.ContextWindow)
	}
	if defaults.TruncationStrategy != "" {
		cfg.TruncationStrategy = string(defaults.TruncationStrategy)
	}
}

// loadFromInlineProvider loads config from an inline ProviderConfig.
func loadFromInlineProvider(cfg *Config, provider *v1alpha1.ProviderConfig) {
	cfg.ProviderType = string(provider.Type)
	cfg.Model = provider.Model
	cfg.BaseURL = provider.BaseURL

	if provider.Config != nil {
		if provider.Config.ContextWindow != nil {
			cfg.ContextWindow = int(*provider.Config.ContextWindow)
		}
		if provider.Config.TruncationStrategy != "" {
			cfg.TruncationStrategy = string(provider.Config.TruncationStrategy)
		}
	}
}

// injectAPIKey reads the provider's secret and sets the appropriate env var
// for the PromptKit SDK (e.g., ANTHROPIC_API_KEY, OPENAI_API_KEY).
func injectAPIKey(ctx context.Context, c client.Client, cfg *Config, provider *v1alpha1.Provider) error {
	ref := k8s.EffectiveSecretRef(provider)
	if ref == nil {
		return nil // No secret configured (e.g., ollama, mock)
	}

	secret, err := k8s.GetSecret(ctx, c, ref.Name, provider.Namespace)
	if err != nil {
		return fmt.Errorf("read provider secret: %w", err)
	}

	// Determine which key in the Secret to read
	secretKey := k8s.DetermineSecretKey(ref, provider.Spec.Type)
	apiKeyValue, ok := secret.Data[secretKey]
	if !ok {
		return fmt.Errorf("secret %s/%s does not contain key %q", provider.Namespace, ref.Name, secretKey)
	}

	// Set the env var the PromptKit SDK expects
	envVarName := pkgprovider.APIKeyEnvVarName(string(provider.Spec.Type))
	if envVarName == "" {
		return nil // Provider type doesn't use API key env vars
	}

	if err := os.Setenv(envVarName, string(apiKeyValue)); err != nil {
		return fmt.Errorf("set env var %s: %w", envVarName, err)
	}

	return nil
}
