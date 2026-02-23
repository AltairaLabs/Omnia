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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// validPackJSON is a minimal pack.json with eval definitions for testing.
const validPackJSON = `{
  "id": "test-pack",
  "name": "Test Pack",
  "version": "1.0.0",
  "template_engine": {"version": "v1", "syntax": "{{variable}}"},
  "prompts": {
    "support": {
      "id": "support",
      "name": "Support",
      "version": "1.0.0",
      "system_template": "You are a support assistant."
    }
  },
  "evals": [
    {
      "id": "tone-check",
      "type": "llm_judge",
      "trigger": "per_turn",
      "description": "Check response tone is professional",
      "judgeName": "tone-judge",
      "params": {"rubric": "professional tone"}
    },
    {
      "id": "summary-quality",
      "type": "similarity",
      "trigger": "on_session_complete",
      "description": "Check summary quality against reference"
    },
    {
      "id": "no-pii",
      "type": "rule",
      "trigger": "per_turn",
      "description": "Ensure no PII is leaked",
      "params": {"pattern": "\\d{3}-\\d{2}-\\d{4}"}
    }
  ]
}`

// packJSONNoEvals is a pack.json with no evals section.
const packJSONNoEvals = `{
  "id": "no-evals-pack",
  "name": "No Evals Pack",
  "version": "2.0.0",
  "template_engine": {"version": "v1", "syntax": "{{variable}}"},
  "prompts": {
    "chat": {
      "id": "chat",
      "name": "Chat",
      "version": "1.0.0",
      "system_template": "You are a chat assistant."
    }
  }
}`

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	return s
}

func newConfigMap(namespace, name, packData string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			packJSONKey: packData,
		},
	}
}

func TestLoadEvals(t *testing.T) {
	tests := []struct {
		name        string
		namespace   string
		packName    string
		packVersion string
		configMaps  []*corev1.ConfigMap
		wantErr     bool
		wantEvals   int
		wantPackID  string
		wantVersion string
	}{
		{
			name:        "loads evals from valid pack.json",
			namespace:   "default",
			packName:    "test-pack",
			packVersion: "1.0.0",
			configMaps:  []*corev1.ConfigMap{newConfigMap("default", "test-pack", validPackJSON)},
			wantEvals:   3,
			wantPackID:  "test-pack",
			wantVersion: "1.0.0",
		},
		{
			name:        "returns empty evals when pack has no evals section",
			namespace:   "default",
			packName:    "no-evals-pack",
			packVersion: "2.0.0",
			configMaps:  []*corev1.ConfigMap{newConfigMap("default", "no-evals-pack", packJSONNoEvals)},
			wantEvals:   0,
			wantPackID:  "no-evals-pack",
			wantVersion: "2.0.0",
		},
		{
			name:        "error when ConfigMap not found",
			namespace:   "default",
			packName:    "missing-pack",
			packVersion: "1.0.0",
			configMaps:  nil,
			wantErr:     true,
		},
		{
			name:        "error when ConfigMap has no pack.json key",
			namespace:   "default",
			packName:    "empty-pack",
			packVersion: "1.0.0",
			configMaps: []*corev1.ConfigMap{{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-pack", Namespace: "default"},
				Data:       map[string]string{"other.json": "{}"},
			}},
			wantErr: true,
		},
		{
			name:        "error when pack.json contains invalid JSON",
			namespace:   "default",
			packName:    "bad-json",
			packVersion: "1.0.0",
			configMaps:  []*corev1.ConfigMap{newConfigMap("default", "bad-json", "not-json{{{")},
			wantErr:     true,
		},
		{
			name:        "falls back to provided name/version when pack.json fields are empty",
			namespace:   "ns1",
			packName:    "fallback-pack",
			packVersion: "3.0.0",
			configMaps: []*corev1.ConfigMap{newConfigMap("ns1", "fallback-pack", `{
				"prompts": {"x": {"id":"x","name":"X","version":"1.0.0","system_template":"hi"}},
				"template_engine": {"version":"v1","syntax":"{{v}}"},
				"name": "Fallback",
				"evals": [{"id":"e1","type":"rule","trigger":"per_turn","description":"test"}]
			}`)},
			wantEvals:   1,
			wantPackID:  "fallback-pack",
			wantVersion: "3.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(newScheme())
			for _, cm := range tt.configMaps {
				builder = builder.WithObjects(cm)
			}
			c := builder.Build()
			loader := NewPromptPackLoader(c)

			result, err := loader.LoadEvals(context.Background(), tt.namespace, tt.packName, tt.packVersion)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Evals) != tt.wantEvals {
				t.Errorf("got %d evals, want %d", len(result.Evals), tt.wantEvals)
			}
			if result.PackName != tt.wantPackID {
				t.Errorf("got pack name %q, want %q", result.PackName, tt.wantPackID)
			}
			if result.PackVersion != tt.wantVersion {
				t.Errorf("got pack version %q, want %q", result.PackVersion, tt.wantVersion)
			}
		})
	}
}

