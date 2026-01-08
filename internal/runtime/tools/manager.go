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

package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// Manager manages tool adapters and routes tool calls.
type Manager struct {
	log      logr.Logger
	adapters map[string]ToolAdapter // adapter name -> adapter
	tools    map[string]string      // tool name -> adapter name
	mu       sync.RWMutex
}

// NewManager creates a new tool manager.
func NewManager(log logr.Logger) *Manager {
	return &Manager{
		log:      log,
		adapters: make(map[string]ToolAdapter),
		tools:    make(map[string]string),
	}
}

// RegisterAdapter registers a tool adapter.
func (m *Manager) RegisterAdapter(adapter ToolAdapter) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := adapter.Name()
	if _, exists := m.adapters[name]; exists {
		return fmt.Errorf("adapter %q already registered", name)
	}

	m.adapters[name] = adapter
	return nil
}

// Connect connects all registered adapters and discovers tools.
func (m *Manager) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, adapter := range m.adapters {
		m.log.Info("connecting adapter", "adapter", name)

		if err := adapter.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect adapter %q: %w", name, err)
		}

		// Discover tools from this adapter
		tools, err := adapter.ListTools(ctx)
		if err != nil {
			return fmt.Errorf("failed to list tools from adapter %q: %w", name, err)
		}

		for _, tool := range tools {
			if existing, exists := m.tools[tool.Name]; exists {
				m.log.Info("tool already registered, skipping",
					"tool", tool.Name,
					"existingAdapter", existing,
					"newAdapter", name)
				continue
			}
			m.tools[tool.Name] = name
			m.log.V(1).Info("registered tool", "tool", tool.Name, "adapter", name)
		}
	}

	m.log.Info("all adapters connected", "adapterCount", len(m.adapters), "toolCount", len(m.tools))
	return nil
}

// ListTools returns all available tools across all adapters.
func (m *Manager) ListTools() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.tools))
	for name := range m.tools {
		names = append(names, name)
	}
	return names
}

// Call invokes a tool by name.
func (m *Manager) Call(ctx context.Context, toolName string, args map[string]any) (*ToolResult, error) {
	m.mu.RLock()
	adapterName, exists := m.tools[toolName]
	if !exists {
		m.mu.RUnlock()
		return nil, fmt.Errorf("tool %q not found", toolName)
	}
	adapter := m.adapters[adapterName]
	m.mu.RUnlock()

	m.log.V(1).Info("calling tool", "tool", toolName, "adapter", adapterName)
	return adapter.Call(ctx, toolName, args)
}

// Close closes all adapters.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for name, adapter := range m.adapters {
		if err := adapter.Close(); err != nil {
			m.log.Error(err, "failed to close adapter", "adapter", name)
			errs = append(errs, fmt.Errorf("adapter %q: %w", name, err))
		}
	}

	m.adapters = make(map[string]ToolAdapter)
	m.tools = make(map[string]string)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing adapters: %v", errs)
	}
	return nil
}

// LoadFromConfig loads adapters from a tool configuration file.
func (m *Manager) LoadFromConfig(configPath string) error {
	config, err := LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	return m.LoadFromToolConfig(config)
}

// LoadFromToolConfig loads adapters from a parsed tool configuration.
// It prioritizes the new Handlers format over the legacy Tools format.
func (m *Manager) LoadFromToolConfig(config *ToolConfig) error {
	// Use new Handlers format if available, otherwise fall back to legacy Tools
	if len(config.Handlers) > 0 {
		return m.loadFromHandlers(config.Handlers)
	}

	// Legacy format - process Tools
	return m.loadFromLegacyTools(config.Tools)
}

// loadFromHandlers loads adapters from the new handler-based config format.
func (m *Manager) loadFromHandlers(handlers []HandlerEntry) error {
	for _, h := range handlers {
		if err := m.loadHandler(h); err != nil {
			return err
		}
	}
	return nil
}

// loadHandler processes a single handler entry and registers the appropriate adapter.
func (m *Manager) loadHandler(h HandlerEntry) error {
	timeout := m.parseTimeout(h.Name, h.Timeout)

	switch h.Type {
	case ToolTypeMCP:
		return m.loadMCPHandler(h)
	case ToolTypeOpenAPI:
		return m.loadOpenAPIHandler(h, timeout)
	case ToolTypeHTTP:
		return m.loadHTTPHandler(h, timeout)
	case ToolTypeGRPC:
		return m.loadGRPCHandler(h, timeout)
	default:
		m.log.Info("unknown handler type", "handler", h.Name, "type", h.Type)
	}
	return nil
}

