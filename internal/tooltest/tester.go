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

package tooltest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/runtime/tools"
)

// maxTestTimeout caps the maximum duration for a single tool test.
const maxTestTimeout = 60 * time.Second

// Tester executes tool calls against a ToolRegistry using PromptKit executors.
type Tester struct {
	client client.Client
	log    logr.Logger
}

// NewTester creates a new tool tester.
func NewTester(c client.Client, log logr.Logger) *Tester {
	return &Tester{
		client: c,
		log:    log.WithName("tool-tester"),
	}
}

// testOutcome bundles the execution result with the handler definition
// so the caller can perform schema validation.
type testOutcome struct {
	result      json.RawMessage
	handlerType string
	handler     *omniav1alpha1.HandlerDefinition
}

// Test executes a single tool call against the specified ToolRegistry handler.
func (t *Tester) Test(
	ctx context.Context,
	namespace, registryName string,
	req *TestRequest,
) *TestResponse {
	start := time.Now()

	outcome, err := t.executeTest(ctx, namespace, registryName, req)

	resp := &TestResponse{
		DurationMs: time.Since(start).Milliseconds(),
	}
	if outcome != nil {
		resp.HandlerType = outcome.handlerType
	}

	// Run schema validation regardless of execution success
	resp.Validation = t.validate(outcome, req.Arguments)

	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	resp.Success = true
	resp.Result = outcome.result
	return resp
}

// validate performs input and output schema validation.
func (t *Tester) validate(
	outcome *testOutcome,
	args json.RawMessage,
) *ValidationResult {
	if outcome == nil || outcome.handler == nil || outcome.handler.Tool == nil {
		return nil
	}
	tool := outcome.handler.Tool

	v := &ValidationResult{}
	hasChecks := false

	// Validate request arguments against inputSchema
	reqCheck := validateAgainstSchema(&tool.InputSchema, args)
	if reqCheck != nil {
		v.Request = reqCheck
		hasChecks = true
	}

	// Validate response against outputSchema
	if outcome.result != nil {
		respCheck := validateAgainstSchema(tool.OutputSchema, outcome.result)
		if respCheck != nil {
			v.Response = respCheck
			hasChecks = true
		}
	}

	if !hasChecks {
		return nil
	}
	return v
}

func (t *Tester) executeTest(
	ctx context.Context,
	namespace, registryName string,
	req *TestRequest,
) (*testOutcome, error) {
	// Fetch the ToolRegistry CRD
	registry := &omniav1alpha1.ToolRegistry{}
	key := types.NamespacedName{Name: registryName, Namespace: namespace}
	if err := t.client.Get(ctx, key, registry); err != nil {
		return nil, fmt.Errorf("failed to get ToolRegistry %q: %w", registryName, err)
	}

	// Find the handler
	handler, err := t.findHandler(registry, req.HandlerName)
	if err != nil {
		return nil, err
	}

	outcome := &testOutcome{
		handlerType: string(handler.Type),
		handler:     handler,
	}

	// Resolve the tool name
	toolName := req.ToolName
	if toolName == "" && handler.Tool != nil {
		toolName = handler.Tool.Name
	}
	if toolName == "" {
		return outcome, fmt.Errorf("toolName is required for %s handlers without an inline tool definition", outcome.handlerType)
	}

	// Resolve auth secrets if needed
	if err := t.resolveAuthSecrets(ctx, namespace, handler); err != nil {
		return outcome, fmt.Errorf("failed to resolve auth secrets: %w", err)
	}

	// Build a single-handler config for the executor
	handlerCfg := t.buildHandlerConfig(handler)

	// Determine timeout
	timeout := t.resolveTimeout(handler)
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create a short-lived executor, initialize, execute, and close
	executor := tools.NewOmniaExecutor(t.log, nil)
	if err := executor.LoadConfigFromEntries([]tools.HandlerEntry{handlerCfg}); err != nil {
		return outcome, fmt.Errorf("failed to load handler config: %w", err)
	}
	if err := executor.Initialize(testCtx); err != nil {
		return outcome, fmt.Errorf("failed to initialize handler: %w", err)
	}
	defer func() {
		if closeErr := executor.Close(); closeErr != nil {
			t.log.Error(closeErr, "failed to close executor")
		}
	}()

	// Execute the tool call
	result, err := executor.ExecuteTool(testCtx, toolName, req.Arguments)
	if err != nil {
		return outcome, err
	}

	outcome.result = result
	return outcome, nil
}

func (t *Tester) findHandler(
	registry *omniav1alpha1.ToolRegistry,
	handlerName string,
) (*omniav1alpha1.HandlerDefinition, error) {
	for i := range registry.Spec.Handlers {
		if registry.Spec.Handlers[i].Name == handlerName {
			return &registry.Spec.Handlers[i], nil
		}
	}
	return nil, fmt.Errorf("handler %q not found in ToolRegistry", handlerName)
}

