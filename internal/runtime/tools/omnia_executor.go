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
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"

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

	// policyBroker enforces ToolPolicy decisions per tool call. Disabled
	// (zero behavior change) unless POLICY_BROKER_URL is set.
	policyBroker *PolicyBrokerClient

	mu sync.RWMutex
}

// NewOmniaExecutor creates a new executor.
func NewOmniaExecutor(log logr.Logger, tp *tracing.Provider) *OmniaExecutor {
	e := &OmniaExecutor{
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
		policyBroker:    NewPolicyBrokerClient(log),
	}
	// Enforcement visibility: a disabled broker client makes Decide return a
	// synthetic allow for every tool call — enforcement silently no-ops. Log its
	// state at construction so "is this pod actually enforcing ToolPolicy?" is a
	// one-line grep. enabled=false when a ToolPolicy is expected means the pod
	// came up without POLICY_BROKER_URL wired (e.g. started before the operator
	// injected the broker and never rolled).
	log.Info("policy broker client initialized", "enabled", e.policyBroker.Enabled())
	return e
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
	if err := ResolveAuthTokenPaths(config); err != nil {
		return fmt.Errorf("resolve tool auth tokens: %w", err)
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
	e.log.V(1).Info("OmniaExecutor.Execute ENTER", "tool", descriptor.Name)
	e.mu.RLock()
	handlerName, ok := e.toolHandlers[descriptor.Name]
	if !ok {
		e.mu.RUnlock()
		e.log.V(1).Info("OmniaExecutor.Execute tool NOT FOUND", "tool", descriptor.Name)
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

// dispatch routes to the type-specific executor. Every handler type funnels
// through here, so this is the single chokepoint where ToolPolicy broker
// enforcement is hooked in: the broker is asked for a decision before any
// backend call is made, and a deny aborts dispatch entirely.
func (e *OmniaExecutor) dispatch(
	ctx context.Context,
	toolName, handlerName string,
	handler *HandlerEntry,
	args json.RawMessage,
) (json.RawMessage, error) {
	e.log.V(1).Info("OmniaExecutor.dispatch ENTER", "tool", toolName, "handlerType", handler.Type)
	ctx, err := e.enforcePolicy(ctx, toolName, handlerName, args)
	if err != nil {
		e.log.V(1).Info("OmniaExecutor.dispatch DENIED by policy", "tool", toolName, "err", err.Error())
		return nil, err
	}

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

// enforcePolicy calls the policy broker for a decision on this tool call.
// A real denial (enforce mode, not matched-but-audit) aborts dispatch with
// errPolicyDenied. An allow — including audit-mode "would deny" — proceeds,
// stashing any broker-injected headers on ctx for the executor's
// header/metadata builder to merge in. Decide never fails transport-side
// (fail-mode always resolves to a decision), so there is no error path here.
func (e *OmniaExecutor) enforcePolicy(
	ctx context.Context,
	toolName, handlerName string,
	args json.RawMessage,
) (context.Context, error) {
	// ToolPolicy `registry:` selectors match on the ToolRegistry NAME, so the
	// decision request must carry that — not the handler name. The two differ in
	// practice (e.g. handler "echo" inside registry "orders"); sending the
	// handler name made every registry-scoped policy silently fail to match and
	// allow. Fall back to the handler name only when registry metadata is unset.
	registryName := handlerName
	if meta, ok := e.GetToolMeta(toolName); ok && meta.RegistryName != "" {
		registryName = meta.RegistryName
	}
	e.log.V(1).Info("enforcePolicy calling broker", "tool", toolName, "registry", registryName, "brokerEnabled", e.policyBroker.Enabled())
	decision := e.policyBroker.Decide(ctx, toolName, registryName, args)
	e.log.V(1).Info("enforcePolicy decision", "tool", toolName, "allow", decision.Allow, "wouldDeny", decision.WouldDeny, "deniedBy", decision.DeniedBy)

	if !decision.Allow && !decision.WouldDeny {
		return ctx, fmt.Errorf("%w: %s (rule %q)", errPolicyDenied, decision.Message, decision.DeniedBy)
	}

	if len(decision.InjectedHeaders) > 0 {
		ctx = WithInjectedHeaders(ctx, decision.InjectedHeaders)
	}
	return ctx, nil
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
