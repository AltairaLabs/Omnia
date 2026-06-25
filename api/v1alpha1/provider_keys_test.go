/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpectedKeysForProvider(t *testing.T) {
	cases := []struct {
		name string
		typ  ProviderType
		want []string
	}{
		{"claude", ProviderTypeClaude, []string{"ANTHROPIC_API_KEY", "CLAUDE_API_KEY", "api-key"}},
		{"openai", ProviderTypeOpenAI, []string{"OPENAI_API_KEY", "OPENAI_TOKEN", "api-key"}},
		{"gemini", ProviderTypeGemini, []string{"GEMINI_API_KEY", "GOOGLE_API_KEY", "api-key"}},
		{"voyageai", ProviderTypeVoyageAI, []string{"VOYAGE_API_KEY", "api-key"}},
		{"default/unknown", ProviderType("ollama"), []string{"api-key", "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// role is forward-looking and ignored today; pass LLM for all.
			assert.Equal(t, tc.want, ExpectedKeysForProvider(ProviderRoleLLM, tc.typ))
		})
	}
}

func TestExpectedPlatformSecretKeys(t *testing.T) {
	cases := []struct {
		name     string
		platform PlatformType
		auth     AuthMethod
		want     []string
	}{
		{"bedrock accessKey", PlatformTypeBedrock, AuthMethodAccessKey, []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}},
		{"vertex serviceAccount", PlatformTypeVertex, AuthMethodServiceAccount, []string{"credentials.json"}},
		{"azure servicePrincipal", PlatformTypeAzure, AuthMethodServicePrincipal, []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET"}},
		{"workloadIdentity uses no secret", PlatformTypeBedrock, AuthMethodWorkloadIdentity, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ExpectedPlatformSecretKeys(tc.platform, tc.auth))
		})
	}
}
