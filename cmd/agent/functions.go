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
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
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

	rc, err := newFunctionsRuntimeClient(cfg, log, tracingProvider)
	if err != nil {
		log.Error(err, "failed to dial runtime sidecar")
		os.Exit(1)
	}
	defer func() {
		if closeErr := rc.Close(); closeErr != nil {
			log.Error(closeErr, "failed to close runtime client")
		}
	}()

	handler := facade.NewFunctionsHandler(registry, rc, log)

	mux := http.NewServeMux()
	mux.Handle("POST /functions/{name}", handler)
	mux.Handle("/metrics", promhttp.Handler())

	facadeServer := newFacadeHTTPServer(cfg, mux)
	healthServer := newFunctionsHealthServer(cfg, rc)

	startFunctionsAndServe(log, facadeServer, healthServer)
}

// validateFunctionMode enforces the invariants the rest of the
// function-mode startup path depends on. The CRD's CEL gate already
// catches most of these, but defending in the binary keeps a
// downstream operator misconfiguration from booting a half-working
// pod.
func validateFunctionMode(cfg *agent.Config) error {
	if cfg.AgentName == "" {
		return fmt.Errorf("agent name is required")
	}
	if len(cfg.FunctionInputSchemaJSON) == 0 {
		return fmt.Errorf("function-mode runtime is missing spec.inputSchema")
	}
	if len(cfg.FunctionOutputSchemaJSON) == 0 {
		return fmt.Errorf("function-mode runtime is missing spec.outputSchema")
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
		Name:               strings.ToLower(cfg.AgentName),
		InputSchema:        inputSchema,
		OutputSchema:       outputSchema,
		RecordsInvocations: cfg.FunctionRecordsInvocations,
	})
	return registry, nil
}

// newFunctionsRuntimeClient dials the gRPC runtime sidecar. Mirrors
// the WebSocket path's runtime-client setup so the facade speaks to
// the same Invoke RPC.
func newFunctionsRuntimeClient(
	cfg *agent.Config,
	log logr.Logger,
	tracingProvider *tracing.Provider,
) (*facade.RuntimeClient, error) {
	clientCfg := facade.RuntimeClientConfig{
		Address:     cfg.RuntimeAddress,
		DialTimeout: 5 * time.Second,
		Log:         log,
	}
	if tracingProvider != nil {
		clientCfg.TracerProvider = tracingProvider.TracerProvider()
	}
	return facade.NewRuntimeClient(clientCfg)
}

// newFunctionsHealthServer mounts /healthz + /readyz on the health
// port. Readiness is "the runtime sidecar's gRPC Health says ok".
func newFunctionsHealthServer(cfg *agent.Config, rc *facade.RuntimeClient) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
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
	return &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HealthPort),
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}
}

// startFunctionsAndServe runs the facade + health servers and blocks
// until SIGINT/SIGTERM or a fatal server error.
func startFunctionsAndServe(log logr.Logger, facadeServer, healthServer *http.Server) {
	errChan := make(chan error, 2)

	go func() {
		log.Info("starting functions facade", "addr", facadeServer.Addr)
		if err := facadeServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("functions facade error: %w", err)
		}
	}()

	go func() {
		log.Info("starting health server", "addr", healthServer.Addr)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("health server error: %w", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Info("received shutdown signal", "signal", sig)
	case err := <-errChan:
		log.Error(err, "server error")
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := facadeServer.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down functions facade")
	}
	if err := healthServer.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down health server")
	}
	log.Info("shutdown complete")
}