// resolveAuthSecrets reads K8s secrets referenced by authSecretRef and injects
// the token value into the handler's config so the executor can use it.
func (t *Tester) resolveAuthSecrets(
	ctx context.Context,
	namespace string,
	handler *omniav1alpha1.HandlerDefinition,
) error {
	switch handler.Type {
	case omniav1alpha1.HandlerTypeHTTP:
		return t.resolveHTTPAuth(ctx, namespace, handler)
	case omniav1alpha1.HandlerTypeOpenAPI:
		return t.resolveOpenAPIAuth(ctx, namespace, handler)
	default:
		return nil
	}
}

func (t *Tester) resolveHTTPAuth(
	ctx context.Context,
	namespace string,
	handler *omniav1alpha1.HandlerDefinition,
) error {
	if handler.HTTPConfig == nil || handler.HTTPConfig.AuthSecretRef == nil {
		return nil
	}
	ref := handler.HTTPConfig.AuthSecretRef
	token, err := t.readSecretKey(ctx, namespace, ref.Name, ref.Key)
	if err != nil {
		return err
	}
	// Inject into headers so the runtime config builder can use it
	if handler.HTTPConfig.Headers == nil {
		handler.HTTPConfig.Headers = make(map[string]string)
	}
	authType := "bearer"
	if handler.HTTPConfig.AuthType != nil {
		authType = *handler.HTTPConfig.AuthType
	}
	applyAuthHeader(handler.HTTPConfig.Headers, authType, token)
	return nil
}

func (t *Tester) resolveOpenAPIAuth(
	ctx context.Context,
	namespace string,
	handler *omniav1alpha1.HandlerDefinition,
) error {
	if handler.OpenAPIConfig == nil || handler.OpenAPIConfig.AuthSecretRef == nil {
		return nil
	}
	ref := handler.OpenAPIConfig.AuthSecretRef
	token, err := t.readSecretKey(ctx, namespace, ref.Name, ref.Key)
	if err != nil {
		return err
	}
	if handler.OpenAPIConfig.Headers == nil {
		handler.OpenAPIConfig.Headers = make(map[string]string)
	}
	authType := "bearer"
	if handler.OpenAPIConfig.AuthType != nil {
		authType = *handler.OpenAPIConfig.AuthType
	}
	applyAuthHeader(handler.OpenAPIConfig.Headers, authType, token)
	return nil
}

func (t *Tester) readSecretKey(ctx context.Context, namespace, name, key string) (string, error) {
	secret := &corev1.Secret{}
	if err := t.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err != nil {
		return "", fmt.Errorf("failed to read secret %q: %w", name, err)
	}
	data, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %q", key, name)
	}
	return string(data), nil
}

func applyAuthHeader(headers map[string]string, authType, token string) {
	switch authType {
	case "bearer":
		headers["Authorization"] = "Bearer " + token
	case "basic":
		headers["Authorization"] = "Basic " + token
	case "api-key":
		headers["X-API-Key"] = token
	}
}

func (t *Tester) resolveTimeout(handler *omniav1alpha1.HandlerDefinition) time.Duration {
	if handler.Timeout != nil {
		if d, err := time.ParseDuration(*handler.Timeout); err == nil {
			if d > maxTestTimeout {
				return maxTestTimeout
			}
			return d
		}
	}
	return maxTestTimeout
}

// buildHandlerConfig converts a CRD HandlerDefinition into a runtime HandlerEntry
// that the OmniaExecutor can consume.
func (t *Tester) buildHandlerConfig(h *omniav1alpha1.HandlerDefinition) tools.HandlerEntry {
	entry := tools.HandlerEntry{
		Name: h.Name,
		Type: string(h.Type),
	}

	if h.Timeout != nil {
		entry.Timeout = *h.Timeout
	}
	if h.Retries != nil {
		entry.Retries = *h.Retries
	}

	switch h.Type {
	case omniav1alpha1.HandlerTypeHTTP:
		entry = t.buildHTTPEntry(entry, h)
	case omniav1alpha1.HandlerTypeMCP:
		entry = t.buildMCPEntry(entry, h)
	case omniav1alpha1.HandlerTypeGRPC:
		entry = t.buildGRPCEntry(entry, h)
	case omniav1alpha1.HandlerTypeOpenAPI:
		entry = t.buildOpenAPIEntry(entry, h)
	}

	return entry
}

