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

package v1alpha1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test constants to avoid duplicate string literals
const (
	testToolRegistryName      = "test-toolregistry"
	testToolRegistryNamespace = "test-namespace"
	testToolName              = "my-tool"
	testToolEndpoint          = "https://api.example.com/tool"
	testToolDescription       = "A test tool"
	testToolModifiedName      = "modified-name"
)

func TestToolTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant ToolType
		expected string
	}{
		{
			name:     "HTTP tool type",
			constant: ToolTypeHTTP,
			expected: "http",
		},
		{
			name:     "gRPC tool type",
			constant: ToolTypeGRPC,
			expected: "grpc",
		},
		{
			name:     "MCP tool type",
			constant: ToolTypeMCP,
			expected: "mcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("ToolType constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestToolRegistryPhaseConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant ToolRegistryPhase
		expected string
	}{
		{
			name:     "Pending phase",
			constant: ToolRegistryPhasePending,
			expected: "Pending",
		},
		{
			name:     "Ready phase",
			constant: ToolRegistryPhaseReady,
			expected: "Ready",
		},
		{
			name:     "Degraded phase",
			constant: ToolRegistryPhaseDegraded,
			expected: "Degraded",
		},
		{
			name:     "Failed phase",
			constant: ToolRegistryPhaseFailed,
			expected: "Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("ToolRegistryPhase constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestToolStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "Available status",
			constant: ToolStatusAvailable,
			expected: "Available",
		},
		{
			name:     "Unavailable status",
			constant: ToolStatusUnavailable,
			expected: "Unavailable",
		},
		{
			name:     "Unknown status",
			constant: ToolStatusUnknown,
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("ToolStatus constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestToolRegistryCreationWithURL(t *testing.T) {
	url := testToolEndpoint
	description := testToolDescription
	timeout := "60s"
	retries := int32(3)

	registry := &ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testToolRegistryName,
			Namespace: testToolRegistryNamespace,
		},
		Spec: ToolRegistrySpec{
			Tools: []ToolDefinition{
				{
					Name:        testToolName,
					Description: &description,
					Type:        ToolTypeHTTP,
					Endpoint: ToolEndpoint{
						URL: &url,
					},
					Timeout: &timeout,
					Retries: &retries,
				},
			},
		},
	}

	if registry.Name != testToolRegistryName {
		t.Errorf("ToolRegistry.Name = %v, want %v", registry.Name, testToolRegistryName)
	}

	if registry.Namespace != testToolRegistryNamespace {
		t.Errorf("ToolRegistry.Namespace = %v, want %v", registry.Namespace, testToolRegistryNamespace)
	}

	if len(registry.Spec.Tools) != 1 {
		t.Fatalf("len(ToolRegistry.Spec.Tools) = %v, want 1", len(registry.Spec.Tools))
	}

	tool := registry.Spec.Tools[0]
	if tool.Name != testToolName {
		t.Errorf("Tool.Name = %v, want %v", tool.Name, testToolName)
	}

	if tool.Type != ToolTypeHTTP {
		t.Errorf("Tool.Type = %v, want %v", tool.Type, ToolTypeHTTP)
	}

	if *tool.Endpoint.URL != testToolEndpoint {
		t.Errorf("Tool.Endpoint.URL = %v, want %v", *tool.Endpoint.URL, testToolEndpoint)
	}

	if *tool.Description != testToolDescription {
		t.Errorf("Tool.Description = %v, want %v", *tool.Description, testToolDescription)
	}

	if *tool.Timeout != "60s" {
		t.Errorf("Tool.Timeout = %v, want %v", *tool.Timeout, "60s")
	}

	if *tool.Retries != 3 {
		t.Errorf("Tool.Retries = %v, want %v", *tool.Retries, 3)
	}
}

func TestToolRegistryCreationWithSelector(t *testing.T) {
	namespace := "tools-namespace"
	port := "http"

	registry := &ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testToolRegistryName,
			Namespace: testToolRegistryNamespace,
		},
		Spec: ToolRegistrySpec{
			Tools: []ToolDefinition{
				{
					Name: testToolName,
					Type: ToolTypeGRPC,
					Endpoint: ToolEndpoint{
						Selector: &ToolSelector{
							MatchLabels: map[string]string{
								"app":  "my-tool",
								"tier": "backend",
							},
							Namespace: &namespace,
							Port:      &port,
						},
					},
				},
			},
		},
	}

	tool := registry.Spec.Tools[0]
	if tool.Endpoint.Selector == nil {
		t.Fatal("Tool.Endpoint.Selector should not be nil")
	}

	if len(tool.Endpoint.Selector.MatchLabels) != 2 {
		t.Errorf("len(Tool.Endpoint.Selector.MatchLabels) = %v, want 2", len(tool.Endpoint.Selector.MatchLabels))
	}

	if tool.Endpoint.Selector.MatchLabels["app"] != "my-tool" {
		t.Errorf("Tool.Endpoint.Selector.MatchLabels[app] = %v, want my-tool", tool.Endpoint.Selector.MatchLabels["app"])
	}

	if *tool.Endpoint.Selector.Namespace != namespace {
		t.Errorf("Tool.Endpoint.Selector.Namespace = %v, want %v", *tool.Endpoint.Selector.Namespace, namespace)
	}

	if *tool.Endpoint.Selector.Port != port {
		t.Errorf("Tool.Endpoint.Selector.Port = %v, want %v", *tool.Endpoint.Selector.Port, port)
	}
}

