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
func (m *Manager) LoadFromToolConfig(config *ToolConfig) error {
	for _, tool := range config.Tools {
		switch tool.Type {
		case ToolTypeMCP:
			if tool.MCPConfig == nil {
				m.log.Info("skipping MCP tool without config", "tool", tool.Name)
				continue
			}

			adapterConfig := MCPAdapterConfig{
				Name:      tool.Name,
				Transport: MCPTransportType(tool.MCPConfig.Transport),
				Endpoint:  tool.MCPConfig.Endpoint,
				Command:   tool.MCPConfig.Command,
				Args:      tool.MCPConfig.Args,
				WorkDir:   tool.MCPConfig.WorkDir,
				Env:       tool.MCPConfig.Env,
			}

			adapter := NewMCPAdapter(adapterConfig, m.log)
			if err := m.RegisterAdapter(adapter); err != nil {
				return fmt.Errorf("failed to register MCP adapter %q: %w", tool.Name, err)
			}

		case ToolTypeHTTP, ToolTypeGRPC:
			// HTTP and gRPC tools are handled differently (not via adapters)
			m.log.V(1).Info("skipping non-MCP tool", "tool", tool.Name, "type", tool.Type)

		default:
			m.log.Info("unknown tool type", "tool", tool.Name, "type", tool.Type)
		}
	}

	return nil
}
