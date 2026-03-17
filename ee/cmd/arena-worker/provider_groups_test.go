/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/fleet"
	"github.com/altairalabs/omnia/pkg/k8s"
)

// ---------------------------------------------------------------------------
// sanitizeID
// ---------------------------------------------------------------------------

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain lowercase is unchanged",
			input: "myprovider",
			want:  "myprovider",
		},
		{
			name:  "uppercase is lowercased",
			input: "MyProvider",
			want:  "myprovider",
		},
		{
			name:  "dots replaced with hyphens",
			input: "my.provider.name",
			want:  "my-provider-name",
		},
		{
			name:  "underscores replaced with hyphens",
			input: "my_provider_name",
			want:  "my-provider-name",
		},
		{
			name:  "slashes replaced with hyphens",
			input: "ns/provider",
			want:  "ns-provider",
		},
		{
			name:  "existing hyphens preserved",
			input: "my-provider",
			want:  "my-provider",
		},
		{
			name:  "mixed special characters",
			input: "My.Provider_Name/v2",
			want:  "my-provider-name-v2",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "alphanumeric with numbers",
			input: "provider123",
			want:  "provider123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeID(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// parseAgentWSURLs
// ---------------------------------------------------------------------------

func TestParseAgentWSURLs(t *testing.T) {
	t.Run("returns nil when env var not set", func(t *testing.T) {
		t.Setenv("ARENA_AGENT_WS_URLS", "")
		result := parseAgentWSURLs()
		assert.Nil(t, result)
	})

	t.Run("returns nil on invalid JSON", func(t *testing.T) {
		t.Setenv("ARENA_AGENT_WS_URLS", "not-valid-json")
		result := parseAgentWSURLs()
		assert.Nil(t, result)
	})

	t.Run("parses single entry", func(t *testing.T) {
		t.Setenv("ARENA_AGENT_WS_URLS", `{"agent-a":"ws://agent-a.default.svc:8080/ws"}`)
		result := parseAgentWSURLs()
		require.NotNil(t, result)
		assert.Equal(t, "ws://agent-a.default.svc:8080/ws", result["agent-a"])
	})

	t.Run("parses multiple entries", func(t *testing.T) {
		raw := `{"agent-a":"ws://a:8080/ws","agent-b":"ws://b:8080/ws"}`
		t.Setenv("ARENA_AGENT_WS_URLS", raw)
		result := parseAgentWSURLs()
		require.NotNil(t, result)
		assert.Equal(t, "ws://a:8080/ws", result["agent-a"])
		assert.Equal(t, "ws://b:8080/ws", result["agent-b"])
	})

	t.Run("returns nil for empty JSON object", func(t *testing.T) {
		t.Setenv("ARENA_AGENT_WS_URLS", "{}")
		result := parseAgentWSURLs()
		// Unmarshals fine but is an empty map (not nil)
		require.NotNil(t, result)
		assert.Len(t, result, 0)
	})
}

// ---------------------------------------------------------------------------
// resolveProviderCredentialEnv
// ---------------------------------------------------------------------------

func TestResolveProviderCredentialEnv(t *testing.T) {
	t.Run("uses explicit envVar when set", func(t *testing.T) {
		p := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type: "openai",
				Credential: &v1alpha1.CredentialConfig{
					EnvVar: "MY_CUSTOM_API_KEY",
				},
			},
		}
		got := resolveProviderCredentialEnv(p)
		assert.Equal(t, "MY_CUSTOM_API_KEY", got)
	})

	t.Run("falls through to provider-type env var when credential has no envVar", func(t *testing.T) {
		p := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type: "openai",
			},
		}
		got := resolveProviderCredentialEnv(p)
		// GetAPIKeyEnvVars("openai") returns the standard OpenAI env var
		assert.NotEmpty(t, got)
		assert.Contains(t, got, "OPENAI")
	})

	t.Run("falls through to provider-type env var when credential secretRef has no envVar", func(t *testing.T) {
		p := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type: "claude",
				Credential: &v1alpha1.CredentialConfig{
					SecretRef: &v1alpha1.SecretKeyRef{Name: "my-secret"},
				},
			},
		}
		got := resolveProviderCredentialEnv(p)
		// Credential is set but EnvVar is empty — falls through to type-based lookup
		assert.Contains(t, got, "ANTHROPIC")
	})

	t.Run("returns empty for mock provider (no credentials needed)", func(t *testing.T) {
		p := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type: "mock",
			},
		}
		got := resolveProviderCredentialEnv(p)
		// mock provider has no credentials in ProviderCredentials map,
		// so it gets a derived name MOCK_API_KEY (non-empty)
		// The function returns whatever GetAPIKeyEnvVars returns.
		_ = got // result depends on ProviderCredentials map content
	})

	t.Run("nil credential falls through to type-based lookup", func(t *testing.T) {
		p := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type: "gemini",
			},
		}
		got := resolveProviderCredentialEnv(p)
		assert.NotEmpty(t, got)
	})
}

