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
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	runtimetools "github.com/altairalabs/omnia/internal/runtime/tools"
)

// errFmtHandler wraps per-handler build errors (HTTP/gRPC/MCP/OpenAPI)
// so downstream callers can attribute failures to the owning handler. Extracted
// to silence go:S1192 (was duplicated across every handler-type branch).
const errFmtHandler = "handler %q: %w"

// authTypeBearer is the default auth type applied when a handler references
// an auth secret but doesn't specify a type. Extracted to a constant to
// silence go:S1192 / goconst (duplicated across builders and tests).
const authTypeBearer = "bearer"

// ToolConfig represents the tools configuration file format for the runtime.
// This is passed to the runtime container as a YAML file.
type ToolConfig struct {
	Handlers []HandlerEntry `json:"handlers"`
}

// HandlerEntry represents a single handler in the config.
type HandlerEntry struct {
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Endpoint      string          `json:"endpoint,omitempty"`
	Tool          *ToolDefinition `json:"tool,omitempty"` // For http/grpc/client handlers
	HTTPConfig    *ToolHTTP       `json:"httpConfig,omitempty"`
	GRPCConfig    *ToolGRPC       `json:"grpcConfig,omitempty"`
	MCPConfig     *ToolMCP        `json:"mcpConfig,omitempty"`
	OpenAPIConfig *ToolOpenAPI    `json:"openAPIConfig,omitempty"`
	ClientConfig  *ToolClient     `json:"clientConfig,omitempty"`
	Timeout       string          `json:"timeout,omitempty"`
}

// ToolClient contains client-side tool configuration for the runtime.
type ToolClient struct {
	ConsentMessage string   `json:"consentMessage,omitempty"`
	Categories     []string `json:"categories,omitempty"`
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
	Endpoint        string                               `json:"endpoint"`
	Method          string                               `json:"method,omitempty"`
	Headers         map[string]string                    `json:"headers,omitempty"`
	ContentType     string                               `json:"contentType,omitempty"`
	AuthType        string                               `json:"authType,omitempty"`
	AuthTokenPath   string                               `json:"authTokenPath,omitempty"`
	AuthCloud       string                               `json:"authCloud,omitempty"`
	AuthAudience    string                               `json:"authAudience,omitempty"`
	AuthHeader      string                               `json:"authHeader,omitempty"`
	QueryParams     []string                             `json:"queryParams,omitempty"`
	HeaderParams    map[string]string                    `json:"headerParams,omitempty"`
	StaticQuery     map[string]string                    `json:"staticQuery,omitempty"`
	StaticBody      interface{}                          `json:"staticBody,omitempty"`
	BodyMapping     string                               `json:"bodyMapping,omitempty"`
	ResponseMapping string                               `json:"responseMapping,omitempty"`
	Redact          []string                             `json:"redact,omitempty"`
	URLTemplate     string                               `json:"urlTemplate,omitempty"`
	RetryPolicy     *runtimetools.RuntimeHTTPRetryPolicy `json:"retryPolicy,omitempty"`
}

// ToolGRPC represents gRPC configuration for a handler.
type ToolGRPC struct {
	Endpoint              string                               `json:"endpoint"`
	TLS                   bool                                 `json:"tls,omitempty"`
	TLSCertPath           string                               `json:"tlsCertPath,omitempty"`
	TLSKeyPath            string                               `json:"tlsKeyPath,omitempty"`
	TLSCAPath             string                               `json:"tlsCAPath,omitempty"`
	TLSInsecureSkipVerify bool                                 `json:"tlsInsecureSkipVerify,omitempty"`
	AuthType              string                               `json:"authType,omitempty"`
	AuthTokenPath         string                               `json:"authTokenPath,omitempty"`
	AuthCloud             string                               `json:"authCloud,omitempty"`
	AuthAudience          string                               `json:"authAudience,omitempty"`
	AuthHeader            string                               `json:"authHeader,omitempty"`
	RetryPolicy           *runtimetools.RuntimeGRPCRetryPolicy `json:"retryPolicy,omitempty"`
}