func (t *Tester) buildHTTPEntry(entry tools.HandlerEntry, h *omniav1alpha1.HandlerDefinition) tools.HandlerEntry {
	if h.HTTPConfig == nil {
		return entry
	}
	cfg := h.HTTPConfig
	entry.Endpoint = cfg.Endpoint
	entry.HTTPConfig = &tools.HTTPCfg{
		Endpoint:     cfg.Endpoint,
		Method:       cfg.Method,
		Headers:      cfg.Headers,
		ContentType:  cfg.ContentType,
		QueryParams:  cfg.QueryParams,
		HeaderParams: cfg.HeaderParams,
		StaticQuery:  cfg.StaticQuery,
		Redact:       cfg.Redact,
	}
	if cfg.StaticBody != nil {
		entry.HTTPConfig.StaticBody = unmarshalRaw(cfg.StaticBody.Raw)
	}
	if cfg.BodyMapping != nil {
		entry.HTTPConfig.BodyMapping = *cfg.BodyMapping
	}
	if cfg.ResponseMapping != nil {
		entry.HTTPConfig.ResponseMapping = *cfg.ResponseMapping
	}
	if cfg.URLTemplate != nil {
		entry.HTTPConfig.URLTemplate = *cfg.URLTemplate
	}
	if h.Tool != nil {
		entry.Tool = &tools.ToolDefCfg{
			Name:        h.Tool.Name,
			Description: h.Tool.Description,
			InputSchema: unmarshalRaw(h.Tool.InputSchema.Raw),
		}
		if h.Tool.OutputSchema != nil {
			entry.Tool.OutputSchema = unmarshalRaw(h.Tool.OutputSchema.Raw)
		}
	}
	return entry
}

func (t *Tester) buildMCPEntry(entry tools.HandlerEntry, h *omniav1alpha1.HandlerDefinition) tools.HandlerEntry {
	if h.MCPConfig == nil {
		return entry
	}
	cfg := h.MCPConfig
	entry.MCPConfig = &tools.MCPCfg{
		Transport: string(cfg.Transport),
		Env:       cfg.Env,
	}
	if cfg.Endpoint != nil {
		entry.Endpoint = *cfg.Endpoint
		entry.MCPConfig.Endpoint = *cfg.Endpoint
	}
	if cfg.Command != nil {
		entry.MCPConfig.Command = *cfg.Command
	}
	if len(cfg.Args) > 0 {
		entry.MCPConfig.Args = cfg.Args
	}
	if cfg.WorkDir != nil {
		entry.MCPConfig.WorkDir = *cfg.WorkDir
	}
	if cfg.ToolFilter != nil {
		entry.MCPConfig.ToolFilter = &tools.MCPToolFilterCfg{
			Allowlist: cfg.ToolFilter.Allowlist,
			Blocklist: cfg.ToolFilter.Blocklist,
		}
	}
	return entry
}

func (t *Tester) buildGRPCEntry(entry tools.HandlerEntry, h *omniav1alpha1.HandlerDefinition) tools.HandlerEntry {
	if h.GRPCConfig == nil {
		return entry
	}
	cfg := h.GRPCConfig
	entry.Endpoint = cfg.Endpoint
	entry.GRPCConfig = &tools.GRPCCfg{
		Endpoint:              cfg.Endpoint,
		TLS:                   cfg.TLS,
		TLSInsecureSkipVerify: cfg.TLSInsecureSkipVerify,
	}
	if cfg.TLSCertPath != nil {
		entry.GRPCConfig.TLSCertPath = *cfg.TLSCertPath
	}
	if cfg.TLSKeyPath != nil {
		entry.GRPCConfig.TLSKeyPath = *cfg.TLSKeyPath
	}
	if cfg.TLSCAPath != nil {
		entry.GRPCConfig.TLSCAPath = *cfg.TLSCAPath
	}
	if h.Tool != nil {
		entry.Tool = &tools.ToolDefCfg{
			Name:        h.Tool.Name,
			Description: h.Tool.Description,
			InputSchema: unmarshalRaw(h.Tool.InputSchema.Raw),
		}
		if h.Tool.OutputSchema != nil {
			entry.Tool.OutputSchema = unmarshalRaw(h.Tool.OutputSchema.Raw)
		}
	}
	return entry
}

func (t *Tester) buildOpenAPIEntry(entry tools.HandlerEntry, h *omniav1alpha1.HandlerDefinition) tools.HandlerEntry {
	if h.OpenAPIConfig == nil {
		return entry
	}
	cfg := h.OpenAPIConfig
	entry.OpenAPIConfig = &tools.OpenAPICfg{
		SpecURL:         cfg.SpecURL,
		OperationFilter: cfg.OperationFilter,
		Headers:         cfg.Headers,
	}
	if cfg.BaseURL != nil {
		entry.Endpoint = *cfg.BaseURL
		entry.OpenAPIConfig.BaseURL = *cfg.BaseURL
	}
	if cfg.AuthType != nil {
		entry.OpenAPIConfig.AuthType = *cfg.AuthType
	}
	return entry
}

// unmarshalRaw converts raw JSON bytes into a typed interface{} value.
func unmarshalRaw(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}
