/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package selector

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestSelectToolRegistries(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	// Create test tool registries
	registries := []runtime.Object{
		&omniav1alpha1.ToolRegistry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "production-tools",
				Namespace: "test-ns",
				Labels: map[string]string{
					"environment": "production",
					"tier":        "standard",
				},
			},
			Spec: omniav1alpha1.ToolRegistrySpec{
				Handlers: []omniav1alpha1.HandlerDefinition{
					{
						Name: "weather-api",
						Type: omniav1alpha1.HandlerTypeHTTP,
					},
				},
			},
		},
		&omniav1alpha1.ToolRegistry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "staging-tools",
				Namespace: "test-ns",
				Labels: map[string]string{
					"environment": "staging",
					"tier":        "standard",
				},
			},
			Spec: omniav1alpha1.ToolRegistrySpec{
				Handlers: []omniav1alpha1.HandlerDefinition{
					{
						Name: "search-api",
						Type: omniav1alpha1.HandlerTypeHTTP,
					},
				},
			},
		},
		&omniav1alpha1.ToolRegistry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-ns-tools",
				Namespace: "other-ns",
				Labels: map[string]string{
					"environment": "production",
				},
			},
			Spec: omniav1alpha1.ToolRegistrySpec{
				Handlers: []omniav1alpha1.HandlerDefinition{
					{
						Name: "other-api",
						Type: omniav1alpha1.HandlerTypeHTTP,
					},
				},
			},
		},
	}

	ctx := context.Background()

	t.Run("select by environment label", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(registries...).
			Build()

		result, err := SelectToolRegistries(ctx, fakeClient, "test-ns", &metav1.LabelSelector{
			MatchLabels: map[string]string{"environment": "production"},
		})

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "production-tools", result[0].Name)
	})

	t.Run("select by tier label", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(registries...).
			Build()

		result, err := SelectToolRegistries(ctx, fakeClient, "test-ns", &metav1.LabelSelector{
			MatchLabels: map[string]string{"tier": "standard"},
		})

		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("nil selector returns all in namespace", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(registries...).
			Build()

		result, err := SelectToolRegistries(ctx, fakeClient, "test-ns", nil)

		require.NoError(t, err)
		assert.Len(t, result, 2) // Only in test-ns
	})

	t.Run("no matches returns empty slice", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(registries...).
			Build()

		result, err := SelectToolRegistries(ctx, fakeClient, "test-ns", &metav1.LabelSelector{
			MatchLabels: map[string]string{"environment": "development"},
		})

		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestResolveToolRegistryOverride(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	registry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "production-tools",
			Namespace: "test-ns",
			Labels: map[string]string{
				"environment": "production",
			},
		},
	}

	ctx := context.Background()

	t.Run("nil override returns nil", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(registry).
			Build()

		result, err := ResolveToolRegistryOverride(ctx, fakeClient, "test-ns", nil)

		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("with override selector", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(registry).
			Build()

		result, err := ResolveToolRegistryOverride(ctx, fakeClient, "test-ns", &omniav1alpha1.ToolRegistrySelector{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"environment": "production"},
			},
		})

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "production-tools", result[0].Name)
	})
}