// loadMCPHandler processes an MCP handler entry.
func (m *Manager) loadMCPHandler(h HandlerEntry) error {
	if h.MCPConfig == nil {
		m.log.Info("skipping MCP handler without config", "handler", h.Name)
		return nil
	}
	return m.registerMCPAdapter(h.Name, h.MCPConfig)
}

// loadOpenAPIHandler processes an OpenAPI handler entry.
func (m *Manager) loadOpenAPIHandler(h HandlerEntry, timeout time.Duration) error {
	if h.OpenAPIConfig == nil {
		m.log.Info("skipping OpenAPI handler without config", "handler", h.Name)
		return nil
	}
	return m.registerOpenAPIAdapter(h.Name, h.OpenAPIConfig, timeout)
}

// loadHTTPHandler processes an HTTP handler entry.
func (m *Manager) loadHTTPHandler(h HandlerEntry, timeout time.Duration) error {
	if h.HTTPConfig == nil {
		m.log.Info("skipping HTTP handler without config", "handler", h.Name)
		return nil
	}
	if h.Tool == nil {
		m.log.Info("skipping HTTP handler without tool definition", "handler", h.Name)
		return nil
	}
	return m.registerHTTPAdapter(h.Name, h.HTTPConfig, h.Tool, timeout)
}

// loadGRPCHandler processes a gRPC handler entry.
func (m *Manager) loadGRPCHandler(h HandlerEntry, timeout time.Duration) error {
	if h.GRPCConfig == nil {
		m.log.Info("skipping gRPC handler without config", "handler", h.Name)
		return nil
	}
	if h.Tool == nil {
		m.log.Info("skipping gRPC handler without tool definition", "handler", h.Name)
		return nil
	}
	return m.registerGRPCAdapter(h.Name, h.GRPCConfig, h.Tool, timeout)
}

// loadFromLegacyTools loads adapters from the legacy tool-based config format.
// Deprecated: This is maintained for backward compatibility.
func (m *Manager) loadFromLegacyTools(tools []ToolEntry) error {
	for _, tool := range tools {
		if err := m.loadLegacyTool(tool); err != nil {
			return err
		}
	}
	return nil
}

// loadLegacyTool processes a single legacy tool entry and registers the appropriate adapter.
func (m *Manager) loadLegacyTool(tool ToolEntry) error {
	timeout := m.parseTimeout(tool.Name, tool.Timeout)

	switch tool.Type {
	case ToolTypeMCP:
		return m.loadLegacyMCPTool(tool)
	case ToolTypeGRPC:
		return m.loadLegacyGRPCTool(tool, timeout)
	case ToolTypeHTTP:
		return m.loadLegacyHTTPTool(tool, timeout)
	case ToolTypeOpenAPI:
		return m.loadLegacyOpenAPITool(tool, timeout)
	default:
		m.log.Info("unknown tool type", "tool", tool.Name, "type", tool.Type)
	}
	return nil
}

// loadLegacyMCPTool processes a legacy MCP tool entry.
func (m *Manager) loadLegacyMCPTool(tool ToolEntry) error {
	if tool.MCPConfig == nil {
		m.log.Info("skipping MCP tool without config", "tool", tool.Name)
		return nil
	}
	return m.registerMCPAdapter(tool.Name, tool.MCPConfig)
}

// loadLegacyGRPCTool processes a legacy gRPC tool entry.
func (m *Manager) loadLegacyGRPCTool(tool ToolEntry, timeout time.Duration) error {
	if tool.GRPCConfig == nil {
		m.log.Info("skipping gRPC tool without config", "tool", tool.Name)
		return nil
	}
	return m.registerGRPCAdapter(tool.Name, tool.GRPCConfig, nil, timeout)
}

// loadLegacyHTTPTool processes a legacy HTTP tool entry.
func (m *Manager) loadLegacyHTTPTool(tool ToolEntry, timeout time.Duration) error {
	if tool.HTTPConfig == nil {
		m.log.Info("skipping HTTP tool without config", "tool", tool.Name)
		return nil
	}
	return m.registerHTTPAdapter(tool.Name, tool.HTTPConfig, nil, timeout)
}

// loadLegacyOpenAPITool processes a legacy OpenAPI tool entry.
func (m *Manager) loadLegacyOpenAPITool(tool ToolEntry, timeout time.Duration) error {
	if tool.OpenAPIConfig == nil {
		m.log.Info("skipping OpenAPI tool without config", "tool", tool.Name)
		return nil
	}
	return m.registerOpenAPIAdapter(tool.Name, tool.OpenAPIConfig, timeout)
}

