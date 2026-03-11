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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigMap data key containing the pack definition.
const packJSONKey = "pack.json"

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

// PromptPackLoader loads and caches raw pack data from PromptPack ConfigMaps.
type PromptPackLoader struct {
	client  client.Client
	cache   map[string]*CachedPack
	cacheMu sync.RWMutex
}

// NewPromptPackLoader creates a new loader.
func NewPromptPackLoader(c client.Client) *PromptPackLoader {
	return &PromptPackLoader{
		client: c,
		cache:  make(map[string]*CachedPack),
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

	cm := &corev1.ConfigMap{}
	if err := l.client.Get(ctx, types.NamespacedName{Name: packName, Namespace: namespace}, cm); err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w", namespace, packName, err)
	}

	result, err := parsePackData(cm, packName, packVersion)
	if err != nil {
		return nil, err
	}

	l.cacheMu.Lock()
	l.cache[key] = result
	l.cacheMu.Unlock()

	return result, nil
}

// parsePackData extracts raw pack bytes and identity from a ConfigMap.
func parsePackData(cm *corev1.ConfigMap, packName, packVersion string) (*CachedPack, error) {
	raw, ok := cm.Data[packJSONKey]
	if !ok {
		return nil, fmt.Errorf("ConfigMap %s/%s does not contain %q key", cm.Namespace, cm.Name, packJSONKey)
	}

	var identity packIdentity
	if err := json.Unmarshal([]byte(raw), &identity); err != nil {
		return nil, fmt.Errorf("failed to parse %s in ConfigMap %s/%s: %w", packJSONKey, cm.Namespace, cm.Name, err)
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
		PackData:    []byte(raw),
	}, nil
}

// InvalidateCache removes a cached pack (called when ConfigMap changes).
func (l *PromptPackLoader) InvalidateCache(namespace, packName string) {
	key := cacheKey(namespace, packName)
	l.cacheMu.Lock()
	delete(l.cache, key)
	l.cacheMu.Unlock()
}