func TestToolRegistryWithSchema(t *testing.T) {
	url := testToolEndpoint
	inputSchema := `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"number"}},"required":["query"]}`
	outputSchema := `{"type":"array","items":{"type":"object"}}`

	registry := &ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testToolRegistryName,
			Namespace: testToolRegistryNamespace,
		},
		Spec: ToolRegistrySpec{
			Tools: []ToolDefinition{
				{
					Name: testToolName,
					Type: ToolTypeHTTP,
					Endpoint: ToolEndpoint{
						URL: &url,
					},
					Schema: &ToolSchema{
						Input:  &inputSchema,
						Output: &outputSchema,
					},
				},
			},
		},
	}

	tool := registry.Spec.Tools[0]
	if tool.Schema == nil {
		t.Fatal("Tool.Schema should not be nil")
	}

	if tool.Schema.Input == nil {
		t.Fatal("Tool.Schema.Input should not be nil")
	}

	if *tool.Schema.Input != inputSchema {
		t.Errorf("Tool.Schema.Input = %v, want %v", *tool.Schema.Input, inputSchema)
	}

	if tool.Schema.Output == nil {
		t.Fatal("Tool.Schema.Output should not be nil")
	}

	if *tool.Schema.Output != outputSchema {
		t.Errorf("Tool.Schema.Output = %v, want %v", *tool.Schema.Output, outputSchema)
	}
}

