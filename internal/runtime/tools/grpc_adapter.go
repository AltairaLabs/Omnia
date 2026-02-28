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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	toolsv1 "github.com/altairalabs/omnia/pkg/tools/v1"
)

// GRPCAdapterConfig contains configuration for the gRPC adapter.
type GRPCAdapterConfig struct {
	// Name is the adapter's unique name.
	Name string

	// Endpoint is the gRPC server address (host:port).
	Endpoint string

	// TLS enables TLS for the connection.
	TLS bool

	// TLSCertPath is the path to the TLS certificate (optional).
	TLSCertPath string

	// TLSKeyPath is the path to the TLS key (optional).
	TLSKeyPath string

	// TLSCAPath is the path to the CA certificate (optional).
	TLSCAPath string

	// TLSInsecureSkipVerify skips TLS verification (not recommended for production).
	TLSInsecureSkipVerify bool

	// Timeout is the connection timeout.
	Timeout time.Duration

	// ToolName is the tool name exposed by this handler.
	// If provided, tool discovery via ListTools is skipped.
	ToolName string

	// ToolDescription is the tool description (shown to LLM).
	ToolDescription string

	// ToolInputSchema is the JSON Schema for the tool's input.
	ToolInputSchema map[string]any
}

// GRPCAdapter implements ToolAdapter for gRPC tool services.
type GRPCAdapter struct {
	config GRPCAdapterConfig
	log    logr.Logger
	conn   *grpc.ClientConn
	client toolsv1.ToolServiceClient
	tools  map[string]*toolsv1.ToolInfo
	mu     sync.RWMutex
}

// NewGRPCAdapter creates a new gRPC adapter.
func NewGRPCAdapter(config GRPCAdapterConfig, log logr.Logger) *GRPCAdapter {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	return &GRPCAdapter{
		config: config,
		log:    log.WithValues("adapter", config.Name, "endpoint", config.Endpoint),
		tools:  make(map[string]*toolsv1.ToolInfo),
	}
}

// Name returns the adapter's name.
func (a *GRPCAdapter) Name() string {
	return a.config.Name
}

// Connect establishes connection to the gRPC server.
func (a *GRPCAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	opts, err := a.buildDialOptions()
	if err != nil {
		return err
	}

	if err := a.establishConnection(ctx, opts); err != nil {
		return err
	}

	a.tools = make(map[string]*toolsv1.ToolInfo)
	a.initializeTools(ctx)

	a.log.Info("connected to gRPC server", "toolCount", len(a.tools))
	return nil
}

// buildDialOptions builds gRPC dial options including TLS configuration.
func (a *GRPCAdapter) buildDialOptions() ([]grpc.DialOption, error) {
	if !a.config.TLS {
		a.log.Info("connecting without TLS", "endpoint", a.config.Endpoint)
		return []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, nil
	}

	tlsConfig, err := a.buildTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS config: %w", err)
	}
	a.log.Info("connecting with TLS", "endpoint", a.config.Endpoint)
	return []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))}, nil
}

// establishConnection establishes the gRPC connection.
func (a *GRPCAdapter) establishConnection(ctx context.Context, opts []grpc.DialOption) error {
	ctx, cancel := context.WithTimeout(ctx, a.config.Timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, a.config.Endpoint, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server: %w", err)
	}
	a.conn = conn
	a.client = toolsv1.NewToolServiceClient(conn)
	return nil
}

// initializeTools initializes the tool map from config or discovery.
func (a *GRPCAdapter) initializeTools(ctx context.Context) {
	if a.config.ToolName != "" {
		a.registerConfiguredTool()
		return
	}
	a.discoverTools(ctx)
}

// registerConfiguredTool registers a tool from the adapter configuration.
func (a *GRPCAdapter) registerConfiguredTool() {
	inputSchemaJSON := ""
	if a.config.ToolInputSchema != nil {
		if schemaBytes, err := json.Marshal(a.config.ToolInputSchema); err == nil {
			inputSchemaJSON = string(schemaBytes)
		}
	}
	a.tools[a.config.ToolName] = &toolsv1.ToolInfo{
		Name:        a.config.ToolName,
		Description: a.config.ToolDescription,
		InputSchema: inputSchemaJSON,
	}
	a.log.Info("using configured tool definition", "tool", a.config.ToolName)
}

// discoverTools discovers available tools via ListTools RPC.
func (a *GRPCAdapter) discoverTools(ctx context.Context) {
	resp, err := a.client.ListTools(ctx, &toolsv1.ListToolsRequest{})
	if err != nil {
		a.log.V(1).Info("ListTools not available, tools will be discovered on first call", "error", err)
		return
	}
	for _, tool := range resp.Tools {
		a.tools[tool.Name] = tool
		a.log.V(1).Info("discovered tool", "name", tool.Name, "description", tool.Description)
	}
}

// buildTLSConfig builds the TLS configuration.
func (a *GRPCAdapter) buildTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: a.config.TLSInsecureSkipVerify,
	}

	// Load CA certificate if provided
	if a.config.TLSCAPath != "" {
		caCert, err := os.ReadFile(a.config.TLSCAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate if provided
	if a.config.TLSCertPath != "" && a.config.TLSKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(a.config.TLSCertPath, a.config.TLSKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// ListTools returns available tools from this adapter.
func (a *GRPCAdapter) ListTools(ctx context.Context) ([]ToolInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	tools := make([]ToolInfo, 0, len(a.tools))
	for _, tool := range a.tools {
		info := ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
		}
		if tool.InputSchema != "" {
			var schema map[string]any
			if json.Unmarshal([]byte(tool.InputSchema), &schema) == nil {
				info.InputSchema = schema
			}
		}
		tools = append(tools, info)
	}
	return tools, nil
}

// Call invokes a tool with the given arguments.
func (a *GRPCAdapter) Call(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("adapter not connected")
	}

	// Serialize arguments to JSON
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal arguments: %w", err)
	}

	a.log.V(1).Info("calling tool", "name", name)

	// Inject policy propagation metadata into outgoing gRPC context
	md := PolicyGRPCMetadata(ctx, name, a.config.Name, args)
	if len(md) > 0 {
		pairs := make([]string, 0, len(md)*2)
		for k, v := range md {
			pairs = append(pairs, k, v)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	}

	resp, err := client.Execute(ctx, &toolsv1.ToolRequest{
		ToolName:      name,
		ArgumentsJson: string(argsJSON),
	})
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	// Parse result
	var content any
	if resp.ResultJson != "" {
		if json.Unmarshal([]byte(resp.ResultJson), &content) != nil {
			// If not valid JSON, use as string
			content = resp.ResultJson
		}
	}

	if resp.IsError {
		return &ToolResult{
			Content: resp.ErrorMessage,
			IsError: true,
		}, nil
	}

	return &ToolResult{
		Content: content,
		IsError: false,
	}, nil
}

// Close closes the connection.
func (a *GRPCAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn != nil {
		a.log.Info("closing gRPC connection")
		if err := a.conn.Close(); err != nil {
			return fmt.Errorf("failed to close connection: %w", err)
		}
		a.conn = nil
		a.client = nil
	}

	a.tools = make(map[string]*toolsv1.ToolInfo)
	return nil
}
