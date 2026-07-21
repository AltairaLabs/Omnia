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
	"context"
	"os"
	"testing"

	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func hfProvider(name, baseURL string) *v1alpha1.Provider {
	return &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.ProviderSpec{
			Type:    v1alpha1.ProviderTypeHuggingFace,
			Model:   "BAAI/bge-large-en",
			BaseURL: baseURL,
		},
	}
}

func TestProviderToSDKSpec_HuggingFaceDedicated(t *testing.T) {
	spec := providerToSDKSpec(hfProvider("embed-1", "https://my-endpoint.hf.space"), "")

	assert.Equal(t, "embed-1", spec.ID)
	assert.Equal(t, "huggingface", spec.Type)
	assert.Equal(t, "BAAI/bge-large-en", spec.Model)
	assert.Equal(t, "https://my-endpoint.hf.space", spec.BaseURL)

	require.NotNil(t, spec.Credential)
	assert.Equal(t, "HF_TOKEN", spec.Credential.CredentialEnv)

	require.NotNil(t, spec.AdditionalConfig)
	assert.Equal(t, true, spec.AdditionalConfig["dedicated"])
}

func TestProviderToSDKSpec_HuggingFaceServerless(t *testing.T) {
	spec := providerToSDKSpec(hfProvider("embed-2", ""), "")

	require.NotNil(t, spec.Credential)
	assert.Equal(t, "HF_TOKEN", spec.Credential.CredentialEnv)
	// No baseURL => not a dedicated endpoint => no additional config.
	assert.Nil(t, spec.AdditionalConfig)
}

func TestProviderToSDKSpec_PrefersCarriedAPIKey(t *testing.T) {
	p := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "embeddings"},
		Spec:       v1alpha1.ProviderSpec{Type: v1alpha1.ProviderTypeOpenAI, Model: "text-embedding-3-small"},
	}
	spec := providerToSDKSpec(p, "sk-embed")
	require.NotNil(t, spec.Credential)
	assert.Equal(t, "sk-embed", spec.Credential.APIKey)
	assert.Empty(t, spec.Credential.CredentialEnv,
		"a carried key must be set as APIKey, not left as an env-var reference")
}

func TestProviderToSDKSpec_CredentialEnvOverride(t *testing.T) {
	p := hfProvider("hf-custom", "")
	p.Spec.Credential = &v1alpha1.CredentialConfig{EnvVar: "CUSTOM_TOKEN"}

	spec := providerToSDKSpec(p, "")

	require.NotNil(t, spec.Credential)
	assert.Equal(t, "CUSTOM_TOKEN", spec.Credential.CredentialEnv,
		"explicit spec.credential.envVar must override the provider-type default")
}

func TestProviderToSDKSpec_PlatformHostedLeavesCredentialNil(t *testing.T) {
	p := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "bedrock-1"},
		Spec: v1alpha1.ProviderSpec{
			Type:     v1alpha1.ProviderTypeClaude,
			Model:    "claude-sonnet-4",
			Platform: &v1alpha1.PlatformConfig{Type: v1alpha1.PlatformTypeBedrock},
		},
	}

	spec := providerToSDKSpec(p, "")

	assert.Nil(t, spec.Credential,
		"platform-hosted providers cannot express auth via ProviderSpec; Credential must stay nil")
}

func TestProviderToSDKSpec_NoCredentialEnv(t *testing.T) {
	p := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "ollama-1"},
		Spec: v1alpha1.ProviderSpec{
			Type:    v1alpha1.ProviderTypeOllama,
			Model:   "nomic-embed-text",
			BaseURL: "http://ollama:11434",
		},
	}

	spec := providerToSDKSpec(p, "")

	assert.Equal(t, "ollama-1", spec.ID)
	assert.Equal(t, "ollama", spec.Type)
	assert.Nil(t, spec.Credential, "ollama has no api-key env var")
	assert.Nil(t, spec.AdditionalConfig)
}

// TestExtraProviderOptions_RoleMapping verifies one SDK option is produced per
// recognized role and unhandled roles are skipped.
func TestExtraProviderOptions_RoleMapping(t *testing.T) {
	t.Setenv("HF_TOKEN", "test-token") // credentials present so providers are wired, not skipped
	roles := []v1alpha1.ProviderRole{
		v1alpha1.ProviderRoleInference,
		v1alpha1.ProviderRoleEmbedding,
		v1alpha1.ProviderRoleTTS,
		v1alpha1.ProviderRoleSTT,
		v1alpha1.ProviderRoleImage,
	}
	s := &Server{}
	for _, r := range roles {
		s.extraProviders = append(s.extraProviders, ResolvedProvider{
			Role:     r,
			Provider: hfProvider("p-"+string(r), ""),
		})
	}
	// An unhandled role must be skipped.
	s.extraProviders = append(s.extraProviders, ResolvedProvider{
		Role:     v1alpha1.ProviderRole("bogus"),
		Provider: hfProvider("p-bogus", ""),
	})

	opts := s.extraProviderOptions(logr.Discard())
	assert.Len(t, opts, len(roles), "one option per recognized role, unhandled role skipped")
	for i, o := range opts {
		assert.NotNil(t, o, "option %d is non-nil", i)
	}
}

