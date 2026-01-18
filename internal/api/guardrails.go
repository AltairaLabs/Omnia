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

package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ProxyMode defines the guardrail enforcement mode.
type ProxyMode string

const (
	// ProxyModeCompat logs all requests but allows everything (backward compatible).
	ProxyModeCompat ProxyMode = "compat"
	// ProxyModeStrict enforces namespace restrictions based on allow/deny lists.
	ProxyModeStrict ProxyMode = "strict"
)

// GuardrailsConfig holds configuration for the proxy guardrails.
type GuardrailsConfig struct {
	Mode              ProxyMode
	AllowedNamespaces []string // Empty = all allowed
	DeniedNamespaces  []string // Deny takes precedence
}

// RequestInfo captures details about an API request for logging and authorization.
type RequestInfo struct {
	User       string `json:"user,omitempty"`
	Verb       string `json:"verb"`
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
	Path       string `json:"path"`
	Allowed    bool   `json:"allowed"`
	DenyReason string `json:"deny_reason,omitempty"`
}

// parseRequestInfo extracts resource, verb, namespace, and name from the request path.
func (s *Server) parseRequestInfo(r *http.Request) RequestInfo {
	info := RequestInfo{
		Path: r.URL.Path,
		User: getUserFromRequest(r),
	}

	// Parse the path to extract resource information
	// Expected formats:
	// /api/v1/agents
	// /api/v1/agents/{ns}/{name}
	// /api/v1/agents/{ns}/{name}/logs
	// /api/v1/agents/{ns}/{name}/events
	// /api/v1/agents/{ns}/{name}/scale
	// /api/v1/promptpacks
	// /api/v1/promptpacks/{ns}/{name}
	// /api/v1/promptpacks/{ns}/{name}/content
	// /api/v1/toolregistries
	// /api/v1/toolregistries/{ns}/{name}
	// /api/v1/providers
	// /api/v1/providers/{ns}/{name}
	// /api/v1/stats
	// /api/v1/namespaces

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	parts := strings.Split(path, "/")

	if len(parts) > 0 {
		info.Resource = parts[0]
	}

	// Determine verb based on HTTP method and path structure
	switch r.Method {
	case http.MethodGet:
		if len(parts) == 1 {
			info.Verb = "list"
		} else {
			info.Verb = "get"
		}
	case http.MethodPost:
		info.Verb = "create"
	case http.MethodPut:
		info.Verb = "update"
	case http.MethodDelete:
		info.Verb = "delete"
	default:
		info.Verb = strings.ToLower(r.Method)
	}

	// Extract namespace and name if present
	if len(parts) >= 3 {
		info.Namespace = parts[1]
		info.Name = parts[2]

		// Handle sub-resources like /logs, /events, /scale, /content
		if len(parts) >= 4 {
			info.Resource = parts[0] + "/" + parts[3]
		}
	} else if len(parts) == 2 {
		// Could be namespace or name depending on context
		info.Namespace = parts[1]
	}

	// Handle namespace wildcard for list operations
	if info.Verb == "list" && info.Namespace == "" {
		info.Namespace = "*"
	}

	return info
}

// getUserFromRequest extracts the user identity from request headers.
// Supports common proxy authentication headers.
func getUserFromRequest(r *http.Request) string {
	// Check common proxy auth headers in order of preference
	headers := []string{
		"X-Forwarded-User",
		"X-Forwarded-Email",
		"X-Remote-User",
		"Authorization",
	}

	for _, h := range headers {
		if v := r.Header.Get(h); v != "" {
			// For Authorization header, try to extract a meaningful identifier
			if h == "Authorization" && strings.HasPrefix(v, "Bearer ") {
				// Don't log the full token, just indicate it's present
				return "bearer-token"
			}
			return v
		}
	}

	return "anonymous"
}

// isNamespaceAllowed checks if a namespace is allowed based on the guardrails config.
// Returns true if the namespace is allowed, false otherwise.
func (s *Server) isNamespaceAllowed(namespace string) bool {
	// Wildcard namespace (list all) - check if any allowed namespaces are configured
	if namespace == "*" {
		// In strict mode with specific allowed namespaces, deny list-all operations
		// unless allowedNamespaces contains "*" or is empty
		if len(s.guardrails.AllowedNamespaces) > 0 {
			for _, allowed := range s.guardrails.AllowedNamespaces {
				if allowed == "*" {
					return true
				}
			}
			// Don't deny list-all, but let individual items be filtered
			// This allows the list operation but the results should be filtered
			return true
		}
		return true
	}

	// Check denied namespaces first (deny takes precedence)
	for _, denied := range s.guardrails.DeniedNamespaces {
		if denied == namespace || denied == "*" {
			return false
		}
	}

	// If no allowed namespaces configured, allow all (except denied)
	if len(s.guardrails.AllowedNamespaces) == 0 {
		return true
	}

	// Check if wildcard is in allowed list
	for _, allowed := range s.guardrails.AllowedNamespaces {
		if allowed == "*" {
			return true
		}
	}

	// Check if namespace is in allowed list
	for _, allowed := range s.guardrails.AllowedNamespaces {
		if allowed == namespace {
			return true
		}
	}

	return false
}

// logRequest logs the request information in a structured format.
func (s *Server) logRequest(info RequestInfo) {
	fields := []any{
		"user", info.User,
		"verb", info.Verb,
		"resource", info.Resource,
		"namespace", info.Namespace,
		"name", info.Name,
		"path", info.Path,
		"allowed", info.Allowed,
	}

	if info.DenyReason != "" {
		fields = append(fields, "deny_reason", info.DenyReason)
	}

	s.log.Info("proxy request", fields...)
}

// guardrailsHandler wraps an HTTP handler with guardrails enforcement.
func (s *Server) guardrailsHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip guardrails for OPTIONS requests (CORS preflight)
		if r.Method == http.MethodOptions {
			h(w, r)
			return
		}

		info := s.parseRequestInfo(r)

		if s.guardrails.Mode == ProxyModeStrict {
			if !s.isNamespaceAllowed(info.Namespace) {
				info.Allowed = false
				info.DenyReason = "namespace not allowed"
				s.logRequest(info)
				s.writeGuardrailError(w, http.StatusForbidden, "access to namespace denied")
				return
			}
		}

		info.Allowed = true
		s.logRequest(info)
		h(w, r)
	}
}

// writeGuardrailError writes a JSON error response for guardrail denials.
func (s *Server) writeGuardrailError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// ParseNamespaceList parses a comma-separated list of namespaces.
func ParseNamespaceList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
