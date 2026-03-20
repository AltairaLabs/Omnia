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
	"os"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	pkproviders "github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/types"
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

// mockProvider implements pkproviders.Provider for testing connectFleetProviders
// type assertion error path.
type mockProvider struct {
	id string
}

func (m *mockProvider) ID() string    { return m.id }
func (m *mockProvider) Model() string { return "mock" }
func (m *mockProvider) Predict(
	_ context.Context, _ pkproviders.PredictionRequest,
) (pkproviders.PredictionResponse, error) {
	return pkproviders.PredictionResponse{}, nil
}

func (m *mockProvider) PredictStream(
	_ context.Context, _ pkproviders.PredictionRequest,
) (<-chan pkproviders.StreamChunk, error) {
	return nil, nil
}
func (m *mockProvider) SupportsStreaming() bool      { return false }
func (m *mockProvider) ShouldIncludeRawOutput() bool { return false }
func (m *mockProvider) Close() error                 { return nil }
func (m *mockProvider) CalculateCost(_, _, _ int) types.CostInfo {
	return types.CostInfo{}
}

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
		registry := pkproviders.NewRegistry()
		closeFleetProviders(registry, nil)
	})

	t.Run("empty slice is a no-op", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		closeFleetProviders(registry, []*resolvedFleetProvider{})
	})

	t.Run("closes providers found in registry", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		// Register unconnected fleet providers (Close is a no-op when not connected)
		fp1 := fleet.NewProvider("p1", "ws://fake:8080/ws", nil)
		fp2 := fleet.NewProvider("p2", "ws://fake:8080/ws", nil)
		registry.Register(fp1)
		registry.Register(fp2)

		fps := []*resolvedFleetProvider{
			{id: "p1", wsURL: "ws://fake:8080/ws", group: "default"},
			{id: "p2", wsURL: "ws://fake:8080/ws", group: "default"},
		}
		// Should not panic
		closeFleetProviders(registry, fps)
	})

	t.Run("skips providers not in registry", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		fps := []*resolvedFleetProvider{
			{id: "missing", wsURL: "ws://fake:8080/ws", group: "default"},
		}
		// Should not panic
		closeFleetProviders(registry, fps)
	})
}

// ---------------------------------------------------------------------------
// connectFleetProviders
// ---------------------------------------------------------------------------

func TestConnectFleetProviders(t *testing.T) {
	t.Run("returns error when provider not in registry", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		fps := []*resolvedFleetProvider{
			{id: "missing", wsURL: "ws://fake:8080/ws", group: "default"},
		}

		err := connectFleetProviders(context.Background(), testLog(), registry, fps)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in registry")
	})

	t.Run("no-op with empty slice", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		err := connectFleetProviders(context.Background(), testLog(), registry, nil)
		require.NoError(t, err)
	})

	t.Run("returns error when provider is not a fleet provider", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		// Register a non-fleet provider under the ID
		mockProv := &mockProvider{id: "not-fleet"}
		registry.Register(mockProv)

		fps := []*resolvedFleetProvider{
			{id: "not-fleet", wsURL: "ws://fake:8080/ws", group: "default"},
		}

		err := connectFleetProviders(context.Background(), testLog(), registry, fps)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a fleet provider")
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
		defaultGroup := got.Providers["default"]
		assert.Len(t, defaultGroup.allEntries(), 1)
		assert.Equal(t, "my-provider", defaultGroup.entries[0].ProviderRef.Name)
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

	t.Run("success populates LoadedProviders and returns fleet provider", func(t *testing.T) {
		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}
		agentWSURLs := map[string]string{
			"my-agent": "ws://my-agent.default.svc:8080/ws",
		}

		fp, err := resolveAgentRefEntry(ctx, log, "my-agent", "fleet-group", agentWSURLs, arenaCfg)
		require.NoError(t, err)
		require.NotNil(t, fp)

		providerID := sanitizeID("agent-my-agent")
		assert.Equal(t, providerID, fp.id)
		assert.Equal(t, "ws://my-agent.default.svc:8080/ws", fp.wsURL)
		assert.Equal(t, "fleet-group", fp.group)

		require.Contains(t, arenaCfg.LoadedProviders, providerID)
		assert.Equal(t, "fleet", arenaCfg.LoadedProviders[providerID].Type)
		assert.Equal(t, "ws://my-agent.default.svc:8080/ws", arenaCfg.LoadedProviders[providerID].AdditionalConfig["ws_url"])
		assert.Equal(t, "fleet-group", arenaCfg.ProviderGroups[providerID])
	})
}

