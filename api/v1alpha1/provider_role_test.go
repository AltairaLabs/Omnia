/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_EffectiveRole_DefaultsToInference(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var p *Provider
		assert.Equal(t, ProviderRoleInference, p.EffectiveRole())
	})

	t.Run("empty role", func(t *testing.T) {
		p := &Provider{Spec: ProviderSpec{Type: ProviderTypeClaude}}
		assert.Equal(t, ProviderRoleInference, p.EffectiveRole())
	})

	t.Run("explicit role passes through", func(t *testing.T) {
		p := &Provider{Spec: ProviderSpec{Type: ProviderTypeVoyageAI, Role: ProviderRoleEmbedding}}
		assert.Equal(t, ProviderRoleEmbedding, p.EffectiveRole())
	})
}

func TestRequireProviderRole(t *testing.T) {
	t.Run("matching role returns nil", func(t *testing.T) {
		p := &Provider{Spec: ProviderSpec{Type: ProviderTypeVoyageAI, Role: ProviderRoleEmbedding}}
		require.NoError(t, RequireProviderRole(p, ProviderRoleEmbedding))
	})

	t.Run("empty role on Provider treated as inference", func(t *testing.T) {
		p := &Provider{Spec: ProviderSpec{Type: ProviderTypeClaude}}
		require.NoError(t, RequireProviderRole(p, ProviderRoleInference))
	})

	t.Run("empty required treated as inference", func(t *testing.T) {
		p := &Provider{Spec: ProviderSpec{Type: ProviderTypeClaude}}
		require.NoError(t, RequireProviderRole(p, ""))
	})

	t.Run("mismatch returns error with both roles and provider name", func(t *testing.T) {
		p := &Provider{}
		p.Name = "openai-echo"
		p.Spec.Type = ProviderTypeOpenAI
		p.Spec.Role = ProviderRoleTTS
		err := RequireProviderRole(p, ProviderRoleEmbedding)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "openai-echo")
		assert.Contains(t, err.Error(), "tts")
		assert.Contains(t, err.Error(), "embedding")
	})

	t.Run("inference-default mismatch (caller wants embedding)", func(t *testing.T) {
		p := &Provider{Spec: ProviderSpec{Type: ProviderTypeClaude}} // role defaults to inference
		err := RequireProviderRole(p, ProviderRoleEmbedding)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "inference")
		assert.Contains(t, err.Error(), "embedding")
	})

	t.Run("nil provider", func(t *testing.T) {
		err := RequireProviderRole(nil, ProviderRoleInference)
		require.Error(t, err)
	})
}
