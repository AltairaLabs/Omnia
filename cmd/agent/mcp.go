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

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	facademcp "github.com/altairalabs/omnia/internal/facade/mcp"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/facade/auth"
)

// buildMCPHandler assembles the middleware stack for the MCP server.
// Layout matches buildA2AHandler:
//
//	auth (outermost) → tracing → metrics → inner protocol handler
//
// Auth wraps outermost so rejected requests don't spend the transport's
// metrics counters or otel spans. The 401 path sets WWW-Authenticate
// per the MCP 2025-03-26 spec, pointing at the protected-resource
// metadata endpoint so spec-compliant clients can discover the auth
// challenge.
//
// The empty-chain fallback honours OMNIA_FACADE_ALLOW_UNAUTHENTICATED
// so dev/CI clusters without externalAuth keep working; production
// runs at minimum a mgmt-plane validator so the flag is a no-op there.
func buildMCPHandler(
	inner http.Handler,
	tracingProvider *tracing.Provider,
	chain auth.Chain,
	resourceMetadataURL string,
	log logr.Logger,
) http.Handler {
	handler := inner
	if tracingProvider != nil {
		handler = otelhttp.NewHandler(handler, "mcp-facade",
			otelhttp.WithTracerProvider(tracingProvider.TracerProvider()),
		)
	}
	onReject := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate",
			fmt.Sprintf(`Bearer realm="omnia", resource_metadata=%q`, resourceMetadataURL))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
	return auth.Middleware(chain, handler,
		auth.WithMiddlewareLogger(log),
		auth.WithMiddlewareAllowUnauthenticated(allowUnauthenticatedFallback(log)),
		auth.WithMiddlewareOnReject(onReject))
}

// buildMCPServer constructs the *http.Server hosting the MCP endpoints.
// The caller's runtime client is wrapped in a fresh FunctionInvoker so
// MCP-side session recording matches the HTTP route's semantics.
//
// Returns nil when MCP is not enabled — call sites can switch on the
// returned value to decide whether to spin up the goroutine.
func buildMCPServer(
	cfg *agent.Config,
	rc facade.InvocationInvoker,
	chain auth.Chain,
	tracingProvider *tracing.Provider,
	log logr.Logger,
	port int32,
) *http.Server {
	if !cfg.MCPEnabled || port == 0 {
		return nil
	}

	registry, err := buildFunctionRegistry(cfg)
	if err != nil {
		log.Error(err, "build function registry for MCP failed")
		return nil
	}

	invoker := facade.NewFunctionInvoker(facade.FunctionInvokerConfig{
		Registry:     registry,
		Invoker:      rc,
		MaxBodyBytes: 0, // HTTP path has its own MaxBytesReader; MCP path validates via the transport's body cap
		Log:          log,
	})

	resourceURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/mcp",
		cfg.AgentName, cfg.Namespace, port)

	srv := facademcp.NewServer(facademcp.ServerConfig{
		Adapter: facademcp.NewFunctionToolAdapter(facademcp.FunctionToolAdapterConfig{
			Invoker: invoker,
			Tool: facademcp.Tool{
				Name:        strings.ToLower(cfg.AgentName),
				Description: fmt.Sprintf("Omnia function: %s", cfg.AgentName),
				InputSchema: json.RawMessage(cfg.FunctionInputSchemaJSON),
			},
			Log: log,
		}),
		ServerInfo:       facademcp.ServerInfo{Name: cfg.AgentName, Version: "1.0.0"},
		Resource:         resourceURL,
		DocumentationURL: "https://omnia.altairalabs.ai/docs/functions/mcp",
		Log:              log,
	})

	handler := buildMCPHandler(srv.Handler(), tracingProvider, chain, srv.ResourceMetadataURL(), log)

	mux := http.NewServeMux()
	mux.Handle("/", handler)

	log.Info("MCP server configured", "port", port, "resource", resourceURL)

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}
}