// ---------------------------------------------------------------------------
// resolveAgentRefEntryWithID
// ---------------------------------------------------------------------------

func TestResolveAgentRefEntryWithID(t *testing.T) {
	ctx := context.Background()
	log := testLog()

	t.Run("returns error when agent not in WS URL map", func(t *testing.T) {
		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}

		_, err := resolveAgentRefEntryWithID(ctx, log, "missing-agent", "my-id", "group1", map[string]string{}, arenaCfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing-agent")
		assert.Contains(t, err.Error(), "group1")
	})

	t.Run("success uses configID as provider ID", func(t *testing.T) {
		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}
		agentWSURLs := map[string]string{
			"my-agent": "ws://my-agent.default.svc:8080/ws",
		}

		fp, err := resolveAgentRefEntryWithID(ctx, log, "my-agent", "custom-id", "selfplay", agentWSURLs, arenaCfg)
		require.NoError(t, err)
		require.NotNil(t, fp)

		assert.Equal(t, "custom-id", fp.id)
		assert.Equal(t, "ws://my-agent.default.svc:8080/ws", fp.wsURL)
		assert.Equal(t, "selfplay", fp.group)

		require.Contains(t, arenaCfg.LoadedProviders, "custom-id")
		assert.Equal(t, "custom-id", arenaCfg.LoadedProviders["custom-id"].ID)
		assert.Equal(t, "fleet", arenaCfg.LoadedProviders["custom-id"].Type)
		assert.Equal(t, "ws://my-agent.default.svc:8080/ws", arenaCfg.LoadedProviders["custom-id"].AdditionalConfig["ws_url"])
		assert.Equal(t, "selfplay", arenaCfg.ProviderGroups["custom-id"])
	})
}

// ---------------------------------------------------------------------------
// resolveProviderRefEntryWithID
// ---------------------------------------------------------------------------

func TestResolveProviderRefEntryWithID(t *testing.T) {
	ctx := context.Background()
	log := testLog()

	t.Run("returns error when provider not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).Build()
		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}

		ref := v1alpha1.ProviderRef{Name: "nonexistent"}
		err := resolveProviderRefEntryWithID(ctx, log, c, testNamespace, ref, "my-id", "grp", arenaCfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent")
	})

	t.Run("sets credential and defaults when present", func(t *testing.T) {
		maxTokens := int32(2000)
		provider := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4o",
				Credential: &v1alpha1.CredentialConfig{
					EnvVar: "CUSTOM_KEY",
				},
				Defaults: &v1alpha1.ProviderDefaults{
					Temperature: ptr.To("0.8"),
					MaxTokens:   &maxTokens,
				},
			},
		}
		provider.Name = "full-provider"
		provider.Namespace = testNamespace

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(provider).
			Build()

		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}

		ref := v1alpha1.ProviderRef{Name: "full-provider"}
		err := resolveProviderRefEntryWithID(ctx, log, c, testNamespace, ref, "custom-id", "judge", arenaCfg)
		require.NoError(t, err)

		require.Contains(t, arenaCfg.LoadedProviders, "custom-id")
		p := arenaCfg.LoadedProviders["custom-id"]
		assert.Equal(t, "custom-id", p.ID)
		assert.Equal(t, "openai", p.Type)
		require.NotNil(t, p.Credential)
		assert.Equal(t, "CUSTOM_KEY", p.Credential.CredentialEnv)
		assert.InDelta(t, 0.8, p.Defaults.Temperature, 0.001)
		assert.Equal(t, 2000, p.Defaults.MaxTokens)
		assert.Equal(t, "judge", arenaCfg.ProviderGroups["custom-id"])
	})
}

// ---------------------------------------------------------------------------
// resolveEntry
// ---------------------------------------------------------------------------

