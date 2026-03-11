/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCacheKey(t *testing.T) {
	assert.Equal(t, "ns/pack", cacheKey("ns", "pack"))
	assert.Equal(t, "a/b", cacheKey("a", "b"))
}

func TestParsePackData_Success(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pack",
			Namespace: "default",
		},
		Data: map[string]string{
			packJSONKey: `{"id":"test-id","version":"v2"}`,
		},
	}

	result, err := parsePackData(cm, "my-pack", "v1")
	require.NoError(t, err)
	assert.Equal(t, "test-id", result.PackName)
	assert.Equal(t, "v2", result.PackVersion)
	assert.JSONEq(t,
		`{"id":"test-id","version":"v2"}`,
		string(result.PackData))
}

func TestParsePackData_FallbackNameVersion(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pack",
			Namespace: "default",
		},
		Data: map[string]string{
			packJSONKey: `{"evals":[]}`,
		},
	}

	result, err := parsePackData(cm, "fallback-name", "v3")
	require.NoError(t, err)
	assert.Equal(t, "fallback-name", result.PackName)
	assert.Equal(t, "v3", result.PackVersion)
}

func TestParsePackData_MissingKey(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pack",
			Namespace: "default",
		},
		Data: map[string]string{"other": "data"},
	}

	_, err := parsePackData(cm, "my-pack", "v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain")
}

func TestParsePackData_InvalidJSON(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pack",
			Namespace: "default",
		},
		Data: map[string]string{
			packJSONKey: `{not valid json`,
		},
	}

	_, err := parsePackData(cm, "my-pack", "v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestLoadEvals_CacheHit(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pack1",
			Namespace: "ns",
		},
		Data: map[string]string{
			packJSONKey: `{"id":"p1","version":"v1"}`,
		},
	}

	k8s := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	loader := NewPromptPackLoader(k8s)

	// First load — fetches from K8s.
	r1, err := loader.LoadEvals(
		context.Background(), "ns", "pack1", "v1",
	)
	require.NoError(t, err)
	assert.Equal(t, "p1", r1.PackName)

	// Second load — cache hit (same version).
	r2, err := loader.LoadEvals(
		context.Background(), "ns", "pack1", "v1",
	)
	require.NoError(t, err)
	assert.Same(t, r1, r2)
}

func TestLoadEvals_CacheMissOnVersionChange(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pack1",
			Namespace: "ns",
		},
		Data: map[string]string{
			packJSONKey: `{"id":"p1","version":"v2"}`,
		},
	}

	k8s := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	loader := NewPromptPackLoader(k8s)

	// Pre-populate cache with v1.
	loader.cache[cacheKey("ns", "pack1")] = &CachedPack{
		PackName:    "p1",
		PackVersion: "v1",
		PackData:    []byte(`old`),
	}

	// Load with v2 — should refetch.
	result, err := loader.LoadEvals(
		context.Background(), "ns", "pack1", "v2",
	)
	require.NoError(t, err)
	assert.Equal(t, "v2", result.PackVersion)
	assert.NotEqual(t, "old", string(result.PackData))
}

func TestLoadEvals_ConfigMapNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	k8s := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	loader := NewPromptPackLoader(k8s)
	_, err := loader.LoadEvals(
		context.Background(), "ns", "missing", "v1",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get ConfigMap")
}

func TestInvalidateCache(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	k8s := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	loader := NewPromptPackLoader(k8s)
	loader.cache[cacheKey("ns", "pack1")] = &CachedPack{
		PackName:    "p1",
		PackVersion: "v1",
	}

	loader.InvalidateCache("ns", "pack1")
	_, ok := loader.cache[cacheKey("ns", "pack1")]
	assert.False(t, ok)
}

func TestInvalidateCache_NoOp(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	k8s := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	loader := NewPromptPackLoader(k8s)
	// Invalidating a key that doesn't exist should not panic.
	loader.InvalidateCache("ns", "nonexistent")
}