func TestLoadEvals_Caching(t *testing.T) {
	cm := newConfigMap("default", "cached-pack", validPackJSON)
	c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(cm).Build()
	loader := NewPromptPackLoader(c)
	ctx := context.Background()

	// First load populates cache.
	result1, err := loader.LoadEvals(ctx, "default", "cached-pack", "1.0.0")
	if err != nil {
		t.Fatalf("first load failed: %v", err)
	}

	// Delete the ConfigMap to prove second call uses cache.
	if err := c.Delete(ctx, cm); err != nil {
		t.Fatalf("failed to delete ConfigMap: %v", err)
	}

	// Second load should return cached result.
	result2, err := loader.LoadEvals(ctx, "default", "cached-pack", "1.0.0")
	if err != nil {
		t.Fatalf("cached load failed: %v", err)
	}
	if result1.PackName != result2.PackName || result1.PackVersion != result2.PackVersion {
		t.Error("cached result does not match original")
	}
}

func TestLoadEvals_CacheInvalidation(t *testing.T) {
	cm := newConfigMap("default", "inv-pack", validPackJSON)
	c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(cm).Build()
	loader := NewPromptPackLoader(c)
	ctx := context.Background()

	// Populate cache.
	_, err := loader.LoadEvals(ctx, "default", "inv-pack", "1.0.0")
	if err != nil {
		t.Fatalf("first load failed: %v", err)
	}

	// Invalidate cache.
	loader.InvalidateCache("default", "inv-pack")

	// Delete ConfigMap so next load fails (proving cache was cleared).
	if err := c.Delete(ctx, cm); err != nil {
		t.Fatalf("failed to delete ConfigMap: %v", err)
	}

	_, err = loader.LoadEvals(ctx, "default", "inv-pack", "1.0.0")
	if err == nil {
		t.Fatal("expected error after cache invalidation and ConfigMap deletion")
	}
}

func TestLoadEvals_CacheVersionMismatch(t *testing.T) {
	cm := newConfigMap("default", "ver-pack", validPackJSON)
	c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(cm).Build()
	loader := NewPromptPackLoader(c)
	ctx := context.Background()

	// Load with version 1.0.0 (matches pack.json).
	_, err := loader.LoadEvals(ctx, "default", "ver-pack", "1.0.0")
	if err != nil {
		t.Fatalf("first load failed: %v", err)
	}

	// Load with different version forces re-fetch.
	result, err := loader.LoadEvals(ctx, "default", "ver-pack", "2.0.0")
	if err != nil {
		t.Fatalf("second load failed: %v", err)
	}
	// pack.json has version 1.0.0, so that should be used.
	if result.PackVersion != "1.0.0" {
		t.Errorf("got version %q, want %q", result.PackVersion, "1.0.0")
	}
}