func TestToolRegistryStatus(t *testing.T) {
	now := metav1.NewTime(time.Now())

	registry := &ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testToolRegistryName,
			Namespace: testToolRegistryNamespace,
		},
		Spec: ToolRegistrySpec{
			Tools: []ToolDefinition{
				{
					Name: testToolName,
					Type: ToolTypeHTTP,
					Endpoint: ToolEndpoint{
						URL: ptrString(testToolEndpoint),
					},
				},
			},
		},
		Status: ToolRegistryStatus{
			Phase:                ToolRegistryPhaseReady,
			DiscoveredToolsCount: 1,
			DiscoveredTools: []DiscoveredTool{
				{
					Name:        testToolName,
					Endpoint:    testToolEndpoint,
					Status:      ToolStatusAvailable,
					LastChecked: &now,
				},
			},
			LastDiscoveryTime: &now,
			Conditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "AllToolsAvailable",
					Message:            "All tools are available",
				},
			},
		},
	}

	if registry.Status.Phase != ToolRegistryPhaseReady {
		t.Errorf("ToolRegistry.Status.Phase = %v, want %v", registry.Status.Phase, ToolRegistryPhaseReady)
	}

	if registry.Status.DiscoveredToolsCount != 1 {
		t.Errorf("ToolRegistry.Status.DiscoveredToolsCount = %v, want 1", registry.Status.DiscoveredToolsCount)
	}

	if len(registry.Status.DiscoveredTools) != 1 {
		t.Fatalf("len(ToolRegistry.Status.DiscoveredTools) = %v, want 1", len(registry.Status.DiscoveredTools))
	}

	discoveredTool := registry.Status.DiscoveredTools[0]
	if discoveredTool.Name != testToolName {
		t.Errorf("DiscoveredTool.Name = %v, want %v", discoveredTool.Name, testToolName)
	}

	if discoveredTool.Status != ToolStatusAvailable {
		t.Errorf("DiscoveredTool.Status = %v, want %v", discoveredTool.Status, ToolStatusAvailable)
	}

	if registry.Status.LastDiscoveryTime == nil {
		t.Error("ToolRegistry.Status.LastDiscoveryTime should not be nil")
	}

	if len(registry.Status.Conditions) != 1 {
		t.Errorf("len(ToolRegistry.Status.Conditions) = %v, want 1", len(registry.Status.Conditions))
	}
}

func TestToolRegistryDeepCopy(t *testing.T) {
	url := testToolEndpoint
	timeout := "30s"
	retries := int32(2)
	now := metav1.NewTime(time.Now())

	original := &ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testToolRegistryName,
			Namespace: testToolRegistryNamespace,
		},
		Spec: ToolRegistrySpec{
			Tools: []ToolDefinition{
				{
					Name: testToolName,
					Type: ToolTypeHTTP,
					Endpoint: ToolEndpoint{
						URL: &url,
					},
					Timeout: &timeout,
					Retries: &retries,
				},
			},
		},
		Status: ToolRegistryStatus{
			Phase:                ToolRegistryPhaseReady,
			DiscoveredToolsCount: 1,
			DiscoveredTools: []DiscoveredTool{
				{
					Name:     testToolName,
					Endpoint: testToolEndpoint,
					Status:   ToolStatusAvailable,
				},
			},
			LastDiscoveryTime: &now,
		},
	}

	copied := original.DeepCopy()

	// Verify the copy is independent
	if copied == original {
		t.Error("DeepCopy should return a new object, not the same pointer")
	}

	// Verify values are equal
	if copied.Name != original.Name {
		t.Errorf("DeepCopy().Name = %v, want %v", copied.Name, original.Name)
	}

	if len(copied.Spec.Tools) != len(original.Spec.Tools) {
		t.Errorf("DeepCopy().Spec.Tools length = %v, want %v", len(copied.Spec.Tools), len(original.Spec.Tools))
	}

	if copied.Status.Phase != original.Status.Phase {
		t.Errorf("DeepCopy().Status.Phase = %v, want %v", copied.Status.Phase, original.Status.Phase)
	}

	// Modify the copy and verify original is unchanged
	copied.Name = testToolModifiedName
	if original.Name == testToolModifiedName {
		t.Error("Modifying copy should not affect original")
	}

	// Verify nested pointer fields are also deep copied
	if copied.Spec.Tools[0].Endpoint.URL == original.Spec.Tools[0].Endpoint.URL {
		t.Error("DeepCopy should create new URL pointer")
	}
}

func TestToolRegistryListDeepCopy(t *testing.T) {
	url := testToolEndpoint

	original := &ToolRegistryList{
		Items: []ToolRegistry{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testToolRegistryName,
					Namespace: testToolRegistryNamespace,
				},
				Spec: ToolRegistrySpec{
					Tools: []ToolDefinition{
						{
							Name: testToolName,
							Type: ToolTypeHTTP,
							Endpoint: ToolEndpoint{
								URL: &url,
							},
						},
					},
				},
			},
		},
	}

	copied := original.DeepCopy()

	if copied == original {
		t.Error("DeepCopy should return a new object")
	}

	if len(copied.Items) != len(original.Items) {
		t.Errorf("DeepCopy().Items length = %v, want %v", len(copied.Items), len(original.Items))
	}

	// Modify the copy and verify original is unchanged
	copied.Items[0].Name = testToolModifiedName
	if original.Items[0].Name == testToolModifiedName {
		t.Error("Modifying copy should not affect original")
	}
}

