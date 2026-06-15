/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/altairalabs/omnia/internal/promptpack"
)

// CachedPack holds the cached raw pack data.
// PackData is passed directly to sdk.Evaluate() — the SDK handles all parsing.
type CachedPack struct {
	PackName    string
	PackVersion string
	// PackData is the raw pack.json bytes passed to sdk.Evaluate().
	PackData []byte
}

// packIdentity is the minimal subset of pack.json we parse (name and version only).
type packIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// packResolver loads raw pack.json bytes for a PromptPack. Satisfied by
// *promptpack.Resolver; an interface so tests can stub it.
type packResolver interface {
	Load(ctx context.Context, namespace, name string) ([]byte, error)
}

// PromptPackLoader loads and caches raw pack data. It resolves content through
// the PromptPack CR via promptpack.Resolver — it never touches the backing
// store (ConfigMap, etc.) directly.
type PromptPackLoader struct {
	resolver packResolver
	cache    map[string]*CachedPack
	cacheMu  sync.RWMutex
}

// NewPromptPackLoader creates a new loader.
func NewPromptPackLoader(c client.Client) *PromptPackLoader {
	return &PromptPackLoader{
		resolver: promptpack.NewResolver(c),
		cache:    make(map[string]*CachedPack),
	}
}

// cacheKey builds a deterministic cache key from namespace and pack name.
func cacheKey(namespace, packName string) string {
	return namespace + "/" + packName
}

// LoadEvals loads pack data for the given PromptPack ConfigMap.
func (l *PromptPackLoader) LoadEvals(
	ctx context.Context, namespace, packName, packVersion string,
) (*CachedPack, error) {
	key := cacheKey(namespace, packName)

	l.cacheMu.RLock()
	cached, ok := l.cache[key]
	l.cacheMu.RUnlock()

	if ok && cached.PackVersion == packVersion {
		return cached, nil
	}

	raw, err := l.resolver.Load(ctx, namespace, packName)
	if err != nil {
		return nil, err
	}

	result, err := parsePackData(raw, packName, packVersion)
	if err != nil {
		return nil, err
	}

	l.cacheMu.Lock()
	l.cache[key] = result
	l.cacheMu.Unlock()

	return result, nil
}

// parsePackData extracts identity (name, version) from raw pack.json bytes,
// falling back to the supplied pack name/version when the pack omits them.
func parsePackData(raw []byte, packName, packVersion string) (*CachedPack, error) {
	var identity packIdentity
	if err := json.Unmarshal(raw, &identity); err != nil {
		return nil, fmt.Errorf("failed to parse pack.json for %s: %w", packName, err)
	}

	name := identity.ID
	if name == "" {
		name = packName
	}
	version := identity.Version
	if version == "" {
		version = packVersion
	}

	return &CachedPack{
		PackName:    name,
		PackVersion: version,
		PackData:    raw,
	}, nil
}

// InvalidateCache removes a cached pack (called when ConfigMap changes).
func (l *PromptPackLoader) InvalidateCache(namespace, packName string) {
	key := cacheKey(namespace, packName)
	l.cacheMu.Lock()
	delete(l.cache, key)
	l.cacheMu.Unlock()
}