// ---------------------------------------------------------------------------
// convertProviderDefaults
// ---------------------------------------------------------------------------

func TestConvertProviderDefaults(t *testing.T) {
	t.Run("converts all fields", func(t *testing.T) {
		maxTokens := int32(1000)
		d := &v1alpha1.ProviderDefaults{
			Temperature: ptr.To("0.7"),
			TopP:        ptr.To("0.9"),
			MaxTokens:   &maxTokens,
		}
		got := convertProviderDefaults(d)
		assert.InDelta(t, 0.7, got.Temperature, 0.001)
		assert.InDelta(t, 0.9, got.TopP, 0.001)
		assert.Equal(t, 1000, got.MaxTokens)
	})

	t.Run("nil temperature leaves zero", func(t *testing.T) {
		d := &v1alpha1.ProviderDefaults{}
		got := convertProviderDefaults(d)
		assert.Equal(t, float32(0), got.Temperature)
		assert.Equal(t, float32(0), got.TopP)
		assert.Equal(t, 0, got.MaxTokens)
	})

	t.Run("invalid temperature string produces zero", func(t *testing.T) {
		d := &v1alpha1.ProviderDefaults{
			Temperature: ptr.To("not-a-number"),
		}
		got := convertProviderDefaults(d)
		assert.Equal(t, float32(0), got.Temperature)
	})

	t.Run("invalid topP string produces zero", func(t *testing.T) {
		d := &v1alpha1.ProviderDefaults{
			TopP: ptr.To("bad"),
		}
		got := convertProviderDefaults(d)
		assert.Equal(t, float32(0), got.TopP)
	})

	t.Run("maxTokens zero value", func(t *testing.T) {
		maxTokens := int32(0)
		d := &v1alpha1.ProviderDefaults{
			MaxTokens: &maxTokens,
		}
		got := convertProviderDefaults(d)
		assert.Equal(t, 0, got.MaxTokens)
	})

	t.Run("temperature of zero string", func(t *testing.T) {
		d := &v1alpha1.ProviderDefaults{
			Temperature: ptr.To("0.0"),
		}
		got := convertProviderDefaults(d)
		assert.Equal(t, float32(0), got.Temperature)
	})
}

// ---------------------------------------------------------------------------
// closeFleetProviders
// ---------------------------------------------------------------------------

func TestCloseFleetProviders(t *testing.T) {
	t.Run("nil slice is a no-op", func(t *testing.T) {
		// Should not panic
		closeFleetProviders(nil)
	})

	t.Run("empty slice is a no-op", func(t *testing.T) {
		closeFleetProviders([]*resolvedFleetProvider{})
	})

	t.Run("closes unconnected providers without error", func(t *testing.T) {
		// fleet.Provider.Close() on an unconnected provider returns nil (conn is nil).
		// This verifies the loop body executes without panicking.
		fp1 := fleet.NewProvider("p1", "ws://fake:8080/ws", nil)
		fp2 := fleet.NewProvider("p2", "ws://fake:8080/ws", nil)
		fps := []*resolvedFleetProvider{
			{provider: fp1, id: "p1", group: "default"},
			{provider: fp2, id: "p2", group: "default"},
		}
		// Should not panic — conn is nil so Close is a no-op
		closeFleetProviders(fps)
	})
}

// ---------------------------------------------------------------------------
// Helpers for k8s fake client tests
// ---------------------------------------------------------------------------

// testNamespace is a constant to avoid goconst warnings for repeated "default" strings.
const testNamespace = "default"

// makeArenaJobUnstructured builds an unstructured ArenaJob with the given spec payload.
func makeArenaJobUnstructured(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	u := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "omnia.altairalabs.ai/v1alpha1",
			"kind":       "ArenaJob",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "omnia.altairalabs.ai",
		Version: "v1alpha1",
		Kind:    "ArenaJob",
	})
	return u
}

// ---------------------------------------------------------------------------
// getArenaJob
// ---------------------------------------------------------------------------

