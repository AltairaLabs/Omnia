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
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// buildSessionEnvVars creates environment variables for session configuration.
// The urlEnvName parameter allows different env var names for different containers.
func buildSessionEnvVars(session *omniav1alpha1.SessionConfig, urlEnvName string) []corev1.EnvVar {
	if session == nil {
		return nil
	}

	envVars := []corev1.EnvVar{
		{
			Name:  "OMNIA_SESSION_TYPE",
			Value: string(session.Type),
		},
	}

	if session.TTL != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_SESSION_TTL",
			Value: *session.TTL,
		})
	}

	if session.StoreRef != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: urlEnvName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *session.StoreRef,
					Key:                  "url",
				},
			},
		})
	}

	return envVars
}

// providerEnvConfig holds common provider configuration for building environment variables.
type providerEnvConfig struct {
	Type               omniav1alpha1.ProviderType
	Model              string
	BaseURL            string
	Temperature        *string
	TopP               *string
	MaxTokens          *int32
	ContextWindow      *int32
	TruncationStrategy omniav1alpha1.TruncationStrategy
	InputCost          *string
	OutputCost         *string
	CachedCost         *string
	AdditionalConfig   map[string]string
}

// addProviderEnvVars adds provider configuration environment variables to the slice.
func addProviderEnvVars(envVars []corev1.EnvVar, cfg providerEnvConfig) []corev1.EnvVar {
	envVars = append(envVars, corev1.EnvVar{
		Name:  "OMNIA_PROVIDER_TYPE",
		Value: string(cfg.Type),
	})
	if cfg.Model != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "OMNIA_PROVIDER_MODEL", Value: cfg.Model})
	}
	if cfg.BaseURL != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "OMNIA_PROVIDER_BASE_URL", Value: cfg.BaseURL})
	}
	if cfg.Temperature != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "OMNIA_PROVIDER_TEMPERATURE", Value: *cfg.Temperature})
	}
	if cfg.TopP != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "OMNIA_PROVIDER_TOP_P", Value: *cfg.TopP})
	}
	if cfg.MaxTokens != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "OMNIA_PROVIDER_MAX_TOKENS", Value: fmt.Sprintf("%d", *cfg.MaxTokens)})
	}
	if cfg.ContextWindow != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "OMNIA_CONTEXT_WINDOW", Value: fmt.Sprintf("%d", *cfg.ContextWindow)})
	}
	if cfg.TruncationStrategy != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "OMNIA_TRUNCATION_STRATEGY", Value: string(cfg.TruncationStrategy)})
	}
	if cfg.InputCost != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "OMNIA_PROVIDER_INPUT_COST", Value: *cfg.InputCost})
	}
	if cfg.OutputCost != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "OMNIA_PROVIDER_OUTPUT_COST", Value: *cfg.OutputCost})
	}
	if cfg.CachedCost != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "OMNIA_PROVIDER_CACHED_COST", Value: *cfg.CachedCost})
	}
	// Add additional config as environment variables with OMNIA_PROVIDER_ prefix
	for key, value := range cfg.AdditionalConfig {
		envName := "OMNIA_PROVIDER_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
		envVars = append(envVars, corev1.EnvVar{Name: envName, Value: value})
	}
	return envVars
}

func buildProviderEnvVars(provider *omniav1alpha1.ProviderConfig) []corev1.EnvVar {
	if provider == nil {
		return nil
	}

	cfg := providerEnvConfig{
		Type:             provider.Type,
		Model:            provider.Model,
		BaseURL:          provider.BaseURL,
		AdditionalConfig: provider.AdditionalConfig,
	}
	if provider.Config != nil {
		cfg.Temperature = provider.Config.Temperature
		cfg.TopP = provider.Config.TopP
		cfg.MaxTokens = provider.Config.MaxTokens
		cfg.ContextWindow = provider.Config.ContextWindow
		cfg.TruncationStrategy = provider.Config.TruncationStrategy
	}
	if provider.Pricing != nil {
		cfg.InputCost = provider.Pricing.InputCostPer1K
		cfg.OutputCost = provider.Pricing.OutputCostPer1K
		cfg.CachedCost = provider.Pricing.CachedCostPer1K
	}

	envVars := addProviderEnvVars(nil, cfg)

	// Add API key from secret
	if provider.SecretRef != nil {
		envVars = append(envVars, buildSecretEnvVars(provider.SecretRef, cfg.Type)...)
	}

	return envVars
}

// buildSecretEnvVars creates environment variables from a provider secret.
// It maps secret keys to the appropriate environment variable names expected by PromptKit.
func buildSecretEnvVars(secretRef *corev1.LocalObjectReference, providerType omniav1alpha1.ProviderType) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Inject the primary API key env var for the provider type
	if keyNames, ok := providerKeyMapping[providerType]; ok && len(keyNames) > 0 {
		envVars = append(envVars, buildSecretKeyEnvVar(secretRef, keyNames[0], keyNames[0]))
		envVars = append(envVars, buildSecretKeyEnvVar(secretRef, keyNames[0], "api-key"))
	}

	return envVars
}

// buildSecretKeyEnvVar creates a single environment variable from a secret key.
func buildSecretKeyEnvVar(secretRef *corev1.LocalObjectReference, envName, secretKey string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: *secretRef,
				Key:                  secretKey,
				Optional:             ptr.To(true),
			},
		},
	}
}

