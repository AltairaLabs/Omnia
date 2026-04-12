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
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pktools "github.com/AltairaLabs/PromptKit/runtime/tools"

	"github.com/altairalabs/omnia/internal/tracing"
	toolsv1 "github.com/altairalabs/omnia/pkg/tools/v1"
)

// executorName is the name used for registry dispatch. Tools with
// Mode set to this value are routed to the OmniaExecutor.
const executorName = "omnia"

// defaultTimeout is the default timeout for tool execution backends.
const defaultTimeout = 30 * time.Second

// OmniaExecutor implements PromptKit's tools.Executor interface. It loads
// handler configuration from the ToolRegistry-generated tools.yaml and
// dispatches tool calls to the appropriate PromptKit or Omnia backend:
//
//   - HTTP  → delegates to PromptKit's HTTPExecutor (gains request/response
//     mapping, redact, multimodal, aggregate size limits for free).
//   - MCP   → manages MCP client sessions directly.
//   - gRPC  → calls the Omnia ToolService proto with circuit breakers.
//   - OpenAPI → parses the spec, then delegates to the HTTP executor.
//
// It also injects Omnia policy headers and OTel tracing spans.
type OmniaExecutor struct {
	log             logr.Logger
	tracingProvider *tracing.Provider

	// Config loaded from tools.yaml
	config   *ToolConfig
	handlers map[string]*HandlerEntry // handler name → config
	// Tool routing: tool name → handler name
	toolHandlers map[string]string
	// Registry metadata for tracing/event store
	toolMeta map[string]ToolMeta

	// MCP sessions keyed by handler name
	mcpClients  map[string]*mcp.Client
	mcpSessions map[string]*mcp.ClientSession
	mcpTools    map[string]map[string]*mcp.Tool // handler → tool name → tool

	// gRPC connections keyed by handler name
	grpcConns   map[string]*grpc.ClientConn
	grpcClients map[string]toolsv1.ToolServiceClient
	grpcTools   map[string]map[string]*toolsv1.ToolInfo // handler → tool name → tool
	breakers    *ToolCircuitBreakers

	// OpenAPI parsed operations keyed by handler name
	openAPIBaseURLs map[string]string
	openAPIOps      map[string]map[string]*OpenAPIOperation // handler → opID → operation
	openAPIHeaders  map[string]map[string]string            // handler → headers

	mu sync.RWMutex
}

// NewOmniaExecutor creates a new executor.
func NewOmniaExecutor(log logr.Logger, tp *tracing.Provider) *OmniaExecutor {
	return &OmniaExecutor{
		log:             log.WithName("tools"),
		tracingProvider: tp,
		handlers:        make(map[string]*HandlerEntry),
		toolHandlers:    make(map[string]string),
		toolMeta:        make(map[string]ToolMeta),
		mcpClients:      make(map[string]*mcp.Client),
		mcpSessions:     make(map[string]*mcp.ClientSession),
		mcpTools:        make(map[string]map[string]*mcp.Tool),
		grpcConns:       make(map[string]*grpc.ClientConn),
		grpcClients:     make(map[string]toolsv1.ToolServiceClient),
		grpcTools:       make(map[string]map[string]*toolsv1.ToolInfo),
		breakers:        NewToolCircuitBreakers(),
		openAPIBaseURLs: make(map[string]string),
		openAPIOps:      make(map[string]map[string]*OpenAPIOperation),
		openAPIHeaders:  make(map[string]map[string]string),
	}
}

// Name implements tools.Executor. The PromptKit registry uses this to match
// tool Mode values to executors.
func (e *OmniaExecutor) Name() string {
	return executorName
}

// LoadConfig loads handler configuration from a tools.yaml file.
func (e *OmniaExecutor) LoadConfig(path string) error {
	config, err := LoadConfig(path)
	if err != nil {
		return fmt.Errorf("failed to load tools config: %w", err)
	}
	e.config = config
	for i := range config.Handlers {
		h := &config.Handlers[i]
		e.handlers[h.Name] = h
	}
	return nil
}

// LoadConfigFromEntries loads handler configuration from pre-built entries.
// This is used by the tool tester to create a short-lived executor without
// needing a tools.yaml file.
func (e *OmniaExecutor) LoadConfigFromEntries(entries []HandlerEntry) error {
	e.config = &ToolConfig{Handlers: entries}
	for i := range entries {
		h := &e.config.Handlers[i]
		e.handlers[h.Name] = h
	}
	return nil
}