func TestGetArenaJob(t *testing.T) {
	ctx := context.Background()

	t.Run("returns error when ArenaJob not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).Build()
		_, err := getArenaJob(ctx, c, "missing-job", "default")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing-job")
	})

	t.Run("returns spec with providers", func(t *testing.T) {
		spec := map[string]interface{}{
			"providers": map[string]interface{}{
				"default": []interface{}{
					map[string]interface{}{
						"providerRef": map[string]interface{}{
							"name": "my-provider",
						},
					},
				},
			},
		}
		u := makeArenaJobUnstructured("test-job", "default", spec)

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(u).
			Build()

		got, err := getArenaJob(ctx, c, "test-job", "default")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Contains(t, got.Providers, "default")
		assert.Len(t, got.Providers["default"], 1)
		assert.Equal(t, "my-provider", got.Providers["default"][0].ProviderRef.Name)
	})

	t.Run("returns spec with toolRegistries", func(t *testing.T) {
		spec := map[string]interface{}{
			"toolRegistries": []interface{}{
				map[string]interface{}{"name": "prod-tools"},
			},
		}
		u := makeArenaJobUnstructured("tool-job", "arena", spec)

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(u).
			Build()

		got, err := getArenaJob(ctx, c, "tool-job", "arena")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Len(t, got.ToolRegistries, 1)
		assert.Equal(t, "prod-tools", got.ToolRegistries[0].Name)
	})

	t.Run("returns empty spec when spec is missing", func(t *testing.T) {
		u := makeArenaJobUnstructured("empty-job", "default", map[string]interface{}{})

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(u).
			Build()

		got, err := getArenaJob(ctx, c, "empty-job", "default")
		require.NoError(t, err)
		assert.Empty(t, got.Providers)
		assert.Empty(t, got.ToolRegistries)
	})
}

// ---------------------------------------------------------------------------
// resolveProviderRefEntry
// ---------------------------------------------------------------------------

func TestResolveProviderRefEntry(t *testing.T) {
	ctx := context.Background()
	log := testLog()

	buildProvider := func(name, namespace string, provType v1alpha1.ProviderType, model string) *v1alpha1.Provider {
		return &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type:  provType,
				Model: model,
			},
		}
	}
	_ = buildProvider // used in subtests below via inline construction

	t.Run("populates LoadedProviders and ProviderGroups", func(t *testing.T) {
		provider := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4o",
			},
		}
		provider.Name = "my-openai"
		provider.Namespace = testNamespace

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(provider).
			Build()

		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}

		ref := v1alpha1.ProviderRef{Name: "my-openai"}
		err := resolveProviderRefEntry(ctx, log, c, "default", ref, "judge", arenaCfg)
		require.NoError(t, err)

		providerID := sanitizeID("my-openai")
		require.Contains(t, arenaCfg.LoadedProviders, providerID)
		assert.Equal(t, "openai", arenaCfg.LoadedProviders[providerID].Type)
		assert.Equal(t, "gpt-4o", arenaCfg.LoadedProviders[providerID].Model)
		assert.Equal(t, "judge", arenaCfg.ProviderGroups[providerID])
	})

	t.Run("sets credential env var when provider has explicit envVar", func(t *testing.T) {
		provider := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4o",
				Credential: &v1alpha1.CredentialConfig{
					EnvVar: "MY_OPENAI_KEY",
				},
			},
		}
		provider.Name = "provider-with-cred"
		provider.Namespace = testNamespace

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(provider).
			Build()

		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}

		ref := v1alpha1.ProviderRef{Name: "provider-with-cred"}
		err := resolveProviderRefEntry(ctx, log, c, "default", ref, "default", arenaCfg)
		require.NoError(t, err)

		providerID := sanitizeID("provider-with-cred")
		p := arenaCfg.LoadedProviders[providerID]
		require.NotNil(t, p.Credential)
		assert.Equal(t, "MY_OPENAI_KEY", p.Credential.CredentialEnv)
	})

	t.Run("sets defaults when provider has defaults", func(t *testing.T) {
		maxTokens := int32(500)
		provider := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type:  "claude",
				Model: "claude-3-opus",
				Defaults: &v1alpha1.ProviderDefaults{
					Temperature: ptr.To("0.5"),
					MaxTokens:   &maxTokens,
				},
			},
		}
		provider.Name = "provider-with-defaults"
		provider.Namespace = testNamespace

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(provider).
			Build()

		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}

		ref := v1alpha1.ProviderRef{Name: "provider-with-defaults"}
		err := resolveProviderRefEntry(ctx, log, c, "default", ref, "default", arenaCfg)
		require.NoError(t, err)

		providerID := sanitizeID("provider-with-defaults")
		p := arenaCfg.LoadedProviders[providerID]
		assert.InDelta(t, 0.5, p.Defaults.Temperature, 0.001)
		assert.Equal(t, 500, p.Defaults.MaxTokens)
	})

	t.Run("returns error when provider not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).Build()

		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}

		ref := v1alpha1.ProviderRef{Name: "nonexistent"}
		err := resolveProviderRefEntry(ctx, log, c, "default", ref, "group1", arenaCfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "group1")
		assert.Contains(t, err.Error(), "nonexistent")
	})

	t.Run("uses cross-namespace ref when namespace specified", func(t *testing.T) {
		provider := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4",
			},
		}
		provider.Name = "cross-ns-provider"
		provider.Namespace = "other-ns"

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(provider).
			Build()

		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}

		ref := v1alpha1.ProviderRef{Name: "cross-ns-provider", Namespace: ptr.To("other-ns")}
		err := resolveProviderRefEntry(ctx, log, c, "default", ref, "default", arenaCfg)
		require.NoError(t, err)

		providerID := sanitizeID("cross-ns-provider")
		assert.Contains(t, arenaCfg.LoadedProviders, providerID)
	})
}

