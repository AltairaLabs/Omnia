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
	"syscall"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	facadea2a "github.com/altairalabs/omnia/internal/facade/a2a"
	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/tracing"

	a2aserver "github.com/AltairaLabs/PromptKit/server/a2a"
)

// runWebSocketFacade starts the traditional WebSocket facade with a gRPC runtime sidecar.
// When A2A is enabled (dual-protocol mode), it also starts an A2A JSON-RPC server
// on a separate port.
func runWebSocketFacade(cfg *agent.Config, log logr.Logger, tracingProvider *tracing.Provider) {
	// Initialize session store
	store, err := initSessionStore(log)
	if err != nil {
		log.Error(err, "failed to initialize session store")
		os.Exit(1)
	}
	defer closeStore(store, log)

	// Create message handler based on mode
	handler, handlerCleanup := createHandler(cfg, log, tracingProvider)
	if handlerCleanup != nil {
		defer handlerCleanup()
	}

	// Create Prometheus metrics
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)

	// Initialize media storage BEFORE building the WS server so it can be
	// threaded into the facade via WithMediaStorage. Without this, the facade
	// server's mediaStorage is nil and the WS upload_request flow fails even
	// though the REST media handler routes are registered.
	mediaStorage, mediaCleanup := initMediaStorage(cfg, log)
	if mediaCleanup != nil {
		defer mediaCleanup()
	}

	wsServer, mux := buildWebSocketServer(cfg, log, store, handler, metrics, tracingProvider, mediaStorage)

	if mediaStorage != nil {
		mediaHandler := media.NewHandler(mediaStorage, log, media.WithHandlerMetrics(metrics))
		mediaHandler.RegisterRoutes(mux)
		log.Info("media storage enabled", "type", cfg.MediaStorageType, "path", cfg.MediaStoragePath)
	}

	facadeServer := newFacadeHTTPServer(cfg, mux)
	healthServer := newHealthHTTPServer(cfg, store, handler)

	// Dual-protocol: optionally start A2A server alongside WebSocket.
	var a2aSrv *facadea2a.Server
	var a2aHTTPServer *http.Server
	if cfg.A2AEnabled {
		a2aSrv, a2aHTTPServer = startA2AServer(cfg, log, tracingProvider)
	}

	startAndServe(log, wsServer, facadeServer, healthServer, a2aSrv, a2aHTTPServer)
}

// buildWebSocketServer creates the WebSocket server and HTTP mux.
//
// mediaStorage may be nil; if non-nil it is passed to facade.NewServer via
// WithMediaStorage so the WebSocket upload_request flow can resolve
// upload/download URLs. Without this, the facade's s.mediaStorage stays nil
// and WS media flows always error (even though REST media routes work).
func buildWebSocketServer(
	cfg *agent.Config,
	log logr.Logger,
	store session.Store,
	handler facade.MessageHandler,
	metrics *agent.Metrics,
	tracingProvider *tracing.Provider,
	mediaStorage media.Storage,
) (*facade.Server, *http.ServeMux) {
	wsConfig := facade.DefaultServerConfig()
	wsConfig.SessionTTL = cfg.SessionTTL
	wsConfig.PromptPackName = cfg.PromptPackName
	wsConfig.PromptPackVersion = cfg.PromptPackVersion
	wsConfig.WorkspaceName = cfg.WorkspaceName
	recordingPool := facade.NewRecordingPool(
		facade.DefaultRecordingPoolSize,
		facade.DefaultRecordingQueueSize,
		log,
	)
	serverOpts := []facade.ServerOption{
		facade.WithMetrics(metrics),
		facade.WithRecordingPool(recordingPool),
	}
	if tracingProvider != nil {
		serverOpts = append(serverOpts, facade.WithTracingProvider(tracingProvider))
	}
	if mediaStorage != nil {
		serverOpts = append(serverOpts, facade.WithMediaStorage(mediaStorage))
	}
	if pf, ok := store.(facade.PolicyFetcher); ok {
		serverOpts = append(serverOpts, facade.WithPolicyFetcher(pf))
	}
	// Load the mgmt-plane validator when the operator has pointed us at a
	// mounted dashboard public key. A loading failure (malformed PEM,
	// non-RSA key) is fatal — silently downgrading to "no auth" would
	// mask a real misconfiguration.
	if v, err := loadMgmtPlaneValidator(log); err != nil {
		log.Error(err, "mgmt-plane validator load failed")
		os.Exit(1)
	} else if v != nil {
		serverOpts = append(serverOpts, facade.WithMgmtPlaneValidator(v))
	}
	wsServer := facade.NewServer(wsConfig, store, handler, log, serverOpts...)

	mux := http.NewServeMux()
	mux.Handle("/ws", wsServer)
	mux.Handle("/api/agents/", wsServer)
	mux.Handle("/metrics", promhttp.Handler())

	return wsServer, mux
}