// ExecuteTool executes a single tool by name with the given arguments.
// This is a convenience method for testing that bypasses the PromptKit
// registry dispatch.
func (e *OmniaExecutor) ExecuteTool(
	ctx context.Context,
	toolName string,
	args json.RawMessage,
) (json.RawMessage, error) {
	e.mu.RLock()
	handlerName, ok := e.toolHandlers[toolName]
	if !ok {
		e.mu.RUnlock()
		return nil, fmt.Errorf("tool %q not found", toolName)
	}
	handler := e.handlers[handlerName]
	e.mu.RUnlock()

	return e.dispatch(ctx, toolName, handlerName, handler, args)
}

// Initialize connects all configured backends and discovers tools.
func (e *OmniaExecutor) Initialize(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.config == nil {
		return nil
	}

	for name, h := range e.handlers {
		if err := e.initHandler(ctx, name, h); err != nil {
			return fmt.Errorf("handler %q: %w", name, err)
		}
	}

	e.log.Info("tools initialized", "toolCount", len(e.toolHandlers))
	return nil
}

// initHandler connects a single handler and registers its tools.
func (e *OmniaExecutor) initHandler(ctx context.Context, name string, h *HandlerEntry) error {
	switch h.Type {
	case ToolTypeHTTP:
		return e.initHTTPHandler(name, h)
	case ToolTypeMCP:
		return e.initMCPHandler(ctx, name, h)
	case ToolTypeGRPC:
		return e.initGRPCHandler(ctx, name, h)
	case ToolTypeOpenAPI:
		return e.initOpenAPIHandler(ctx, name, h)
	case ToolTypeClient:
		return e.initClientHandler(name, h)
	default:
		e.log.Info("unknown handler type", "handler", name, "type", h.Type)
		return nil
	}
}

// ToolNames returns all registered tool names.
func (e *OmniaExecutor) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names := make([]string, 0, len(e.toolHandlers))
	for name := range e.toolHandlers {
		names = append(names, name)
	}
	return names
}

// ToolDescriptors returns PromptKit-compatible descriptors for all tools.
// These are used to register tools with the conversation's tool registry
// when the tools are not already defined in the pack.
func (e *OmniaExecutor) ToolDescriptors() []*pktools.ToolDescriptor {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var descs []*pktools.ToolDescriptor
	for toolName, handlerName := range e.toolHandlers {
		h := e.handlers[handlerName]
		desc := e.buildDescriptor(toolName, h)
		if desc != nil {
			descs = append(descs, desc)
		}
	}
	return descs
}

// marshalSchema marshals v to JSON if non-nil, returning nil on error.
func marshalSchema(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	bytes, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return bytes
}

// buildDescriptor creates a PromptKit ToolDescriptor for a tool.
func (e *OmniaExecutor) buildDescriptor(toolName string, h *HandlerEntry) *pktools.ToolDescriptor {
	desc := &pktools.ToolDescriptor{
		Name:   toolName,
		Mode:   executorName,
		Labels: e.buildToolLabels(toolName, h),
	}

	// For HTTP tools, the tool definition comes from the handler config
	if h.Tool != nil {
		desc.Description = h.Tool.Description
		desc.InputSchema = marshalSchema(h.Tool.InputSchema)
		desc.OutputSchema = marshalSchema(h.Tool.OutputSchema)
	}

	switch h.Type {
	case ToolTypeMCP:
		e.buildMCPDescriptor(desc, toolName, h.Name)
	case ToolTypeOpenAPI:
		e.buildOpenAPIDescriptor(desc, toolName, h.Name)
	case ToolTypeGRPC:
		e.buildGRPCDescriptor(desc, toolName, h.Name)
	case ToolTypeClient:
		// Client tools use mode="client" so the SDK emits ChunkClientTool
		// instead of executing them server-side.
		desc.Mode = ToolTypeClient
	}

	return desc
}

// buildToolLabels creates labels from the tool's registry metadata.
// These labels propagate through PromptKit events, metrics, and OTel traces.
func (e *OmniaExecutor) buildToolLabels(toolName string, h *HandlerEntry) map[string]string {
	labels := map[string]string{
		"handler_type": h.Type,
		"handler_name": h.Name,
	}
	if meta, ok := e.toolMeta[toolName]; ok {
		if meta.RegistryName != "" {
			labels["registry_name"] = meta.RegistryName
		}
		if meta.RegistryNamespace != "" {
			labels["registry_namespace"] = meta.RegistryNamespace
		}
	}
	return labels
}