func TestToolRegistryTypeRegistration(t *testing.T) {
	// Verify that ToolRegistry types are registered with the scheme
	registry := &ToolRegistry{}
	registryList := &ToolRegistryList{}

	// These should not panic if types are registered correctly
	_ = registry.DeepCopyObject()
	_ = registryList.DeepCopyObject()
}

func TestToolRegistryMultipleTools(t *testing.T) {
	httpURL := "https://api.example.com/http-tool"
	grpcURL := "grpc://api.example.com:9090"
	mcpURL := "mcp://localhost:3000"

	registry := &ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testToolRegistryName,
			Namespace: testToolRegistryNamespace,
		},
		Spec: ToolRegistrySpec{
			Tools: []ToolDefinition{
				{
					Name: "http-tool",
					Type: ToolTypeHTTP,
					Endpoint: ToolEndpoint{
						URL: &httpURL,
					},
				},
				{
					Name: "grpc-tool",
					Type: ToolTypeGRPC,
					Endpoint: ToolEndpoint{
						URL: &grpcURL,
					},
				},
				{
					Name: "mcp-tool",
					Type: ToolTypeMCP,
					Endpoint: ToolEndpoint{
						URL: &mcpURL,
					},
				},
			},
		},
	}

	if len(registry.Spec.Tools) != 3 {
		t.Errorf("len(ToolRegistry.Spec.Tools) = %v, want 3", len(registry.Spec.Tools))
	}

	expectedTypes := []ToolType{ToolTypeHTTP, ToolTypeGRPC, ToolTypeMCP}
	for i, tool := range registry.Spec.Tools {
		if tool.Type != expectedTypes[i] {
			t.Errorf("Tool[%d].Type = %v, want %v", i, tool.Type, expectedTypes[i])
		}
	}
}

func TestToolEndpointMutualExclusivity(t *testing.T) {
	// Test with only URL
	endpointWithURL := ToolEndpoint{
		URL: ptrString(testToolEndpoint),
	}
	if endpointWithURL.URL == nil {
		t.Error("ToolEndpoint.URL should not be nil")
	}
	if endpointWithURL.Selector != nil {
		t.Error("ToolEndpoint.Selector should be nil when URL is set")
	}

	// Test with only Selector
	endpointWithSelector := ToolEndpoint{
		Selector: &ToolSelector{
			MatchLabels: map[string]string{"app": "tool"},
		},
	}
	if endpointWithSelector.Selector == nil {
		t.Error("ToolEndpoint.Selector should not be nil")
	}
	if endpointWithSelector.URL != nil {
		t.Error("ToolEndpoint.URL should be nil when Selector is set")
	}
}

func TestToolDefinitionDefaults(t *testing.T) {
	// Test ToolDefinition with minimal required fields
	tool := ToolDefinition{
		Name: testToolName,
		Type: ToolTypeHTTP,
		Endpoint: ToolEndpoint{
			URL: ptrString(testToolEndpoint),
		},
	}

	if tool.Name != testToolName {
		t.Errorf("Tool.Name = %v, want %v", tool.Name, testToolName)
	}

	if tool.Description != nil {
		t.Error("Tool.Description should be nil when not set")
	}

	if tool.Schema != nil {
		t.Error("Tool.Schema should be nil when not set")
	}

	if tool.Timeout != nil {
		t.Error("Tool.Timeout should be nil when not set (default applied by API server)")
	}

	if tool.Retries != nil {
		t.Error("Tool.Retries should be nil when not set (default applied by API server)")
	}
}