// ToolMCP represents MCP configuration for a handler.
type ToolMCP struct {
	Transport     string                              `json:"transport"`
	Endpoint      string                              `json:"endpoint,omitempty"`
	Command       string                              `json:"command,omitempty"`
	Args          []string                            `json:"args,omitempty"`
	WorkDir       string                              `json:"workDir,omitempty"`
	Env           map[string]string                   `json:"env,omitempty"`
	AuthType      string                              `json:"authType,omitempty"`
	AuthTokenPath string                              `json:"authTokenPath,omitempty"`
	ToolFilter    *ToolMCPFilter                      `json:"toolFilter,omitempty"`
	RetryPolicy   *runtimetools.RuntimeMCPRetryPolicy `json:"retryPolicy,omitempty"`
}

// ToolMCPFilter controls which tools from an MCP server are exposed.
type ToolMCPFilter struct {
	Allowlist []string `json:"allowlist,omitempty"`
	Blocklist []string `json:"blocklist,omitempty"`
}

// ToolOpenAPI represents OpenAPI configuration for a handler.
// OpenAPI delegates execution to the HTTP executor, so it reuses
// RuntimeHTTPRetryPolicy for its retry policy.
type ToolOpenAPI struct {
	SpecURL         string                               `json:"specURL"`
	BaseURL         string                               `json:"baseURL,omitempty"`
	OperationFilter []string                             `json:"operationFilter,omitempty"`
	Headers         map[string]string                    `json:"headers,omitempty"`
	AuthType        string                               `json:"authType,omitempty"`
	AuthTokenPath   string                               `json:"authTokenPath,omitempty"`
	RetryPolicy     *runtimetools.RuntimeHTTPRetryPolicy `json:"retryPolicy,omitempty"`
}

