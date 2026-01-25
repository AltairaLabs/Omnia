/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package selector

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// ToolRegistryResult contains a matched ToolRegistry and its discovered tools.
type ToolRegistryResult struct {
	// Registry is the matched ToolRegistry CRD.
	Registry *corev1alpha1.ToolRegistry
	// Tools are the discovered tools from this registry.
	Tools []corev1alpha1.DiscoveredTool
}

// SelectToolRegistries returns all ToolRegistry CRDs in the namespace that match the label selector.
func SelectToolRegistries(
	ctx context.Context,
	c client.Client,
	namespace string,
	selector *metav1.LabelSelector,
) ([]*corev1alpha1.ToolRegistry, error) {
	opts, err := ListOptions(selector, namespace)
	if err != nil {
		return nil, fmt.Errorf("invalid tool registry selector: %w", err)
	}

	registryList := &corev1alpha1.ToolRegistryList{}
	if err := c.List(ctx, registryList, opts...); err != nil {
		return nil, fmt.Errorf("failed to list tool registries: %w", err)
	}

	// Convert to pointer slice for easier manipulation
	registries := make([]*corev1alpha1.ToolRegistry, len(registryList.Items))
	for i := range registryList.Items {
		registries[i] = &registryList.Items[i]
	}

	return registries, nil
}

// ResolveToolRegistryOverride resolves tool registry override for an ArenaJob.
// It returns all ToolRegistries matching the selector.
func ResolveToolRegistryOverride(
	ctx context.Context,
	c client.Client,
	namespace string,
	override *omniav1alpha1.ToolRegistrySelector,
) ([]*corev1alpha1.ToolRegistry, error) {
	if override == nil {
		return nil, nil
	}

	return SelectToolRegistries(ctx, c, namespace, &override.Selector)
}

// GetToolOverridesFromRegistries extracts tool information from ToolRegistry CRDs
// for passing to the arena worker. Returns a map of tool name -> handler endpoint.
func GetToolOverridesFromRegistries(registries []*corev1alpha1.ToolRegistry) map[string]ToolOverrideConfig {
	if len(registries) == 0 {
		return nil
	}

	overrides := make(map[string]ToolOverrideConfig)

	for _, registry := range registries {
		// Use discovered tools from status if available
		for _, tool := range registry.Status.DiscoveredTools {
			if tool.Status == "Available" {
				overrides[tool.Name] = ToolOverrideConfig{
					Name:         tool.Name,
					Description:  tool.Description,
					Endpoint:     tool.Endpoint,
					HandlerName:  tool.HandlerName,
					RegistryName: registry.Name,
					InputSchema:  tool.InputSchema,
					OutputSchema: tool.OutputSchema,
				}
			}
		}

		// Also include tools defined directly in handlers
		for _, handler := range registry.Spec.Handlers {
			if handler.Tool != nil {
				// Only add if not already present from discovered tools
				if _, exists := overrides[handler.Tool.Name]; !exists {
					endpoint := resolveHandlerEndpoint(handler)
					overrides[handler.Tool.Name] = ToolOverrideConfig{
						Name:         handler.Tool.Name,
						Description:  handler.Tool.Description,
						Endpoint:     endpoint,
						HandlerName:  handler.Name,
						RegistryName: registry.Name,
						HandlerType:  string(handler.Type),
					}
				}
			}
		}
	}

	return overrides
}

// ToolOverrideConfig contains the configuration for a tool override.
type ToolOverrideConfig struct {
	// Name is the tool name (matches tool name in arena.config.yaml).
	Name string `json:"name"`
	// Description of the tool.
	Description string `json:"description,omitempty"`
	// Endpoint is the resolved endpoint URL for the tool.
	Endpoint string `json:"endpoint,omitempty"`
	// HandlerName is the name of the handler in the ToolRegistry.
	HandlerName string `json:"handlerName"`
	// RegistryName is the name of the ToolRegistry CRD.
	RegistryName string `json:"registryName"`
	// HandlerType is the type of handler (http, grpc, mcp, openapi).
	HandlerType string `json:"handlerType,omitempty"`
	// InputSchema is the JSON schema for tool inputs.
	InputSchema *apiextensionsv1.JSON `json:"inputSchema,omitempty"`
	// OutputSchema is the JSON schema for tool outputs.
	OutputSchema *apiextensionsv1.JSON `json:"outputSchema,omitempty"`
}

// resolveHandlerEndpoint extracts the endpoint from a handler definition.
func resolveHandlerEndpoint(handler corev1alpha1.HandlerDefinition) string {
	switch handler.Type {
	case corev1alpha1.HandlerTypeHTTP:
		if handler.HTTPConfig != nil {
			return handler.HTTPConfig.Endpoint
		}
	case corev1alpha1.HandlerTypeGRPC:
		if handler.GRPCConfig != nil {
			return handler.GRPCConfig.Endpoint
		}
	case corev1alpha1.HandlerTypeMCP:
		if handler.MCPConfig != nil && handler.MCPConfig.Endpoint != nil {
			return *handler.MCPConfig.Endpoint
		}
	case corev1alpha1.HandlerTypeOpenAPI:
		if handler.OpenAPIConfig != nil && handler.OpenAPIConfig.BaseURL != nil {
			return *handler.OpenAPIConfig.BaseURL
		}
	}
	return ""
}