// buildMCPDescriptor populates the descriptor from discovered MCP tools.
func (e *OmniaExecutor) buildMCPDescriptor(desc *pktools.ToolDescriptor, toolName, handlerName string) {
	tools, ok := e.mcpTools[handlerName]
	if !ok {
		return
	}
	tool, ok := tools[toolName]
	if !ok {
		return
	}
	desc.Description = tool.Description
	desc.InputSchema = marshalSchema(tool.InputSchema)
}

// buildOpenAPIDescriptor populates the descriptor from parsed OpenAPI operations.
func (e *OmniaExecutor) buildOpenAPIDescriptor(desc *pktools.ToolDescriptor, toolName, handlerName string) {
	ops, ok := e.openAPIOps[handlerName]
	if !ok {
		return
	}
	op, ok := ops[toolName]
	if !ok {
		return
	}
	adapter := &OpenAPIAdapter{log: e.log}
	desc.Description = adapter.buildDescription(op)
	desc.InputSchema = marshalSchema(adapter.buildInputSchema(op))
}

// buildGRPCDescriptor populates the descriptor from discovered gRPC tools.
func (e *OmniaExecutor) buildGRPCDescriptor(desc *pktools.ToolDescriptor, toolName, handlerName string) {
	tools, ok := e.grpcTools[handlerName]
	if !ok {
		return
	}
	tool, ok := tools[toolName]
	if !ok {
		return
	}
	desc.Description = tool.Description
	if tool.InputSchema != "" {
		desc.InputSchema = json.RawMessage(tool.InputSchema)
	}
}

// GetToolMeta returns registry/handler metadata for a tool.
func (e *OmniaExecutor) GetToolMeta(toolName string) (ToolMeta, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	meta, ok := e.toolMeta[toolName]
	return meta, ok
}

// SetRegistryInfo populates registry metadata for tracing/events.
func (e *OmniaExecutor) SetRegistryInfo(registryName, registryNamespace string, cfgHandlers []HandlerEntry) {
	e.mu.Lock()
	defer e.mu.Unlock()

	handlerLookup := make(map[string]HandlerEntry, len(cfgHandlers))
	for _, h := range cfgHandlers {
		handlerLookup[h.Name] = h
	}

	for toolName, handlerName := range e.toolHandlers {
		meta := ToolMeta{
			RegistryName:      registryName,
			RegistryNamespace: registryNamespace,
			HandlerName:       handlerName,
		}
		if h, ok := handlerLookup[handlerName]; ok {
			meta.HandlerType = h.Type
			meta.Endpoint = h.Endpoint
		}
		e.toolMeta[toolName] = meta
	}
}

// Execute implements tools.Executor. It dispatches tool calls to the
// appropriate backend based on handler type.
func (e *OmniaExecutor) Execute(
	ctx context.Context,
	descriptor *pktools.ToolDescriptor,
	args json.RawMessage,
) (json.RawMessage, error) {
	e.mu.RLock()
	handlerName, ok := e.toolHandlers[descriptor.Name]
	if !ok {
		e.mu.RUnlock()
		return nil, fmt.Errorf("tool %q not found", descriptor.Name)
	}
	handler := e.handlers[handlerName]
	e.mu.RUnlock()

	ctx, span := e.startSpan(ctx, descriptor.Name)
	defer span.End()

	result, err := e.dispatch(ctx, descriptor.Name, handlerName, handler, args)
	e.recordResult(span, result, err)
	return result, err
}

// dispatch routes to the type-specific executor.
func (e *OmniaExecutor) dispatch(
	ctx context.Context,
	toolName, handlerName string,
	handler *HandlerEntry,
	args json.RawMessage,
) (json.RawMessage, error) {
	switch handler.Type {
	case ToolTypeHTTP:
		return e.executeHTTP(ctx, toolName, handlerName, handler, args)
	case ToolTypeMCP:
		return e.executeMCP(ctx, toolName, handlerName, args)
	case ToolTypeGRPC:
		return e.executeGRPC(ctx, toolName, handlerName, args)
	case ToolTypeOpenAPI:
		return e.executeOpenAPI(ctx, toolName, handlerName, handler, args)
	default:
		return nil, fmt.Errorf("unsupported handler type: %s", handler.Type)
	}
}

