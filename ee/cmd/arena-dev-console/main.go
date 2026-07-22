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
	"strconv"
	"syscall"
	"time"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/altairalabs/omnia/ee/cmd/arena-dev-console/server"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/pkg/facade/auth"
	"github.com/altairalabs/omnia/pkg/k8s"
	"github.com/altairalabs/omnia/pkg/servicediscovery"
	"github.com/altairalabs/omnia/pkg/session/httpclient"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

const (
	shutdownTimeout = 30 * time.Second
	readTimeout     = 10 * time.Second
	writeTimeout    = 10 * time.Second
	idleTimeout     = 120 * time.Second

	envMgmtPlaneJWKSURL           = "OMNIA_MGMT_PLANE_JWKS_URL"
	envFacadeAllowUnauthenticated = "OMNIA_FACADE_ALLOW_UNAUTHENTICATED"
)

var (
	httpPort      = flag.Int("http-port", 8080, "HTTP server port")
	healthPort    = flag.Int("health-port", 8081, "Health check server port")
	devMode       = flag.Bool("dev-mode", false, "Enable development mode (verbose logging)")
	workspacePath = flag.String("workspace-path", "/workspace-content", "Base path for workspace content")
	configFile    = flag.String("config-file", "", "Optional: Path to arena config file for initialization")
	sessionAPIURL = flag.String("session-api-url", "", "URL of session-api service for session recording")
)

func main() {
	// Disable PromptKit JSON schema validation — the Go structs are the
	// source of truth and the remote schema may lag behind.  This also
	// avoids network fetches in air-gapped environments.
	config.SchemaValidationDisabled.Store(true)

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
		"workspacePath", *workspacePath,
	)

	// Start health server early so liveness probes pass during service
	// discovery retry (same pattern as eval-worker, see #750).
	go startHealthServer(*healthPort, log)

	// Resolve session-api URL: flag → env var → workspace CRD service discovery.
	apiURL := *sessionAPIURL
	if apiURL == "" {
		apiURL = os.Getenv("SESSION_API_URL")
	}
	if apiURL == "" {
		// Post-#717: resolve from Workspace CRD with retry.
		k8sClient, _ := k8s.NewClient()
		resolver := servicediscovery.NewResolver(k8sClient)
		group := os.Getenv("OMNIA_SERVICE_GROUP")
		if group == "" {
			group = "default"
		}
		// Operator-injected workspace name; never inferred from the namespace,
		// which is a different identifier (#1875). Retrying is pointless when it
		// is absent — it is a static env var, not a value that becomes available
		// — so fail fast rather than burning the backoff schedule.
		wsName, wsErr := k8s.WorkspaceNameFromEnvOrLabels(nil)
		if wsErr != nil {
			log.Error(wsErr, "cannot resolve session-api URL",
				"reason", "workspace name not injected",
				"impact", "dev console cannot reach session-api")
			os.Exit(1)
		}
		backoff := 2 * time.Second
		for attempt := 1; attempt <= 15; attempt++ {
			urls, resolveErr := resolver.ResolveServiceURLs(
				context.Background(), wsName, group,
			)
			if resolveErr == nil {
				apiURL = urls.SessionURL
				break
			}
			log.Info("session-api URL not ready, retrying",
				"attempt", attempt, "error", resolveErr.Error(),
				"retryIn", backoff.String())
			time.Sleep(backoff)
			backoff = min(backoff*2, 30*time.Second)
		}
		if apiURL == "" {
			log.Error(nil, "failed to resolve session-api URL after retries")
			os.Exit(1)
		}
	}
	store := httpclient.NewStore(apiURL, log)
	log.Info("session recording enabled via session-api", "url", apiURL)

	// Create the PromptKit handler
	handler, cleanup, err := createHandler(log, *configFile)
	if err != nil {
		log.Error(err, "failed to create handler")
		os.Exit(1)
	}
	if cleanup != nil {
		defer cleanup()
	}
	handler.SetReloadBasePath(*workspacePath)

	mgmtPlaneValidator, err := loadMgmtPlaneValidator(log)
	if err != nil {
		log.Error(err, "failed to initialize mgmt-plane validator")
		os.Exit(1)
	}
	authChain := auth.Chain{}
	if mgmtPlaneValidator != nil {
		authChain = append(authChain, mgmtPlaneValidator)
	}
	allowUnauthenticated := allowUnauthenticatedFallback(log)

	// Create WebSocket server using the facade pattern
	wsConfig := facade.DefaultServerConfig()
	serverOpts := []facade.ServerOption{
		facade.WithAllowUnauthenticated(allowUnauthenticated),
	}
	if len(authChain) > 0 {
		serverOpts = append(serverOpts, facade.WithAuthChain(authChain))
	}
	wsServer := facade.NewServer(wsConfig, store, handler, log, serverOpts...)

	mux := buildFacadeMux(wsServer, handler, log, authChain, allowUnauthenticated)

	// Create facade HTTP server
	facadeServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *httpPort),
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	// Health server already started early (before service discovery).
	// Start only the facade server here.
	errChan := make(chan error, 1)

	go func() {
		log.Info("starting facade server", "addr", facadeServer.Addr)
		if err := facadeServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("facade server error: %w", err)
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
	// Health server runs on a basic http.Server started in startHealthServer;
	// it shuts down when the process exits.

	log.Info("shutdown complete")
}

