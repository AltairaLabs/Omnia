/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

// Package evals provides eval definition loading from PromptPack ConfigMaps.
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

// EvalDef represents an eval definition from a PromptPack.
type EvalDef struct {
	// ID is the unique identifier for this eval.
	ID string `json:"id" yaml:"id"`
	// Type is the eval type (e.g. "rule", "llm_judge", "similarity").
	Type string `json:"type" yaml:"type"`
	// Trigger is when the eval runs (e.g. "per_turn", "on_session_complete").
	Trigger string `json:"trigger" yaml:"trigger"`
	// Description explains what the eval checks.
	Description string `json:"description" yaml:"description"`
	// Params holds type-specific configuration.
	Params map[string]any `json:"params,omitempty" yaml:"params,omitempty"`
	// JudgeName references an AgentRuntime.Spec.Evals.Judges entry.
	JudgeName string `json:"judgeName,omitempty" yaml:"judgeName,omitempty"`
}

// PromptPackEvals holds the parsed eval definitions from a PromptPack.
type PromptPackEvals struct {
	PackName    string    `json:"packName"`
	PackVersion string    `json:"packVersion"`
	Evals       []EvalDef `json:"evals"`
}

// packJSON is the subset of pack.json we parse for eval definitions.
type packJSON struct {
	ID      string    `json:"id"`
	Version string    `json:"version"`
	Evals   []EvalDef `json:"evals"`
}

// PromptPackLoader loads and caches eval definitions from PromptPack ConfigMaps.
type PromptPackLoader struct {
	client  client.Client
	cache   map[string]*PromptPackEvals
	cacheMu sync.RWMutex
}

// NewPromptPackLoader creates a new loader.
func NewPromptPackLoader(c client.Client) *PromptPackLoader {
	return &PromptPackLoader{
		client: c,
		cache:  make(map[string]*PromptPackEvals),
	}
}

// cacheKey builds a deterministic cache key from namespace and pack name.
func cacheKey(namespace, packName string) string {
	return namespace + "/" + packName
}

// LoadEvals loads eval definitions for the given PromptPack.
// It reads the ConfigMap referenced by the PromptPack, parses the evals
// section from pack.json, and caches the result.
func (l *PromptPackLoader) LoadEvals(
	ctx context.Context, namespace, packName, packVersion string,
) (*PromptPackEvals, error) {
	key := cacheKey(namespace, packName)

	// Check cache first.
	l.cacheMu.RLock()
	cached, ok := l.cache[key]
	l.cacheMu.RUnlock()

	if ok && cached.PackVersion == packVersion {
		return cached, nil
	}

	// Fetch the ConfigMap.
	cm := &corev1.ConfigMap{}
	if err := l.client.Get(ctx, types.NamespacedName{Name: packName, Namespace: namespace}, cm); err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w", namespace, packName, err)
	}

	// Parse pack.json from ConfigMap data.
	result, err := parsePackEvals(cm, packName, packVersion)
	if err != nil {
		return nil, err
	}

	// Store in cache.
	l.cacheMu.Lock()
	l.cache[key] = result
	l.cacheMu.Unlock()

	return result, nil
}

// parsePackEvals extracts eval definitions from a ConfigMap's pack.json data.
func parsePackEvals(cm *corev1.ConfigMap, packName, packVersion string) (*PromptPackEvals, error) {
	raw, ok := cm.Data[packJSONKey]
	if !ok {
		return nil, fmt.Errorf("ConfigMap %s/%s does not contain %q key", cm.Namespace, cm.Name, packJSONKey)
	}

	var pack packJSON
	if err := json.Unmarshal([]byte(raw), &pack); err != nil {
		return nil, fmt.Errorf("failed to parse %s in ConfigMap %s/%s: %w", packJSONKey, cm.Namespace, cm.Name, err)
	}

	// Use pack.json id/version if available, fall back to provided values.
	name := pack.ID
	if name == "" {
		name = packName
	}
	version := pack.Version
	if version == "" {
		version = packVersion
	}

	return &PromptPackEvals{
		PackName:    name,
		PackVersion: version,
		Evals:       pack.Evals,
	}, nil
}

// ResolveEvals returns the evals applicable for the given trigger type.
// If trigger is empty, all evals are returned.
func (l *PromptPackLoader) ResolveEvals(evals *PromptPackEvals, trigger string) []EvalDef {
	if evals == nil {
		return nil
	}
	if trigger == "" {
		result := make([]EvalDef, len(evals.Evals))
		copy(result, evals.Evals)
		return result
	}

	var matched []EvalDef
	for _, e := range evals.Evals {
		if e.Trigger == trigger {
			matched = append(matched, e)
		}
	}
	return matched
}

// InvalidateCache removes a cached pack (called when ConfigMap changes).
func (l *PromptPackLoader) InvalidateCache(namespace, packName string) {
	key := cacheKey(namespace, packName)
	l.cacheMu.Lock()
	delete(l.cache, key)
	l.cacheMu.Unlock()
}
