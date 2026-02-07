/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package binding

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/altairalabs/omnia/ee/pkg/arena/overrides"
)

func TestParseProviderAnnotations(t *testing.T) {
	t.Run("with binding annotations", func(t *testing.T) {
		dir := t.TempDir()

		// Write a provider YAML with annotations
		providerYAML := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: workspace-default-gpt4
  annotations:
    omnia.altairalabs.ai/provider-name: gpt4
    omnia.altairalabs.ai/provider-namespace: workspace-default
spec:
  id: workspace-default-gpt4
  type: openai
  model: gpt-4o
`
		if err := os.WriteFile(filepath.Join(dir, "gpt4.provider.yaml"), []byte(providerYAML), 0644); err != nil {
			t.Fatal(err)
		}

		cfg := &config.Config{
			Providers: []config.ProviderRef{
				{File: "gpt4.provider.yaml"},
			},
		}

		bindings, err := ParseProviderAnnotations(cfg, filepath.Join(dir, "config.arena.yaml"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(bindings) != 1 {
			t.Fatalf("expected 1 binding, got %d", len(bindings))
		}
		if bindings[0].ProviderID != "workspace-default-gpt4" {
			t.Errorf("expected provider ID workspace-default-gpt4, got %s", bindings[0].ProviderID)
		}
		if bindings[0].SourceName != "gpt4" {
			t.Errorf("expected source name gpt4, got %s", bindings[0].SourceName)
		}
		if bindings[0].SourceNamespace != "workspace-default" {
			t.Errorf("expected source namespace workspace-default, got %s", bindings[0].SourceNamespace)
		}
	})

	t.Run("without annotations", func(t *testing.T) {
		dir := t.TempDir()

		providerYAML := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: local-provider
spec:
  id: local-provider
  type: openai
  model: gpt-4o
`
		if err := os.WriteFile(filepath.Join(dir, "local.provider.yaml"), []byte(providerYAML), 0644); err != nil {
			t.Fatal(err)
		}

		cfg := &config.Config{
			Providers: []config.ProviderRef{
				{File: "local.provider.yaml"},
			},
		}

		bindings, err := ParseProviderAnnotations(cfg, filepath.Join(dir, "config.arena.yaml"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(bindings) != 0 {
			t.Fatalf("expected 0 bindings, got %d", len(bindings))
		}
	})

	t.Run("partial annotations", func(t *testing.T) {
		dir := t.TempDir()

		providerYAML := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: partial
  annotations:
    omnia.altairalabs.ai/provider-name: gpt4
spec:
  id: partial
  type: openai
`
		if err := os.WriteFile(filepath.Join(dir, "partial.provider.yaml"), []byte(providerYAML), 0644); err != nil {
			t.Fatal(err)
		}

		cfg := &config.Config{
			Providers: []config.ProviderRef{
				{File: "partial.provider.yaml"},
			},
		}

		bindings, err := ParseProviderAnnotations(cfg, filepath.Join(dir, "config.arena.yaml"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(bindings) != 0 {
			t.Fatalf("expected 0 bindings (partial annotations should be skipped), got %d", len(bindings))
		}
	})

	t.Run("invalid YAML", func(t *testing.T) {
		dir := t.TempDir()

		if err := os.WriteFile(filepath.Join(dir, "bad.provider.yaml"), []byte("not: valid: yaml: ["), 0644); err != nil {
			t.Fatal(err)
		}

		cfg := &config.Config{
			Providers: []config.ProviderRef{
				{File: "bad.provider.yaml"},
			},
		}

		bindings, err := ParseProviderAnnotations(cfg, filepath.Join(dir, "config.arena.yaml"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(bindings) != 0 {
			t.Fatalf("expected 0 bindings for invalid YAML, got %d", len(bindings))
		}
	})

	t.Run("missing file", func(t *testing.T) {
		dir := t.TempDir()

		cfg := &config.Config{
			Providers: []config.ProviderRef{
				{File: "missing.provider.yaml"},
			},
		}

		bindings, err := ParseProviderAnnotations(cfg, filepath.Join(dir, "config.arena.yaml"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(bindings) != 0 {
			t.Fatalf("expected 0 bindings for missing file, got %d", len(bindings))
		}
	})

	t.Run("empty providers", func(t *testing.T) {
		cfg := &config.Config{}
		bindings, err := ParseProviderAnnotations(cfg, "/some/path/config.yaml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bindings != nil {
			t.Fatalf("expected nil bindings, got %v", bindings)
		}
	})
}

func TestApplyBindings(t *testing.T) {
	t.Run("match found", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"workspace-default-gpt4": {
					ID:   "workspace-default-gpt4",
					Type: "openai",
				},
			},
		}

		bindings := []ProviderBinding{
			{
				ProviderID:      "workspace-default-gpt4",
				SourceName:      "gpt4",
				SourceNamespace: "workspace-default",
			},
		}

		registry := map[string]overrides.ProviderOverride{
			"workspace-default/gpt4": {
				ID:           "gpt4",
				Type:         "openai",
				SecretEnvVar: "OPENAI_API_KEY",
			},
		}

		count := ApplyBindings(cfg, bindings, registry, false)
		if count != 1 {
			t.Fatalf("expected 1 binding applied, got %d", count)
		}

		p := cfg.LoadedProviders["workspace-default-gpt4"]
		if p.Credential == nil {
			t.Fatal("expected credential to be set")
		}
		if p.Credential.CredentialEnv != "OPENAI_API_KEY" {
			t.Errorf("expected credential env OPENAI_API_KEY, got %s", p.Credential.CredentialEnv)
		}
	})

	t.Run("already has credentials", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"workspace-default-gpt4": {
					ID:   "workspace-default-gpt4",
					Type: "openai",
					Credential: &config.CredentialConfig{
						CredentialEnv: "MY_CUSTOM_KEY",
					},
				},
			},
		}

		bindings := []ProviderBinding{
			{
				ProviderID:      "workspace-default-gpt4",
				SourceName:      "gpt4",
				SourceNamespace: "workspace-default",
			},
		}

		registry := map[string]overrides.ProviderOverride{
			"workspace-default/gpt4": {
				ID:           "gpt4",
				SecretEnvVar: "OPENAI_API_KEY",
			},
		}

		count := ApplyBindings(cfg, bindings, registry, false)
		if count != 0 {
			t.Fatalf("expected 0 bindings (already has credentials), got %d", count)
		}

		// Verify original credential was preserved
		p := cfg.LoadedProviders["workspace-default-gpt4"]
		if p.Credential.CredentialEnv != "MY_CUSTOM_KEY" {
			t.Errorf("expected credential env MY_CUSTOM_KEY preserved, got %s", p.Credential.CredentialEnv)
		}
	})

	t.Run("key not found in registry", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"workspace-default-gpt4": {
					ID:   "workspace-default-gpt4",
					Type: "openai",
				},
			},
		}

		bindings := []ProviderBinding{
			{
				ProviderID:      "workspace-default-gpt4",
				SourceName:      "gpt4",
				SourceNamespace: "other-namespace",
			},
		}

		registry := map[string]overrides.ProviderOverride{
			"workspace-default/gpt4": {
				ID:           "gpt4",
				SecretEnvVar: "OPENAI_API_KEY",
			},
		}

		count := ApplyBindings(cfg, bindings, registry, false)
		if count != 0 {
			t.Fatalf("expected 0 bindings (key mismatch), got %d", count)
		}
	})

	t.Run("empty bindings", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"test": {ID: "test", Type: "openai"},
			},
		}
		registry := map[string]overrides.ProviderOverride{
			"ns/test": {ID: "test", SecretEnvVar: "KEY"},
		}

		count := ApplyBindings(cfg, nil, registry, false)
		if count != 0 {
			t.Fatalf("expected 0 bindings for empty bindings, got %d", count)
		}
	})

	t.Run("empty registry", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"test": {ID: "test", Type: "openai"},
			},
		}
		bindings := []ProviderBinding{
			{ProviderID: "test", SourceName: "test", SourceNamespace: "ns"},
		}

		count := ApplyBindings(cfg, bindings, nil, false)
		if count != 0 {
			t.Fatalf("expected 0 bindings for empty registry, got %d", count)
		}
	})

	t.Run("platform injection", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"workspace-default-bedrock": {
					ID:   "workspace-default-bedrock",
					Type: "bedrock",
				},
			},
		}

		bindings := []ProviderBinding{
			{
				ProviderID:      "workspace-default-bedrock",
				SourceName:      "bedrock",
				SourceNamespace: "workspace-default",
			},
		}

		registry := map[string]overrides.ProviderOverride{
			"workspace-default/bedrock": {
				ID:           "bedrock",
				Type:         "bedrock",
				SecretEnvVar: "AWS_ACCESS_KEY_ID",
				Platform: &overrides.PlatformOverride{
					Type:   "aws",
					Region: "us-west-2",
				},
			},
		}

		count := ApplyBindings(cfg, bindings, registry, false)
		if count != 1 {
			t.Fatalf("expected 1 binding, got %d", count)
		}

		p := cfg.LoadedProviders["workspace-default-bedrock"]
		if p.Platform == nil {
			t.Fatal("expected platform to be injected")
		}
		if p.Platform.Type != "aws" {
			t.Errorf("expected platform type aws, got %s", p.Platform.Type)
		}
		if p.Platform.Region != "us-west-2" {
			t.Errorf("expected region us-west-2, got %s", p.Platform.Region)
		}
	})

	t.Run("credential file binding", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"workspace-default-vertex": {
					ID:   "workspace-default-vertex",
					Type: "vertex",
				},
			},
		}

		bindings := []ProviderBinding{
			{
				ProviderID:      "workspace-default-vertex",
				SourceName:      "vertex",
				SourceNamespace: "workspace-default",
			},
		}

		registry := map[string]overrides.ProviderOverride{
			"workspace-default/vertex": {
				ID:             "vertex",
				Type:           "vertex",
				CredentialFile: "/var/run/secrets/gcp/key.json",
			},
		}

		count := ApplyBindings(cfg, bindings, registry, false)
		if count != 1 {
			t.Fatalf("expected 1 binding, got %d", count)
		}

		p := cfg.LoadedProviders["workspace-default-vertex"]
		if p.Credential == nil {
			t.Fatal("expected credential to be set")
		}
		if p.Credential.CredentialFile != "/var/run/secrets/gcp/key.json" {
			t.Errorf("expected credential file path, got %s", p.Credential.CredentialFile)
		}
	})
}

func TestApplyNameMatching(t *testing.T) {
	t.Run("match found", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"workspace-default-gpt4": {
					ID:   "workspace-default-gpt4",
					Type: "openai",
				},
			},
		}

		registry := map[string]overrides.ProviderOverride{
			"workspace-default/gpt4": {
				ID:           "gpt4",
				Type:         "openai",
				SecretEnvVar: "OPENAI_API_KEY",
			},
		}

		count := ApplyNameMatching(cfg, registry, false)
		if count != 1 {
			t.Fatalf("expected 1 match, got %d", count)
		}

		p := cfg.LoadedProviders["workspace-default-gpt4"]
		if p.Credential == nil {
			t.Fatal("expected credential to be set")
		}
		if p.Credential.CredentialEnv != "OPENAI_API_KEY" {
			t.Errorf("expected OPENAI_API_KEY, got %s", p.Credential.CredentialEnv)
		}
	})

	t.Run("no match", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"unrelated-provider": {
					ID:   "unrelated-provider",
					Type: "openai",
				},
			},
		}

		registry := map[string]overrides.ProviderOverride{
			"workspace-default/gpt4": {
				ID:           "gpt4",
				SecretEnvVar: "OPENAI_API_KEY",
			},
		}

		count := ApplyNameMatching(cfg, registry, false)
		if count != 0 {
			t.Fatalf("expected 0 matches, got %d", count)
		}
	})

	t.Run("skip mock and ollama", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"workspace-default-mock": {
					ID:   "workspace-default-mock",
					Type: "mock",
				},
				"workspace-default-ollama": {
					ID:   "workspace-default-ollama",
					Type: "ollama",
				},
			},
		}

		registry := map[string]overrides.ProviderOverride{
			"workspace-default/mock": {
				ID:           "mock",
				SecretEnvVar: "MOCK_KEY",
			},
			"workspace-default/ollama": {
				ID:           "ollama",
				SecretEnvVar: "OLLAMA_KEY",
			},
		}

		count := ApplyNameMatching(cfg, registry, false)
		if count != 0 {
			t.Fatalf("expected 0 matches (mock/ollama should be skipped), got %d", count)
		}
	})

	t.Run("skip already credentialed", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"workspace-default-gpt4": {
					ID:   "workspace-default-gpt4",
					Type: "openai",
					Credential: &config.CredentialConfig{
						CredentialEnv: "EXISTING_KEY",
					},
				},
			},
		}

		registry := map[string]overrides.ProviderOverride{
			"workspace-default/gpt4": {
				ID:           "gpt4",
				SecretEnvVar: "OPENAI_API_KEY",
			},
		}

		count := ApplyNameMatching(cfg, registry, false)
		if count != 0 {
			t.Fatalf("expected 0 matches (already has credentials), got %d", count)
		}
	})

	t.Run("empty registry", func(t *testing.T) {
		cfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"test": {ID: "test", Type: "openai"},
			},
		}
		count := ApplyNameMatching(cfg, nil, false)
		if count != 0 {
			t.Fatalf("expected 0 for empty registry, got %d", count)
		}
	})

	t.Run("empty loaded providers", func(t *testing.T) {
		cfg := &config.Config{}
		registry := map[string]overrides.ProviderOverride{
			"ns/test": {ID: "test", SecretEnvVar: "KEY"},
		}
		count := ApplyNameMatching(cfg, registry, false)
		if count != 0 {
			t.Fatalf("expected 0 for empty providers, got %d", count)
		}
	})
}

func TestHasCredentials(t *testing.T) {
	tests := []struct {
		name     string
		provider *config.Provider
		expected bool
	}{
		{
			name:     "nil credential",
			provider: &config.Provider{ID: "test"},
			expected: false,
		},
		{
			name: "empty credential",
			provider: &config.Provider{
				ID:         "test",
				Credential: &config.CredentialConfig{},
			},
			expected: false,
		},
		{
			name: "with API key",
			provider: &config.Provider{
				ID: "test",
				Credential: &config.CredentialConfig{
					APIKey: "sk-test",
				},
			},
			expected: true,
		},
		{
			name: "with credential env",
			provider: &config.Provider{
				ID: "test",
				Credential: &config.CredentialConfig{
					CredentialEnv: "OPENAI_API_KEY",
				},
			},
			expected: true,
		},
		{
			name: "with credential file",
			provider: &config.Provider{
				ID: "test",
				Credential: &config.CredentialConfig{
					CredentialFile: "/path/to/key",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasCredentials(tt.provider)
			if result != tt.expected {
				t.Errorf("hasCredentials() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRequiresCredentials(t *testing.T) {
	tests := []struct {
		providerType string
		expected     bool
	}{
		{"openai", true},
		{"claude", true},
		{"bedrock", true},
		{"mock", false},
		{"ollama", false},
	}

	for _, tt := range tests {
		t.Run(tt.providerType, func(t *testing.T) {
			result := requiresCredentials(tt.providerType)
			if result != tt.expected {
				t.Errorf("requiresCredentials(%s) = %v, want %v", tt.providerType, result, tt.expected)
			}
		})
	}
}