// createHandler creates the PromptKit handler.
// Returns the handler and an optional cleanup function.
//
// The handler supports two modes:
//  1. Static config: Load from a config file at startup
//  2. K8s dynamic: No config file, providers loaded dynamically from K8s based on namespace
//
// When running in K8s without a config file, providers are loaded dynamically
// from Provider CRDs in the namespace specified in the WebSocket connection.
func createHandler(log logr.Logger, configFile string) (*server.PromptKitHandler, func(), error) {
	// Load initial configuration if provided
	var cfg *arenaconfig.Config
	if configFile != "" {
		var err error
		cfg, err = arenaconfig.LoadConfig(configFile)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load config from %s: %w", configFile, err)
		}
		log.Info("loaded initial configuration", "file", configFile)
	} else {
		// No config file - handler will use K8s provider loading
		// or accept configuration via reload endpoint
		log.Info("no config file provided, will use K8s dynamic provider loading or reload endpoint")
	}

	// Create the handler
	// With K8s support, it can work without an initial config
	handler, err := server.NewPromptKitHandler(cfg, log.WithName("handler"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create handler: %w", err)
	}

	cleanup := func() {
		if err := handler.Close(); err != nil {
			log.Error(err, "error closing handler")
		}
	}

	return handler, cleanup, nil
}

// buildFacadeMux registers the dev console's three HTTP routes:
//   - /ws         — WebSocket endpoint backed by the facade server
//   - /api/providers — list configured providers (GET only)
//   - /api/reload    — hot-reload config from disk (POST only)
//
// Extracted so a wiring test can assert all three routes are registered
// without spinning up a real listener or PromptKit handler.
func buildFacadeMux(
	wsServer http.Handler,
	handler *server.PromptKitHandler,
	log logr.Logger,
	authChain auth.Chain,
	allowUnauthenticated bool,
) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/ws", wsServer)

	providersHandler := auth.Middleware(
		authChain,
		handleListProviders(handler),
		auth.WithMiddlewareLogger(log),
		auth.WithMiddlewareAllowUnauthenticated(allowUnauthenticated),
	)
	reloadHandler := auth.Middleware(
		authChain,
		handleReload(handler, log),
		auth.WithMiddlewareLogger(log),
		auth.WithMiddlewareAllowUnauthenticated(allowUnauthenticated),
	)
	mux.Handle("/api/providers", providersHandler)
	mux.Handle("/api/reload", reloadHandler)
	return mux
}

func loadMgmtPlaneValidator(log logr.Logger) (auth.Validator, error) {
	url := os.Getenv(envMgmtPlaneJWKSURL)
	if url == "" {
		log.V(1).Info("mgmt-plane validator skipped", "reason", "env var unset", "envVar", envMgmtPlaneJWKSURL)
		return nil, nil
	}
	v, err := auth.NewMgmtPlaneValidator(url)
	if err != nil {
		return nil, err
	}
	log.Info("mgmt-plane validator enabled", "jwksURL", url)
	return v, nil
}

func allowUnauthenticatedFallback(log logr.Logger) bool {
	raw := os.Getenv(envFacadeAllowUnauthenticated)
	if raw == "" {
		return false
	}
	allow, err := strconv.ParseBool(raw)
	if err != nil {
		log.Error(err, "strict auth fallback",
			"reason", "invalid env value",
			"var", envFacadeAllowUnauthenticated,
			"value", raw)
		return false
	}
	if allow {
		log.Info("strict auth disabled",
			"var", envFacadeAllowUnauthenticated,
			"reason", "dev/test escape hatch",
			"impact", "unauthenticated requests admitted when auth chain is empty")
	}
	return allow
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

// startHealthServer starts a minimal health endpoint so Kubernetes liveness
// probes pass while the main server is still initialising (e.g. during
// service-discovery retry). The full readyz handler is added later.
func startHealthServer(port int, log logr.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/readyz", healthzHandler) // basic readyz until full handler is wired
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Info("starting early health server", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error(err, "early health server failed")
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
