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

// Package policy provides context propagation types, header constants, and helper
// functions shared across facade, runtime, and tool adapters.
package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

// Context keys for policy propagation.
const (
	// ContextKeyAgentName holds the agent name.
	ContextKeyAgentName contextKey = "omnia-agent-name"
	// ContextKeyNamespace holds the Kubernetes namespace.
	ContextKeyNamespace contextKey = "omnia-namespace"
	// ContextKeySessionID holds the session identifier.
	ContextKeySessionID contextKey = "omnia-session-id"
	// ContextKeyRequestID holds the per-request identifier.
	ContextKeyRequestID contextKey = "omnia-request-id"
	// ContextKeyUserID holds the authenticated user identity.
	ContextKeyUserID contextKey = "omnia-user-id"
	// ContextKeyUserRoles holds comma-separated user roles.
	ContextKeyUserRoles contextKey = "omnia-user-roles"
	// ContextKeyUserEmail holds the user email address.
	ContextKeyUserEmail contextKey = "omnia-user-email"
	// ContextKeyAuthorization holds the JWT token for passthrough.
	ContextKeyAuthorization contextKey = "omnia-authorization"
	// ContextKeyProvider holds the LLM provider type.
	ContextKeyProvider contextKey = "omnia-provider"
	// ContextKeyModel holds the LLM model name.
	ContextKeyModel contextKey = "omnia-model"
	// ContextKeyClaims holds extracted JWT claims as a map.
	ContextKeyClaims contextKey = "omnia-claims"
)

// HTTP/gRPC header constants for context propagation.
// These use lowercase for gRPC metadata compatibility.
const (
	// HeaderAgentName identifies the agent.
	HeaderAgentName = "x-omnia-agent-name"
	// HeaderNamespace identifies the Kubernetes namespace.
	HeaderNamespace = "x-omnia-namespace"
	// HeaderSessionID identifies the session.
	HeaderSessionID = "x-omnia-session-id"
	// HeaderRequestID identifies the request.
	HeaderRequestID = "x-omnia-request-id"
	// HeaderUserID identifies the authenticated user.
	HeaderUserID = "x-omnia-user-id"
	// HeaderUserRoles holds the user's roles.
	HeaderUserRoles = "x-omnia-user-roles"
	// HeaderUserEmail holds the user's email.
	HeaderUserEmail = "x-omnia-user-email"
	// HeaderAuthorization holds the JWT token.
	HeaderAuthorization = "authorization"
	// HeaderProvider identifies the LLM provider.
	HeaderProvider = "x-omnia-provider"
	// HeaderModel identifies the LLM model.
	HeaderModel = "x-omnia-model"
	// HeaderToolName identifies the tool being called.
	HeaderToolName = "x-omnia-tool-name"
	// HeaderToolRegistry identifies the tool registry.
	HeaderToolRegistry = "x-omnia-tool-registry"
	// HeaderClaimPrefix is the prefix for claim-mapped headers.
	HeaderClaimPrefix = "x-omnia-claim-"
	// HeaderParamPrefix is the prefix for promoted tool parameters.
	HeaderParamPrefix = "x-omnia-param-"
)

// Istio-injected header names that the facade reads from the WebSocket upgrade request.
const (
	// IstioHeaderUserID is the Istio header for user identity.
	IstioHeaderUserID = "x-user-id"
	// IstioHeaderUserRoles is the Istio header for user roles.
	IstioHeaderUserRoles = "x-user-roles"
	// IstioHeaderUserEmail is the Istio header for user email.
	IstioHeaderUserEmail = "x-user-email"
)

// PropagationFields holds all values for context propagation across service boundaries.
type PropagationFields struct {
	AgentName     string
	Namespace     string
	SessionID     string
	RequestID     string
	UserID        string
	UserRoles     string
	UserEmail     string
	Authorization string
	Provider      string
	Model         string
	Claims        map[string]string
}

// WithAgentName returns a context with the agent name set.
func WithAgentName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, ContextKeyAgentName, name)
}

// WithNamespace returns a context with the namespace set.
func WithNamespace(ctx context.Context, ns string) context.Context {
	return context.WithValue(ctx, ContextKeyNamespace, ns)
}

