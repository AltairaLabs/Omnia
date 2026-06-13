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

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/internal/tracing"
)

// runFunctionsFacade starts the HTTP facade for a function-mode
// AgentRuntime (spec.mode == "function"). The pod shape is the same as
// the WebSocket case — facade container + runtime sidecar — but the
// facade exposes POST /functions/{name} instead of /ws.
//
// The route name {name} resolves to this AgentRuntime's own name. A
// function-mode pod serves exactly one Function — the one defined by
// the CRD it was deployed for. Any other name returns 404.
//
// Auth: the function route runs the same data-plane + mgmt-plane
// validator chain that the WebSocket upgrade path uses. When the chain
// loads cleanly, every request must present a credential admitted by
// one of the validators. When no chain is configured (no externalAuth,
// mgmt-plane pubkey unreadable), the route falls back to strict-default
// 401 unless OMNIA_FACADE_ALLOW_UNAUTHENTICATED=true.
func runFunctionsFacade(cfg *agent.Config, log logr.Logger, tracingProvider *tracing.Provider) {
	if err := validateFunctionMode(cfg); err != nil {
		log.Error(err, "invalid function-mode configuration")
		os.Exit(1)
	}

	registry, err := buildFunctionRegistry(cfg)
	if err != nil {
		log.Error(err, "failed to build function registry")
		os.Exit(1)
	}

	rc, err := dialRuntime(newDialRuntimeConfig(cfg.RuntimeAddress, tracingProvider), log)
	if err != nil {
		log.Error(err, "failed to dial runtime sidecar after retries")
		os.Exit(1)
	}
	defer func() {
		if closeErr := rc.Close(); closeErr != nil {
			log.Error(closeErr, "failed to close runtime client")
		}
	}()

	handler := facade.NewFunctionsHandler(registry, rc, log)

	// Wire session persistence: each invocation creates one `sessions`
	// row tagged "function". Failure to resolve session-api isn't
	// fatal — the runtime still serves the call, audit rows just land
	// without a parent (matches the pre-PR behaviour).
	if store, _, err := initSessionStore(log); err != nil {
		log.Error(err, "failed to init session store; function invocations will not be recorded")
	} else {
		handler = handler.WithSessionStore(store, facade.FunctionSessionMeta{
			Namespace:         cfg.Namespace,
			AgentName:         cfg.AgentName,
			WorkspaceName:     cfg.WorkspaceName,
			PromptPackName:    cfg.PromptPackName,
			PromptPackVersion: cfg.PromptPackVersion,
		})
	}

	// Build the shared auth chain once; both the HTTP route and (when
	// enabled) the MCP server's outer middleware use it.
	chain := buildFunctionAuthChain(cfg, log)

	mux := http.NewServeMux()
	mux.Handle("POST /functions/{name}", wrapWithAuthChain(chain, handler, log))

	facadeServer := newFunctionsHTTPServer(cfg, mux)
	healthServer := newFunctionsHealthServer(cfg, rc)
	mcpServer := buildMCPServer(cfg, rc, chain, tracingProvider, log)

	startFunctionsAndServe(log, facadeServer, healthServer, mcpServer)
}

// buildFunctionAuthChain loads the mgmt-plane + data-plane validator
// chain. Failures are fatal — silent downgrade to no-auth would mask
// a real misconfig.
func buildFunctionAuthChain(cfg *agent.Config, log logr.Logger) auth.Chain {
	mgmtPlane, err := loadMgmtPlaneValidator(log, cfg.AgentName, cfg.WorkspaceName)
	if err != nil {
		log.Error(err, "mgmt-plane validator load failed")
		os.Exit(1)
	}
	chain, err := buildAuthChain(context.Background(), buildK8sClient(), log,
		cfg.AgentName, cfg.Namespace, mgmtPlane)
	if err != nil {
		log.Error(err, "auth chain build failed")
		os.Exit(1)
	}
	return chain
}

// wrapWithAuthChain applies the auth middleware to the function HTTP
// route. The empty-chain fallback honours OMNIA_FACADE_ALLOW_UNAUTHENTICATED
// so dev/CI clusters without externalAuth keep working; production runs
// at minimum a mgmt-plane validator so the flag is a no-op for them.
func wrapWithAuthChain(chain auth.Chain, next http.Handler, log logr.Logger) http.Handler {
	return auth.Middleware(chain, next,
		auth.WithMiddlewareLogger(log),
		auth.WithMiddlewareAllowUnauthenticated(allowUnauthenticatedFallback(log)))
}

