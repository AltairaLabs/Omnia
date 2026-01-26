/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package selector

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func TestSelectProviders(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1alpha1.AddToScheme(scheme))
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	// Create test providers
	providers := []runtime.Object{
		&corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openai-gpt4",
				Namespace: "test-ns",
				Labels: map[string]string{
					"provider-type": "openai",
					"tier":          "premium",
				},
			},
			Spec: corev1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4",
			},
		},
		&corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openai-gpt35",
				Namespace: "test-ns",
				Labels: map[string]string{
					"provider-type": "openai",
					"tier":          "standard",
				},
			},
			Spec: corev1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-3.5-turbo",
			},
		},
		&corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "anthropic-claude",
				Namespace: "test-ns",
				Labels: map[string]string{
					"provider-type": "anthropic",
					"tier":          "premium",
				},
			},
			Spec: corev1alpha1.ProviderSpec{
				Type:  "anthropic",
				Model: "claude-3-opus",
			},
		},
		&corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-ns-provider",
				Namespace: "other-ns",
				Labels: map[string]string{
					"provider-type": "openai",
				},
			},
			Spec: corev1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(providers...).
		Build()

	ctx := context.Background()

	t.Run("select all providers in namespace", func(t *testing.T) {
		result, err := SelectProviders(ctx, client, "test-ns", nil)
		require.NoError(t, err)
		assert.Len(t, result, 3)
	})

	t.Run("select by matchLabels", func(t *testing.T) {
		result, err := SelectProviders(ctx, client, "test-ns", &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"provider-type": "openai",
			},
		})
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("select by tier=premium", func(t *testing.T) {
		result, err := SelectProviders(ctx, client, "test-ns", &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"tier": "premium",
			},
		})
		require.NoError(t, err)
		assert.Len(t, result, 2)

		// Should have openai-gpt4 and anthropic-claude
		names := make([]string, len(result))
		for i, p := range result {
			names[i] = p.Name
		}
		assert.Contains(t, names, "openai-gpt4")
		assert.Contains(t, names, "anthropic-claude")
	})

	t.Run("select with matchExpressions In", func(t *testing.T) {
		result, err := SelectProviders(ctx, client, "test-ns", &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "tier",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"premium", "enterprise"},
				},
			},
		})
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("select with matchExpressions NotIn", func(t *testing.T) {
		result, err := SelectProviders(ctx, client, "test-ns", &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "tier",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"standard"},
				},
			},
		})
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("combined matchLabels and matchExpressions", func(t *testing.T) {
		result, err := SelectProviders(ctx, client, "test-ns", &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"provider-type": "openai",
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "tier",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"premium"},
				},
			},
		})
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "openai-gpt4", result[0].Name)
	})

	t.Run("no matches returns empty slice", func(t *testing.T) {
		result, err := SelectProviders(ctx, client, "test-ns", &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"provider-type": "nonexistent",
			},
		})
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("different namespace returns different providers", func(t *testing.T) {
		result, err := SelectProviders(ctx, client, "other-ns", nil)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "other-ns-provider", result[0].Name)
	})
}

func TestGetProvidersForGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1alpha1.AddToScheme(scheme))
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	providers := []runtime.Object{
		&corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openai-gpt4",
				Namespace: "test-ns",
				Labels: map[string]string{
					"provider-type": "openai",
					"tier":          "premium",
				},
			},
			Spec: corev1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4",
			},
		},
		&corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "anthropic-claude",
				Namespace: "test-ns",
				Labels: map[string]string{
					"provider-type": "anthropic",
					"tier":          "premium",
				},
			},
			Spec: corev1alpha1.ProviderSpec{
				Type:  "anthropic",
				Model: "claude-3-opus",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(providers...).
		Build()

	ctx := context.Background()

	t.Run("no overrides returns false", func(t *testing.T) {
		providers, found, err := GetProvidersForGroup(ctx, client, "test-ns", "default", nil)
		require.NoError(t, err)
		assert.False(t, found)
		assert.Nil(t, providers)
	})

	t.Run("empty overrides returns false", func(t *testing.T) {
		providers, found, err := GetProvidersForGroup(ctx, client, "test-ns", "default",
			map[string]omniav1alpha1.ProviderGroupSelector{})
		require.NoError(t, err)
		assert.False(t, found)
		assert.Nil(t, providers)
	})

	t.Run("explicit group override found", func(t *testing.T) {
		overrides := map[string]omniav1alpha1.ProviderGroupSelector{
			"default": {
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"provider-type": "openai"},
				},
			},
		}

		providers, found, err := GetProvidersForGroup(ctx, client, "test-ns", "default", overrides)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Len(t, providers, 1)
		assert.Equal(t, "openai-gpt4", providers[0].Name)
	})

	t.Run("wildcard override applies to unmatched groups", func(t *testing.T) {
		overrides := map[string]omniav1alpha1.ProviderGroupSelector{
			"judge": {
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"provider-type": "openai"},
				},
			},
			"*": {
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"tier": "premium"},
				},
			},
		}

		// "default" group should use wildcard
		providers, found, err := GetProvidersForGroup(ctx, client, "test-ns", "default", overrides)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Len(t, providers, 2) // Both premium providers
	})

	t.Run("explicit group takes precedence over wildcard", func(t *testing.T) {
		overrides := map[string]omniav1alpha1.ProviderGroupSelector{
			"judge": {
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"provider-type": "anthropic"},
				},
			},
			"*": {
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"provider-type": "openai"},
				},
			},
		}

		// "judge" should use explicit selector, not wildcard
		providers, found, err := GetProvidersForGroup(ctx, client, "test-ns", "judge", overrides)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Len(t, providers, 1)
		assert.Equal(t, "anthropic-claude", providers[0].Name)
	})

	t.Run("unmatched group without wildcard returns not found", func(t *testing.T) {
		overrides := map[string]omniav1alpha1.ProviderGroupSelector{
			"judge": {
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"provider-type": "openai"},
				},
			},
		}

		// "default" group has no override and no wildcard
		providers, found, err := GetProvidersForGroup(ctx, client, "test-ns", "default", overrides)
		require.NoError(t, err)
		assert.False(t, found)
		assert.Nil(t, providers)
	})
}

func TestResolveProviderOverrides(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1alpha1.AddToScheme(scheme))
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	providers := []runtime.Object{
		&corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openai-gpt4",
				Namespace: "test-ns",
				Labels: map[string]string{
					"provider-type": "openai",
				},
			},
		},
		&corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "anthropic-claude",
				Namespace: "test-ns",
				Labels: map[string]string{
					"provider-type": "anthropic",
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(providers...).
		Build()

	ctx := context.Background()

	t.Run("nil overrides returns nil", func(t *testing.T) {
		result, err := ResolveProviderOverrides(ctx, client, "test-ns", nil)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("resolves multiple groups", func(t *testing.T) {
		overrides := map[string]omniav1alpha1.ProviderGroupSelector{
			"default": {
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"provider-type": "openai"},
				},
			},
			"judge": {
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"provider-type": "anthropic"},
				},
			},
		}

		result, err := ResolveProviderOverrides(ctx, client, "test-ns", overrides)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Len(t, result["default"], 1)
		assert.Len(t, result["judge"], 1)
		assert.Equal(t, "openai-gpt4", result["default"][0].Name)
		assert.Equal(t, "anthropic-claude", result["judge"][0].Name)
	})
}
