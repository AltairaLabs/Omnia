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
	"testing"

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
	spec := providerToSDKSpec(hfProvider("embed-1", "https://my-endpoint.hf.space"))

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
	spec := providerToSDKSpec(hfProvider("embed-2", ""))

	require.NotNil(t, spec.Credential)
	assert.Equal(t, "HF_TOKEN", spec.Credential.CredentialEnv)
	// No baseURL => not a dedicated endpoint => no additional config.
	assert.Nil(t, spec.AdditionalConfig)
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

	spec := providerToSDKSpec(p)

	assert.Equal(t, "ollama-1", spec.ID)
	assert.Equal(t, "ollama", spec.Type)
	assert.Nil(t, spec.Credential, "ollama has no api-key env var")
	assert.Nil(t, spec.AdditionalConfig)
}

// TestExtraProviderOptions_RoleMapping verifies one SDK option is produced per
// recognized role and unhandled roles are skipped.
func TestExtraProviderOptions_RoleMapping(t *testing.T) {
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

// TestExtraProviderOptions_Empty verifies no options when no extra providers.
func TestExtraProviderOptions_Empty(t *testing.T) {
	s := &Server{}
	assert.Empty(t, s.extraProviderOptions(logr.Discard()))
}

// TestBuildConversationOptions_WiresExtraProviders verifies the resolved
// non-default providers are actually appended to the conversation options —
// guarding against the "code exists but isn't wired" failure mode. The only
// difference between the two servers is one extra inference provider, so the
// option count must grow by exactly one.
func TestBuildConversationOptions_WiresExtraProviders(t *testing.T) {
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