// startSpan starts an OTel span for a tool call.
func (e *OmniaExecutor) startSpan(ctx context.Context, toolName string) (context.Context, trace.Span) {
	if e.tracingProvider == nil {
		return ctx, tracenoop.Span{}
	}
	meta, _ := e.GetToolMeta(toolName)
	return e.tracingProvider.StartToolSpan(ctx, toolName, tracing.ToolSpanMeta{
		RegistryName:      meta.RegistryName,
		RegistryNamespace: meta.RegistryNamespace,
		HandlerName:       meta.HandlerName,
		HandlerType:       meta.HandlerType,
	})
}

// currentSpan extracts the active span from context.
func (e *OmniaExecutor) currentSpan(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// recordResult records span attributes from the execution result.
func (e *OmniaExecutor) recordResult(span trace.Span, _ json.RawMessage, err error) {
	if err != nil {
		tracing.RecordError(span, err)
		return
	}
	tracing.SetSuccess(span)
}

// CheckHealth probes all backends and returns per-tool health status.
func (e *OmniaExecutor) CheckHealth(ctx context.Context) []ToolHealth {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var results []ToolHealth
	for toolName, handlerName := range e.toolHandlers {
		th := ToolHealth{
			ToolName:    toolName,
			AdapterName: handlerName,
			Healthy:     true,
		}
		if err := e.probeHandler(ctx, handlerName); err != nil {
			th.Healthy = false
			th.Error = err.Error()
		}
		results = append(results, th)
	}
	return results
}

// probeHandler performs a basic health check for a handler.
func (e *OmniaExecutor) probeHandler(ctx context.Context, handlerName string) error {
	h, ok := e.handlers[handlerName]
	if !ok {
		return fmt.Errorf("handler %q not found", handlerName)
	}

	switch h.Type {
	case ToolTypeHTTP, ToolTypeOpenAPI:
		return e.probeHTTP(ctx, h)
	default:
		return nil // MCP/gRPC health is implicit from connection state
	}
}

// probeHTTP performs a lightweight HTTP health probe.
func (e *OmniaExecutor) probeHTTP(ctx context.Context, h *HandlerEntry) error {
	endpoint := h.Endpoint
	if h.HTTPConfig != nil {
		endpoint = h.HTTPConfig.Endpoint
	}
	if endpoint == "" {
		return nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodHead, endpoint, nil)
	if err != nil {
		return fmt.Errorf("health check request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("health check: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Close shuts down all connections.
func (e *OmniaExecutor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var errs []error

	for name, session := range e.mcpSessions {
		if err := session.Close(); err != nil {
			errs = append(errs, fmt.Errorf("mcp %q: %w", name, err))
		}
	}
	e.mcpSessions = make(map[string]*mcp.ClientSession)
	e.mcpClients = make(map[string]*mcp.Client)
	e.mcpTools = make(map[string]map[string]*mcp.Tool)

	for name, conn := range e.grpcConns {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("grpc %q: %w", name, err))
		}
	}
	e.grpcConns = make(map[string]*grpc.ClientConn)
	e.grpcClients = make(map[string]toolsv1.ToolServiceClient)
	e.grpcTools = make(map[string]map[string]*toolsv1.ToolInfo)

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// --- HTTP handler ---

// initClientHandler registers a client-side tool. No backend connection needed —
// the SDK handles routing to the client via ChunkClientTool.
func (e *OmniaExecutor) initClientHandler(name string, h *HandlerEntry) error {
	if h.Tool == nil {
		e.log.Info("skipping client handler without tool definition", "handler", name)
		return nil
	}
	toolName := h.Tool.Name
	e.toolHandlers[toolName] = name
	e.log.V(1).Info("registered client tool", "tool", toolName, "handler", name)
	return nil
}

func (e *OmniaExecutor) initHTTPHandler(name string, h *HandlerEntry) error {
	if h.HTTPConfig == nil || h.Tool == nil {
		e.log.Info("skipping HTTP handler without config or tool", "handler", name)
		return nil
	}
	toolName := h.Tool.Name
	e.toolHandlers[toolName] = name
	e.log.V(1).Info("registered HTTP tool", "tool", toolName, "handler", name)
	return nil
}

func (e *OmniaExecutor) executeHTTP(
	ctx context.Context,
	toolName, handlerName string,
	handler *HandlerEntry,
	args json.RawMessage,
) (json.RawMessage, error) {
	cfg := handler.HTTPConfig
	if cfg == nil {
		return nil, fmt.Errorf("handler %q has no HTTP config", handlerName)
	}

	headers := e.buildHTTPHeaders(ctx, cfg, toolName, handlerName, args)
	policy := httpRetryParams(cfg)

	var lastCallResult httpCallResult
	classify := func(_ error) (bool, time.Duration) {
		if cfg.RetryPolicy == nil {
			return false, 0
		}
		return classifyHTTPResult(lastCallResult, cfg.RetryPolicy)
	}

	return retryWithBackoff(ctx, e.log, e.currentSpan(ctx), policy, handler.Timeout.Get(), classify,
		func(attemptCtx context.Context) (json.RawMessage, error) {
			result, callResult, err := doHTTPRequest(attemptCtx, &http.Client{}, cfg, headers, args)
			lastCallResult = callResult
			return result, err
		},
	)
}

// buildHTTPHeaders merges static headers, auth headers, and policy headers.
func (e *OmniaExecutor) buildHTTPHeaders(
	ctx context.Context,
	cfg *HTTPCfg,
	toolName, handlerName string,
	args json.RawMessage,
) map[string]string {
	headers := make(map[string]string)

	// Static headers from config
	for k, v := range cfg.Headers {
		headers[k] = v
	}

	// Auth headers
	if err := mergeAuthHeaders(headers, cfg.AuthType, cfg.AuthToken); err != nil {
		e.log.Error(err, "invalid auth config", "handler", handlerName)
	}

	// Omnia policy headers
	var argsMap map[string]any
	if len(args) > 0 {
		_ = json.Unmarshal(args, &argsMap)
	}
	req := &http.Request{Header: http.Header{}}
	SetAllOutboundHeaders(ctx, req, toolName, handlerName, argsMap)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	return headers
}

// --- MCP handler ---

func (e *OmniaExecutor) initMCPHandler(ctx context.Context, name string, h *HandlerEntry) error {
	if h.MCPConfig == nil {
		e.log.Info("skipping MCP handler without config", "handler", name)
		return nil
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "omnia-runtime", Version: "v1.0.0"},
		nil,
	)

	transport, err := e.buildMCPTransport(h.MCPConfig)
	if err != nil {
		return err
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect MCP: %w", err)
	}

	e.mcpClients[name] = client
	e.mcpSessions[name] = session
	e.mcpTools[name] = make(map[string]*mcp.Tool)

	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			e.log.Error(err, "failed to list MCP tool", "handler", name)
			continue
		}
		if h.MCPConfig.ToolFilter != nil && !h.MCPConfig.ToolFilter.Includes(tool.Name) {
			e.log.V(1).Info("filtered out MCP tool", "tool", tool.Name, "handler", name)
			continue
		}
		e.mcpTools[name][tool.Name] = tool
		e.toolHandlers[tool.Name] = name
		e.log.V(1).Info("registered MCP tool", "tool", tool.Name, "handler", name)
	}

	return nil
}