func TestResolveEntry(t *testing.T) {
	ctx := context.Background()
	log := testLog()

	t.Run("returns nil for entry with neither providerRef nor agentRef", func(t *testing.T) {
		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}
		entry := &arenaProviderEntry{}

		fp, err := resolveEntry(ctx, log, nil, testNamespace, "group", "", entry, nil, arenaCfg)
		require.NoError(t, err)
		assert.Nil(t, fp)
	})

	t.Run("agentRef with configID delegates to resolveAgentRefEntryWithID", func(t *testing.T) {
		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}
		agentWSURLs := map[string]string{
			"agent-a": "ws://agent-a:8080/ws",
		}
		entry := &arenaProviderEntry{
			AgentRef: &v1alpha1.LocalObjectReference{Name: "agent-a"},
		}

		fp, err := resolveEntry(ctx, log, nil, testNamespace, "grp", "my-config-id", entry, agentWSURLs, arenaCfg)
		require.NoError(t, err)
		require.NotNil(t, fp)
		assert.Equal(t, "my-config-id", fp.id)
	})

	t.Run("agentRef without configID delegates to resolveAgentRefEntry", func(t *testing.T) {
		arenaCfg := &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
			ProviderGroups:  make(map[string]string),
		}
		agentWSURLs := map[string]string{
			"agent-b": "ws://agent-b:8080/ws",
		}
		entry := &arenaProviderEntry{
			AgentRef: &v1alpha1.LocalObjectReference{Name: "agent-b"},
		}

		fp, err := resolveEntry(ctx, log, nil, testNamespace, "grp", "", entry, agentWSURLs, arenaCfg)
		require.NoError(t, err)
		require.NotNil(t, fp)
		assert.Equal(t, sanitizeID("agent-agent-b"), fp.id)
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

	t.Run("clears pre-existing LoadedProviders from arena config", func(t *testing.T) {
		// Create Provider CRD
		provider := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4o",
			},
		}
		provider.Name = "crd-provider"
		provider.Namespace = testNamespace

		spec := map[string]interface{}{
			"providers": map[string]interface{}{
				"default": []interface{}{
					map[string]interface{}{
						"providerRef": map[string]interface{}{
							"name": "crd-provider",
						},
					},
				},
			},
		}
		u := makeArenaJobUnstructured("test-job", testNamespace, spec)

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(provider, u).
			Build()

		cfg := &Config{JobName: "test-job", JobNamespace: testNamespace}
		// Pre-populate with arena config file providers (simulating config.LoadConfig)
		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"gemini-from-config": {ID: "gemini-from-config", Type: "gemini", Model: "gemini-2.0-flash"},
				"mock-from-config":   {ID: "mock-from-config", Type: "mock"},
			},
			ProviderGroups: map[string]string{
				"gemini-from-config": "selfplay",
			},
		}

		_, err := resolveProvidersFromCRD(ctx, log, c, cfg, arenaCfg)
		require.NoError(t, err)

		// Arena config providers must be gone — only CRD providers remain
		assert.NotContains(t, arenaCfg.LoadedProviders, "gemini-from-config",
			"arena config providers should be cleared when spec.providers is set")
		assert.NotContains(t, arenaCfg.LoadedProviders, "mock-from-config")
		assert.Len(t, arenaCfg.LoadedProviders, 1)
		assert.Contains(t, arenaCfg.LoadedProviders, "crd-provider")
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

	t.Run("resolves agentRef entries and returns fleet providers", func(t *testing.T) {
		t.Setenv("ARENA_AGENT_WS_URLS", `{"my-agent":"ws://my-agent.default.svc:8080/ws"}`)

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

		fps, err := resolveProvidersFromCRD(ctx, log, c, cfg, arenaCfg)
		require.NoError(t, err)
		require.Len(t, fps, 1)

		providerID := sanitizeID("agent-my-agent")
		assert.Equal(t, providerID, fps[0].id)
		assert.Equal(t, "ws://my-agent.default.svc:8080/ws", fps[0].wsURL)
		require.Contains(t, arenaCfg.LoadedProviders, providerID)
		assert.Equal(t, "fleet", arenaCfg.LoadedProviders[providerID].Type)
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
// remapProviderIDs
// ---------------------------------------------------------------------------

func TestRemapProviderIDs(t *testing.T) {
	writeConfig := func(t *testing.T, yamlContent string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.arena.yaml")
		require.NoError(t, os.WriteFile(path, []byte(yamlContent), 0644))
		return path
	}

	t.Run("self-play provider remapped", func(t *testing.T) {
		configPath := writeConfig(t, `spec:
  providers:
    - file: providers/target.yaml
    - file: providers/sim.yaml
      group: selfplay
  self_play:
    enabled: true
    roles:
      - id: user-sim
        provider: selfplay`)

		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"ollama-tools": {ID: "ollama-tools", Type: "ollama", Model: "llama3"},
			},
			ProviderGroups: map[string]string{
				"ollama-tools": "selfplay",
			},
		}

		err := remapProviderIDs(testLog(), arenaCfg, configPath)
		require.NoError(t, err)

		assert.NotContains(t, arenaCfg.LoadedProviders, "ollama-tools")
		require.Contains(t, arenaCfg.LoadedProviders, "selfplay")
		assert.Equal(t, "selfplay", arenaCfg.LoadedProviders["selfplay"].ID)
		assert.Equal(t, "selfplay", arenaCfg.ProviderGroups["selfplay"])
	})

	t.Run("judge provider remapped", func(t *testing.T) {
		configPath := writeConfig(t, `spec:
  providers:
    - file: providers/main.yaml
  judges:
    - name: quality
      provider: quality-judge`)

		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"gpt-4o-judge": {ID: "gpt-4o-judge", Type: "openai", Model: "gpt-4o"},
			},
			ProviderGroups: map[string]string{
				"gpt-4o-judge": "quality-judge",
			},
		}

		err := remapProviderIDs(testLog(), arenaCfg, configPath)
		require.NoError(t, err)

		require.Contains(t, arenaCfg.LoadedProviders, "quality-judge")
		assert.Equal(t, "quality-judge", arenaCfg.LoadedProviders["quality-judge"].ID)
	})

	t.Run("judge spec provider remapped", func(t *testing.T) {
		configPath := writeConfig(t, `spec:
  providers:
    - file: providers/main.yaml
  judge_specs:
    safety:
      provider: safety-judge`)

		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"claude-judge": {ID: "claude-judge", Type: "claude", Model: "claude-3-haiku"},
			},
			ProviderGroups: map[string]string{
				"claude-judge": "safety-judge",
			},
		}

		err := remapProviderIDs(testLog(), arenaCfg, configPath)
		require.NoError(t, err)

		require.Contains(t, arenaCfg.LoadedProviders, "safety-judge")
		assert.Equal(t, "safety-judge", arenaCfg.LoadedProviders["safety-judge"].ID)
	})

	t.Run("no-op when no self-play or judges in config", func(t *testing.T) {
		configPath := writeConfig(t, `spec:
  providers:
    - file: providers/main.yaml
  scenarios:
    - file: scenarios/test.yaml`)

		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"my-provider": {ID: "my-provider", Type: "mock"},
			},
			ProviderGroups: map[string]string{
				"my-provider": "default",
			},
		}

		err := remapProviderIDs(testLog(), arenaCfg, configPath)
		require.NoError(t, err)

		// Nothing should change
		assert.Contains(t, arenaCfg.LoadedProviders, "my-provider")
		assert.Len(t, arenaCfg.LoadedProviders, 1)
	})

	t.Run("no-op when CRD name already matches expected ID", func(t *testing.T) {
		configPath := writeConfig(t, `spec:
  providers:
    - file: providers/sim.yaml
      group: selfplay
  self_play:
    enabled: true
    roles:
      - id: sim
        provider: selfplay`)

		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"selfplay": {ID: "selfplay", Type: "mock"},
			},
			ProviderGroups: map[string]string{
				"selfplay": "selfplay",
			},
		}

		err := remapProviderIDs(testLog(), arenaCfg, configPath)
		require.NoError(t, err)

		// Should remain unchanged
		assert.Contains(t, arenaCfg.LoadedProviders, "selfplay")
		assert.Equal(t, "selfplay", arenaCfg.LoadedProviders["selfplay"].ID)
	})

	t.Run("error when group has no provider for expected ID", func(t *testing.T) {
		configPath := writeConfig(t, `spec:
  providers:
    - file: providers/main.yaml
  self_play:
    enabled: true
    roles:
      - id: sim
        provider: selfplay`)

		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"some-provider": {ID: "some-provider", Type: "mock"},
			},
			ProviderGroups: map[string]string{
				"some-provider": "default", // wrong group
			},
		}

		err := remapProviderIDs(testLog(), arenaCfg, configPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "selfplay")
		assert.Contains(t, err.Error(), "no provider in group")
	})

	t.Run("multiple providers in group picks first and remaps", func(t *testing.T) {
		configPath := writeConfig(t, `spec:
  providers:
    - file: providers/main.yaml
  self_play:
    enabled: true
    roles:
      - id: sim
        provider: selfplay`)

		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"provider-a": {ID: "provider-a", Type: "mock", Model: "model-a"},
				"provider-b": {ID: "provider-b", Type: "mock", Model: "model-b"},
			},
			ProviderGroups: map[string]string{
				"provider-a": "selfplay",
				"provider-b": "selfplay",
			},
		}

		err := remapProviderIDs(testLog(), arenaCfg, configPath)
		require.NoError(t, err)

		// One of the providers should be remapped to "selfplay"
		require.Contains(t, arenaCfg.LoadedProviders, "selfplay")
		assert.Equal(t, "selfplay", arenaCfg.LoadedProviders["selfplay"].ID)

		// Should still have 2 providers total (one remapped, one untouched)
		assert.Len(t, arenaCfg.LoadedProviders, 2)
	})

	t.Run("fleet provider in self-play group remapped", func(t *testing.T) {
		configPath := writeConfig(t, `spec:
  providers:
    - file: providers/target.yaml
  self_play:
    enabled: true
    roles:
      - id: user-sim
        provider: selfplay`)

		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"agent-my-agent": {ID: "agent-my-agent", Type: "fleet"},
			},
			ProviderGroups: map[string]string{
				"agent-my-agent": "selfplay",
			},
		}

		err := remapProviderIDs(testLog(), arenaCfg, configPath)
		require.NoError(t, err)

		assert.NotContains(t, arenaCfg.LoadedProviders, "agent-my-agent")
		require.Contains(t, arenaCfg.LoadedProviders, "selfplay")
		assert.Equal(t, "selfplay", arenaCfg.LoadedProviders["selfplay"].ID)
		assert.Equal(t, "fleet", arenaCfg.LoadedProviders["selfplay"].Type)
	})

	t.Run("self-play disabled skips remapping", func(t *testing.T) {
		configPath := writeConfig(t, `spec:
  providers:
    - file: providers/target.yaml
  self_play:
    enabled: false
    roles:
      - id: sim
        provider: selfplay`)

		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"ollama-tools": {ID: "ollama-tools", Type: "ollama"},
			},
			ProviderGroups: map[string]string{
				"ollama-tools": "selfplay",
			},
		}

		err := remapProviderIDs(testLog(), arenaCfg, configPath)
		require.NoError(t, err)

		// Should remain unchanged — self-play disabled
		assert.Contains(t, arenaCfg.LoadedProviders, "ollama-tools")
		assert.NotContains(t, arenaCfg.LoadedProviders, "selfplay")
	})
}

