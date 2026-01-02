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
	"os/exec"
	"sync"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPTransportType defines the transport type for MCP connections.
type MCPTransportType string

const (
	// MCPTransportSSE indicates connection via Server-Sent Events.
	MCPTransportSSE MCPTransportType = "sse"
	// MCPTransportStdio indicates connection via subprocess stdin/stdout.
	MCPTransportStdio MCPTransportType = "stdio"
)

// MCPAdapterConfig contains configuration for the MCP adapter.
type MCPAdapterConfig struct {
	// Name is the adapter's unique name.
	Name string

	// Transport is the transport type (sse or stdio).
	Transport MCPTransportType

	// Endpoint is the SSE server URL (for SSE transport).
	Endpoint string

	// Command is the command to run (for stdio transport).
	Command string

	// Args are command arguments (for stdio transport).
	Args []string

	// WorkDir is the working directory (for stdio transport).
	WorkDir string

	// Env are additional environment variables (for stdio transport).
	Env map[string]string
}

// MCPAdapter implements ToolAdapter for MCP servers.
type MCPAdapter struct {
	config  MCPAdapterConfig
	log     logr.Logger
	client  *mcp.Client
	session *mcp.ClientSession
	tools   map[string]*mcp.Tool
	mu      sync.RWMutex
}

// NewMCPAdapter creates a new MCP adapter.
func NewMCPAdapter(config MCPAdapterConfig, log logr.Logger) *MCPAdapter {
	return &MCPAdapter{
		config: config,
		log:    log.WithValues("adapter", config.Name, "transport", config.Transport),
		tools:  make(map[string]*mcp.Tool),
	}
}

// Name returns the adapter's name.
func (a *MCPAdapter) Name() string {
	return a.config.Name
}

// Connect establishes connection to the MCP server.
func (a *MCPAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Create MCP client
	a.client = mcp.NewClient(
		&mcp.Implementation{
			Name:    "omnia-runtime",
			Version: "v1.0.0",
		},
		nil,
	)

	// Create transport based on configuration
	var transport mcp.Transport
	switch a.config.Transport {
	case MCPTransportSSE:
		a.log.Info("connecting via SSE", "endpoint", a.config.Endpoint)
		transport = &mcp.SSEClientTransport{
			Endpoint: a.config.Endpoint,
		}

	case MCPTransportStdio:
		a.log.Info("connecting via stdio", "command", a.config.Command)
		cmd := exec.Command(a.config.Command, a.config.Args...)
		if a.config.WorkDir != "" {
			cmd.Dir = a.config.WorkDir
		}
		if len(a.config.Env) > 0 {
			for k, v := range a.config.Env {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}
		}
		transport = &mcp.CommandTransport{
			Command: cmd,
		}

	default:
		return fmt.Errorf("unsupported transport type: %s", a.config.Transport)
	}

	// Connect to the server
	session, err := a.client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to MCP server: %w", err)
	}
	a.session = session

	// Discover available tools
	a.tools = make(map[string]*mcp.Tool)
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			a.log.Error(err, "failed to list tool")
			continue
		}
		a.tools[tool.Name] = tool
		a.log.V(1).Info("discovered tool", "name", tool.Name, "description", tool.Description)
	}

	a.log.Info("connected to MCP server", "toolCount", len(a.tools))
	return nil
}

// ListTools returns available tools from this adapter.
func (a *MCPAdapter) ListTools(ctx context.Context) ([]ToolInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	tools := make([]ToolInfo, 0, len(a.tools))
	for _, tool := range a.tools {
		info := ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
		}
		if tool.InputSchema != nil {
			// Convert the input schema to map[string]any
			if schema, ok := tool.InputSchema.(map[string]any); ok {
				info.InputSchema = schema
			}
		}
		tools = append(tools, info)
	}
	return tools, nil
}

// Call invokes a tool with the given arguments.
func (a *MCPAdapter) Call(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	a.mu.RLock()
	session := a.session
	_, exists := a.tools[name]
	a.mu.RUnlock()

	if session == nil {
		return nil, fmt.Errorf("adapter not connected")
	}
	if !exists {
		return nil, fmt.Errorf("tool %q not found in adapter %q", name, a.config.Name)
	}

	a.log.V(1).Info("calling tool", "name", name)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("tool call failed: %w", err)
	}

	// Extract content from result
	var content any
	if len(result.Content) > 0 {
		// For simplicity, return the first content item
		// In a more complete implementation, you might want to handle multiple content items
		content = result.Content[0]
	} else if result.StructuredContent != nil {
		content = result.StructuredContent
	}

	return &ToolResult{
		Content: content,
		IsError: result.IsError,
	}, nil
}

// Close closes the connection.
func (a *MCPAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.session != nil {
		a.log.Info("closing MCP connection")
		if err := a.session.Close(); err != nil {
			return fmt.Errorf("failed to close session: %w", err)
		}
		a.session = nil
	}

	a.tools = make(map[string]*mcp.Tool)
	return nil
}