// reconcileToolsConfigMap creates or updates the tools ConfigMap from ToolRegistry.
func (r *AgentRuntimeReconciler) reconcileToolsConfigMap(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
	toolRegistry *omniav1alpha1.ToolRegistry,
) error {
	log := logf.FromContext(ctx)

	// Build tools config from ToolRegistry
	toolsConfig, err := r.buildToolsConfig(toolRegistry)
	if err != nil {
		return fmt.Errorf("failed to build tools config: %w", err)
	}

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

// toolAuthRef pairs a handler name with the secret key it authenticates from.
type toolAuthRef struct {
	handler string
	ref     *omniav1alpha1.SecretKeySelector
}

// collectToolAuthSecrets returns, in handler order, every handler whose effective
// auth resolves a credential from a Secret (bearer/basic). It reads
// HandlerDefinition.EffectiveAuth so it is handler-generic — any protocol whose
// auth is bearer/basic contributes a companion-Secret key, whether configured via
// the new auth stanza or the deprecated authType/authSecretRef fields. Non-Secret
// auth types (serviceAccount, workloadIdentity) do not use the companion Secret.
func collectToolAuthSecrets(tr *omniav1alpha1.ToolRegistry) []toolAuthRef {
	var out []toolAuthRef
	for i := range tr.Spec.Handlers {
		h := &tr.Spec.Handlers[i]
		auth := h.EffectiveAuth()
		if auth == nil || auth.SecretRef == nil {
			continue
		}
		switch auth.Type {
		case omniav1alpha1.ToolAuthTypeBearer, omniav1alpha1.ToolAuthTypeBasic:
			out = append(out, toolAuthRef{handler: h.Name, ref: auth.SecretRef})
		}
	}
	return out
}

// validateToolAuthTypes rejects tool handlers whose effective auth the operator
// cannot honor: stdio MCP has no header channel, and workloadIdentity is
// currently resolved only on http handlers (runtime-ambient azure).
func validateToolAuthTypes(tr *omniav1alpha1.ToolRegistry) error {
	if tr == nil {
		return nil
	}
	for i := range tr.Spec.Handlers {
		if err := validateHandlerAuth(&tr.Spec.Handlers[i]); err != nil {
			return err
		}
	}
	return nil
}

// wifSupportedHandlerType reports whether the runtime resolves workloadIdentity
// for a handler of this type today. Extended per protocol as support lands.
func wifSupportedHandlerType(t omniav1alpha1.HandlerType) bool {
	switch t {
	case omniav1alpha1.HandlerTypeHTTP, omniav1alpha1.HandlerTypeGRPC:
		return true
	default:
		return false
	}
}

// validateHandlerAuth rejects a single handler's effective auth the operator
// cannot honor: stdio MCP has no header channel, and workloadIdentity is
// currently resolved only on handler types the runtime knows how to apply it
// to (see wifSupportedHandlerType).
func validateHandlerAuth(h *omniav1alpha1.HandlerDefinition) error {
	auth := h.EffectiveAuth()
	if auth == nil || auth.Type == omniav1alpha1.ToolAuthTypeNone {
		return nil
	}
	if h.MCPConfig != nil && h.MCPConfig.Transport == omniav1alpha1.MCPTransportStdio {
		return fmt.Errorf("handler %q: auth is not supported on a stdio MCP transport (no header channel); use an sse or streamable-http transport", h.Name)
	}
	if auth.Type == omniav1alpha1.ToolAuthTypeWorkloadIdentity && !wifSupportedHandlerType(h.Type) {
		return fmt.Errorf("handler %q: auth.type workloadIdentity is not yet supported on %s handlers", h.Name, h.Type)
	}
	return nil
}

// reconcileToolSecrets resolves each referenced auth secret into a single
// operator-managed Secret (<name>-tool-secrets) with one key per authed handler.
// The token value never enters the tools ConfigMap. Returns an error (surfaced
// via the caller's reconcile) if any referenced secret/key is missing.
func (r *AgentRuntimeReconciler) reconcileToolSecrets(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
	toolRegistry *omniav1alpha1.ToolRegistry,
) error {
	refs := collectToolAuthSecrets(toolRegistry)
	if len(refs) == 0 {
		// No authed handlers remain — remove any previously-created companion
		// Secret so a stale token doesn't linger in the namespace.
		stale := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name + ToolSecretsSecretSuffix,
			Namespace: agentRuntime.Namespace,
		}}
		if err := r.Delete(ctx, stale); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete stale tool-secrets Secret: %w", err)
		}
		return nil
	}

	data := make(map[string][]byte, len(refs))
	for _, ar := range refs {
		src := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: ar.ref.Name, Namespace: agentRuntime.Namespace}, src); err != nil {
			return fmt.Errorf("handler %q: read auth secret %q: %w", ar.handler, ar.ref.Name, err)
		}
		val, ok := src.Data[ar.ref.Key]
		if !ok {
			return fmt.Errorf("handler %q: key %q not found in secret %q", ar.handler, ar.ref.Key, ar.ref.Name)
		}
		data[ar.handler] = val
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name + ToolSecretsSecretSuffix,
			Namespace: agentRuntime.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if err := controllerutil.SetControllerReference(agentRuntime, secret, r.Scheme); err != nil {
			return err
		}
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = data
		return nil
	})
	if err != nil {
		return fmt.Errorf("reconcile tool-secrets Secret: %w", err)
	}
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

// defaultHTTPRetryOn is the default list of HTTP status codes that trigger a
// retry when the user doesn't specify RetryOn explicitly. Kept in sync with
// the documentation in HTTPRetryPolicy.
var defaultHTTPRetryOn = []int32{408, 429, 500, 502, 503, 504}

// defaultGRPCRetryableCodes is the default RetryableStatusCodes list applied
// when the user doesn't specify one explicitly.
var defaultGRPCRetryableCodes = []string{"UNAVAILABLE", "DEADLINE_EXCEEDED", "RESOURCE_EXHAUSTED"}

// validGRPCStatusCodes is the authoritative set of gRPC status code names
// accepted in GRPCRetryPolicy.RetryableStatusCodes. Matches google.rpc.Code.
var validGRPCStatusCodes = map[string]struct{}{
	"OK":                  {},
	"CANCELLED":           {},
	"UNKNOWN":             {},
	"INVALID_ARGUMENT":    {},
	"DEADLINE_EXCEEDED":   {},
	"NOT_FOUND":           {},
	"ALREADY_EXISTS":      {},
	"PERMISSION_DENIED":   {},
	"RESOURCE_EXHAUSTED":  {},
	"FAILED_PRECONDITION": {},
	"ABORTED":             {},
	"OUT_OF_RANGE":        {},
	"UNIMPLEMENTED":       {},
	"INTERNAL":            {},
	"UNAVAILABLE":         {},
	"DATA_LOSS":           {},
	"UNAUTHENTICATED":     {},
}