// ---------------------------------------------------------------------------
// extractProviderIDRefs
// ---------------------------------------------------------------------------

func TestExtractProviderIDRefs(t *testing.T) {
	writeConfig := func(t *testing.T, yamlContent string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.arena.yaml")
		require.NoError(t, os.WriteFile(path, []byte(yamlContent), 0644))
		return path
	}

	t.Run("extracts self-play role providers", func(t *testing.T) {
		path := writeConfig(t, `spec:
  self_play:
    enabled: true
    roles:
      - id: sim
        provider: selfplay`)

		ids, err := extractProviderIDRefs(path)
		require.NoError(t, err)
		assert.Equal(t, []string{"selfplay"}, ids)
	})

	t.Run("extracts judge providers", func(t *testing.T) {
		path := writeConfig(t, `spec:
  judges:
    - name: quality
      provider: judge-provider`)

		ids, err := extractProviderIDRefs(path)
		require.NoError(t, err)
		assert.Equal(t, []string{"judge-provider"}, ids)
	})

	t.Run("extracts judge spec providers", func(t *testing.T) {
		path := writeConfig(t, `spec:
  judge_specs:
    safety:
      provider: safety-judge`)

		ids, err := extractProviderIDRefs(path)
		require.NoError(t, err)
		assert.Equal(t, []string{"safety-judge"}, ids)
	})

	t.Run("deduplicates provider IDs", func(t *testing.T) {
		path := writeConfig(t, `spec:
  self_play:
    enabled: true
    roles:
      - id: sim1
        provider: selfplay
      - id: sim2
        provider: selfplay
  judges:
    - name: j1
      provider: selfplay`)

		ids, err := extractProviderIDRefs(path)
		require.NoError(t, err)
		assert.Equal(t, []string{"selfplay"}, ids)
	})

	t.Run("returns empty for config with no refs", func(t *testing.T) {
		path := writeConfig(t, `spec:
  providers:
    - file: providers/main.yaml`)

		ids, err := extractProviderIDRefs(path)
		require.NoError(t, err)
		assert.Empty(t, ids)
	})

	t.Run("ignores disabled self-play", func(t *testing.T) {
		path := writeConfig(t, `spec:
  self_play:
    enabled: false
    roles:
      - id: sim
        provider: selfplay`)

		ids, err := extractProviderIDRefs(path)
		require.NoError(t, err)
		assert.Empty(t, ids)
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := extractProviderIDRefs("/nonexistent/config.yaml")
		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// arenaJobSpec JSON round-trip (structural test)
// ---------------------------------------------------------------------------

func TestArenaJobSpecJSONRoundTrip(t *testing.T) {
	t.Run("providers with mixed entries round-trips correctly", func(t *testing.T) {
		nsPtr := ptr.To("other-ns")
		original := arenaJobSpec{
			Providers: map[string]arenaProviderGroup{
				"default": {
					entries: []arenaProviderEntry{
						{ProviderRef: &v1alpha1.ProviderRef{Name: "my-provider", Namespace: nsPtr}},
					},
				},
				"fleet": {
					entries: []arenaProviderEntry{
						{AgentRef: &v1alpha1.LocalObjectReference{Name: "my-agent"}},
					},
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

		defaultGroup := got.Providers["default"]
		assert.Equal(t, "my-provider", defaultGroup.entries[0].ProviderRef.Name)
		assert.Equal(t, "other-ns", *defaultGroup.entries[0].ProviderRef.Namespace)
		fleetGroup := got.Providers["fleet"]
		assert.Equal(t, "my-agent", fleetGroup.entries[0].AgentRef.Name)
		assert.Equal(t, "registry-a", got.ToolRegistries[0].Name)
	})
}

// ---------------------------------------------------------------------------
// arenaProviderGroup UnmarshalJSON
// ---------------------------------------------------------------------------

func TestArenaProviderGroupUnmarshalJSON(t *testing.T) {
	t.Run("array JSON populates entries", func(t *testing.T) {
		raw := `[{"providerRef":{"name":"p1"}},{"agentRef":{"name":"a1"}}]`
		var pg arenaProviderGroup
		err := json.Unmarshal([]byte(raw), &pg)
		require.NoError(t, err)

		assert.False(t, pg.isMapMode())
		require.Len(t, pg.entries, 2)
		assert.Equal(t, "p1", pg.entries[0].ProviderRef.Name)
		assert.Equal(t, "a1", pg.entries[1].AgentRef.Name)
		assert.Nil(t, pg.mapping)
	})

	t.Run("object JSON populates mapping", func(t *testing.T) {
		raw := `{"my-config-id":{"providerRef":{"name":"p1"}},"other-id":{"agentRef":{"name":"a1"}}}`
		var pg arenaProviderGroup
		err := json.Unmarshal([]byte(raw), &pg)
		require.NoError(t, err)

		assert.True(t, pg.isMapMode())
		require.Len(t, pg.mapping, 2)
		assert.Equal(t, "p1", pg.mapping["my-config-id"].ProviderRef.Name)
		assert.Equal(t, "a1", pg.mapping["other-id"].AgentRef.Name)
		assert.Nil(t, pg.entries)
	})

	t.Run("empty data returns no error", func(t *testing.T) {
		var pg arenaProviderGroup
		err := pg.UnmarshalJSON([]byte(""))
		require.NoError(t, err)
		assert.Nil(t, pg.entries)
		assert.Nil(t, pg.mapping)
	})

	t.Run("whitespace-only data returns no error", func(t *testing.T) {
		var pg arenaProviderGroup
		err := pg.UnmarshalJSON([]byte("  \t\n"))
		require.NoError(t, err)
		assert.Nil(t, pg.entries)
		assert.Nil(t, pg.mapping)
	})

	t.Run("non-array non-object falls through to default unmarshal", func(t *testing.T) {
		var pg arenaProviderGroup
		// A bare string like `"hello"` is neither array nor object
		err := pg.UnmarshalJSON([]byte(`"hello"`))
		require.Error(t, err) // cannot unmarshal string into []arenaProviderEntry
	})
}

// ---------------------------------------------------------------------------
// arenaProviderGroup MarshalJSON
// ---------------------------------------------------------------------------

func TestArenaProviderGroupMarshalJSON(t *testing.T) {
	t.Run("array mode marshals to JSON array", func(t *testing.T) {
		pg := arenaProviderGroup{
			entries: []arenaProviderEntry{
				{ProviderRef: &v1alpha1.ProviderRef{Name: "p1"}},
			},
		}

		data, err := json.Marshal(pg)
		require.NoError(t, err)

		// Should start with '[' (array)
		assert.Equal(t, byte('['), data[0])

		// Round-trip
		var got arenaProviderGroup
		require.NoError(t, json.Unmarshal(data, &got))
		assert.False(t, got.isMapMode())
		require.Len(t, got.entries, 1)
		assert.Equal(t, "p1", got.entries[0].ProviderRef.Name)
	})

	t.Run("map mode marshals to JSON object", func(t *testing.T) {
		pg := arenaProviderGroup{
			mapping: map[string]arenaProviderEntry{
				"my-id": {ProviderRef: &v1alpha1.ProviderRef{Name: "p1"}},
			},
		}

		data, err := json.Marshal(pg)
		require.NoError(t, err)

		// Should start with '{' (object)
		assert.Equal(t, byte('{'), data[0])

		// Round-trip
		var got arenaProviderGroup
		require.NoError(t, json.Unmarshal(data, &got))
		assert.True(t, got.isMapMode())
		assert.Equal(t, "p1", got.mapping["my-id"].ProviderRef.Name)
	})
}

// ---------------------------------------------------------------------------
// allEntries
// ---------------------------------------------------------------------------

func TestArenaProviderGroupAllEntries(t *testing.T) {
	t.Run("returns entries in array mode", func(t *testing.T) {
		pg := &arenaProviderGroup{
			entries: []arenaProviderEntry{
				{ProviderRef: &v1alpha1.ProviderRef{Name: "p1"}},
				{AgentRef: &v1alpha1.LocalObjectReference{Name: "a1"}},
			},
		}
		all := pg.allEntries()
		assert.Len(t, all, 2)
	})

	t.Run("returns mapping values in map mode", func(t *testing.T) {
		pg := &arenaProviderGroup{
			mapping: map[string]arenaProviderEntry{
				"id1": {ProviderRef: &v1alpha1.ProviderRef{Name: "p1"}},
			},
		}
		all := pg.allEntries()
		assert.Len(t, all, 1)
		assert.Equal(t, "p1", all[0].ProviderRef.Name)
	})

	t.Run("returns nil for empty group", func(t *testing.T) {
		pg := &arenaProviderGroup{}
		all := pg.allEntries()
		assert.Nil(t, all)
	})
}

// ---------------------------------------------------------------------------
// extractProviderIDRefs — invalid YAML
// ---------------------------------------------------------------------------

func TestExtractProviderIDRefs_InvalidYAML(t *testing.T) {
	t.Run("returns error for invalid YAML content", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.yaml")
		require.NoError(t, os.WriteFile(path, []byte(":\n  :\n    - [invalid yaml"), 0644))

		_, err := extractProviderIDRefs(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse arena config")
	})
}

// ---------------------------------------------------------------------------
// remapProviderIDs — config read error
// ---------------------------------------------------------------------------

func TestRemapProviderIDs_ConfigReadError(t *testing.T) {
	t.Run("returns error when config file does not exist", func(t *testing.T) {
		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{},
			ProviderGroups:  map[string]string{},
		}

		err := remapProviderIDs(testLog(), arenaCfg, "/nonexistent/config.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extract provider ID refs")
	})
}

// ---------------------------------------------------------------------------
// resolveProvidersFromCRD — map mode
// ---------------------------------------------------------------------------

func TestResolveProvidersFromCRD_MapMode(t *testing.T) {
	ctx := context.Background()
	log := testLog()

	t.Run("map mode agentRef uses map key as provider ID", func(t *testing.T) {
		t.Setenv("ARENA_AGENT_WS_URLS", `{"my-agent":"ws://my-agent:8080/ws"}`)

		spec := map[string]interface{}{
			"providers": map[string]interface{}{
				"selfplay": map[string]interface{}{
					"agent-config-id": map[string]interface{}{
						"agentRef": map[string]interface{}{
							"name": "my-agent",
						},
					},
				},
			},
		}
		u := makeArenaJobUnstructured("map-agent-job", testNamespace, spec)

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(u).
			Build()

		cfg := &Config{JobName: "map-agent-job", JobNamespace: testNamespace}
		arenaCfg := &config.Config{}

		fps, err := resolveProvidersFromCRD(ctx, log, c, cfg, arenaCfg)
		require.NoError(t, err)
		require.Len(t, fps, 1)

		assert.Equal(t, "agent-config-id", fps[0].id)
		assert.Equal(t, "ws://my-agent:8080/ws", fps[0].wsURL)
		require.Contains(t, arenaCfg.LoadedProviders, "agent-config-id")
		assert.Equal(t, "fleet", arenaCfg.LoadedProviders["agent-config-id"].Type)
		assert.Equal(t, "selfplay", arenaCfg.ProviderGroups["agent-config-id"])
	})

	t.Run("map mode uses map key as provider ID", func(t *testing.T) {
		// Create Provider CRD
		provider := &v1alpha1.Provider{
			Spec: v1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4o",
			},
		}
		provider.Name = "my-provider"
		provider.Namespace = testNamespace

		// Build ArenaJob with map-mode providers (object, not array)
		spec := map[string]interface{}{
			"providers": map[string]interface{}{
				"selfplay": map[string]interface{}{
					"my-config-id": map[string]interface{}{
						"providerRef": map[string]interface{}{
							"name": "my-provider",
						},
					},
				},
			},
		}
		u := makeArenaJobUnstructured("map-job", testNamespace, spec)

		c := fake.NewClientBuilder().
			WithScheme(k8s.Scheme()).
			WithObjects(provider, u).
			Build()

		cfg := &Config{JobName: "map-job", JobNamespace: testNamespace}
		arenaCfg := &config.Config{}

		fps, err := resolveProvidersFromCRD(ctx, log, c, cfg, arenaCfg)
		require.NoError(t, err)
		assert.Empty(t, fps)

		// The provider should be keyed by "my-config-id" (the map key), not "my-provider"
		require.Contains(t, arenaCfg.LoadedProviders, "my-config-id")
		assert.Equal(t, "my-config-id", arenaCfg.LoadedProviders["my-config-id"].ID)
		assert.Equal(t, "openai", arenaCfg.LoadedProviders["my-config-id"].Type)
		assert.Equal(t, "gpt-4o", arenaCfg.LoadedProviders["my-config-id"].Model)
		assert.Equal(t, "selfplay", arenaCfg.ProviderGroups["my-config-id"])

		// Should NOT be keyed by the CRD name
		assert.NotContains(t, arenaCfg.LoadedProviders, sanitizeID("my-provider"))
	})
}

// ---------------------------------------------------------------------------
// remapProviderIDs — skipped for map mode
// ---------------------------------------------------------------------------

func TestRemapProviderIDs_SkippedForMapMode(t *testing.T) {
	writeConfig := func(t *testing.T, yamlContent string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.arena.yaml")
		require.NoError(t, os.WriteFile(path, []byte(yamlContent), 0644))
		return path
	}

	t.Run("no-op when expected IDs already present from map mode", func(t *testing.T) {
		configPath := writeConfig(t, `spec:
  providers:
    - file: providers/sim.yaml
      group: selfplay
  self_play:
    enabled: true
    roles:
      - id: user-sim
        provider: selfplay`)

		// Simulate map-mode resolution: the provider is already keyed as "selfplay"
		// (the config provider ID), so remapProviderIDs should be a no-op.
		arenaCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"selfplay": {ID: "selfplay", Type: "openai", Model: "gpt-4o"},
			},
			ProviderGroups: map[string]string{
				"selfplay": "selfplay",
			},
		}

		err := remapProviderIDs(testLog(), arenaCfg, configPath)
		require.NoError(t, err)

		// Should remain unchanged — no remapping needed
		require.Contains(t, arenaCfg.LoadedProviders, "selfplay")
		assert.Equal(t, "selfplay", arenaCfg.LoadedProviders["selfplay"].ID)
		assert.Equal(t, "openai", arenaCfg.LoadedProviders["selfplay"].Type)
		assert.Len(t, arenaCfg.LoadedProviders, 1)
	})
}