// newFacadeHTTPServer creates the facade HTTP server.
// WriteTimeout is intentionally omitted: WebSocket connections are long-lived
// and use ping/pong for keepalive.
func newFacadeHTTPServer(cfg *agent.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:        fmt.Sprintf(":%d", cfg.FacadePort),
		Handler:     handler,
		ReadTimeout: readTimeout,
		IdleTimeout: idleTimeout,
	}
}

// newHealthHTTPServer creates the health check HTTP server.
func newHealthHTTPServer(cfg *agent.Config, store session.Store, handler facade.MessageHandler) *http.Server {
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", healthzHandler)
	healthMux.HandleFunc("/readyz", readyzHandler(store, handler))

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HealthPort),
		Handler:      healthMux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}
}

// startAndServe starts all servers and blocks until shutdown signal or error.
func startAndServe(
	log logr.Logger,
	wsServer *facade.Server,
	facadeServer, healthServer *http.Server,
	a2aSrv *facadea2a.Server,
	a2aHTTPServer *http.Server,
) {
	errChan := make(chan error, 3)

	go func() {
		log.Info("starting facade server", "addr", facadeServer.Addr)
		if err := facadeServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("facade server error: %w", err)
		}
	}()

	go func() {
		log.Info("starting health server", "addr", healthServer.Addr)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("health server error: %w", err)
		}
	}()

	if a2aHTTPServer != nil {
		go func() {
			log.Info("starting A2A server (dual-protocol)", "addr", a2aHTTPServer.Addr)
			if err := a2aHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errChan <- fmt.Errorf("a2a server error: %w", err)
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Info("received shutdown signal", "signal", sig)
	case err := <-errChan:
		log.Error(err, "server error")
	}

	shutdownAll(log, wsServer, facadeServer, healthServer, a2aSrv, a2aHTTPServer)
}

// shutdownAll gracefully shuts down all servers.
func shutdownAll(
	log logr.Logger,
	wsServer *facade.Server,
	facadeServer, healthServer *http.Server,
	a2aSrv *facadea2a.Server,
	a2aHTTPServer *http.Server,
) {
	log.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := wsServer.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down websocket server")
	}
	if a2aSrv != nil {
		if err := a2aSrv.Shutdown(ctx); err != nil {
			log.Error(err, "error shutting down A2A server")
		}
	}
	if a2aHTTPServer != nil {
		if err := a2aHTTPServer.Shutdown(ctx); err != nil {
			log.Error(err, "error shutting down A2A HTTP server")
		}
	}
	if err := facadeServer.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down facade server")
	}
	if err := healthServer.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down health server")
	}

	log.Info("shutdown complete")
}

// startA2AServer creates and configures the A2A server for dual-protocol mode.
// Returns the A2A server (for shutdown) and the HTTP server (for ListenAndServe).
func startA2AServer(
	cfg *agent.Config,
	log logr.Logger,
	tracingProvider *tracing.Provider,
) (*facadea2a.Server, *http.Server) {
	log.Info("dual-protocol mode: starting A2A alongside WebSocket",
		"a2aPort", cfg.A2APort,
		"taskTTL", cfg.A2ATaskTTL,
		"conversationTTL", cfg.A2AConversationTTL,
	)

	// Build authenticator
	var auth a2aserver.Authenticator
	if cfg.A2AAuthToken != "" {
		auth = facadea2a.NewBearerAuthenticator(cfg.A2AAuthToken)
		log.Info("A2A bearer auth enabled")
	}

	// Build card provider
	cardProvider := buildCardProvider(cfg, log)

	// Build task store
	taskStore, storeCleanup := buildTaskStore(cfg, log)
	if storeCleanup != nil {
		// Note: cleanup is handled by the deferred call in runA2AFacade for standalone mode.
		// In dual-protocol mode, we register the cleanup here. The main goroutine will
		// handle shutdown via signal.
		// TODO: wire cleanup into the shutdown path if needed.
		_ = storeCleanup
	}

	// Pack path: for A2A, the SDK reads the pack directly
	packPath := cfg.PromptPackPath + "/pack.json"

	a2aSrv := facadea2a.NewServer(facadea2a.ServerConfig{
		PackPath:        packPath,
		PromptName:      "default",
		Port:            cfg.A2APort,
		TaskTTL:         cfg.A2ATaskTTL,
		ConversationTTL: cfg.A2AConversationTTL,
		CardProvider:    cardProvider,
		Authenticator:   auth,
		TaskStore:       taskStore,
		Log:             log,
	})

	// Create A2A metrics.
	a2aMetrics := facadea2a.NewMetrics(cfg.AgentName, cfg.Namespace)

	// Wrap with metrics + (optional) tracing middleware. Shared with
	// standalone mode via buildA2AHandler so both paths get tracing spans
	// when OMNIA_TRACING_ENABLED=true.
	a2aHandler := buildA2AHandler(a2aSrv.Handler(), a2aMetrics, tracingProvider)

	a2aHTTPServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.A2APort),
		Handler: a2aHandler,
	}

	return a2aSrv, a2aHTTPServer
}