// newFunctionsHTTPServer is the function-mode counterpart to
// newFacadeHTTPServer. Unlike WebSocket — which deliberately omits
// WriteTimeout because connections are long-lived — function calls
// are one-shot and benefit from a hard upper bound. 60s is generous
// (most Function invocations are sub-10s) but covers the slow-provider
// tail without leaving sockets open forever when the runtime stalls.
func newFunctionsHTTPServer(cfg *agent.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.FacadePort),
		Handler:      handler,
		ReadTimeout:  readTimeout,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  idleTimeout,
	}
}

// validateFunctionMode is a thin wrapper over Config.Validate that
// surfaces the function-mode required-field errors with the runtime's
// own agent name attached for log correlation. Config.Validate() is
// the source of truth; this function exists so the test exercising
// validateFunctionMode and the production startup path call the same
// validation surface.
func validateFunctionMode(cfg *agent.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("function-mode config %q: %w", cfg.AgentName, err)
	}
	return nil
}

// buildFunctionRegistry compiles the input + output schemas once and
// returns a single-entry registry keyed by the canonical (lowercase)
// AgentRuntime name. The CRD-backed registry intentionally lives here
// rather than as a long-running watch: every AgentRuntime change
// triggers a Deployment rollout that restarts the pod, so a snapshot
// at startup is correct by construction.
func buildFunctionRegistry(cfg *agent.Config) (facade.FunctionRegistry, error) {
	inputSchema, err := facade.CompileSchema(cfg.FunctionInputSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("compile input schema: %w", err)
	}
	outputSchema, err := facade.CompileSchema(cfg.FunctionOutputSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("compile output schema: %w", err)
	}
	registry := facade.NewMapFunctionRegistry()
	registry.Register(&facade.FunctionSpec{
		Name:         strings.ToLower(cfg.AgentName),
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	})
	return registry, nil
}

// newFunctionsHealthServer mounts /healthz + /readyz on the health
// port. Readiness is "the runtime sidecar's gRPC Health says ok".
func newFunctionsHealthServer(cfg *agent.Config, rc *facade.RuntimeClient) *http.Server {
	return newHealthServer(cfg, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if _, err := rc.Health(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("runtime unavailable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// startFunctionsAndServe runs the facade + health (+ optional MCP)
// servers and blocks until SIGINT/SIGTERM or a fatal server error.
// mcpServer may be nil when MCP is disabled.
func startFunctionsAndServe(log logr.Logger, facadeServer, healthServer, mcpServer *http.Server) {
	errChan := make(chan error, 3)
	servers := nonNilServers(map[string]*http.Server{
		"functions facade": facadeServer,
		"health server":    healthServer,
		"mcp server":       mcpServer,
	})
	for name, srv := range servers {
		startServerGoroutine(name, srv, errChan, log)
	}

	waitForShutdownSignal(log, errChan)
	shutdownServers(log, servers)
	log.Info("shutdown complete")
}

// nonNilServers filters out nil entries so callers can pass optional
// servers (e.g. mcpServer when MCP is disabled) without per-key nil
// checks at the start/stop sites.
func nonNilServers(in map[string]*http.Server) map[string]*http.Server {
	out := make(map[string]*http.Server, len(in))
	for name, srv := range in {
		if srv != nil {
			out[name] = srv
		}
	}
	return out
}

// startServerGoroutine spawns the listen loop for one *http.Server and
// forwards any non-graceful shutdown error onto errChan.
func startServerGoroutine(name string, srv *http.Server, errChan chan<- error, log logr.Logger) {
	go func() {
		log.Info("starting "+name, "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("%s error: %w", name, err)
		}
	}()
}

// waitForShutdownSignal blocks until SIGINT/SIGTERM is received or any
// server's goroutine reports a fatal error.
func waitForShutdownSignal(log logr.Logger, errChan <-chan error) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigChan:
		log.Info("received shutdown signal", "signal", sig)
	case err := <-errChan:
		log.Error(err, "server error")
	}
}

// shutdownServers calls Shutdown on each server with a shared deadline.
// Errors are logged but not returned — there's nothing useful to do
// with them past this point in the lifecycle.
func shutdownServers(log logr.Logger, servers map[string]*http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	for name, srv := range servers {
		if err := srv.Shutdown(ctx); err != nil {
			log.Error(err, "error shutting down "+name)
		}
	}
}
