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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFromLabelSelector(t *testing.T) {
	tests := []struct {
		name        string
		selector    *metav1.LabelSelector
		wantErr     bool
		description string
	}{
		{
			name:        "nil selector returns Everything",
			selector:    nil,
			wantErr:     false,
			description: "nil selector should match everything",
		},
		{
			name:        "empty selector returns Everything",
			selector:    &metav1.LabelSelector{},
			wantErr:     false,
			description: "empty selector should match everything",
		},
		{
			name: "matchLabels only",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			wantErr:     false,
			description: "simple matchLabels selector",
		},
		{
			name: "matchExpressions only",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "tier",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"frontend", "backend"},
					},
				},
			},
			wantErr:     false,
			description: "matchExpressions with In operator",
		},
		{
			name: "combined matchLabels and matchExpressions",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "myapp",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "env",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"test"},
					},
				},
			},
			wantErr:     false,
			description: "combined selector",
		},
		{
			name: "invalid operator",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: "InvalidOp",
						Values:   []string{"test"},
					},
				},
			},
			wantErr:     true,
			description: "invalid operator should error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel, err := FromLabelSelector(tt.selector)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, sel)
		})
	}
}

func TestMatches(t *testing.T) {
	tests := []struct {
		name           string
		selector       *metav1.LabelSelector
		resourceLabels map[string]string
		want           bool
		wantErr        bool
	}{
		{
			name:     "nil selector matches everything",
			selector: nil,
			resourceLabels: map[string]string{
				"app": "test",
			},
			want: true,
		},
		{
			name: "matchLabels - exact match",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			resourceLabels: map[string]string{
				"app": "test",
			},
			want: true,
		},
		{
			name: "matchLabels - no match",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			resourceLabels: map[string]string{
				"app": "other",
			},
			want: false,
		},
		{
			name: "matchLabels - superset matches",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			resourceLabels: map[string]string{
				"app":  "test",
				"tier": "frontend",
			},
			want: true,
		},
		{
			name: "matchExpressions - In operator matches",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "tier",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"frontend", "backend"},
					},
				},
			},
			resourceLabels: map[string]string{
				"tier": "frontend",
			},
			want: true,
		},
		{
			name: "matchExpressions - In operator no match",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "tier",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"frontend", "backend"},
					},
				},
			},
			resourceLabels: map[string]string{
				"tier": "database",
			},
			want: false,
		},
		{
			name: "matchExpressions - NotIn operator matches",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "env",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"test", "dev"},
					},
				},
			},
			resourceLabels: map[string]string{
				"env": "prod",
			},
			want: true,
		},
		{
			name: "matchExpressions - Exists operator matches",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "version",
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			resourceLabels: map[string]string{
				"version": "v1",
			},
			want: true,
		},
		{
			name: "matchExpressions - Exists operator no match",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "version",
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			resourceLabels: map[string]string{
				"app": "test",
			},
			want: false,
		},
		{
			name: "matchExpressions - DoesNotExist operator matches",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "deprecated",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
				},
			},
			resourceLabels: map[string]string{
				"app": "test",
			},
			want: true,
		},
		{
			name: "combined - both must match",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "myapp",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "tier",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"frontend"},
					},
				},
			},
			resourceLabels: map[string]string{
				"app":  "myapp",
				"tier": "frontend",
			},
			want: true,
		},
		{
			name: "combined - matchLabels fails",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "myapp",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "tier",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"frontend"},
					},
				},
			},
			resourceLabels: map[string]string{
				"app":  "other",
				"tier": "frontend",
			},
			want: false,
		},
		{
			name: "invalid selector returns error",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: "InvalidOp",
						Values:   []string{"test"},
					},
				},
			},
			resourceLabels: map[string]string{
				"app": "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Matches(tt.selector, tt.resourceLabels)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMustFromLabelSelector(t *testing.T) {
	t.Run("valid selector does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			sel := MustFromLabelSelector(&metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			})
			assert.NotNil(t, sel)
		})
	})

	t.Run("invalid selector panics", func(t *testing.T) {
		assert.Panics(t, func() {
			MustFromLabelSelector(&metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: "InvalidOp",
					},
				},
			})
		})
	})
}