// TestProviderSpec_SameTypeProvidersDoNotCollide is the regression for the
// credential-bleed bug (design §5.3.1): two providers of the SAME type with
// DIFFERENT carried keys must each produce a spec with its own key. Before the
// fix both keys went to one process-type-keyed env var and last-write-wins.
func TestProviderSpec_SameTypeProvidersDoNotCollide(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "") // no shared env fallback

	openai := func(name string) *v1alpha1.Provider {
		return &v1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       v1alpha1.ProviderSpec{Type: v1alpha1.ProviderTypeOpenAI, Model: "gpt-4o"},
		}
	}

	specA := providerToSDKSpec(openai("default"), "sk-default")
	specB := providerToSDKSpec(openai("embeddings"), "sk-embed")

	require.NotNil(t, specA.Credential)
	require.NotNil(t, specB.Credential)
	assert.Equal(t, "sk-default", specA.Credential.APIKey)
	assert.Equal(t, "sk-embed", specB.Credential.APIKey,
		"same-type providers must keep distinct keys — no shared env, no last-write-wins")
	assert.Empty(t, os.Getenv("OPENAI_API_KEY"), "no key leaked to process env")
}

// TestExtraProviderOptions_Empty verifies no options when no extra providers.
func TestExtraProviderOptions_Empty(t *testing.T) {
	s := &Server{}
	assert.Empty(t, s.extraProviderOptions(logr.Discard()))
}

// TestExtraProviderOptions_SkipsWhenCredentialMissing is the regression for the
// "runtime cares about providers it isn't using" bug: an extra provider whose
// credential env isn't present in the runtime must be SKIPPED, not wired —
// wiring it makes sdk.Open fail and takes the whole pack down (every Converse
// errors), even though the agent's pack may not use that role.
func TestExtraProviderOptions_SkipsWhenCredentialMissing(t *testing.T) {
	t.Setenv("HF_TOKEN", "") // explicitly absent
	s := &Server{extraProviders: []ResolvedProvider{{
		Role:     v1alpha1.ProviderRoleEmbedding,
		Provider: hfProvider("rag-hero-embeddings", ""),
	}}}
	assert.Empty(t, s.extraProviderOptions(logr.Discard()),
		"a provider with no credential in the runtime must be skipped, not wired")
}

// TestExtraProviderOptions_WiresWhenCredentialPresent is the counterpart: with
// the credential present the provider IS wired.
func TestExtraProviderOptions_WiresWhenCredentialPresent(t *testing.T) {
	t.Setenv("HF_TOKEN", "tok")
	s := &Server{extraProviders: []ResolvedProvider{{
		Role:     v1alpha1.ProviderRoleEmbedding,
		Provider: hfProvider("rag-hero-embeddings", ""),
	}}}
	assert.Len(t, s.extraProviderOptions(logr.Discard()), 1)
}

// TestExtraProviderOptions_PlatformProviderNotCredentialGated proves the
// credential skip applies only to env-credentialed providers; a platform-hosted
// provider (auths via the cloud SDK chain, no env var) is still wired.
func TestExtraProviderOptions_PlatformProviderNotCredentialGated(t *testing.T) {
	p := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "bedrock-embed"},
		Spec: v1alpha1.ProviderSpec{
			Type:     v1alpha1.ProviderTypeClaude,
			Platform: &v1alpha1.PlatformConfig{Type: v1alpha1.PlatformTypeBedrock},
		},
	}
	s := &Server{extraProviders: []ResolvedProvider{{Role: v1alpha1.ProviderRoleEmbedding, Provider: p}}}
	assert.Len(t, s.extraProviderOptions(logr.Discard()), 1)
}

// TestExtraProviderOption_RoleMapping covers the pure role→option mapping.
func TestExtraProviderOption_RoleMapping(t *testing.T) {
	for _, r := range []v1alpha1.ProviderRole{
		v1alpha1.ProviderRoleInference,
		v1alpha1.ProviderRoleEmbedding,
		v1alpha1.ProviderRoleTTS,
		v1alpha1.ProviderRoleSTT,
		v1alpha1.ProviderRoleImage,
	} {
		_, ok := extraProviderOption(r, sdk.ProviderSpec{})
		assert.True(t, ok, "role %s should map to an option", r)
	}
	_, ok := extraProviderOption(v1alpha1.ProviderRole("bogus"), sdk.ProviderSpec{})
	assert.False(t, ok, "unhandled role must return ok=false")
}

// TestBuildConversationOptions_WiresExtraProviders verifies the resolved
// non-default providers are actually appended to the conversation options —
// guarding against the "code exists but isn't wired" failure mode. The only
// difference between the two servers is one extra inference provider, so the
// option count must grow by exactly one.
func TestBuildConversationOptions_WiresExtraProviders(t *testing.T) {
	t.Setenv("HF_TOKEN", "test-token") // credential present so the extra provider is wired
	withExtra := NewServer(
		WithLogger(logr.Discard()),
		WithExtraProviders([]ResolvedProvider{{
			Role:     v1alpha1.ProviderRoleInference,
			Provider: hfProvider("inf-1", "https://infer.hf.space"),
		}}),
	)
	withoutExtra := NewServer(WithLogger(logr.Discard()))

	extraOpts, err := withExtra.buildConversationOptions(context.Background(), "sess-1")
	require.NoError(t, err)
	baseOpts, err := withoutExtra.buildConversationOptions(context.Background(), "sess-1")
	require.NoError(t, err)

	assert.Equal(t, 1, len(extraOpts)-len(baseOpts),
		"one extra provider should append exactly one SDK option")
}
