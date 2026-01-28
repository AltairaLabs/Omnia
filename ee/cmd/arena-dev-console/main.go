/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.

arena-dev-console provides a WebSocket-based interactive testing service
for PromptKit agents in the Arena project editor.

It allows developers to:
  - Create interactive chat sessions with their agents
  - Hot-reload agent configuration without restarting
  - Test tool calls and provider integrations in real-time

Architecture:

	Browser <--WebSocket--> Dashboard <--WebSocket--> Dev Console <---> PromptKit Runtime

This service reuses the facade/runtime pattern from the agent framework,
adding only the dynamic reload capability needed for interactive development.
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/altairalabs/omnia/ee/cmd/arena-dev-console/server"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

const (
	shutdownTimeout = 30 * time.Second
	readTimeout     = 10 * time.Second
	writeTimeout    = 10 * time.Second
	idleTimeout     = 120 * time.Second
)

var (
	httpPort      = flag.Int("http-port", 8080, "HTTP server port")
	healthPort    = flag.Int("health-port", 8081, "Health check server port")
	devMode       = flag.Bool("dev-mode", false, "Enable development mode (verbose logging)")
	sessionTTL    = flag.Duration("session-ttl", 30*time.Minute, "Session timeout duration")
	workspacePath = flag.String("workspace-path", "/workspace-content", "Base path for workspace content")
	configFile    = flag.String("config-file", "", "Optional: Path to arena config file for initialization")
)

func main() {
	flag.Parse()

	// Initialize logger
	var zapLog *zap.Logger
	var err error
	if *devMode {
		zapLog, err = zap.NewDevelopment()
	} else {
		zapLog, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = zapLog.Sync() }()
	log := zapr.NewLogger(zapLog)

	log.Info("starting arena-dev-console",
		"httpPort", *httpPort,
		"healthPort", *healthPort,
		"devMode", *devMode,
		"sessionTTL", *sessionTTL,
		"workspacePath", *workspacePath,
	)

	// Initialize session store (in-memory for dev console)
	store := session.NewMemoryStore()

	// Create the PromptKit handler
	handler, cleanup, err := createHandler(log, *configFile)
	if err != nil {
		log.Error(err, "failed to create handler")
		os.Exit(1)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Create WebSocket server using the facade pattern
	wsConfig := facade.DefaultServerConfig()
	wsConfig.SessionTTL = *sessionTTL
	wsServer := facade.NewServer(wsConfig, store, handler, log)

	// Create HTTP mux for routing
	mux := http.NewServeMux()

	// Main WebSocket endpoint
	mux.Handle("/ws", wsServer)

	// REST endpoints for session management and configuration
	mux.HandleFunc("/api/providers", handleListProviders(handler))
	mux.HandleFunc("/api/reload", handleReload(handler, log))

	// Create facade HTTP server
	facadeServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *httpPort),
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	// Create health check server
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", healthzHandler)
	healthMux.HandleFunc("/readyz", readyzHandler(handler))

	healthServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *healthPort),
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

// createHandler creates the PromptKit handler.
// Returns the handler and an optional cleanup function.
func createHandler(log logr.Logger, configFile string) (*server.PromptKitHandler, func(), error) {
	// Load initial configuration if provided
	var cfg *config.Config
	if configFile != "" {
		var err error
		cfg, err = config.LoadConfig(configFile)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load config from %s: %w", configFile, err)
		}
		log.Info("loaded initial configuration", "file", configFile)
	} else {
		// Create minimal default configuration
		cfg = &config.Config{
			LoadedProviders: make(map[string]*config.Provider),
		}
		log.Info("using empty configuration (will be loaded via reload)")
	}

	// Create the handler
	handler, err := server.NewPromptKitHandler(cfg, log.WithName("handler"))
	if err != nil {
		// Handler creation failed, but this might be expected if no config is provided
		// Return a nil handler that will be configured later via reload
		log.Info("handler creation deferred (no initial config)", "error", err.Error())
		return nil, nil, nil
	}

	cleanup := func() {
		if err := handler.Close(); err != nil {
			log.Error(err, "error closing handler")
		}
	}

	return handler, cleanup, nil
}

// handleListProviders returns the list of available providers.
func handleListProviders(handler *server.PromptKitHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if handler == nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
			return
		}

		providers := handler.ListProviders()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"providers":%q}`, providers)
	}
}

// handleReload handles configuration reload requests.
func handleReload(handler *server.PromptKitHandler, log logr.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if handler == nil {
			http.Error(w, "handler not initialized", http.StatusServiceUnavailable)
			return
		}

		// Get config path from query or body
		configPath := r.URL.Query().Get("path")
		if configPath == "" {
			http.Error(w, "path parameter required", http.StatusBadRequest)
			return
		}

		if err := handler.ReloadFromPath(configPath); err != nil {
			log.Error(err, "reload failed", "path", configPath)
			http.Error(w, fmt.Sprintf("reload failed: %v", err), http.StatusInternalServerError)
			return
		}

		log.Info("configuration reloaded", "path", configPath)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"reloaded"}`))
	}
}

func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func readyzHandler(handler *server.PromptKitHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The dev console is always ready to accept connections
		// Configuration can be loaded later via reload
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}