// parseBackoffMultiplier parses a decimal-string multiplier and validates it.
// Shared by HTTP, gRPC, and MCP retry policy builders.
func parseBackoffMultiplier(s string) (float64, error) {
	mult, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid backoffMultiplier %q: %w", s, err)
	}
	if mult < 1.0 {
		return 0, fmt.Errorf("backoffMultiplier %v must be >= 1.0", mult)
	}
	return mult, nil
}

// validateBackoffBounds ensures MaxBackoff >= InitialBackoff. Shared by all
// three retry policy builders.
func validateBackoffBounds(initial, max runtimetools.Duration) error {
	if max.Get() < initial.Get() {
		return fmt.Errorf("maxBackoff (%v) must be >= initialBackoff (%v)", max.Get(), initial.Get())
	}
	return nil
}

// buildHTTPRetryPolicy validates an HTTPRetryPolicy from the CRD and returns
// the runtime-side representation with defaults applied. Returns (nil, nil)
// when the input is nil.
func buildHTTPRetryPolicy(p *omniav1alpha1.HTTPRetryPolicy) (*runtimetools.RuntimeHTTPRetryPolicy, error) {
	if p == nil {
		return nil, nil
	}

	out := &runtimetools.RuntimeHTTPRetryPolicy{
		MaxAttempts:         p.MaxAttempts,
		InitialBackoff:      runtimetools.Duration(100 * time.Millisecond),
		BackoffMultiplier:   2.0,
		MaxBackoff:          runtimetools.Duration(30 * time.Second),
		RetryOn:             defaultHTTPRetryOn,
		RetryOnNetworkError: true,
		RespectRetryAfter:   true,
	}

	if p.InitialBackoff != nil {
		out.InitialBackoff = runtimetools.Duration(p.InitialBackoff.Duration)
	}
	if p.MaxBackoff != nil {
		out.MaxBackoff = runtimetools.Duration(p.MaxBackoff.Duration)
	}
	if p.BackoffMultiplier != nil {
		mult, err := parseBackoffMultiplier(*p.BackoffMultiplier)
		if err != nil {
			return nil, fmt.Errorf("http retry policy: %w", err)
		}
		out.BackoffMultiplier = mult
	}
	// RetryOn: nil means "apply defaults"; empty slice means "user opted out".
	if p.RetryOn != nil {
		out.RetryOn = p.RetryOn
	}
	if p.RetryOnNetworkError != nil {
		out.RetryOnNetworkError = *p.RetryOnNetworkError
	}
	if p.RespectRetryAfter != nil {
		out.RespectRetryAfter = *p.RespectRetryAfter
	}

	if err := validateBackoffBounds(out.InitialBackoff, out.MaxBackoff); err != nil {
		return nil, fmt.Errorf("http retry policy: %w", err)
	}

	return out, nil
}

// buildGRPCRetryPolicy validates a GRPCRetryPolicy from the CRD and returns
// the runtime-side representation with defaults applied.
func buildGRPCRetryPolicy(p *omniav1alpha1.GRPCRetryPolicy) (*runtimetools.RuntimeGRPCRetryPolicy, error) {
	if p == nil {
		return nil, nil
	}

	out := &runtimetools.RuntimeGRPCRetryPolicy{
		MaxAttempts:          p.MaxAttempts,
		InitialBackoff:       runtimetools.Duration(100 * time.Millisecond),
		BackoffMultiplier:    2.0,
		MaxBackoff:           runtimetools.Duration(30 * time.Second),
		RetryableStatusCodes: defaultGRPCRetryableCodes,
	}

	if p.InitialBackoff != nil {
		out.InitialBackoff = runtimetools.Duration(p.InitialBackoff.Duration)
	}
	if p.MaxBackoff != nil {
		out.MaxBackoff = runtimetools.Duration(p.MaxBackoff.Duration)
	}
	if p.BackoffMultiplier != nil {
		mult, err := parseBackoffMultiplier(*p.BackoffMultiplier)
		if err != nil {
			return nil, fmt.Errorf("grpc retry policy: %w", err)
		}
		out.BackoffMultiplier = mult
	}
	if p.RetryableStatusCodes != nil {
		for _, code := range p.RetryableStatusCodes {
			if _, ok := validGRPCStatusCodes[code]; !ok {
				return nil, fmt.Errorf("grpc retry policy: unknown gRPC status code %q", code)
			}
		}
		out.RetryableStatusCodes = p.RetryableStatusCodes
	}

	if err := validateBackoffBounds(out.InitialBackoff, out.MaxBackoff); err != nil {
		return nil, fmt.Errorf("grpc retry policy: %w", err)
	}

	return out, nil
}

