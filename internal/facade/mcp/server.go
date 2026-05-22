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
	"net/http"

	"github.com/go-logr/logr"
)

const (
	// PathMCP is the Streamable HTTP endpoint clients POST JSON-RPC to.
	PathMCP = "/mcp"

	// PathResourceMetadata is the RFC 9728 discovery endpoint targeted
	// by the WWW-Authenticate header on 401 responses.
	PathResourceMetadata = "/.well-known/oauth-protected-resource"
)

// ServerConfig assembles the MCP server's dependencies.
type ServerConfig struct {
	// Adapter handles tools/list and tools/call. Required.
	Adapter ToolAdapter

	// ServerInfo identifies the MCP server in InitializeResult. Required.
	ServerInfo ServerInfo

	// Resource is the public URL of /mcp on this agent. Used as the
	// "resource" field in the protected-resource metadata and as the
	// base for ResourceMetadataURL.
	Resource string

	// DocumentationURL is optional; appears in the protected-resource
	// metadata for spec-compliant clients to deep-link operators to
	// user docs.
	DocumentationURL string

	// Log receives diagnostic events. logr.Discard() is fine.
	Log logr.Logger

	// Metrics is optional; nil disables MCP-level metrics.
	Metrics *Metrics
}

// Server bundles the MCP wire handlers into one http.Handler. The
// caller wraps the returned handler with auth middleware (so /mcp is
// protected) and tracing.
type Server struct {
	cfg     ServerConfig
	handler http.Handler
}

// NewServer constructs the server. Callers receive the assembled mux
// via Handler().
func NewServer(cfg ServerConfig) *Server {
	if cfg.Log.GetSink() == nil {
		cfg.Log = logr.Discard()
	}
	mux := http.NewServeMux()
	mux.Handle(PathMCP, NewTransport(TransportConfig{
		Adapter:    cfg.Adapter,
		ServerInfo: cfg.ServerInfo,
		Log:        cfg.Log,
	}))
	mux.Handle(PathResourceMetadata, NewAuthDiscoveryHandler(AuthDiscoveryConfig{
		Resource:         cfg.Resource,
		DocumentationURL: cfg.DocumentationURL,
	}))
	return &Server{cfg: cfg, handler: mux}
}

// Handler returns the assembled MCP HTTP handler. The caller is
// responsible for wrapping it with auth, tracing, and metrics
// middleware.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// ResourceMetadataURL returns the URL spec-compliant MCP clients
// dereference from the WWW-Authenticate header on 401 responses.
// Derived by appending the well-known path to a base URL the caller
// constructs (the Resource URL's host + scheme).
func (s *Server) ResourceMetadataURL() string {
	return resourceBase(s.cfg.Resource) + PathResourceMetadata
}

// resourceBase strips the /mcp path suffix (if present) from a Resource
// URL so we can append the well-known path cleanly. Pure helper —
// avoids pulling in net/url for this tiny manipulation.
func resourceBase(resource string) string {
	if len(resource) >= len(PathMCP) && resource[len(resource)-len(PathMCP):] == PathMCP {
		return resource[:len(resource)-len(PathMCP)]
	}
	return resource
}
