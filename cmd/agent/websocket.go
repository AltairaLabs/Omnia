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
	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/tracing"
)

// runWebSocketFacade starts the traditional WebSocket facade with a gRPC runtime sidecar.
//
//nolint:gocognit // main entry point
func runWebSocketFacade(cfg *agent.Config, log logr.Logger, tracingProvider *tracing.Provider) {
	// Initialize session store
	store, err := initSessionStore(cfg, log)
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

	// Create WebSocket server with metrics
	wsConfig := facade.DefaultServerConfig()
	wsConfig.SessionTTL = cfg.SessionTTL
	wsConfig.PromptPackName = cfg.PromptPackName
	wsConfig.PromptPackVersion = cfg.PromptPackVersion
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
	wsServer := facade.NewServer(wsConfig, store, handler, log, serverOpts...)

	// Create HTTP mux for routing
	mux := http.NewServeMux()
	mux.Handle("/ws", wsServer)
	// Also handle the dashboard's expected path format for E2E testing
	mux.Handle("/api/agents/", wsServer)
	mux.Handle("/metrics", promhttp.Handler())

	// Initialize media storage if configured
	mediaStorage, mediaCleanup := initMediaStorage(cfg, log)
	if mediaCleanup != nil {
		defer mediaCleanup()
	}
	if mediaStorage != nil {
		mediaHandler := media.NewHandler(mediaStorage, log, media.WithHandlerMetrics(metrics))
		mediaHandler.RegisterRoutes(mux)
		log.Info("media storage enabled", "type", cfg.MediaStorageType, "path", cfg.MediaStoragePath)
	}

	// Create facade HTTP server.
	// WriteTimeout is intentionally omitted: WebSocket connections are long-lived
	// and use ping/pong for keepalive. An HTTP WriteTimeout would kill the
	// connection during slow LLM inference (e.g. Ollama tool-calling).
	facadeServer := &http.Server{
		Addr:        fmt.Sprintf(":%d", cfg.FacadePort),
		Handler:     mux,
		ReadTimeout: readTimeout,
		IdleTimeout: idleTimeout,
	}

	// Create health check server
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", healthzHandler)
	healthMux.HandleFunc("/readyz", readyzHandler(store, handler))

	healthServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HealthPort),
		Handler:      healthMux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}

	// Start servers
	errChan := make(chan error, 2)

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

	// Shutdown WebSocket connections first
	if err := wsServer.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down websocket server")
	}

	// Shutdown HTTP servers
	if err := facadeServer.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down facade server")
	}
	if err := healthServer.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down health server")
	}

	log.Info("shutdown complete")
}
