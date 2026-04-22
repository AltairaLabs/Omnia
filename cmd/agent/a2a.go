/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/agent"
	facadea2a "github.com/altairalabs/omnia/internal/facade/a2a"
	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/internal/tracing"

	"github.com/AltairaLabs/PromptKit/sdk"
	a2aserver "github.com/AltairaLabs/PromptKit/server/a2a"
)

// buildA2AHandler assembles the HTTP handler chain for an A2A server:
//
//	auth middleware -> inner handler -> metrics middleware -> OpenTelemetry tracing
//
// The tracing wrapper is only applied when tracingProvider is non-nil. The
// auth middleware is only applied when authChain is non-empty; an empty
// chain preserves the PR 1 unauthenticated-upgrade default. This is
// factored out of runA2AFacade and startA2AServer so both standalone and
// dual-protocol modes share the same middleware stack, and so wiring tests
// can assert that tracing + auth are wired when configured.
//
// Auth wraps OUTERMOST (before metrics/tracing) because rejected requests
// should be cheap — a caller spamming wrong bearer tokens shouldn't
// inflate the metrics counters or spawn otel spans for pipeline noise.
//
// inner is the handler returned by facadea2a.Server.Handler(). It is taken as
// an http.Handler (not *facadea2a.Server) so the wiring test can exercise
// this function without standing up a real A2A server.
func buildA2AHandler(
	inner http.Handler,
	metrics *facadea2a.Metrics,
	tracingProvider *tracing.Provider,
	authChain auth.Chain,
	log logr.Logger,
) http.Handler {
	var handler http.Handler = facadea2a.NewMetricsMiddleware(inner, metrics)
	if tracingProvider != nil {
		handler = otelhttp.NewHandler(handler, "a2a-facade",
			otelhttp.WithTracerProvider(tracingProvider.TracerProvider()),
		)
	}
	// Always wrap in the auth middleware, even when the chain is empty.
	// The middleware itself handles the empty-chain case: strict mode
	// (default) rejects all requests with 401; permissive mode (dev
	// escape hatch via OMNIA_FACADE_ALLOW_UNAUTHENTICATED) falls through.
	// Dropping the len-gate closes the residual C-3 bypass where a race
	// between the facade and the Workspace controller could produce an
	// empty chain and let unauthenticated A2A traffic through.
	handler = auth.Middleware(authChain, handler,
		auth.WithMiddlewareLogger(log),
		auth.WithMiddlewareAllowUnauthenticated(allowUnauthenticatedFallback(log)))
	return handler
}