func (e *OmniaExecutor) buildMCPTransport(cfg *MCPCfg) (mcp.Transport, error) {
	switch MCPTransportType(cfg.Transport) {
	case MCPTransportSSE:
		return &mcp.SSEClientTransport{Endpoint: cfg.Endpoint}, nil
	case MCPTransportStdio:
		cmd := exec.Command(cfg.Command, cfg.Args...)
		if cfg.WorkDir != "" {
			cmd.Dir = cfg.WorkDir
		}
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		return &mcp.CommandTransport{Command: cmd}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP transport: %s", cfg.Transport)
	}
}

func (e *OmniaExecutor) executeMCP(
	ctx context.Context,
	toolName, handlerName string,
	args json.RawMessage,
) (json.RawMessage, error) {
	e.mu.RLock()
	session := e.mcpSessions[handlerName]
	handler := e.handlers[handlerName]
	e.mu.RUnlock()

	if session == nil {
		return nil, fmt.Errorf("MCP handler %q not connected", handlerName)
	}

	policy, classify := mcpRetryParams(handler.MCPConfig)

	return retryWithBackoff(ctx, e.log, e.currentSpan(ctx), policy, handler.Timeout.Get(), classify,
		func(attemptCtx context.Context) (json.RawMessage, error) {
			var argsMap map[string]any
			if len(args) > 0 {
				if err := json.Unmarshal(args, &argsMap); err != nil {
					return nil, fmt.Errorf("failed to parse MCP args: %w", err)
				}
			}

			result, err := session.CallTool(attemptCtx, &mcp.CallToolParams{
				Name:      toolName,
				Arguments: argsMap,
			})
			if err != nil {
				return nil, fmt.Errorf("MCP tool call failed: %w", err)
			}

			// Convert MCP tool errors to mcpToolError so the classifier
			// can distinguish them from transport errors.
			if result.IsError {
				msg := "MCP tool returned error"
				if len(result.Content) > 0 {
					if tc, ok := result.Content[0].(*mcp.TextContent); ok && tc.Text != "" {
						msg = tc.Text
					}
				}
				return nil, &mcpToolError{message: msg}
			}

			return marshalMCPResult(result)
		},
	)
}

