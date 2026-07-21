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

package runtime

import (
	"os"

	pkgconfig "github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	provider "github.com/altairalabs/omnia/pkg/provider"
)

// providerToSDKSpec maps a resolved Provider CRD to the SDK's uniform
// ProviderSpec. When a resolved API key is carried (apiKey != ""), it travels
// on the spec as CredentialConfig.APIKey (design §5.3.1); otherwise it falls
// back to a CredentialEnv reference. Platform-hosted providers keep Credential
// nil and authenticate via the cloud SDK credential chain (still process-env in
// this wave; follow-up 2b-1b).
func providerToSDKSpec(p *v1alpha1.Provider, apiKey string) sdk.ProviderSpec {
	spec := sdk.ProviderSpec{
		ID:      p.Name,
		Type:    string(p.Spec.Type),
		Model:   p.Spec.Model,
		BaseURL: p.Spec.BaseURL,
	}
	if p.Spec.Platform == nil {
		switch {
		case apiKey != "":
			spec.Credential = &pkgconfig.CredentialConfig{APIKey: apiKey}
		default:
			if env := credentialEnvVar(p); env != "" {
				spec.Credential = &pkgconfig.CredentialConfig{CredentialEnv: env}
			}
		}
	}
	if p.Spec.Type == v1alpha1.ProviderTypeHuggingFace {
		spec.AdditionalConfig = provider.HuggingFaceAdditionalConfig(p.Spec.BaseURL)
	}
	return spec
}

// credentialEnvVar returns the env var PromptKit should read the credential
// from: an explicit spec.credential.envVar override when set, otherwise the
// provider-type default (e.g. HF_TOKEN).
func credentialEnvVar(p *v1alpha1.Provider) string {
	if p.Spec.Credential != nil && p.Spec.Credential.EnvVar != "" {
		return p.Spec.Credential.EnvVar
	}
	return provider.APIKeyEnvVarName(string(p.Spec.Type))
}

// extraProviderOptions maps each resolved non-default provider to its role's
// SDK option. A provider whose credential isn't present in the runtime is
// skipped with a loud warning rather than wired: wiring it would make sdk.Open
// fail and take the whole pack down, even though the agent may not use that
// role (e.g. an embedding provider the export bundled in but no OPENAI_API_KEY
// was projected). Skipping keeps the LLM serving and surfaces the misconfig
// instead of making it fatal. Unhandled roles are skipped with a debug log.
func (s *Server) extraProviderOptions(log logr.Logger) []sdk.Option {
	opts := make([]sdk.Option, 0, len(s.extraProviders))
	for _, rp := range s.extraProviders {
		if rp.Provider.Spec.Platform != nil {
			log.V(0).Info("platform-hosted non-llm provider not yet supported via spec.providers[]; skipping credential",
				"name", rp.Provider.Name, "role", rp.Role)
		} else if rp.APIKey == "" {
			// No carried key — fall back to the env contract, and skip (rather
			// than wire and fail sdk.Open) when the env var isn't present either.
			if env := credentialEnvVar(rp.Provider); env != "" && os.Getenv(env) == "" {
				log.V(0).Info("skipping extra provider: credential not set in runtime",
					"name", rp.Provider.Name, "role", rp.Role, "envVar", env,
					"impact", "this role is unavailable; the agent still serves its default LLM")
				continue
			}
		}
		opt, ok := extraProviderOption(rp.Role, providerToSDKSpec(rp.Provider, rp.APIKey))
		if !ok {
			log.V(1).Info("skipping provider with unhandled role",
				"name", rp.Provider.Name, "role", rp.Role)
			continue
		}
		opts = append(opts, opt)
	}
	return opts
}

// extraProviderOption returns the SDK option for a non-default provider role,
// and false when the role has no mapping. Pure; exported-for-test via the
// package test.
func extraProviderOption(role v1alpha1.ProviderRole, spec sdk.ProviderSpec) (sdk.Option, bool) {
	switch role {
	case v1alpha1.ProviderRoleInference:
		return sdk.WithInferenceProvider(spec), true
	case v1alpha1.ProviderRoleEmbedding:
		return sdk.WithEmbeddingProvider(spec), true
	case v1alpha1.ProviderRoleTTS:
		return sdk.WithTTSProvider(spec), true
	case v1alpha1.ProviderRoleSTT:
		return sdk.WithSTTProvider(spec), true
	case v1alpha1.ProviderRoleImage:
		return sdk.WithImageProvider(spec), true
	default:
		return nil, false
	}
}