func TestResolveEvals(t *testing.T) {
	evals := &PromptPackEvals{
		PackName:    "test-pack",
		PackVersion: "1.0.0",
		Evals: []EvalDef{
			{ID: "e1", Type: "rule", Trigger: "per_turn", Description: "eval 1"},
			{ID: "e2", Type: "llm_judge", Trigger: "on_session_complete", Description: "eval 2"},
			{ID: "e3", Type: "similarity", Trigger: "per_turn", Description: "eval 3"},
		},
	}

	tests := []struct {
		name    string
		evals   *PromptPackEvals
		trigger string
		want    int
	}{
		{
			name:    "filter by per_turn trigger",
			evals:   evals,
			trigger: "per_turn",
			want:    2,
		},
		{
			name:    "filter by on_session_complete trigger",
			evals:   evals,
			trigger: "on_session_complete",
			want:    1,
		},
		{
			name:    "empty trigger returns all evals",
			evals:   evals,
			trigger: "",
			want:    3,
		},
		{
			name:    "unknown trigger returns no evals",
			evals:   evals,
			trigger: "unknown",
			want:    0,
		},
		{
			name:    "nil evals returns nil",
			evals:   nil,
			trigger: "per_turn",
			want:    0,
		},
		{
			name: "empty evals list returns empty",
			evals: &PromptPackEvals{
				PackName: "empty",
				Evals:    []EvalDef{},
			},
			trigger: "",
			want:    0,
		},
	}

	loader := NewPromptPackLoader(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := loader.ResolveEvals(tt.evals, tt.trigger)
			if len(result) != tt.want {
				t.Errorf("got %d evals, want %d", len(result), tt.want)
			}
		})
	}
}

func TestResolveEvals_ReturnsCopy(t *testing.T) {
	evals := &PromptPackEvals{
		Evals: []EvalDef{
			{ID: "e1", Type: "rule", Trigger: "per_turn"},
		},
	}

	loader := NewPromptPackLoader(nil)
	result := loader.ResolveEvals(evals, "")

	// Modify the returned slice.
	result[0].ID = "modified"

	// Original should be unchanged.
	if evals.Evals[0].ID != "e1" {
		t.Error("ResolveEvals did not return a copy; original was modified")
	}
}

func TestInvalidateCache(t *testing.T) {
	loader := NewPromptPackLoader(nil)

	// Pre-populate cache directly.
	loader.cacheMu.Lock()
	loader.cache["ns/pack1"] = &PromptPackEvals{PackName: "pack1"}
	loader.cache["ns/pack2"] = &PromptPackEvals{PackName: "pack2"}
	loader.cacheMu.Unlock()

	// Invalidate one entry.
	loader.InvalidateCache("ns", "pack1")

	loader.cacheMu.RLock()
	defer loader.cacheMu.RUnlock()

	if _, ok := loader.cache["ns/pack1"]; ok {
		t.Error("pack1 should have been invalidated")
	}
	if _, ok := loader.cache["ns/pack2"]; !ok {
		t.Error("pack2 should still be cached")
	}
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		namespace string
		packName  string
		want      string
	}{
		{"default", "my-pack", "default/my-pack"},
		{"ns1", "pack", "ns1/pack"},
		{"", "pack", "/pack"},
	}
	for _, tt := range tests {
		got := cacheKey(tt.namespace, tt.packName)
		if got != tt.want {
			t.Errorf("cacheKey(%q, %q) = %q, want %q", tt.namespace, tt.packName, got, tt.want)
		}
	}
}

