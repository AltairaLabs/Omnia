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
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	content := `tools:
  - name: http-tool
    type: http
    description: An HTTP tool
    httpConfig:
      endpoint: http://example.com/api
      method: POST
    timeout: "30s"
    retries: 3
  - name: mcp-tool
    type: mcp
    description: An MCP tool
    mcpConfig:
      transport: sse
      endpoint: http://mcp-server:8080/sse
  - name: mcp-stdio-tool
    type: mcp
    mcpConfig:
      transport: stdio
      command: /usr/bin/mcp-server
      args:
        - --verbose
        - --port=9000
      workDir: /app
      env:
        DEBUG: "true"
        LOG_LEVEL: info
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tools.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(config.Tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(config.Tools))
	}

	// Verify HTTP tool
	httpTool := config.Tools[0]
	if httpTool.Name != "http-tool" {
		t.Errorf("expected name 'http-tool', got %q", httpTool.Name)
	}
	if httpTool.Type != "http" {
		t.Errorf("expected type 'http', got %q", httpTool.Type)
	}
	if httpTool.HTTPConfig == nil {
		t.Fatal("httpConfig should not be nil")
	}
	if httpTool.HTTPConfig.Endpoint != "http://example.com/api" {
		t.Errorf("unexpected endpoint: %q", httpTool.HTTPConfig.Endpoint)
	}
	if httpTool.Timeout != "30s" {
		t.Errorf("expected timeout '30s', got %q", httpTool.Timeout)
	}
	if httpTool.Retries != 3 {
		t.Errorf("expected retries 3, got %d", httpTool.Retries)
	}

	// Verify MCP SSE tool
	mcpTool := config.Tools[1]
	if mcpTool.Name != "mcp-tool" {
		t.Errorf("expected name 'mcp-tool', got %q", mcpTool.Name)
	}
	if mcpTool.Type != "mcp" {
		t.Errorf("expected type 'mcp', got %q", mcpTool.Type)
	}
	if mcpTool.MCPConfig == nil {
		t.Fatal("mcpConfig should not be nil")
	}
	if mcpTool.MCPConfig.Transport != "sse" {
		t.Errorf("expected transport 'sse', got %q", mcpTool.MCPConfig.Transport)
	}
	if mcpTool.MCPConfig.Endpoint != "http://mcp-server:8080/sse" {
		t.Errorf("unexpected endpoint: %q", mcpTool.MCPConfig.Endpoint)
	}

	// Verify MCP stdio tool
	stdioTool := config.Tools[2]
	if stdioTool.MCPConfig.Transport != "stdio" {
		t.Errorf("expected transport 'stdio', got %q", stdioTool.MCPConfig.Transport)
	}
	if stdioTool.MCPConfig.Command != "/usr/bin/mcp-server" {
		t.Errorf("unexpected command: %q", stdioTool.MCPConfig.Command)
	}
	if len(stdioTool.MCPConfig.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(stdioTool.MCPConfig.Args))
	}
	if stdioTool.MCPConfig.WorkDir != "/app" {
		t.Errorf("unexpected workDir: %q", stdioTool.MCPConfig.WorkDir)
	}
	if stdioTool.MCPConfig.Env["DEBUG"] != "true" {
		t.Errorf("unexpected env DEBUG: %q", stdioTool.MCPConfig.Env["DEBUG"])
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