// parseTimeout parses a timeout string and returns the duration.
func (m *Manager) parseTimeout(name, timeoutStr string) time.Duration {
	if timeoutStr == "" {
		return 0
	}
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		m.log.Info("invalid timeout, using default", "name", name, "timeout", timeoutStr)
		return 0
	}
	return timeout
}

// extractInputSchema converts the tool input schema to a map if possible.
func extractInputSchema(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	if schemaMap, ok := schema.(map[string]any); ok {
		return schemaMap
	}
	return nil
}

// registerMCPAdapter creates and registers an MCP adapter from handler config.
func (m *Manager) registerMCPAdapter(name string, config *MCPCfg) error {
	adapterConfig := MCPAdapterConfig{
		Name:      name,
		Transport: MCPTransportType(config.Transport),
		Endpoint:  config.Endpoint,
		Command:   config.Command,
		Args:      config.Args,
		WorkDir:   config.WorkDir,
		Env:       config.Env,
	}
	adapter := NewMCPAdapter(adapterConfig, m.log)
	if err := m.RegisterAdapter(adapter); err != nil {
		return fmt.Errorf("failed to register MCP adapter %q: %w", name, err)
	}
	return nil
}

// registerOpenAPIAdapter creates and registers an OpenAPI adapter from handler config.
func (m *Manager) registerOpenAPIAdapter(name string, config *OpenAPICfg, timeout time.Duration) error {
	adapterConfig := OpenAPIAdapterConfig{
		Name:            name,
		SpecURL:         config.SpecURL,
		BaseURL:         config.BaseURL,
		OperationFilter: config.OperationFilter,
		Headers:         config.Headers,
		AuthType:        config.AuthType,
		AuthToken:       config.AuthToken,
		Timeout:         timeout,
	}
	adapter := NewOpenAPIAdapter(adapterConfig, m.log)
	if err := m.RegisterAdapter(adapter); err != nil {
		return fmt.Errorf("failed to register OpenAPI adapter %q: %w", name, err)
	}
	return nil
}

// registerHTTPAdapter creates and registers an HTTP adapter from handler config.
// If tool is nil, the adapter is created without tool definition (legacy mode).
func (m *Manager) registerHTTPAdapter(name string, config *HTTPCfg, tool *ToolDefCfg, timeout time.Duration) error {
	adapterConfig := HTTPAdapterConfig{
		Name:        name,
		Endpoint:    config.Endpoint,
		Method:      config.Method,
		Headers:     config.Headers,
		ContentType: config.ContentType,
		AuthType:    config.AuthType,
		AuthToken:   config.AuthToken,
		Timeout:     timeout,
	}
	if tool != nil {
		adapterConfig.ToolName = tool.Name
		adapterConfig.ToolDescription = tool.Description
		adapterConfig.ToolInputSchema = extractInputSchema(tool.InputSchema)
	}
	adapter := NewHTTPAdapter(adapterConfig, m.log)
	if err := m.RegisterAdapter(adapter); err != nil {
		return fmt.Errorf("failed to register HTTP adapter %q: %w", name, err)
	}
	return nil
}

// registerGRPCAdapter creates and registers a gRPC adapter from handler config.
// If tool is nil, the adapter is created without tool definition (legacy mode).
func (m *Manager) registerGRPCAdapter(name string, config *GRPCCfg, tool *ToolDefCfg, timeout time.Duration) error {
	adapterConfig := GRPCAdapterConfig{
		Name:                  name,
		Endpoint:              config.Endpoint,
		TLS:                   config.TLS,
		TLSCertPath:           config.TLSCertPath,
		TLSKeyPath:            config.TLSKeyPath,
		TLSCAPath:             config.TLSCAPath,
		TLSInsecureSkipVerify: config.TLSInsecureSkipVerify,
		Timeout:               timeout,
	}
	if tool != nil {
		adapterConfig.ToolName = tool.Name
		adapterConfig.ToolDescription = tool.Description
		adapterConfig.ToolInputSchema = extractInputSchema(tool.InputSchema)
	}
	adapter := NewGRPCAdapter(adapterConfig, m.log)
	if err := m.RegisterAdapter(adapter); err != nil {
		return fmt.Errorf("failed to register gRPC adapter %q: %w", name, err)
	}
	return nil
}