// buildProviderEnvVarsFromCRD creates environment variables from a Provider CRD.
// This is used when an AgentRuntime references a Provider resource.
func buildProviderEnvVarsFromCRD(provider *omniav1alpha1.Provider) []corev1.EnvVar {
	cfg := providerEnvConfig{
		Type:    provider.Spec.Type,
		Model:   provider.Spec.Model,
		BaseURL: provider.Spec.BaseURL,
	}
	if provider.Spec.Defaults != nil {
		cfg.Temperature = provider.Spec.Defaults.Temperature
		cfg.TopP = provider.Spec.Defaults.TopP
		cfg.MaxTokens = provider.Spec.Defaults.MaxTokens
		cfg.ContextWindow = provider.Spec.Defaults.ContextWindow
		cfg.TruncationStrategy = provider.Spec.Defaults.TruncationStrategy
	}
	if provider.Spec.Pricing != nil {
		cfg.InputCost = provider.Spec.Pricing.InputCostPer1K
		cfg.OutputCost = provider.Spec.Pricing.OutputCostPer1K
		cfg.CachedCost = provider.Spec.Pricing.CachedCostPer1K
	}

	envVars := addProviderEnvVars(nil, cfg)

	// Add Provider CRD reference info for metrics labels
	envVars = append(envVars,
		corev1.EnvVar{Name: "OMNIA_PROVIDER_REF_NAME", Value: provider.Name},
		corev1.EnvVar{Name: "OMNIA_PROVIDER_REF_NAMESPACE", Value: provider.Namespace},
	)

	// API key from secret - only if SecretRef is configured
	if provider.Spec.SecretRef != nil {
		secretRef := corev1.LocalObjectReference{Name: provider.Spec.SecretRef.Name}
		if provider.Spec.SecretRef.Key != nil {
			envVars = append(envVars, buildSecretEnvVarsWithKey(&secretRef, provider.Spec.Type, *provider.Spec.SecretRef.Key)...)
		} else {
			envVars = append(envVars, buildSecretEnvVars(&secretRef, provider.Spec.Type)...)
		}
	}

	return envVars
}

// buildSecretEnvVarsWithKey creates environment variables from a secret using a specific key.
func buildSecretEnvVarsWithKey(secretRef *corev1.LocalObjectReference, providerType omniav1alpha1.ProviderType, key string) []corev1.EnvVar {
	// Get the target env var name for this provider type
	envVarName := "ANTHROPIC_API_KEY" // Default
	if keyNames, ok := providerKeyMapping[providerType]; ok && len(keyNames) > 0 {
		envVarName = keyNames[0]
	}

	return []corev1.EnvVar{buildSecretKeyEnvVar(secretRef, envVarName, key)}
}

// buildEvalEnvVars creates eval-related environment variables for the facade container.
// Only called when evals are enabled and the framework is PromptKit.
func buildEvalEnvVars(evalConfig *omniav1alpha1.EvalConfig) []corev1.EnvVar {
	if evalConfig == nil || !evalConfig.Enabled {
		return nil
	}

	envVars := []corev1.EnvVar{
		{Name: envEvalsEnabled, Value: "true"},
	}

	envVars = append(envVars, buildEvalJudgesEnvVar(evalConfig.Judges)...)
	envVars = append(envVars, buildEvalSamplingEnvVars(evalConfig.Sampling)...)

	return envVars
}

// buildEvalJudgesEnvVar creates the OMNIA_EVALS_JUDGES env var from judge mappings.
func buildEvalJudgesEnvVar(judges []omniav1alpha1.JudgeMapping) []corev1.EnvVar {
	if len(judges) == 0 {
		return nil
	}

	judgesJSON, err := json.Marshal(judges)
	if err != nil {
		return nil
	}

	return []corev1.EnvVar{
		{Name: envEvalsJudges, Value: string(judgesJSON)},
	}
}

// buildEvalSamplingEnvVars creates sampling rate env vars with defaults.
func buildEvalSamplingEnvVars(sampling *omniav1alpha1.EvalSampling) []corev1.EnvVar {
	defaultRate := int32(100)
	llmJudgeRate := int32(10)

	if sampling != nil {
		if sampling.DefaultRate != nil {
			defaultRate = *sampling.DefaultRate
		}
		if sampling.LLMJudgeRate != nil {
			llmJudgeRate = *sampling.LLMJudgeRate
		}
	}

	return []corev1.EnvVar{
		{Name: envEvalsSamplingDef, Value: fmt.Sprintf("%d", defaultRate)},
		{Name: envEvalsSamplingJudge, Value: fmt.Sprintf("%d", llmJudgeRate)},
	}
}

// isPromptKit returns true if the AgentRuntime uses the PromptKit framework.
func isPromptKit(spec *omniav1alpha1.AgentRuntimeSpec) bool {
	if spec.Framework == nil {
		return true // default is PromptKit
	}
	return spec.Framework.Type == omniav1alpha1.FrameworkTypePromptKit
}

// hasEvalsEnabled returns true if the AgentRuntime has evals enabled.
func hasEvalsEnabled(spec *omniav1alpha1.AgentRuntimeSpec) bool {
	return spec.Evals != nil && spec.Evals.Enabled
}
