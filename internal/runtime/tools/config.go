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
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ToolConfig represents the tools configuration file format.
type ToolConfig struct {
	Tools []ToolEntry `json:"tools" yaml:"tools"`
}

// ToolEntry represents a single tool in the config.
type ToolEntry struct {
	Name        string   `json:"name" yaml:"name"`
	Type        string   `json:"type" yaml:"type"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	HTTPConfig  *HTTPCfg `json:"httpConfig,omitempty" yaml:"httpConfig,omitempty"`
	GRPCConfig  *GRPCCfg `json:"grpcConfig,omitempty" yaml:"grpcConfig,omitempty"`
	MCPConfig   *MCPCfg  `json:"mcpConfig,omitempty" yaml:"mcpConfig,omitempty"`
	Timeout     string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Retries     int32    `json:"retries,omitempty" yaml:"retries,omitempty"`
}

// HTTPCfg represents HTTP configuration for a tool.
type HTTPCfg struct {
	Endpoint string `json:"endpoint" yaml:"endpoint"`
	Method   string `json:"method,omitempty" yaml:"method,omitempty"`
}

// GRPCCfg represents gRPC configuration for a tool.
type GRPCCfg struct {
	Endpoint              string `json:"endpoint" yaml:"endpoint"`
	TLS                   bool   `json:"tls,omitempty" yaml:"tls,omitempty"`
	TLSCertPath           string `json:"tlsCertPath,omitempty" yaml:"tlsCertPath,omitempty"`
	TLSKeyPath            string `json:"tlsKeyPath,omitempty" yaml:"tlsKeyPath,omitempty"`
	TLSCAPath             string `json:"tlsCAPath,omitempty" yaml:"tlsCAPath,omitempty"`
	TLSInsecureSkipVerify bool   `json:"tlsInsecureSkipVerify,omitempty" yaml:"tlsInsecureSkipVerify,omitempty"`
}

// MCPCfg represents MCP configuration for a tool.
type MCPCfg struct {
	Transport string            `json:"transport" yaml:"transport"`
	Endpoint  string            `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Command   string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args      []string          `json:"args,omitempty" yaml:"args,omitempty"`
	WorkDir   string            `json:"workDir,omitempty" yaml:"workDir,omitempty"`
	Env       map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

// LoadConfig loads tool configuration from a YAML file.
func LoadConfig(path string) (*ToolConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ToolConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// ToolTypeHTTP is the HTTP tool type.
const ToolTypeHTTP = "http"

// ToolTypeGRPC is the gRPC tool type.
const ToolTypeGRPC = "grpc"

// ToolTypeMCP is the MCP tool type.
const ToolTypeMCP = "mcp"
