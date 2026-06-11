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
	pkgconfig "github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	provider "github.com/altairalabs/omnia/pkg/provider"
)

// providerToSDKSpec maps a resolved Provider CRD to the SDK's uniform
// ProviderSpec. Credentials stay unresolved (CredentialEnv only) — the
// WithXProvider option resolves them at construction time.
func providerToSDKSpec(p *v1alpha1.Provider) sdk.ProviderSpec {
	spec := sdk.ProviderSpec{
		ID:      p.Name,
		Type:    string(p.Spec.Type),
		Model:   p.Spec.Model,
		BaseURL: p.Spec.BaseURL,
	}
	// Platform-hosted providers (Bedrock/Vertex/Azure) authenticate via the
	// cloud SDK credential chain, which sdk.ProviderSpec cannot express (it has
	// no Platform field). Leave Credential nil; the caller warns and the
	// provider's process-env credentials still apply where the SDK reads them.
	if p.Spec.Platform == nil {
		if env := credentialEnvVar(p); env != "" {
			spec.Credential = &pkgconfig.CredentialConfig{CredentialEnv: env}
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
// SDK option. Unhandled roles are skipped with a debug log.
func (s *Server) extraProviderOptions(log logr.Logger) []sdk.Option {
	opts := make([]sdk.Option, 0, len(s.extraProviders))
	for _, rp := range s.extraProviders {
		if rp.Provider.Spec.Platform != nil {
			log.V(0).Info("platform-hosted non-llm provider not yet supported via spec.providers[]; skipping credential",
				"name", rp.Provider.Name, "role", rp.Role)
		}
		spec := providerToSDKSpec(rp.Provider)
		switch rp.Role {
		case v1alpha1.ProviderRoleInference:
			opts = append(opts, sdk.WithInferenceProvider(spec))
		case v1alpha1.ProviderRoleEmbedding:
			opts = append(opts, sdk.WithEmbeddingProvider(spec))
		case v1alpha1.ProviderRoleTTS:
			opts = append(opts, sdk.WithTTSProvider(spec))
		case v1alpha1.ProviderRoleSTT:
			opts = append(opts, sdk.WithSTTProvider(spec))
		case v1alpha1.ProviderRoleImage:
			opts = append(opts, sdk.WithImageProvider(spec))
		default:
			log.V(1).Info("skipping provider with unhandled role",
				"name", rp.Provider.Name, "role", rp.Role)
		}
	}
	return opts
}