// buildMCPRetryPolicy validates an MCPRetryPolicy from the CRD and returns
// the runtime-side representation with defaults applied.
func buildMCPRetryPolicy(p *omniav1alpha1.MCPRetryPolicy) (*runtimetools.RuntimeMCPRetryPolicy, error) {
	if p == nil {
		return nil, nil
	}

	out := &runtimetools.RuntimeMCPRetryPolicy{
		MaxAttempts:       p.MaxAttempts,
		InitialBackoff:    runtimetools.Duration(100 * time.Millisecond),
		BackoffMultiplier: 2.0,
		MaxBackoff:        runtimetools.Duration(30 * time.Second),
	}

	if p.InitialBackoff != nil {
		out.InitialBackoff = runtimetools.Duration(p.InitialBackoff.Duration)
	}
	if p.MaxBackoff != nil {
		out.MaxBackoff = runtimetools.Duration(p.MaxBackoff.Duration)
	}
	if p.BackoffMultiplier != nil {
		mult, err := parseBackoffMultiplier(*p.BackoffMultiplier)
		if err != nil {
			return nil, fmt.Errorf("mcp retry policy: %w", err)
		}
		out.BackoffMultiplier = mult
	}

	if err := validateBackoffBounds(out.InitialBackoff, out.MaxBackoff); err != nil {
		return nil, fmt.Errorf("mcp retry policy: %w", err)
	}

	return out, nil
}

// unmarshalRawJSON converts apiextensionsv1.JSON raw bytes into a typed
// interface{} value.  Without this step, []byte assigned to interface{} gets
// base64-encoded when marshaled to YAML, which breaks schema extraction in the
// runtime.
func unmarshalRawJSON(raw []byte) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

// buildToolDefinition builds a ToolDefinition from the handler's tool spec.
func buildToolDefinition(tool *omniav1alpha1.ToolDefinition) *ToolDefinition {
	if tool == nil {
		return nil
	}
	def := &ToolDefinition{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: unmarshalRawJSON(tool.InputSchema.Raw),
	}
	if tool.OutputSchema != nil {
		def.OutputSchema = unmarshalRawJSON(tool.OutputSchema.Raw)
	}
	return def
}

// buildHTTPConfig builds HTTP configuration for a handler entry.
func buildHTTPConfig(h *omniav1alpha1.HandlerDefinition, endpoint string) (*ToolHTTP, error) {
	if h.HTTPConfig == nil {
		return nil, nil
	}
	cfg := &ToolHTTP{
		Endpoint:     endpoint,
		Method:       h.HTTPConfig.Method,
		Headers:      h.HTTPConfig.Headers,
		ContentType:  h.HTTPConfig.ContentType,
		QueryParams:  h.HTTPConfig.QueryParams,
		HeaderParams: h.HTTPConfig.HeaderParams,
		StaticQuery:  h.HTTPConfig.StaticQuery,
		Redact:       h.HTTPConfig.Redact,
	}
	if h.HTTPConfig.StaticBody != nil {
		cfg.StaticBody = unmarshalRawJSON(h.HTTPConfig.StaticBody.Raw)
	}
	if h.HTTPConfig.BodyMapping != nil {
		cfg.BodyMapping = *h.HTTPConfig.BodyMapping
	}
	if h.HTTPConfig.ResponseMapping != nil {
		cfg.ResponseMapping = *h.HTTPConfig.ResponseMapping
	}
	if h.HTTPConfig.URLTemplate != nil {
		cfg.URLTemplate = *h.HTTPConfig.URLTemplate
	}
	rp, err := buildHTTPRetryPolicy(h.HTTPConfig.RetryPolicy)
	if err != nil {
		return nil, err
	}
	cfg.RetryPolicy = rp
	if authType, tokenPath, ok := authFieldsFor(h); ok {
		cfg.AuthType = authType
		cfg.AuthTokenPath = tokenPath
	}
	if cloud, aud, header, ok := workloadIdentityFieldsFor(h); ok {
		cfg.AuthType = string(omniav1alpha1.ToolAuthTypeWorkloadIdentity)
		cfg.AuthCloud, cfg.AuthAudience, cfg.AuthHeader = cloud, aud, header
	}
	return cfg, nil
}