func marshalMCPResult(result *mcp.CallToolResult) (json.RawMessage, error) {
	if result.IsError {
		errMsg := "MCP tool returned error"
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcp.TextContent); ok && tc.Text != "" {
				errMsg = tc.Text
			}
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	var content any
	if len(result.Content) == 1 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			content = tc.Text
		} else {
			content = result.Content[0]
		}
	} else if result.StructuredContent != nil {
		content = result.StructuredContent
	} else if len(result.Content) > 0 {
		content = result.Content
	}

	return json.Marshal(content)
}

// --- gRPC handler ---

func (e *OmniaExecutor) initGRPCHandler(ctx context.Context, name string, h *HandlerEntry) error {
	if h.GRPCConfig == nil {
		e.log.Info("skipping gRPC handler without config", "handler", name)
		return nil
	}

	opts, err := buildGRPCDialOptions(h.GRPCConfig)
	if err != nil {
		return err
	}

	dialCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, h.GRPCConfig.Endpoint, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect gRPC: %w", err)
	}

	client := toolsv1.NewToolServiceClient(conn)
	e.grpcConns[name] = conn
	e.grpcClients[name] = client
	e.grpcTools[name] = make(map[string]*toolsv1.ToolInfo)

	// If tool definition is in the handler config, use it directly
	if h.Tool != nil {
		e.grpcTools[name][h.Tool.Name] = &toolsv1.ToolInfo{
			Name:        h.Tool.Name,
			Description: h.Tool.Description,
		}
		e.toolHandlers[h.Tool.Name] = name
		e.log.V(1).Info("registered gRPC tool", "tool", h.Tool.Name, "handler", name)
		return nil
	}

	// Otherwise discover via ListTools RPC
	resp, err := client.ListTools(ctx, &toolsv1.ListToolsRequest{})
	if err != nil {
		e.log.V(1).Info("gRPC ListTools unavailable", "handler", name, "error", err)
		return nil
	}
	for _, tool := range resp.Tools {
		e.grpcTools[name][tool.Name] = tool
		e.toolHandlers[tool.Name] = name
		e.log.V(1).Info("registered gRPC tool", "tool", tool.Name, "handler", name)
	}
	return nil
}

func buildGRPCDialOptions(cfg *GRPCCfg) ([]grpc.DialOption, error) {
	if !cfg.TLS {
		return []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, nil
	}
	tlsCfg, err := buildGRPCTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	return []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))}, nil
}

func buildGRPCTLSConfig(cfg *GRPCCfg) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify, //nolint:gosec // user-configured
	}
	if cfg.TLSCAPath != "" {
		caCert, err := os.ReadFile(cfg.TLSCAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsConfig.RootCAs = pool
	}
	if cfg.TLSCertPath != "" && cfg.TLSKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertPath, cfg.TLSKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
}

