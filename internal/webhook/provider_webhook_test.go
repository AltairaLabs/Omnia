/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func embeddingProvider(dim int32) *corev1alpha1.Provider {
	p := &corev1alpha1.Provider{}
	p.Spec.Role = corev1alpha1.ProviderRoleEmbedding
	if dim > 0 {
		p.Spec.Embedding = &corev1alpha1.EmbeddingConfig{Dimensions: dim}
	}
	return p
}

func TestProviderValidator_WarnsOnDimensionChange(t *testing.T) {
	v := &ProviderValidator{}
	warns, err := v.ValidateUpdate(context.Background(), embeddingProvider(1536), embeddingProvider(768))
	require.NoError(t, err)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "1536→768")
	assert.Contains(t, warns[0], "consent")
}

func TestProviderValidator_NoWarnSameDimension(t *testing.T) {
	v := &ProviderValidator{}
	warns, err := v.ValidateUpdate(context.Background(), embeddingProvider(768), embeddingProvider(768))
	require.NoError(t, err)
	assert.Empty(t, warns)
}

func TestProviderValidator_NoWarnWhenDimensionUnsetOrFirstDeclared(t *testing.T) {
	v := &ProviderValidator{}
	// unset -> set is model-driven / a first declaration, not a destructive change.
	warns, err := v.ValidateUpdate(context.Background(), embeddingProvider(0), embeddingProvider(768))
	require.NoError(t, err)
	assert.Empty(t, warns)
}

func TestProviderValidator_NoWarnForNonEmbeddingRole(t *testing.T) {
	v := &ProviderValidator{}
	oldP := &corev1alpha1.Provider{}
	oldP.Spec.Role = corev1alpha1.ProviderRoleLLM
	oldP.Spec.Embedding = &corev1alpha1.EmbeddingConfig{Dimensions: 1536}
	newP := &corev1alpha1.Provider{}
	newP.Spec.Role = corev1alpha1.ProviderRoleLLM
	newP.Spec.Embedding = &corev1alpha1.EmbeddingConfig{Dimensions: 768}

	warns, err := v.ValidateUpdate(context.Background(), oldP, newP)
	require.NoError(t, err)
	assert.Empty(t, warns)
}

func TestProviderValidator_CreateAndDeleteNeverWarn(t *testing.T) {
	v := &ProviderValidator{}
	w1, err := v.ValidateCreate(context.Background(), embeddingProvider(1536))
	require.NoError(t, err)
	assert.Empty(t, w1)
	w2, err := v.ValidateDelete(context.Background(), embeddingProvider(1536))
	require.NoError(t, err)
	assert.Empty(t, w2)
}