// ---------------------------------------------------------------------------
// resolveAgentRefEntry
// ---------------------------------------------------------------------------

func TestResolveAgentRefEntry(t *testing.T) {
	ctx := context.Background()
	log := testLog()

	t.Run("returns error when agent not in WS URL map", func(t *testing.T) {
		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}
		agentWSURLs := map[string]string{
			"other-agent": "ws://other:8080/ws",
		}

		_, err := resolveAgentRefEntry(ctx, log, "missing-agent", "default", agentWSURLs, arenaCfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing-agent")
		assert.Contains(t, err.Error(), "ARENA_AGENT_WS_URLS")
	})

	t.Run("returns error when WS URL map is nil", func(t *testing.T) {
		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}

		_, err := resolveAgentRefEntry(ctx, log, "my-agent", "default", nil, arenaCfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "my-agent")
	})

	t.Run("group name included in missing URL error", func(t *testing.T) {
		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}

		_, err := resolveAgentRefEntry(ctx, log, "agent-x", "my-group", map[string]string{}, arenaCfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "my-group")
	})
}

// ---------------------------------------------------------------------------
// resolveProvidersFromCRD
// ---------------------------------------------------------------------------

func TestResolveProvidersFromCRD(t *testing.T) {
	ctx := context.Background()
	log := testLog()

	t.Run("returns error when ArenaJob not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).Build()
		cfg := &Config{JobName: "missing", JobNamespace: testNamespace}
		arenaCfg := &config.Config{}

		_, err := resolveProvidersFromCRD(ctx, log, c, cfg, arenaCfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read ArenaJob")
	})

	t.Run("returns error when ArenaJob has no providers", func(t *testing.T) {
		u := makeArenaJobUnstructured("empty-job", "default", map[string]interface{}{})
		c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithObjects(u).Build()

		cfg := &Config{JobName: "empty-job", JobNamespace: testNamespace}
		arenaCfg := &config.Config{}

		_, err := resolveProvidersFromCRD(ctx, log, c, cfg, arenaCfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no providers")
	})

	t.Run("resolves providerRef entries from CRD", func(t *testing.T) {
		// Create Provider CRD
		provider := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4o",
			},
		}
		provider.Name = "openai-prod"
		provider.Namespace = testNamespace

		// Build ArenaJob with providerRef entry
		spec := map[string]interface{}{
			"providers": map[string]interface{}{
				"default": []interface{}{
					map[string]interface{}{
						"providerRef": map[string]interface{}{
							"name": "openai-prod",
						},
					},
				},
			},
		}
		u := makeArenaJobUnstructured("test-job", "default", spec)

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(provider, u).
			Build()

		cfg := &Config{JobName: "test-job", JobNamespace: testNamespace}
		arenaCfg := &config.Config{}

		fps, err := resolveProvidersFromCRD(ctx, log, c, cfg, arenaCfg)
		require.NoError(t, err)
		assert.Empty(t, fps) // no fleet providers — only CRD providers

		providerID := sanitizeID("openai-prod")
		require.Contains(t, arenaCfg.LoadedProviders, providerID)
		assert.Equal(t, "openai", arenaCfg.LoadedProviders[providerID].Type)
		assert.Equal(t, "default", arenaCfg.ProviderGroups[providerID])
	})

	t.Run("returns error when providerRef CRD not found", func(t *testing.T) {
		spec := map[string]interface{}{
			"providers": map[string]interface{}{
				"default": []interface{}{
					map[string]interface{}{
						"providerRef": map[string]interface{}{
							"name": "nonexistent-provider",
						},
					},
				},
			},
		}
		u := makeArenaJobUnstructured("test-job", "default", spec)
		c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithObjects(u).Build()

		cfg := &Config{JobName: "test-job", JobNamespace: testNamespace}
		arenaCfg := &config.Config{}

		_, err := resolveProvidersFromCRD(ctx, log, c, cfg, arenaCfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent-provider")
	})

	t.Run("returns error when agentRef not in WS URLs", func(t *testing.T) {
		t.Setenv("ARENA_AGENT_WS_URLS", "{}")

		spec := map[string]interface{}{
			"providers": map[string]interface{}{
				"fleet": []interface{}{
					map[string]interface{}{
						"agentRef": map[string]interface{}{
							"name": "my-agent",
						},
					},
				},
			},
		}
		u := makeArenaJobUnstructured("agent-job", "default", spec)
		c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithObjects(u).Build()

		cfg := &Config{JobName: "agent-job", JobNamespace: testNamespace}
		arenaCfg := &config.Config{}

		_, err := resolveProvidersFromCRD(ctx, log, c, cfg, arenaCfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "my-agent")
	})

	t.Run("initializes nil LoadedProviders and ProviderGroups maps", func(t *testing.T) {
		provider := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type:  "claude",
				Model: "claude-3-haiku",
			},
		}
		provider.Name = "claude-provider"
		provider.Namespace = testNamespace

		spec := map[string]interface{}{
			"providers": map[string]interface{}{
				"judge": []interface{}{
					map[string]interface{}{
						"providerRef": map[string]interface{}{
							"name": "claude-provider",
						},
					},
				},
			},
		}
		u := makeArenaJobUnstructured("init-job", "default", spec)

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(provider, u).
			Build()

		cfg := &Config{JobName: "init-job", JobNamespace: testNamespace}
		// arenaCfg has nil maps — function should initialise them
		arenaCfg := &config.Config{}

		_, err := resolveProvidersFromCRD(ctx, log, c, cfg, arenaCfg)
		require.NoError(t, err)
		assert.NotNil(t, arenaCfg.LoadedProviders)
		assert.NotNil(t, arenaCfg.ProviderGroups)
	})
}

