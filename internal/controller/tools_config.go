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

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ToolConfig represents the tools configuration file format for the runtime.
// This is passed to the runtime container as a YAML file.
type ToolConfig struct {
	Handlers []HandlerEntry `json:"handlers"`
}

// HandlerEntry represents a single handler in the config.
type HandlerEntry struct {
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Endpoint      string          `json:"endpoint"`
	Tool          *ToolDefinition `json:"tool,omitempty"` // For http/grpc handlers
	HTTPConfig    *ToolHTTP       `json:"httpConfig,omitempty"`
	GRPCConfig    *ToolGRPC       `json:"grpcConfig,omitempty"`
	MCPConfig     *ToolMCP        `json:"mcpConfig,omitempty"`
	OpenAPIConfig *ToolOpenAPI    `json:"openAPIConfig,omitempty"`
	Timeout       string          `json:"timeout,omitempty"`
	Retries       int32           `json:"retries,omitempty"`
}

// ToolDefinition represents the tool interface for HTTP/gRPC handlers.
type ToolDefinition struct {
	Name         string      `json:"name"`
	Description  string      `json:"description"`
	InputSchema  interface{} `json:"inputSchema"`
	OutputSchema interface{} `json:"outputSchema,omitempty"`
}

// ToolHTTP represents HTTP configuration for a handler.
type ToolHTTP struct {
	Endpoint    string            `json:"endpoint"`
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
}

// ToolGRPC represents gRPC configuration for a handler.
type ToolGRPC struct {
	Endpoint              string `json:"endpoint"`
	TLS                   bool   `json:"tls,omitempty"`
	TLSCertPath           string `json:"tlsCertPath,omitempty"`
	TLSKeyPath            string `json:"tlsKeyPath,omitempty"`
	TLSCAPath             string `json:"tlsCAPath,omitempty"`
	TLSInsecureSkipVerify bool   `json:"tlsInsecureSkipVerify,omitempty"`
}