func TestEvalDefFields(t *testing.T) {
	cm := newConfigMap("default", "field-pack", validPackJSON)
	c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(cm).Build()
	loader := NewPromptPackLoader(c)

	result, err := loader.LoadEvals(context.Background(), "default", "field-pack", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the first eval has all fields populated correctly.
	e := result.Evals[0]
	if e.ID != "tone-check" {
		t.Errorf("got ID %q, want %q", e.ID, "tone-check")
	}
	if e.Type != "llm_judge" {
		t.Errorf("got Type %q, want %q", e.Type, "llm_judge")
	}
	if e.Trigger != "per_turn" {
		t.Errorf("got Trigger %q, want %q", e.Trigger, "per_turn")
	}
	if e.JudgeName != "tone-judge" {
		t.Errorf("got JudgeName %q, want %q", e.JudgeName, "tone-judge")
	}
	if e.Params["rubric"] != "professional tone" {
		t.Errorf("got Params[rubric] %v, want %q", e.Params["rubric"], "professional tone")
	}
}

// packJSONWithAssertions is a pack.json with pack_assertions for testing.
const packJSONWithAssertions = `{
  "id": "assertion-pack",
  "version": "1.0.0",
  "evals": [
    {
      "id": "rule-eval",
      "type": "rule",
      "trigger": "per_turn",
      "description": "A rule eval"
    }
  ],
  "pack_assertions": [
    {
      "type": "tools_called",
      "params": {"tool_names": ["get_weather"]},
      "message": "Must call get_weather"
    },
    {
      "type": "content_includes_any",
      "params": {"patterns": ["hello", "hi"]}
    }
  ]
}`

func TestLoadEvals_WithPackAssertions(t *testing.T) {
	cm := newConfigMap("default", "assertion-pack", packJSONWithAssertions)
	c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(cm).Build()
	loader := NewPromptPackLoader(c)

	result, err := loader.LoadEvals(context.Background(), "default", "assertion-pack", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 explicit eval + 2 converted pack_assertions = 3 total
	if len(result.Evals) != 3 {
		t.Fatalf("got %d evals, want 3", len(result.Evals))
	}

	// First eval is the explicit one.
	if result.Evals[0].ID != "rule-eval" {
		t.Errorf("first eval ID = %q, want %q", result.Evals[0].ID, "rule-eval")
	}

	// Second eval is converted from first pack_assertion.
	a1 := result.Evals[1]
	if a1.ID != "pack-assertion-0" {
		t.Errorf("assertion 0 ID = %q, want %q", a1.ID, "pack-assertion-0")
	}
	if a1.Type != EvalTypeArenaAssertion {
		t.Errorf("assertion 0 Type = %q, want %q", a1.Type, EvalTypeArenaAssertion)
	}
	if a1.Trigger != "on_session_complete" {
		t.Errorf("assertion 0 Trigger = %q, want %q", a1.Trigger, "on_session_complete")
	}
	if a1.Description != "Must call get_weather" {
		t.Errorf("assertion 0 Description = %q, want %q", a1.Description, "Must call get_weather")
	}
	if a1.Params["assertion_type"] != "tools_called" {
		t.Errorf("assertion 0 assertion_type = %v, want %q", a1.Params["assertion_type"], "tools_called")
	}
	if _, ok := a1.Params["assertion_params"]; !ok {
		t.Error("assertion 0 should have assertion_params")
	}

	// Third eval is converted from second pack_assertion (no message).
	a2 := result.Evals[2]
	if a2.ID != "pack-assertion-1" {
		t.Errorf("assertion 1 ID = %q, want %q", a2.ID, "pack-assertion-1")
	}
	if a2.Description != "arena assertion: content_includes_any" {
		t.Errorf("assertion 1 Description = %q, want %q", a2.Description, "arena assertion: content_includes_any")
	}
}

func TestConvertPackAssertions_Empty(t *testing.T) {
	result := convertPackAssertions(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 results for nil input, got %d", len(result))
	}

	result = convertPackAssertions([]PackAssertion{})
	if len(result) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(result))
	}
}

func TestConvertPackAssertions_NoParams(t *testing.T) {
	assertions := []PackAssertion{
		{Type: "tools_called"},
	}

	result := convertPackAssertions(assertions)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if _, ok := result[0].Params["assertion_params"]; ok {
		t.Error("assertion_params should not be set when params are empty")
	}
	if result[0].Params["assertion_type"] != "tools_called" {
		t.Errorf("assertion_type = %v, want %q", result[0].Params["assertion_type"], "tools_called")
	}
}