// workloadIdentityFieldsFor returns the runtime WIF params for a handler whose
// effective auth is workloadIdentity, and false otherwise.
func workloadIdentityFieldsFor(h *omniav1alpha1.HandlerDefinition) (cloud, audience, header string, ok bool) {
	auth := h.EffectiveAuth()
	if auth == nil || auth.Type != omniav1alpha1.ToolAuthTypeWorkloadIdentity || auth.WorkloadIdentity == nil {
		return "", "", "", false
	}
	w := auth.WorkloadIdentity
	return w.Cloud, w.Audience, w.Header, true
}

// authFieldsFor returns the generated-config auth type and mounted token path for
// a handler's effective auth, and false when there is no credential to apply. It
// reads HandlerDefinition.EffectiveAuth, so the new auth stanza and the
// deprecated authType/authSecretRef fields resolve identically. bearer/basic read
// the companion Secret; serviceAccount reads a projected SA-token file and is
// applied as a bearer token (Authorization: Bearer).
func authFieldsFor(h *omniav1alpha1.HandlerDefinition) (authType, tokenPath string, ok bool) {
	auth := h.EffectiveAuth()
	if auth == nil {
		return "", "", false
	}
	switch auth.Type {
	case omniav1alpha1.ToolAuthTypeBearer, omniav1alpha1.ToolAuthTypeBasic:
		if auth.SecretRef == nil {
			return "", "", false
		}
		return auth.Type, ToolSecretsMountPath + "/" + h.Name, true
	case omniav1alpha1.ToolAuthTypeServiceAccount:
		if auth.ServiceAccount == nil {
			return "", "", false
		}
		return omniav1alpha1.ToolAuthTypeBearer, toolSATokenPath(h.Name), true
	}
	return "", "", false
}