// ToolMCP represents MCP configuration for a handler.
type ToolMCP struct {
	Transport string            `json:"transport"`
	Endpoint  string            `json:"endpoint,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	WorkDir   string            `json:"workDir,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// ToolOpenAPI represents OpenAPI configuration for a handler.
type ToolOpenAPI struct {
	SpecURL         string   `json:"specURL"`
	BaseURL         string   `json:"baseURL,omitempty"`
	OperationFilter []string `json:"operationFilter,omitempty"`
}

// reconcileToolsConfigMap creates or updates the tools ConfigMap from ToolRegistry.
func (r *AgentRuntimeReconciler) reconcileToolsConfigMap(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
	toolRegistry *omniav1alpha1.ToolRegistry,
) error {
	log := logf.FromContext(ctx)

	// Build tools config from ToolRegistry
	toolsConfig := r.buildToolsConfig(toolRegistry)

	// Serialize to YAML
	configData, err := yaml.Marshal(toolsConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal tools config: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name + ToolsConfigMapSuffix,
			Namespace: agentRuntime.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(agentRuntime, configMap, r.Scheme); err != nil {
			return err
		}

		labels := map[string]string{
			labelAppName:      labelValueOmniaAgent,
			labelAppInstance:  agentRuntime.Name,
			labelAppManagedBy: labelValueOmniaOperator,
			labelOmniaComp:    toolsConfigVolumeName,
		}

		configMap.Labels = labels
		configMap.Data = map[string]string{
			ToolsConfigFileName: string(configData),
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile tools ConfigMap: %w", err)
	}

	log.Info("Tools ConfigMap reconciled", "result", result, "handlers", len(toolsConfig.Handlers))
	return nil
}

// findEndpoint finds the resolved endpoint for a handler from the discovered tools.
func findEndpoint(toolRegistry *omniav1alpha1.ToolRegistry, handlerName string) string {
	for _, discovered := range toolRegistry.Status.DiscoveredTools {
		if discovered.HandlerName == handlerName && discovered.Status == omniav1alpha1.ToolStatusAvailable {
			return discovered.Endpoint
		}
	}
	return ""
}

// buildToolDefinition builds a ToolDefinition from the handler's tool spec.
func buildToolDefinition(tool *omniav1alpha1.ToolDefinition) *ToolDefinition {
	if tool == nil {
		return nil
	}
	def := &ToolDefinition{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: tool.InputSchema.Raw,
	}
	if tool.OutputSchema != nil {
		def.OutputSchema = tool.OutputSchema.Raw
	}
	return def
}

// buildHTTPConfig builds HTTP configuration for a handler entry.
func buildHTTPConfig(h *omniav1alpha1.HandlerDefinition, endpoint string) *ToolHTTP {
	if h.HTTPConfig == nil {
		return nil
	}
	return &ToolHTTP{
		Endpoint:    endpoint,
		Method:      h.HTTPConfig.Method,
		Headers:     h.HTTPConfig.Headers,
		ContentType: h.HTTPConfig.ContentType,
	}
}

// buildGRPCConfig builds gRPC configuration for a handler entry.
func buildGRPCConfig(h *omniav1alpha1.HandlerDefinition, endpoint string) *ToolGRPC {
	if h.GRPCConfig == nil {
		return nil
	}
	cfg := &ToolGRPC{
		Endpoint:              endpoint,
		TLS:                   h.GRPCConfig.TLS,
		TLSInsecureSkipVerify: h.GRPCConfig.TLSInsecureSkipVerify,
	}
	if h.GRPCConfig.TLSCertPath != nil {
		cfg.TLSCertPath = *h.GRPCConfig.TLSCertPath
	}
	if h.GRPCConfig.TLSKeyPath != nil {
		cfg.TLSKeyPath = *h.GRPCConfig.TLSKeyPath
	}
	if h.GRPCConfig.TLSCAPath != nil {
		cfg.TLSCAPath = *h.GRPCConfig.TLSCAPath
	}
	return cfg
}

// buildMCPConfig builds MCP configuration for a handler entry.
func buildMCPConfig(h *omniav1alpha1.HandlerDefinition) *ToolMCP {
	if h.MCPConfig == nil {
		return nil
	}
	cfg := &ToolMCP{
		Transport: string(h.MCPConfig.Transport),
		Env:       h.MCPConfig.Env,
	}
	if h.MCPConfig.Endpoint != nil {
		cfg.Endpoint = *h.MCPConfig.Endpoint
	}
	if h.MCPConfig.Command != nil {
		cfg.Command = *h.MCPConfig.Command
	}
	if len(h.MCPConfig.Args) > 0 {
		cfg.Args = h.MCPConfig.Args
	}
	if h.MCPConfig.WorkDir != nil {
		cfg.WorkDir = *h.MCPConfig.WorkDir
	}
	return cfg
}

// buildOpenAPIConfig builds OpenAPI configuration for a handler entry.
func buildOpenAPIConfig(h *omniav1alpha1.HandlerDefinition) *ToolOpenAPI {
	if h.OpenAPIConfig == nil {
		return nil
	}
	cfg := &ToolOpenAPI{
		SpecURL:         h.OpenAPIConfig.SpecURL,
		OperationFilter: h.OpenAPIConfig.OperationFilter,
	}
	if h.OpenAPIConfig.BaseURL != nil {
		cfg.BaseURL = *h.OpenAPIConfig.BaseURL
	}
	return cfg
}

// buildHandlerEntry builds a single handler entry from the handler spec.
func buildHandlerEntry(h *omniav1alpha1.HandlerDefinition, endpoint string) HandlerEntry {
	entry := HandlerEntry{
		Name:     h.Name,
		Type:     string(h.Type),
		Endpoint: endpoint,
	}
	if h.Timeout != nil {
		entry.Timeout = *h.Timeout
	}
	if h.Retries != nil {
		entry.Retries = *h.Retries
	}

	switch h.Type {
	case omniav1alpha1.HandlerTypeHTTP:
		entry.HTTPConfig = buildHTTPConfig(h, endpoint)
		entry.Tool = buildToolDefinition(h.Tool)
	case omniav1alpha1.HandlerTypeGRPC:
		entry.GRPCConfig = buildGRPCConfig(h, endpoint)
		entry.Tool = buildToolDefinition(h.Tool)
	case omniav1alpha1.HandlerTypeMCP:
		entry.MCPConfig = buildMCPConfig(h)
	case omniav1alpha1.HandlerTypeOpenAPI:
		entry.OpenAPIConfig = buildOpenAPIConfig(h)
	}

	return entry
}

// buildToolsConfig builds the tools configuration from ToolRegistry spec and status.
func (r *AgentRuntimeReconciler) buildToolsConfig(toolRegistry *omniav1alpha1.ToolRegistry) ToolConfig {
	config := ToolConfig{
		Handlers: make([]HandlerEntry, 0, len(toolRegistry.Spec.Handlers)),
	}

	for _, h := range toolRegistry.Spec.Handlers {
		endpoint := findEndpoint(toolRegistry, h.Name)
		if endpoint == "" {
			continue
		}
		config.Handlers = append(config.Handlers, buildHandlerEntry(&h, endpoint))
	}

	return config
}