// WithSessionID returns a context with the session ID set.
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ContextKeySessionID, id)
}

// WithRequestID returns a context with the request ID set.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ContextKeyRequestID, id)
}

// WithUserID returns a context with the user ID set.
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ContextKeyUserID, id)
}

// WithUserRoles returns a context with the user roles set.
func WithUserRoles(ctx context.Context, roles string) context.Context {
	return context.WithValue(ctx, ContextKeyUserRoles, roles)
}

// WithUserEmail returns a context with the user email set.
func WithUserEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, ContextKeyUserEmail, email)
}

// WithAuthorization returns a context with the authorization token set.
func WithAuthorization(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, ContextKeyAuthorization, token)
}

// WithProvider returns a context with the provider type set.
func WithProvider(ctx context.Context, provider string) context.Context {
	return context.WithValue(ctx, ContextKeyProvider, provider)
}

// WithModel returns a context with the model name set.
func WithModel(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, ContextKeyModel, model)
}

// WithClaims returns a context with the JWT claim map set.
func WithClaims(ctx context.Context, claims map[string]string) context.Context {
	return context.WithValue(ctx, ContextKeyClaims, claims)
}

// WithPropagationFields returns a context with all propagation fields set.
// Only non-empty values are stored.
func WithPropagationFields(ctx context.Context, fields *PropagationFields) context.Context {
	if fields == nil {
		return ctx
	}
	ctx = setIfNonEmpty(ctx, ContextKeyAgentName, fields.AgentName)
	ctx = setIfNonEmpty(ctx, ContextKeyNamespace, fields.Namespace)
	ctx = setIfNonEmpty(ctx, ContextKeySessionID, fields.SessionID)
	ctx = setIfNonEmpty(ctx, ContextKeyRequestID, fields.RequestID)
	ctx = setIfNonEmpty(ctx, ContextKeyUserID, fields.UserID)
	ctx = setIfNonEmpty(ctx, ContextKeyUserRoles, fields.UserRoles)
	ctx = setIfNonEmpty(ctx, ContextKeyUserEmail, fields.UserEmail)
	ctx = setIfNonEmpty(ctx, ContextKeyAuthorization, fields.Authorization)
	ctx = setIfNonEmpty(ctx, ContextKeyProvider, fields.Provider)
	ctx = setIfNonEmpty(ctx, ContextKeyModel, fields.Model)
	if len(fields.Claims) > 0 {
		ctx = WithClaims(ctx, fields.Claims)
	}
	return ctx
}

// setIfNonEmpty sets a context value only if the value is non-empty.
func setIfNonEmpty(ctx context.Context, key contextKey, value string) context.Context {
	if value != "" {
		return context.WithValue(ctx, key, value)
	}
	return ctx
}

// ExtractPropagationFields extracts all propagation fields from a context.
func ExtractPropagationFields(ctx context.Context) PropagationFields {
	return PropagationFields{
		AgentName:     getString(ctx, ContextKeyAgentName),
		Namespace:     getString(ctx, ContextKeyNamespace),
		SessionID:     getString(ctx, ContextKeySessionID),
		RequestID:     getString(ctx, ContextKeyRequestID),
		UserID:        getString(ctx, ContextKeyUserID),
		UserRoles:     getString(ctx, ContextKeyUserRoles),
		UserEmail:     getString(ctx, ContextKeyUserEmail),
		Authorization: getString(ctx, ContextKeyAuthorization),
		Provider:      getString(ctx, ContextKeyProvider),
		Model:         getString(ctx, ContextKeyModel),
		Claims:        getClaims(ctx),
	}
}