// buildGRPCConfig builds gRPC configuration for a handler entry.
func buildGRPCConfig(h *omniav1alpha1.HandlerDefinition, endpoint string) (*ToolGRPC, error) {
	if h.GRPCConfig == nil {
		return nil, nil
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
	rp, err := buildGRPCRetryPolicy(h.GRPCConfig.RetryPolicy)
	if err != nil {
		return nil, err
	}
	cfg.RetryPolicy = rp
	if authType, tokenPath, ok := authFieldsFor(h); ok {
		cfg.AuthType = authType
		cfg.AuthTokenPath = tokenPath
	}
	if cloud, aud, header, ok := workloadIdentityFieldsFor(h); ok {
		cfg.AuthType = string(omniav1alpha1.ToolAuthTypeWorkloadIdentity)
		cfg.AuthCloud, cfg.AuthAudience, cfg.AuthHeader = cloud, aud, header
	}
	return cfg, nil
}

// buildMCPConfig builds MCP configuration for a handler entry.
func buildMCPConfig(h *omniav1alpha1.HandlerDefinition) (*ToolMCP, error) {
	if h.MCPConfig == nil {
		return nil, nil
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
	if h.MCPConfig.ToolFilter != nil {
		cfg.ToolFilter = &ToolMCPFilter{
			Allowlist: h.MCPConfig.ToolFilter.Allowlist,
			Blocklist: h.MCPConfig.ToolFilter.Blocklist,
		}
	}
	rp, err := buildMCPRetryPolicy(h.MCPConfig.RetryPolicy)
	if err != nil {
		return nil, err
	}
	cfg.RetryPolicy = rp
	// stdio+auth is rejected by validateToolAuthTypes, so any auth here is on an
	// HTTP-based transport the executor can carry a header on.
	if authType, tokenPath, ok := authFieldsFor(h); ok {
		cfg.AuthType = authType
		cfg.AuthTokenPath = tokenPath
	}
	return cfg, nil
}

// buildOpenAPIConfig builds OpenAPI configuration for a handler entry.
// OpenAPI handlers delegate execution to the HTTP executor, so they reuse
// HTTPRetryPolicy (translated via buildHTTPRetryPolicy).
func buildOpenAPIConfig(h *omniav1alpha1.HandlerDefinition) (*ToolOpenAPI, error) {
	if h.OpenAPIConfig == nil {
		return nil, nil
	}
	cfg := &ToolOpenAPI{
		SpecURL:         h.OpenAPIConfig.SpecURL,
		OperationFilter: h.OpenAPIConfig.OperationFilter,
		Headers:         h.OpenAPIConfig.Headers,
	}
	if h.OpenAPIConfig.BaseURL != nil {
		cfg.BaseURL = *h.OpenAPIConfig.BaseURL
	}
	rp, err := buildHTTPRetryPolicy(h.OpenAPIConfig.RetryPolicy)
	if err != nil {
		return nil, err
	}
	cfg.RetryPolicy = rp
	if authType, tokenPath, ok := authFieldsFor(h); ok {
		cfg.AuthType = authType
		cfg.AuthTokenPath = tokenPath
	}
	return cfg, nil
}

// buildHandlerEntry builds a single handler entry from the handler spec.
// Returns an error if retry policy translation fails for any transport.
func buildHandlerEntry(h *omniav1alpha1.HandlerDefinition, endpoint string) (HandlerEntry, error) {
	entry := HandlerEntry{
		Name:     h.Name,
		Type:     string(h.Type),
		Endpoint: endpoint,
	}
	if h.Timeout != nil {
		entry.Timeout = h.Timeout.Duration.String()
	}

	switch h.Type {
	case omniav1alpha1.HandlerTypeHTTP:
		cfg, err := buildHTTPConfig(h, endpoint)
		if err != nil {
			return entry, fmt.Errorf(errFmtHandler, h.Name, err)
		}
		entry.HTTPConfig = cfg
		entry.Tool = buildToolDefinition(h.Tool)
	case omniav1alpha1.HandlerTypeGRPC:
		cfg, err := buildGRPCConfig(h, endpoint)
		if err != nil {
			return entry, fmt.Errorf(errFmtHandler, h.Name, err)
		}
		entry.GRPCConfig = cfg
		entry.Tool = buildToolDefinition(h.Tool)
	case omniav1alpha1.HandlerTypeMCP:
		cfg, err := buildMCPConfig(h)
		if err != nil {
			return entry, fmt.Errorf(errFmtHandler, h.Name, err)
		}
		entry.MCPConfig = cfg
	case omniav1alpha1.HandlerTypeOpenAPI:
		cfg, err := buildOpenAPIConfig(h)
		if err != nil {
			return entry, fmt.Errorf(errFmtHandler, h.Name, err)
		}
		entry.OpenAPIConfig = cfg
	case omniav1alpha1.HandlerTypeClient:
		entry.Tool = buildToolDefinition(h.Tool)
		if h.ClientConfig != nil {
			entry.ClientConfig = &ToolClient{
				ConsentMessage: h.ClientConfig.ConsentMessage,
				Categories:     h.ClientConfig.Categories,
			}
		}
	}

	return entry, nil
}

// buildToolsConfig builds the tools configuration from ToolRegistry spec and status.
// Returns an error if any handler has an invalid retry policy. This fails the
// whole reconcile rather than emitting a partial config.
func (r *AgentRuntimeReconciler) buildToolsConfig(toolRegistry *omniav1alpha1.ToolRegistry) (ToolConfig, error) {
	config := ToolConfig{
		Handlers: make([]HandlerEntry, 0, len(toolRegistry.Spec.Handlers)),
	}

	for _, h := range toolRegistry.Spec.Handlers {
		// Client handlers have no backend endpoint
		if h.Type == omniav1alpha1.HandlerTypeClient {
			entry, err := buildHandlerEntry(&h, "")
			if err != nil {
				return ToolConfig{}, err
			}
			config.Handlers = append(config.Handlers, entry)
			continue
		}
		endpoint := findEndpoint(toolRegistry, h.Name)
		if endpoint == "" {
			continue
		}
		entry, err := buildHandlerEntry(&h, endpoint)
		if err != nil {
			return ToolConfig{}, err
		}
		config.Handlers = append(config.Handlers, entry)
	}

	return config, nil
}
