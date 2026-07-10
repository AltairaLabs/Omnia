/*
Copyright 2026.

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

package mcp

import (
	"encoding/json"
	"net/http"
)

// AuthDiscoveryConfig parameterises the protected-resource metadata
// endpoint. authorization_servers is intentionally absent — Omnia
// accepts pre-issued tokens via APIKey / OIDC / EdgeTrust validators,
// with no Omnia-side OAuth issuer to direct clients to.
//
// Customers running an OIDC issuer can later wire it via a CRD field;
// today the array stays empty per RFC 9728 §3.
type AuthDiscoveryConfig struct {
	// Resource is the public URL of /mcp on this agent (the
	// resource the issued tokens are scoped to).
	Resource string

	// DocumentationURL is optional; points to user docs explaining
	// how to obtain a token for this resource.
	DocumentationURL string
}

// protectedResourceMetadata is the RFC 9728 §3 response shape served at
// /.well-known/oauth-protected-resource. Only the fields Omnia actually
// populates are present; the spec defines more (jwks_uri, scopes_supported,
// etc.) that don't apply to opaque pre-issued tokens.
type protectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ResourceDocumentation  string   `json:"resource_documentation,omitempty"`
}

// NewAuthDiscoveryHandler returns an http.Handler serving GET
// /.well-known/oauth-protected-resource. Other methods get 405.
//
// MCP 2025-03-26 clients use this endpoint as the target of the
// WWW-Authenticate header on 401 responses, to discover where to
// obtain a token.
func NewAuthDiscoveryHandler(cfg AuthDiscoveryConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		meta := protectedResourceMetadata{
			Resource:               cfg.Resource,
			AuthorizationServers:   []string{},
			BearerMethodsSupported: []string{"header"},
			ResourceDocumentation:  cfg.DocumentationURL,
		}
		w.Header().Set("Content-Type", "application/json")
		// Best-effort encode; on error the client gets a truncated
		// response. Caller logging would need plumbing for marginal
		// value (the only realistic failure is a closed connection).
		_ = json.NewEncoder(w).Encode(meta)
	})
}
