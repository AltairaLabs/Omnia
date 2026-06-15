/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// stubResolver is a packResolver test double returning canned bytes or an error.
type stubResolver struct {
	data map[string][]byte
	err  error
}

func (s *stubResolver) Load(_ context.Context, namespace, name string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if b, ok := s.data[namespace+"/"+name]; ok {
		return b, nil
	}
	return nil, errors.New("pack not found")
}

// newStubLoader builds a PromptPackLoader backed by a stub resolver.
func newStubLoader(r packResolver) *PromptPackLoader {
	return &PromptPackLoader{resolver: r, cache: make(map[string]*CachedPack)}
}

func TestCacheKey(t *testing.T) {
	assert.Equal(t, "ns/pack", cacheKey("ns", "pack"))
	assert.Equal(t, "a/b", cacheKey("a", "b"))
}

func TestParsePackData_Success(t *testing.T) {
	result, err := parsePackData([]byte(`{"id":"test-id","version":"v2"}`), "my-pack", "v1")
	require.NoError(t, err)
	assert.Equal(t, "test-id", result.PackName)
	assert.Equal(t, "v2", result.PackVersion)
	assert.JSONEq(t, `{"id":"test-id","version":"v2"}`, string(result.PackData))
}

func TestParsePackData_FallbackNameVersion(t *testing.T) {
	result, err := parsePackData([]byte(`{"evals":[]}`), "fallback-name", "v3")
	require.NoError(t, err)
	assert.Equal(t, "fallback-name", result.PackName)
	assert.Equal(t, "v3", result.PackVersion)
}

func TestParsePackData_InvalidJSON(t *testing.T) {
	_, err := parsePackData([]byte(`{not valid json`), "my-pack", "v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestLoadEvals_CacheHit(t *testing.T) {
	loader := newStubLoader(&stubResolver{data: map[string][]byte{
		"ns/pack1": []byte(`{"id":"p1","version":"v1"}`),
	}})

	// First load — fetches via the resolver.
	r1, err := loader.LoadEvals(context.Background(), "ns", "pack1", "v1")
	require.NoError(t, err)
	assert.Equal(t, "p1", r1.PackName)

	// Second load — cache hit (same version).
	r2, err := loader.LoadEvals(context.Background(), "ns", "pack1", "v1")
	require.NoError(t, err)
	assert.Same(t, r1, r2)
}

func TestLoadEvals_CacheMissOnVersionChange(t *testing.T) {
	loader := newStubLoader(&stubResolver{data: map[string][]byte{
		"ns/pack1": []byte(`{"id":"p1","version":"v2"}`),
	}})

	// Pre-populate cache with v1.
	loader.cache[cacheKey("ns", "pack1")] = &CachedPack{
		PackName:    "p1",
		PackVersion: "v1",
		PackData:    []byte(`old`),
	}

	// Load with v2 — should refetch.
	result, err := loader.LoadEvals(context.Background(), "ns", "pack1", "v2")
	require.NoError(t, err)
	assert.Equal(t, "v2", result.PackVersion)
	assert.NotEqual(t, "old", string(result.PackData))
}

func TestLoadEvals_ResolverError(t *testing.T) {
	loader := newStubLoader(&stubResolver{err: errors.New("get PromptPack ns/missing: not found")})

	_, err := loader.LoadEvals(context.Background(), "ns", "missing", "v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PromptPack")
}

func TestInvalidateCache(t *testing.T) {
	loader := NewPromptPackLoader(fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build())
	loader.cache[cacheKey("ns", "pack1")] = &CachedPack{PackName: "p1", PackVersion: "v1"}

	loader.InvalidateCache("ns", "pack1")
	_, ok := loader.cache[cacheKey("ns", "pack1")]
	assert.False(t, ok)
}

func TestInvalidateCache_NoOp(t *testing.T) {
	loader := NewPromptPackLoader(fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build())
	// Invalidating a key that doesn't exist should not panic.
	loader.InvalidateCache("ns", "nonexistent")
}
