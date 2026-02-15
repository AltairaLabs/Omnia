/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"fmt"
	"sort"
	"testing"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// MockProviderLookup implements ProviderLookup for testing.
type MockProviderLookup struct {
	providers map[string]*ProviderInfo
	err       error
}

func (m *MockProviderLookup) GetProvider(_ context.Context, name, namespace string) (*ProviderInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	key := namespace + "/" + name
	info, ok := m.providers[key]
	if !ok {
		return nil, fmt.Errorf("provider %s/%s not found", namespace, name)
	}
	return info, nil
}

func strPtr(s string) *string { return &s }

func TestResolve_Success(t *testing.T) {
	lookup := &MockProviderLookup{
		providers: map[string]*ProviderInfo{
			"default/my-openai": {
				Type:    "openai",
				APIKey:  "sk-test-key",
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-4",
			},
		},
	}

	judges := []v1alpha1.JudgeMapping{
		{
			Name:        "fast-judge",
			ProviderRef: v1alpha1.ProviderRef{Name: "my-openai"},
		},
	}

	jp := NewJudgeProvider(judges, lookup, "default")
	cfg, err := jp.Resolve(context.Background(), "fast-judge")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ProviderType != "openai" {
		t.Errorf("expected provider type openai, got %s", cfg.ProviderType)
	}
	if cfg.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", cfg.Model)
	}
	if cfg.APIKey != "sk-test-key" {
		t.Errorf("expected API key sk-test-key, got %s", cfg.APIKey)
	}
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected base URL https://api.openai.com/v1, got %s", cfg.BaseURL)
	}
}

func TestResolve_FallsBackToProviderModel(t *testing.T) {
	lookup := &MockProviderLookup{
		providers: map[string]*ProviderInfo{
			"default/my-claude": {
				Type:   "claude",
				APIKey: "sk-ant-test",
				Model:  "claude-sonnet-4-20250514",
			},
		},
	}

	judges := []v1alpha1.JudgeMapping{
		{
			Name:        "strong-judge",
			ProviderRef: v1alpha1.ProviderRef{Name: "my-claude"},
			// No model override — should use provider's model.
		},
	}

	jp := NewJudgeProvider(judges, lookup, "default")
	cfg, err := jp.Resolve(context.Background(), "strong-judge")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model claude-sonnet-4-20250514, got %s", cfg.Model)
	}
}