func TestListOptions(t *testing.T) {
	t.Run("with namespace", func(t *testing.T) {
		opts, err := ListOptions(&metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "test"},
		}, "my-namespace")
		require.NoError(t, err)
		assert.Len(t, opts, 2) // selector + namespace
	})

	t.Run("without namespace", func(t *testing.T) {
		opts, err := ListOptions(&metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "test"},
		}, "")
		require.NoError(t, err)
		assert.Len(t, opts, 1) // selector only
	})

	t.Run("nil selector", func(t *testing.T) {
		opts, err := ListOptions(nil, "my-namespace")
		require.NoError(t, err)
		assert.Len(t, opts, 2)
	})
}

func TestFilterBySelector(t *testing.T) {
	type item struct {
		name   string
		labels map[string]string
	}

	items := []item{
		{name: "frontend-1", labels: map[string]string{"app": "web", "tier": "frontend"}},
		{name: "frontend-2", labels: map[string]string{"app": "web", "tier": "frontend"}},
		{name: "backend-1", labels: map[string]string{"app": "api", "tier": "backend"}},
		{name: "database-1", labels: map[string]string{"app": "db", "tier": "data"}},
	}

	labelFunc := func(i item) map[string]string {
		return i.labels
	}

	t.Run("filter by tier=frontend", func(t *testing.T) {
		result, err := FilterBySelector(items, &metav1.LabelSelector{
			MatchLabels: map[string]string{"tier": "frontend"},
		}, labelFunc)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, "frontend-1", result[0].name)
		assert.Equal(t, "frontend-2", result[1].name)
	})

	t.Run("filter by tier In [frontend, backend]", func(t *testing.T) {
		result, err := FilterBySelector(items, &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "tier",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"frontend", "backend"},
				},
			},
		}, labelFunc)
		require.NoError(t, err)
		assert.Len(t, result, 3)
	})

	t.Run("nil selector returns all", func(t *testing.T) {
		result, err := FilterBySelector(items, nil, labelFunc)
		require.NoError(t, err)
		assert.Len(t, result, 4)
	})

	t.Run("no matches returns empty slice", func(t *testing.T) {
		result, err := FilterBySelector(items, &metav1.LabelSelector{
			MatchLabels: map[string]string{"tier": "nonexistent"},
		}, labelFunc)
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestFromLabelSelector_MatchesEveryThing(t *testing.T) {
	// Verify that nil and empty selectors return labels.Everything()
	nilSel, err := FromLabelSelector(nil)
	require.NoError(t, err)
	assert.Equal(t, labels.Everything(), nilSel)

	emptySel, err := FromLabelSelector(&metav1.LabelSelector{})
	require.NoError(t, err)
	assert.Equal(t, labels.Everything(), emptySel)
}

func TestListMatching(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	// Create test ConfigMaps
	configMaps := []runtime.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config-prod",
				Namespace: "test-ns",
				Labels: map[string]string{
					"env":  "production",
					"tier": "frontend",
				},
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config-staging",
				Namespace: "test-ns",
				Labels: map[string]string{
					"env":  "staging",
					"tier": "frontend",
				},
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config-backend",
				Namespace: "test-ns",
				Labels: map[string]string{
					"env":  "production",
					"tier": "backend",
				},
			},
		},
	}

	ctx := context.Background()

	t.Run("select by label", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(configMaps...).
			Build()

		list := &corev1.ConfigMapList{}
		err := ListMatching[corev1.ConfigMap](ctx, fakeClient, list, &metav1.LabelSelector{
			MatchLabels: map[string]string{"env": "production"},
		}, "test-ns")

		require.NoError(t, err)
		assert.Len(t, list.Items, 2)
	})

	t.Run("nil selector returns all", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(configMaps...).
			Build()

		list := &corev1.ConfigMapList{}
		err := ListMatching[corev1.ConfigMap](ctx, fakeClient, list, nil, "test-ns")

		require.NoError(t, err)
		assert.Len(t, list.Items, 3)
	})

	t.Run("invalid selector returns error", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(configMaps...).
			Build()

		list := &corev1.ConfigMapList{}
		err := ListMatching[corev1.ConfigMap](ctx, fakeClient, list, &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "app",
					Operator: "InvalidOp",
					Values:   []string{"test"},
				},
			},
		}, "test-ns")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid label selector")
	})
}