// getString extracts a string value from context, returning empty string if not found.
func getString(ctx context.Context, key contextKey) string {
	if v := ctx.Value(key); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getClaims extracts the claims map from context.
func getClaims(ctx context.Context) map[string]string {
	if v := ctx.Value(ContextKeyClaims); v != nil {
		if m, ok := v.(map[string]string); ok {
			return m
		}
	}
	return nil
}

// AgentName extracts the agent name from context.
func AgentName(ctx context.Context) string { return getString(ctx, ContextKeyAgentName) }

// Namespace extracts the namespace from context.
func Namespace(ctx context.Context) string { return getString(ctx, ContextKeyNamespace) }

// SessionID extracts the session ID from context.
func SessionID(ctx context.Context) string { return getString(ctx, ContextKeySessionID) }

// RequestID extracts the request ID from context.
func RequestID(ctx context.Context) string { return getString(ctx, ContextKeyRequestID) }

// UserID extracts the user ID from context.
func UserID(ctx context.Context) string { return getString(ctx, ContextKeyUserID) }

// UserRoles extracts the user roles from context.
func UserRoles(ctx context.Context) string { return getString(ctx, ContextKeyUserRoles) }

// Authorization extracts the authorization token from context.
func Authorization(ctx context.Context) string { return getString(ctx, ContextKeyAuthorization) }

// Provider extracts the provider type from context.
func Provider(ctx context.Context) string { return getString(ctx, ContextKeyProvider) }

// Model extracts the model name from context.
func Model(ctx context.Context) string { return getString(ctx, ContextKeyModel) }

// Claims extracts the JWT claims from context.
func Claims(ctx context.Context) map[string]string { return getClaims(ctx) }

// headerKeyMap maps context keys to their corresponding header names.
var headerKeyMap = []struct {
	key    contextKey
	header string
}{
	{ContextKeyAgentName, HeaderAgentName},
	{ContextKeyNamespace, HeaderNamespace},
	{ContextKeySessionID, HeaderSessionID},
	{ContextKeyRequestID, HeaderRequestID},
	{ContextKeyUserID, HeaderUserID},
	{ContextKeyUserRoles, HeaderUserRoles},
	{ContextKeyUserEmail, HeaderUserEmail},
	{ContextKeyAuthorization, HeaderAuthorization},
	{ContextKeyProvider, HeaderProvider},
	{ContextKeyModel, HeaderModel},
}

// ToOutboundHeaders converts context propagation fields to a flat map of header key-value pairs.
// This is used by tool adapters for outbound HTTP requests.
func ToOutboundHeaders(ctx context.Context) map[string]string {
	headers := make(map[string]string)
	for _, entry := range headerKeyMap {
		if v := getString(ctx, entry.key); v != "" {
			headers[entry.header] = v
		}
	}
	// Append claim headers
	for name, value := range getClaims(ctx) {
		headers[HeaderClaimPrefix+name] = value
	}
	return headers
}

// ToGRPCMetadata converts context propagation fields to a flat map suitable for gRPC metadata.
// Header keys are already lowercase, which is required for gRPC metadata.
func ToGRPCMetadata(ctx context.Context) map[string]string {
	return ToOutboundHeaders(ctx)
}

// ToPascalCase converts a snake_case or kebab-case string to PascalCase.
// Used for parameter promotion headers (X-Omnia-Param-<PascalCase>).
func ToPascalCase(s string) string {
	var result strings.Builder
	capitalize := true
	for _, r := range s {
		if r == '_' || r == '-' {
			capitalize = true
			continue
		}
		if capitalize {
			result.WriteRune(unicode.ToUpper(r))
			capitalize = false
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// PromoteScalarParams converts scalar tool arguments to parameter headers.
// Only string, number (int/float), and boolean values are promoted.
// Nested objects and arrays are skipped.
func PromoteScalarParams(args map[string]any) map[string]string {
	headers := make(map[string]string)
	for name, value := range args {
		headerVal, ok := scalarToString(value)
		if !ok {
			continue
		}
		headerName := fmt.Sprintf("%s%s", HeaderParamPrefix, ToPascalCase(name))
		headers[headerName] = headerVal
	}
	return headers
}

// scalarToString converts a scalar value to its string representation.
// Returns false for non-scalar types (slices, maps, nil).
func scalarToString(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case bool:
		return fmt.Sprintf("%t", v), true
	case float64:
		return fmt.Sprintf("%g", v), true
	case float32:
		return fmt.Sprintf("%g", v), true
	case int:
		return fmt.Sprintf("%d", v), true
	case int64:
		return fmt.Sprintf("%d", v), true
	case json.Number:
		return v.String(), true
	default:
		return "", false
	}
}