// runA2AFacade starts the A2A JSON-RPC facade with PromptKit SDK in-process.
// Unlike the WebSocket facade, A2A does not use a separate runtime sidecar —
// the SDK handles LLM calls directly.
func runA2AFacade(cfg *agent.Config, log logr.Logger, tracingProvider *tracing.Provider) {
	log.Info("starting A2A facade",
		"port", cfg.FacadePort,
		"taskTTL", cfg.A2ATaskTTL,
		"conversationTTL", cfg.A2AConversationTTL,
		"taskStoreType", cfg.A2ATaskStoreType,
	)

	// Legacy per-SDK bearer authenticator. Kept for back-compat with
	// deployments that haven't migrated to spec.externalAuth.sharedToken
	// yet. Once the projection shim fires (PR 2b), OMNIA_A2A_AUTH_TOKEN
	// is unset because the shared token reaches the facade via the auth
	// chain instead — nothing below runs.
	var a2aAuth a2aserver.Authenticator
	if cfg.A2AAuthToken != "" {
		a2aAuth = facadea2a.NewBearerAuthenticator(cfg.A2AAuthToken)
		log.Info("A2A bearer auth enabled (legacy)")
	}

	// Build the PR 2b-era auth chain. Reads the agent's own
	// spec.externalAuth, then combines data-plane validators with the
	// mgmt-plane validator. Wrapped around buildA2AHandler below so an
	// A2A caller presenting a valid credential sees the same 200-OK
	// path as the WS facade.
	mgmtPlane, err := loadMgmtPlaneValidator(log)
	if err != nil {
		log.Error(err, "mgmt-plane validator load failed")
		os.Exit(1)
	}
	chain, err := buildAuthChain(context.Background(), buildK8sClient(), log, cfg.AgentName, cfg.Namespace, mgmtPlane)
	if err != nil {
		log.Error(err, "auth chain build failed")
		os.Exit(1)
	}

	// Build card provider from CRD config
	cardProvider := buildCardProvider(cfg, log)

	// Build task store
	taskStore, storeCleanup := buildTaskStore(cfg, log)
	if storeCleanup != nil {
		defer storeCleanup()
	}

	// Resolve A2A clients for tool bridge.
	var sdkOptions []sdk.Option
	if cfg.A2AClientsJSON != "" {
		clients, err := facadea2a.ParseResolvedClients(cfg.A2AClientsJSON)
		if err != nil {
			log.Error(err, "failed to parse A2A clients JSON")
		} else {
			sdkOptions = facadea2a.BuildA2AAgentOptions(context.Background(), clients, log)
		}
	}

	// Pack path: for A2A, the SDK reads the pack directly
	packPath := cfg.PromptPackPath + "/pack.json"

	a2aSrv := facadea2a.NewServer(facadea2a.ServerConfig{
		PackPath:        packPath,
		PromptName:      "default",
		Port:            cfg.FacadePort,
		TaskTTL:         cfg.A2ATaskTTL,
		ConversationTTL: cfg.A2AConversationTTL,
		CardProvider:    cardProvider,
		Authenticator:   a2aAuth,
		TaskStore:       taskStore,
		SDKOptions:      sdkOptions,
		Log:             log,
	})

	// Create A2A metrics.
	a2aMetrics := facadea2a.NewMetrics(cfg.AgentName, cfg.Namespace)

	// Wrap the A2A handler with auth + metrics + (optional) tracing middleware.
	a2aHandler := buildA2AHandler(a2aSrv.Handler(), a2aMetrics, tracingProvider, chain, log)

	// Build the mux with metrics endpoint.
	mux := http.NewServeMux()
	mux.Handle("/", a2aHandler)
	mux.Handle("/metrics", promhttp.Handler())

	// Serve A2A handler on the facade port.
	facadeServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.FacadePort),
		Handler: mux,
	}

	errChan := make(chan error, 1)
	go func() {
		log.Info("starting A2A server", "addr", facadeServer.Addr)
		if err := facadeServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("a2a server error: %w", err)
		}
	}()

	// Wait for shutdown signal or error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Info("received shutdown signal", "signal", sig)
	case err := <-errChan:
		log.Error(err, "server error")
	}

	// Graceful shutdown
	log.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := a2aSrv.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down A2A server")
	}
	if err := facadeServer.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down HTTP server")
	}

	log.Info("shutdown complete")
}

// buildCardProvider creates the agent card provider from config.
func buildCardProvider(cfg *agent.Config, log logr.Logger) a2aserver.AgentCardProvider {
	log.V(1).Info("building default agent card", "agentName", cfg.AgentName)

	spec := &omniav1alpha1.AgentCardSpec{
		Name:        cfg.AgentName,
		Description: fmt.Sprintf("Omnia agent: %s", cfg.AgentName),
	}

	endpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		cfg.AgentName, cfg.Namespace, cfg.FacadePort)

	return facadea2a.NewCRDCardProvider(spec, endpoint)
}

// buildTaskStore creates the appropriate task store based on configuration.
// Returns the store and an optional cleanup function.
func buildTaskStore(cfg *agent.Config, log logr.Logger) (a2aserver.TaskStore, func()) {
	if cfg.A2ATaskStoreType != "redis" || cfg.A2ARedisURL == "" {
		log.Info("using in-memory A2A task store")
		return nil, nil // nil means PromptKit uses its default in-memory store
	}

	log.Info("using Redis A2A task store", "taskTTL", cfg.A2ATaskTTL)

	opts, err := redis.ParseURL(cfg.A2ARedisURL)
	if err != nil {
		log.Error(err, "failed to parse A2A Redis URL, falling back to in-memory")
		return nil, nil
	}

	client := redis.NewClient(opts)

	// Verify connectivity.
	ctx, cancel := context.WithTimeout(context.Background(), 5*shutdownTimeout/6)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Error(err, "failed to connect to A2A Redis, falling back to in-memory")
		if closeErr := client.Close(); closeErr != nil {
			log.Error(closeErr, "error closing Redis client after ping failure")
		}
		return nil, nil
	}

	store := facadea2a.NewRedisTaskStore(facadea2a.RedisTaskStoreConfig{
		Client:  client,
		TaskTTL: cfg.A2ATaskTTL,
		Log:     log,
	})

	cleanup := func() {
		if err := store.Close(); err != nil {
			log.Error(err, "error closing A2A Redis task store")
		}
	}

	return store, cleanup
}