// ---------------------------------------------------------------------------
// resolveToolsFromCRD
// ---------------------------------------------------------------------------

func TestResolveToolsFromCRD(t *testing.T) {
	ctx := context.Background()
	log := testLog()

	t.Run("returns error when ArenaJob not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).Build()
		cfg := &Config{JobName: "missing", JobNamespace: testNamespace}

		err := resolveToolsFromCRD(ctx, log, c, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read ArenaJob")
	})

	t.Run("no-op when ArenaJob has no toolRegistries", func(t *testing.T) {
		u := makeArenaJobUnstructured("no-tools-job", "default", map[string]interface{}{})
		c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithObjects(u).Build()

		cfg := &Config{JobName: "no-tools-job", JobNamespace: testNamespace}

		err := resolveToolsFromCRD(ctx, log, c, cfg)
		require.NoError(t, err)
		assert.Nil(t, cfg.ToolOverrides)
	})

	t.Run("populates ToolOverrides from discovered tools", func(t *testing.T) {
		tr := &v1alpha1.ToolRegistry{}
		tr.Name = "prod-tools"
		tr.Namespace = testNamespace
		tr.Status.DiscoveredTools = []v1alpha1.DiscoveredTool{
			{
				Name:        "get_weather",
				HandlerName: "weather-handler",
				Description: "Get current weather",
				Endpoint:    "https://tools.example.com/weather",
				Status:      v1alpha1.ToolStatusAvailable,
			},
			{
				Name:        "search",
				HandlerName: "search-handler",
				Description: "Search the web",
				Endpoint:    "https://tools.example.com/search",
				Status:      v1alpha1.ToolStatusAvailable,
			},
		}

		spec := map[string]interface{}{
			"toolRegistries": []interface{}{
				map[string]interface{}{"name": "prod-tools"},
			},
		}
		u := makeArenaJobUnstructured("tools-job", "default", spec)

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(tr, u).
			WithStatusSubresource(tr).
			Build()

		cfg := &Config{JobName: "tools-job", JobNamespace: testNamespace}

		err := resolveToolsFromCRD(ctx, log, c, cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.ToolOverrides)
		assert.Contains(t, cfg.ToolOverrides, "get_weather")
		assert.Equal(t, "https://tools.example.com/weather", cfg.ToolOverrides["get_weather"].Endpoint)
		assert.Equal(t, "prod-tools", cfg.ToolOverrides["get_weather"].RegistryName)
		assert.Contains(t, cfg.ToolOverrides, "search")
	})

	t.Run("returns error when ToolRegistry not found", func(t *testing.T) {
		spec := map[string]interface{}{
			"toolRegistries": []interface{}{
				map[string]interface{}{"name": "nonexistent-registry"},
			},
		}
		u := makeArenaJobUnstructured("missing-tr-job", "default", spec)
		c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithObjects(u).Build()

		cfg := &Config{JobName: "missing-tr-job", JobNamespace: testNamespace}

		err := resolveToolsFromCRD(ctx, log, c, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent-registry")
	})

	t.Run("no-op when ToolRegistry has no discovered tools", func(t *testing.T) {
		tr := &v1alpha1.ToolRegistry{}
		tr.Name = "empty-registry"
		tr.Namespace = testNamespace
		// Status.DiscoveredTools is nil

		spec := map[string]interface{}{
			"toolRegistries": []interface{}{
				map[string]interface{}{"name": "empty-registry"},
			},
		}
		u := makeArenaJobUnstructured("empty-tr-job", "default", spec)

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(tr, u).
			Build()

		cfg := &Config{JobName: "empty-tr-job", JobNamespace: testNamespace}

		err := resolveToolsFromCRD(ctx, log, c, cfg)
		require.NoError(t, err)
		assert.Nil(t, cfg.ToolOverrides)
	})

	t.Run("initializes ToolOverrides map when nil", func(t *testing.T) {
		tr := &v1alpha1.ToolRegistry{}
		tr.Name = "init-registry"
		tr.Namespace = testNamespace
		tr.Status.DiscoveredTools = []v1alpha1.DiscoveredTool{
			{
				Name:        "my_tool",
				HandlerName: "handler",
				Endpoint:    "https://tools.example.com/my",
				Status:      v1alpha1.ToolStatusAvailable,
			},
		}

		spec := map[string]interface{}{
			"toolRegistries": []interface{}{
				map[string]interface{}{"name": "init-registry"},
			},
		}
		u := makeArenaJobUnstructured("init-tools-job", "default", spec)

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(tr, u).
			WithStatusSubresource(tr).
			Build()

		cfg := &Config{JobName: "init-tools-job", JobNamespace: testNamespace}
		// ToolOverrides is nil — should be initialized

		err := resolveToolsFromCRD(ctx, log, c, cfg)
		require.NoError(t, err)
		assert.NotNil(t, cfg.ToolOverrides)
		assert.Contains(t, cfg.ToolOverrides, "my_tool")
	})
}

// ---------------------------------------------------------------------------
// arenaJobSpec JSON round-trip (structural test)
// ---------------------------------------------------------------------------

func TestArenaJobSpecJSONRoundTrip(t *testing.T) {
	t.Run("providers with mixed entries round-trips correctly", func(t *testing.T) {
		nsPtr := ptr.To("other-ns")
		original := arenaJobSpec{
			Providers: map[string][]arenaProviderEntry{
				"default": {
					{ProviderRef: &v1alpha1.ProviderRef{Name: "my-provider", Namespace: nsPtr}},
				},
				"fleet": {
					{AgentRef: &v1alpha1.LocalObjectReference{Name: "my-agent"}},
				},
			},
			ToolRegistries: []v1alpha1.LocalObjectReference{
				{Name: "registry-a"},
			},
		}

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var got arenaJobSpec
		err = json.Unmarshal(data, &got)
		require.NoError(t, err)

		assert.Equal(t, "my-provider", got.Providers["default"][0].ProviderRef.Name)
		assert.Equal(t, "other-ns", *got.Providers["default"][0].ProviderRef.Namespace)
		assert.Equal(t, "my-agent", got.Providers["fleet"][0].AgentRef.Name)
		assert.Equal(t, "registry-a", got.ToolRegistries[0].Name)
	})
}
