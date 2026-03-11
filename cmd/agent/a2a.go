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

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/agent"
	facadea2a "github.com/altairalabs/omnia/internal/facade/a2a"
	"github.com/altairalabs/omnia/internal/tracing"

	"github.com/AltairaLabs/PromptKit/sdk"
	a2aserver "github.com/AltairaLabs/PromptKit/server/a2a"
)

// runA2AFacade starts the A2A JSON-RPC facade with PromptKit SDK in-process.
// Unlike the WebSocket facade, A2A does not use a separate runtime sidecar —
// the SDK handles LLM calls directly.
func runA2AFacade(cfg *agent.Config, log logr.Logger, _ *tracing.Provider) {
	log.Info("starting A2A facade",
		"port", cfg.FacadePort,
		"taskTTL", cfg.A2ATaskTTL,
		"conversationTTL", cfg.A2AConversationTTL,
		"taskStoreType", cfg.A2ATaskStoreType,
	)

	// Build authenticator
	var auth a2aserver.Authenticator
	if cfg.A2AAuthToken != "" {
		auth = facadea2a.NewBearerAuthenticator(cfg.A2AAuthToken)
		log.Info("A2A bearer auth enabled")
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
		Authenticator:   auth,
		TaskStore:       taskStore,
		SDKOptions:      sdkOptions,
		Log:             log,
	})

	// Create A2A metrics.
	a2aMetrics := facadea2a.NewMetrics(cfg.AgentName, cfg.Namespace)

	// Wrap the A2A handler with metrics middleware.
	a2aHandler := facadea2a.NewMetricsMiddleware(a2aSrv.Handler(), a2aMetrics)

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