func (e *OmniaExecutor) executeGRPC(
	ctx context.Context,
	toolName, handlerName string,
	args json.RawMessage,
) (json.RawMessage, error) {
	e.mu.RLock()
	client := e.grpcClients[handlerName]
	handler := e.handlers[handlerName]
	e.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("gRPC handler %q not connected", handlerName)
	}

	policy, classify := grpcRetryParams(handler.GRPCConfig)

	return retryWithBackoff(ctx, e.log, e.currentSpan(ctx), policy, handler.Timeout.Get(), classify,
		func(attemptCtx context.Context) (json.RawMessage, error) {
			// Inject policy metadata.
			md := PolicyGRPCMetadata(attemptCtx, toolName, handlerName, nil)
			if len(md) > 0 {
				pairs := make([]string, 0, len(md)*2)
				for k, v := range md {
					pairs = append(pairs, k, v)
				}
				attemptCtx = metadata.AppendToOutgoingContext(attemptCtx, pairs...)
			}

			// Execute through circuit breaker.
			var resp *toolsv1.ToolResponse
			_, cbErr := e.breakers.Execute(toolName, func() ([]byte, error) {
				var execErr error
				resp, execErr = client.Execute(attemptCtx, &toolsv1.ToolRequest{
					ToolName:      toolName,
					ArgumentsJson: string(args),
				})
				return nil, execErr
			})
			if cbErr != nil {
				return nil, fmt.Errorf("gRPC tool execution failed: %w", cbErr)
			}

			return marshalGRPCResponse(resp)
		},
	)
}

func marshalGRPCResponse(resp *toolsv1.ToolResponse) (json.RawMessage, error) {
	if resp.IsError {
		return nil, fmt.Errorf("tool error: %s", resp.ErrorMessage)
	}
	if resp.ResultJson != "" {
		return json.RawMessage(resp.ResultJson), nil
	}
	return json.RawMessage("null"), nil
}

// --- OpenAPI handler ---

func (e *OmniaExecutor) initOpenAPIHandler(ctx context.Context, name string, h *HandlerEntry) error {
	if h.OpenAPIConfig == nil {
		e.log.Info("skipping OpenAPI handler without config", "handler", name)
		return nil
	}

	adapter := NewOpenAPIAdapter(OpenAPIAdapterConfig{
		Name:            name,
		SpecURL:         h.OpenAPIConfig.SpecURL,
		BaseURL:         h.OpenAPIConfig.BaseURL,
		OperationFilter: h.OpenAPIConfig.OperationFilter,
		Headers:         h.OpenAPIConfig.Headers,
		AuthType:        h.OpenAPIConfig.AuthType,
		AuthToken:       h.OpenAPIConfig.AuthToken,
	}, e.log)

	if err := adapter.Connect(ctx); err != nil {
		return err
	}

	e.openAPIBaseURLs[name] = adapter.baseURL
	e.openAPIOps[name] = adapter.operations
	e.openAPIHeaders[name] = adapter.config.Headers

	for opID := range adapter.operations {
		e.toolHandlers[opID] = name
		e.log.V(1).Info("registered OpenAPI tool", "tool", opID, "handler", name)
	}

	return nil
}

func (e *OmniaExecutor) executeOpenAPI(
	ctx context.Context,
	toolName, handlerName string,
	handler *HandlerEntry,
	args json.RawMessage,
) (json.RawMessage, error) {
	e.mu.RLock()
	ops := e.openAPIOps[handlerName]
	baseURL := e.openAPIBaseURLs[handlerName]
	hdrs := e.openAPIHeaders[handlerName]
	e.mu.RUnlock()

	op, ok := ops[toolName]
	if !ok {
		return nil, fmt.Errorf("OpenAPI operation %q not found", toolName)
	}

	// Build a synthetic HTTPCfg for the OpenAPI operation.
	cfg := &HTTPCfg{
		Endpoint: baseURL + op.Path,
		Method:   op.Method,
		Headers:  make(map[string]string),
	}
	for k, v := range hdrs {
		cfg.Headers[k] = v
	}
	if handler.OpenAPIConfig != nil {
		cfg.AuthType = handler.OpenAPIConfig.AuthType
		cfg.AuthToken = handler.OpenAPIConfig.AuthToken
		cfg.RetryPolicy = handler.OpenAPIConfig.RetryPolicy
	}

	headers := e.buildHTTPHeaders(ctx, cfg, toolName, handlerName, args)
	policy := httpRetryParams(cfg)

	var lastCallResult httpCallResult
	classify := func(_ error) (bool, time.Duration) {
		if cfg.RetryPolicy == nil {
			return false, 0
		}
		return classifyHTTPResult(lastCallResult, cfg.RetryPolicy)
	}

	return retryWithBackoff(ctx, e.log, e.currentSpan(ctx), policy, handler.Timeout.Get(), classify,
		func(attemptCtx context.Context) (json.RawMessage, error) {
			result, callResult, err := doHTTPRequest(attemptCtx, &http.Client{}, cfg, headers, args)
			lastCallResult = callResult
			return result, err
		},
	)
}
