/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


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

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func TestSelectToolRegistries(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1alpha1.AddToScheme(scheme))
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	// Create test tool registries
	registries := []runtime.Object{
		&corev1alpha1.ToolRegistry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "production-tools",
				Namespace: "test-ns",
				Labels: map[string]string{
					"environment": "production",
					"tier":        "standard",
				},
			},
			Spec: corev1alpha1.ToolRegistrySpec{
				Handlers: []corev1alpha1.HandlerDefinition{
					{
						Name: "weather-api",
						Type: corev1alpha1.HandlerTypeHTTP,
					},
				},
			},
		},
		&corev1alpha1.ToolRegistry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "staging-tools",
				Namespace: "test-ns",
				Labels: map[string]string{
					"environment": "staging",
					"tier":        "standard",
				},
			},
			Spec: corev1alpha1.ToolRegistrySpec{
				Handlers: []corev1alpha1.HandlerDefinition{
					{
						Name: "search-api",
						Type: corev1alpha1.HandlerTypeHTTP,
					},
				},
			},
		},
		&corev1alpha1.ToolRegistry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-ns-tools",
				Namespace: "other-ns",
				Labels: map[string]string{
					"environment": "production",
				},
			},
			Spec: corev1alpha1.ToolRegistrySpec{
				Handlers: []corev1alpha1.HandlerDefinition{
					{
						Name: "other-api",
						Type: corev1alpha1.HandlerTypeHTTP,
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
	require.NoError(t, corev1alpha1.AddToScheme(scheme))
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	registry := &corev1alpha1.ToolRegistry{
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
		registries := []*corev1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-registry",
				},
				Status: corev1alpha1.ToolRegistryStatus{
					DiscoveredTools: []corev1alpha1.DiscoveredTool{
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
		registries := []*corev1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-registry",
				},
				Spec: corev1alpha1.ToolRegistrySpec{
					Handlers: []corev1alpha1.HandlerDefinition{
						{
							Name: "weather-handler",
							Type: corev1alpha1.HandlerTypeHTTP,
							Tool: &corev1alpha1.ToolDefinition{
								Name:        "get_weather",
								Description: "Get weather data",
							},
							HTTPConfig: &corev1alpha1.HTTPConfig{
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
		registries := []*corev1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-registry",
				},
				Spec: corev1alpha1.ToolRegistrySpec{
					Handlers: []corev1alpha1.HandlerDefinition{
						{
							Name: "weather-handler",
							Type: corev1alpha1.HandlerTypeHTTP,
							Tool: &corev1alpha1.ToolDefinition{
								Name:        "get_weather",
								Description: "Handler definition",
							},
							HTTPConfig: &corev1alpha1.HTTPConfig{
								Endpoint: "http://old-endpoint:8080",
							},
						},
					},
				},
				Status: corev1alpha1.ToolRegistryStatus{
					DiscoveredTools: []corev1alpha1.DiscoveredTool{
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
		registries := []*corev1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "grpc-registry",
				},
				Spec: corev1alpha1.ToolRegistrySpec{
					Handlers: []corev1alpha1.HandlerDefinition{
						{
							Name: "grpc-handler",
							Type: corev1alpha1.HandlerTypeGRPC,
							Tool: &corev1alpha1.ToolDefinition{
								Name:        "grpc_tool",
								Description: "GRPC tool",
							},
							GRPCConfig: &corev1alpha1.GRPCConfig{
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
		registries := []*corev1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mcp-registry",
				},
				Spec: corev1alpha1.ToolRegistrySpec{
					Handlers: []corev1alpha1.HandlerDefinition{
						{
							Name: "mcp-handler",
							Type: corev1alpha1.HandlerTypeMCP,
							Tool: &corev1alpha1.ToolDefinition{
								Name:        "mcp_tool",
								Description: "MCP tool",
							},
							MCPConfig: &corev1alpha1.MCPConfig{
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
		registries := []*corev1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "openapi-registry",
				},
				Spec: corev1alpha1.ToolRegistrySpec{
					Handlers: []corev1alpha1.HandlerDefinition{
						{
							Name: "openapi-handler",
							Type: corev1alpha1.HandlerTypeOpenAPI,
							Tool: &corev1alpha1.ToolDefinition{
								Name:        "openapi_tool",
								Description: "OpenAPI tool",
							},
							OpenAPIConfig: &corev1alpha1.OpenAPIConfig{
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
		registries := []*corev1alpha1.ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-endpoint-registry",
				},
				Spec: corev1alpha1.ToolRegistrySpec{
					Handlers: []corev1alpha1.HandlerDefinition{
						{
							Name: "http-handler-no-config",
							Type: corev1alpha1.HandlerTypeHTTP,
							Tool: &corev1alpha1.ToolDefinition{
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