func TestResolve_UnknownJudgeName(t *testing.T) {
	lookup := &MockProviderLookup{}
	jp := NewJudgeProvider(nil, lookup, "default")

	_, err := jp.Resolve(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown judge name")
	}

	expected := fmt.Sprintf(errUnknownJudge, "nonexistent")
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestResolve_ProviderLookupFailure(t *testing.T) {
	lookup := &MockProviderLookup{
		err: fmt.Errorf("connection refused"),
	}

	judges := []v1alpha1.JudgeMapping{
		{
			Name:        "test-judge",
			ProviderRef: v1alpha1.ProviderRef{Name: "missing-provider"},
		},
	}

	jp := NewJudgeProvider(judges, lookup, "default")
	_, err := jp.Resolve(context.Background(), "test-judge")
	if err == nil {
		t.Fatal("expected error for provider lookup failure")
	}

	if got := err.Error(); got == "" {
		t.Error("expected non-empty error message")
	}
}

func TestResolve_MissingAPIKey(t *testing.T) {
	lookup := &MockProviderLookup{
		providers: map[string]*ProviderInfo{
			"default/no-key-provider": {
				Type:  "openai",
				Model: "gpt-4",
				// APIKey intentionally empty.
			},
		},
	}

	judges := []v1alpha1.JudgeMapping{
		{
			Name:        "bad-judge",
			ProviderRef: v1alpha1.ProviderRef{Name: "no-key-provider"},
		},
	}

	jp := NewJudgeProvider(judges, lookup, "default")
	_, err := jp.Resolve(context.Background(), "bad-judge")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}

	expected := fmt.Sprintf(errMissingAPIKey, "no-key-provider", "bad-judge")
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestResolve_MultipleJudges(t *testing.T) {
	lookup := &MockProviderLookup{
		providers: map[string]*ProviderInfo{
			"default/openai-provider": {
				Type:   "openai",
				APIKey: "sk-openai",
				Model:  "gpt-4",
			},
			"default/claude-provider": {
				Type:   "claude",
				APIKey: "sk-claude",
				Model:  "claude-sonnet-4-20250514",
			},
		},
	}

	judges := []v1alpha1.JudgeMapping{
		{
			Name:        "fast-judge",
			ProviderRef: v1alpha1.ProviderRef{Name: "openai-provider"},
		},
		{
			Name:        "strong-judge",
			ProviderRef: v1alpha1.ProviderRef{Name: "claude-provider"},
		},
	}

	jp := NewJudgeProvider(judges, lookup, "default")

	// Resolve fast-judge.
	fast, err := jp.Resolve(context.Background(), "fast-judge")
	if err != nil {
		t.Fatalf("unexpected error resolving fast-judge: %v", err)
	}
	if fast.ProviderType != "openai" || fast.Model != "gpt-4" {
		t.Errorf("fast-judge: got type=%s model=%s", fast.ProviderType, fast.Model)
	}

	// Resolve strong-judge.
	strong, err := jp.Resolve(context.Background(), "strong-judge")
	if err != nil {
		t.Fatalf("unexpected error resolving strong-judge: %v", err)
	}
	if strong.ProviderType != "claude" || strong.Model != "claude-sonnet-4-20250514" {
		t.Errorf("strong-judge: got type=%s model=%s", strong.ProviderType, strong.Model)
	}
}

func TestResolve_CrossNamespaceProviderRef(t *testing.T) {
	lookup := &MockProviderLookup{
		providers: map[string]*ProviderInfo{
			"shared/global-openai": {
				Type:   "openai",
				APIKey: "sk-global",
				Model:  "gpt-4",
			},
		},
	}

	judges := []v1alpha1.JudgeMapping{
		{
			Name: "cross-ns-judge",
			ProviderRef: v1alpha1.ProviderRef{
				Name:      "global-openai",
				Namespace: strPtr("shared"),
			},
		},
	}

	jp := NewJudgeProvider(judges, lookup, "default")
	cfg, err := jp.Resolve(context.Background(), "cross-ns-judge")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIKey != "sk-global" {
		t.Errorf("expected API key sk-global, got %s", cfg.APIKey)
	}
}

func TestResolve_MissingNamespace(t *testing.T) {
	lookup := &MockProviderLookup{}

	judges := []v1alpha1.JudgeMapping{
		{
			Name:        "test-judge",
			ProviderRef: v1alpha1.ProviderRef{Name: "some-provider"},
		},
	}

	// Empty default namespace and no namespace on providerRef.
	jp := NewJudgeProvider(judges, lookup, "")
	_, err := jp.Resolve(context.Background(), "test-judge")
	if err == nil {
		t.Fatal("expected error for missing namespace")
	}

	if err.Error() != errMissingNamespace {
		t.Errorf("expected error %q, got %q", errMissingNamespace, err.Error())
	}
}

func TestResolve_ExtraConfig(t *testing.T) {
	lookup := &MockProviderLookup{
		providers: map[string]*ProviderInfo{
			"default/azure-provider": {
				Type:   "azure-ai",
				APIKey: "azure-key",
				Model:  "gpt-4",
				ExtraConfig: map[string]string{
					"deployment": "my-deployment",
					"apiVersion": "2024-02-15",
				},
			},
		},
	}

	judges := []v1alpha1.JudgeMapping{
		{
			Name:        "azure-judge",
			ProviderRef: v1alpha1.ProviderRef{Name: "azure-provider"},
		},
	}

	jp := NewJudgeProvider(judges, lookup, "default")
	cfg, err := jp.Resolve(context.Background(), "azure-judge")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ExtraConfig["deployment"] != "my-deployment" {
		t.Errorf("expected deployment=my-deployment, got %s", cfg.ExtraConfig["deployment"])
	}
	if cfg.ExtraConfig["apiVersion"] != "2024-02-15" {
		t.Errorf("expected apiVersion=2024-02-15, got %s", cfg.ExtraConfig["apiVersion"])
	}
}

func TestJudgeNames(t *testing.T) {
	lookup := &MockProviderLookup{}

	judges := []v1alpha1.JudgeMapping{
		{Name: "alpha", ProviderRef: v1alpha1.ProviderRef{Name: "p1"}},
		{Name: "beta", ProviderRef: v1alpha1.ProviderRef{Name: "p2"}},
		{Name: "gamma", ProviderRef: v1alpha1.ProviderRef{Name: "p3"}},
	}

	jp := NewJudgeProvider(judges, lookup, "default")
	names := jp.JudgeNames()

	sort.Strings(names)
	expected := []string{"alpha", "beta", "gamma"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected name %q at index %d, got %q", expected[i], i, name)
		}
	}
}

func TestNewJudgeProvider_NilJudges(t *testing.T) {
	lookup := &MockProviderLookup{}
	jp := NewJudgeProvider(nil, lookup, "default")

	names := jp.JudgeNames()
	if len(names) != 0 {
		t.Errorf("expected 0 judge names, got %d", len(names))
	}
}

func TestNewJudgeProvider_EmptyJudges(t *testing.T) {
	lookup := &MockProviderLookup{}
	jp := NewJudgeProvider([]v1alpha1.JudgeMapping{}, lookup, "default")

	names := jp.JudgeNames()
	if len(names) != 0 {
		t.Errorf("expected 0 judge names, got %d", len(names))
	}
}