func TestGetToolOverridesFromRegistries(t *testing.T) {
	endpoint := "http://weather-service:8080"

	t.Run("empty registries returns nil", func(t *testing.T) {
		result := GetToolOverridesFromRegistries(nil)
		assert.Nil(t, result)
	})

	t.Run("extracts tools from discovered tools", func(t *testing.T) {
		registries := []*omniav1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-registry",
				},
				Status: omniav1alpha1.ToolRegistryStatus{
					DiscoveredTools: []omniav1alpha1.DiscoveredTool{
						{
							Name:        "get_weather",
							HandlerName: "weather-handler",
							Description: "Get weather data",
							Endpoint:    endpoint,
							Status:      "Available",
						},
						{
							Name:        "unavailable_tool",
							HandlerName: "broken-handler",
							Status:      "Unavailable",
						},
					},
				},
			},
		}

		result := GetToolOverridesFromRegistries(registries)

		assert.Len(t, result, 1)
		assert.Contains(t, result, "get_weather")
		assert.Equal(t, "get_weather", result["get_weather"].Name)
		assert.Equal(t, endpoint, result["get_weather"].Endpoint)
		assert.Equal(t, "my-registry", result["get_weather"].RegistryName)
	})

	t.Run("extracts tools from handler definitions", func(t *testing.T) {
		registries := []*omniav1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-registry",
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "weather-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "get_weather",
								Description: "Get weather data",
							},
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: endpoint,
							},
						},
					},
				},
			},
		}

		result := GetToolOverridesFromRegistries(registries)

		assert.Len(t, result, 1)
		assert.Contains(t, result, "get_weather")
		assert.Equal(t, endpoint, result["get_weather"].Endpoint)
		assert.Equal(t, "http", result["get_weather"].HandlerType)
	})

	t.Run("discovered tools take precedence over handler definitions", func(t *testing.T) {
		registries := []*omniav1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-registry",
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "weather-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "get_weather",
								Description: "Handler definition",
							},
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: "http://old-endpoint:8080",
							},
						},
					},
				},
				Status: omniav1alpha1.ToolRegistryStatus{
					DiscoveredTools: []omniav1alpha1.DiscoveredTool{
						{
							Name:        "get_weather",
							HandlerName: "weather-handler",
							Description: "Discovered tool",
							Endpoint:    endpoint,
							Status:      "Available",
						},
					},
				},
			},
		}

		result := GetToolOverridesFromRegistries(registries)

		assert.Len(t, result, 1)
		assert.Equal(t, endpoint, result["get_weather"].Endpoint)
		assert.Equal(t, "Discovered tool", result["get_weather"].Description)
	})

	t.Run("extracts tools from GRPC handler", func(t *testing.T) {
		grpcEndpoint := "grpc-service:9090"
		registries := []*omniav1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "grpc-registry",
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "grpc-handler",
							Type: omniav1alpha1.HandlerTypeGRPC,
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "grpc_tool",
								Description: "GRPC tool",
							},
							GRPCConfig: &omniav1alpha1.GRPCConfig{
								Endpoint: grpcEndpoint,
							},
						},
					},
				},
			},
		}

		result := GetToolOverridesFromRegistries(registries)

		assert.Len(t, result, 1)
		assert.Equal(t, grpcEndpoint, result["grpc_tool"].Endpoint)
		assert.Equal(t, "grpc", result["grpc_tool"].HandlerType)
	})

	t.Run("extracts tools from MCP handler", func(t *testing.T) {
		mcpEndpoint := "http://mcp-server:3000"
		registries := []*omniav1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mcp-registry",
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "mcp-handler",
							Type: omniav1alpha1.HandlerTypeMCP,
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "mcp_tool",
								Description: "MCP tool",
							},
							MCPConfig: &omniav1alpha1.MCPConfig{
								Endpoint: &mcpEndpoint,
							},
						},
					},
				},
			},
		}

		result := GetToolOverridesFromRegistries(registries)

		assert.Len(t, result, 1)
		assert.Equal(t, mcpEndpoint, result["mcp_tool"].Endpoint)
		assert.Equal(t, "mcp", result["mcp_tool"].HandlerType)
	})

	t.Run("extracts tools from OpenAPI handler", func(t *testing.T) {
		baseURL := "https://api.example.com/v1"
		registries := []*omniav1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "openapi-registry",
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "openapi-handler",
							Type: omniav1alpha1.HandlerTypeOpenAPI,
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "openapi_tool",
								Description: "OpenAPI tool",
							},
							OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
								BaseURL: &baseURL,
							},
						},
					},
				},
			},
		}

		result := GetToolOverridesFromRegistries(registries)

		assert.Len(t, result, 1)
		assert.Equal(t, baseURL, result["openapi_tool"].Endpoint)
		assert.Equal(t, "openapi", result["openapi_tool"].HandlerType)
	})

	t.Run("handles handler without endpoint config", func(t *testing.T) {
		registries := []*omniav1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-endpoint-registry",
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "http-handler-no-config",
							Type: omniav1alpha1.HandlerTypeHTTP,
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "no_endpoint_tool",
								Description: "Tool without endpoint config",
							},
							// HTTPConfig is nil
						},
					},
				},
			},
		}

		result := GetToolOverridesFromRegistries(registries)

		assert.Len(t, result, 1)
		assert.Equal(t, "", result["no_endpoint_tool"].Endpoint)
	})
}
