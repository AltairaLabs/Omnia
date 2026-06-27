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
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/redis/go-redis/v9"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	facadea2a "github.com/altairalabs/omnia/internal/facade/a2a"
	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/tracing"
)

// runWebSocketFacade starts the traditional WebSocket facade with a gRPC runtime sidecar.
// When A2A is enabled (dual-protocol mode), it also starts an A2A JSON-RPC server
// on a separate port.
func runWebSocketFacade(cfg *agent.Config, log logr.Logger, tracingProvider *tracing.Provider) {
	// Initialize session store
	store, storeMode, err := initSessionStore(log)
	if err != nil {
		log.Error(err, "failed to initialize session store")
		os.Exit(1)
	}
	defer closeStore(store, log)

	// Create Prometheus metrics
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)
	// Surface the active session-store mode so a silent in-memory fallback
	// (no session-api recording) is observable/alertable (issue #1223).
	metrics.SetSessionStoreMode(storeMode)

	// Recording infrastructure, created before the handler so it can be injected
	// into the RuntimeClient's bus recorder (records conversation messages off
	// the gRPC bus) and shared with the server's session-completion writer.
	recordingPool := facade.NewRecordingPool(
		facade.DefaultRecordingPoolSize, facade.DefaultRecordingQueueSize, log, metrics)
	var recordingPolicy *facade.RecordingPolicyCache
	if pf, ok := store.(facade.PolicyFetcher); ok {
		recordingPolicy = facade.NewRecordingPolicyCache(
			pf, cfg.Namespace, cfg.AgentName, 60*time.Second, log)
	}

	// Create message handler based on mode
	handler, handlerCleanup := createHandler(cfg, log, tracingProvider, store, recordingPool, recordingPolicy)
	if handlerCleanup != nil {
		defer handlerCleanup()
	}

	// Initialize media storage BEFORE building the WS server so it can be
	// threaded into the facade via WithMediaStorage. Without this, the facade
	// server's mediaStorage is nil and the WS upload_request flow fails even
	// though the REST media handler routes are registered.
	mediaStorage, mediaCleanup := initMediaStorage(cfg, log)
	if mediaCleanup != nil {
		defer mediaCleanup()
	}

	servers, err := buildWebSocketServer(
		cfg, log, store, handler, metrics, recordingPool, tracingProvider, mediaStorage)
	if err != nil {
		log.Error(err, "failed to build websocket server")
		os.Exit(1)
	}

	if mediaStorage != nil {
		mediaHandler := media.NewHandler(mediaStorage, log, media.WithHandlerMetrics(metrics))
		mediaHandler.RegisterRoutes(servers.externalMux)
		if servers.internalMux != nil {
			mediaHandler.RegisterRoutes(servers.internalMux)
		}
		log.Info("media storage enabled", "type", cfg.MediaStorageType, "path", cfg.MediaStoragePath)
	}

	set := &facadeServerSet{
		wsServer:     servers.external,
		facadeServer: newFacadeHTTPServer(cfg, servers.externalMux),
		healthServer: newHealthHTTPServer(cfg, store, handler, servers.external),
	}
	if servers.internal != nil {
		set.internalWSServer = servers.internal
		set.internalFacadeServer = newInternalFacadeHTTPServer(cfg, servers.internalMux)
		log.Info("management-plane internal listener enabled", "port", cfg.InternalFacadePort)
	}

	// Dual-protocol: optionally start A2A server alongside WebSocket.
	if cfg.A2AEnabled {
		set.a2aSrv, set.a2aHTTPServer, set.internalA2AHTTPServer, set.a2aCleanup = startA2AServer(cfg, log, tracingProvider)
	}

	startAndServe(log, set)
}

// webSocketServers holds the external facade server and its optional internal
// management-plane twin (and their muxes). The internal pair is nil when no
// internal listener is configured (cfg.InternalFacadePort == 0, i.e.
// allowManagementPlane disabled).
type webSocketServers struct {
	external    *facade.Server
	externalMux *http.ServeMux
	internal    *facade.Server
	internalMux *http.ServeMux
}

// facadeServerSet groups every long-lived server for startup/shutdown so the
// internal twin listener can be threaded through without exploding the
// start/shutdown signatures. The internal* fields are nil when no internal
// management-plane listener is configured.
type facadeServerSet struct {
	wsServer              *facade.Server
	facadeServer          *http.Server
	internalWSServer      *facade.Server
	internalFacadeServer  *http.Server
	healthServer          *http.Server
	a2aSrv                *facadea2a.Server
	a2aHTTPServer         *http.Server
	internalA2AHTTPServer *http.Server
	a2aCleanup            func()
}

// newWSMux mounts the WebSocket routes onto a fresh mux for a facade server.
func newWSMux(server *facade.Server) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/ws", server)
	mux.Handle("/api/agents/", server)
	return mux
}

// cloneFacadeOpts copies a ServerOption slice so the external and internal
// servers can extend the same base opts without aliasing each other's backing
// array.
func cloneFacadeOpts(opts []facade.ServerOption) []facade.ServerOption {
	return append([]facade.ServerOption(nil), opts...)
}

// buildWebSocketServer creates the external WebSocket server (and, when an
// internal management-plane port is configured, its internal twin) plus their
// HTTP muxes.
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
	recordingPool *facade.RecordingPool,
	tracingProvider *tracing.Provider,
	mediaStorage media.Storage,
) (*webSocketServers, error) {
	wsConfig := facade.DefaultServerConfig()
	wsConfig.SessionTTL = cfg.SessionTTL
	wsConfig.PromptPackName = cfg.PromptPackName
	wsConfig.PromptPackVersion = cfg.PromptPackVersion
	wsConfig.WorkspaceName = cfg.WorkspaceName
	// Only override when the CRD field is explicitly set; zero means "unset"
	// and must not clobber the 30s default from DefaultServerConfig.
	if cfg.DrainTimeout > 0 {
		wsConfig.DrainTimeout = cfg.DrainTimeout
	}
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
	// Wire the duplex sink factory if the handler is a RuntimeHandler — that
	// means a runtime gRPC client is available and audio duplex is supported.
	// When the handler is echo/demo mode there is no runtime client, so the
	// factory stays nil and inbound audio frames are rejected gracefully.
	if rh, ok := handler.(*agent.RuntimeHandler); ok {
		runtimeClient := rh.Client()
		serverOpts = append(serverOpts, facade.WithDuplexSinkFactory(
			func(sessionID string, w facade.ResponseWriter) facade.DuplexSink {
				return agent.NewGRPCDuplexSink(sessionID, runtimeClient, w)
			},
		))
	}
	// Wire the route store for blip-resume: parked sessions need a Redis-backed
	// hint so a peer can redirect reconnecting clients to the pod holding the
	// open audio stream. When OMNIA_ROUTE_REDIS_URL is unset (non-audio or
	// no Redis configured) the server falls back to noopRouteStore silently.
	podAddr := net.JoinHostPort(os.Getenv("POD_IP"), strconv.Itoa(cfg.FacadePort))
	serverOpts = append(serverOpts, facade.WithPodAddr(podAddr))
	const defaultGraceWindow = 15 // seconds
	graceWindowSecs := defaultGraceWindow
	if gs := os.Getenv("OMNIA_GRACE_WINDOW_SECONDS"); gs != "" {
		if n, parseErr := strconv.Atoi(gs); parseErr == nil && n > 0 {
			graceWindowSecs = n
		}
	}
	serverOpts = append(serverOpts, facade.WithGraceWindow(graceWindowDuration(graceWindowSecs)))
	if routeURL := os.Getenv("OMNIA_ROUTE_REDIS_URL"); routeURL != "" {
		ropts, parseErr := redis.ParseURL(routeURL)
		if parseErr != nil {
			return nil, fmt.Errorf("parse route redis url: %w", parseErr)
		}
		serverOpts = append(serverOpts, facade.WithRouteStore(agent.NewRedisRouteStore(redis.NewClient(ropts))))
	}

	// Build the auth chain: data-plane validators (sharedToken in PR 2b;
	// apiKeys/oidc/edgeTrust in PRs 2c–2e) followed by the mgmt-plane
	// validator. Loading failures (malformed PEM, missing Secret data
	// key, empty shared token) are fatal — silent downgrade to no-auth
	// would mask real operator misconfig.
	mgmtPlane, err := loadMgmtPlaneValidator(log, cfg.AgentName, cfg.WorkspaceName)
	if err != nil {
		return nil, fmt.Errorf("mgmt-plane validator load failed: %w", err)
	}
	k8sClient := buildK8sClient()

	// External listener: data-plane validators only. The management plane is
	// fully isolated onto the internal twin listener (mgmt-plane callers — the
	// dashboard and Doctor — dial the internal port), so the external chain no
	// longer carries the mgmt-plane validator.
	externalChain, err := buildExternalChain(context.Background(), k8sClient, log, cfg.AgentName, cfg.Namespace)
	if err != nil {
		return nil, fmt.Errorf("external auth chain build failed: %w", err)
	}
	extOpts := cloneFacadeOpts(serverOpts)
	if len(externalChain) > 0 {
		extOpts = append(extOpts, facade.WithAuthChain(externalChain))
	}
	// Strict by default. When the chain is empty (no externalAuth
	// configured AND mgmt-plane pubkey unreadable — typically a boot race
	// with the Workspace controller or dashboard.enabled=false), the
	// facade rejects unauthenticated upgrades. This closes the residual
	// C-3 bypass that PR 3's WithAllowUnauthenticated default preserved
	// for back-compat. Set OMNIA_FACADE_ALLOW_UNAUTHENTICATED=true only
	// in dev/CI.
	extOpts = append(extOpts,
		facade.WithAllowUnauthenticated(allowUnauthenticatedFallback(log)))
	external := facade.NewServer(wsConfig, store, handler, log, extOpts...)

	servers := &webSocketServers{external: external, externalMux: newWSMux(external)}

	// Internal twin listener: management-plane-only chain. Started only when the
	// controller has allocated an internal port (allowManagementPlane enabled).
	// It never permits unauthenticated upgrades — it exists solely for
	// mgmt-plane callers, which always present a dashboard-minted JWT.
	if cfg.InternalFacadePort != 0 {
		intOpts := cloneFacadeOpts(serverOpts)
		if mgmtChain := buildMgmtChain(mgmtPlane); len(mgmtChain) > 0 {
			intOpts = append(intOpts, facade.WithAuthChain(mgmtChain))
		}
		intOpts = append(intOpts, facade.WithAllowUnauthenticated(false))
		internal := facade.NewServer(wsConfig, store, handler, log, intOpts...)
		servers.internal = internal
		servers.internalMux = newWSMux(internal)
	}

	return servers, nil
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

// newInternalFacadeHTTPServer creates the internal management-plane facade HTTP
// server on cfg.InternalFacadePort. Same timeouts as the external listener.
func newInternalFacadeHTTPServer(cfg *agent.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:        fmt.Sprintf(":%d", cfg.InternalFacadePort),
		Handler:     handler,
		ReadTimeout: readTimeout,
		IdleTimeout: idleTimeout,
	}
}

// newHealthHTTPServer creates the health check HTTP server.
func newHealthHTTPServer(
	cfg *agent.Config, store session.Store,
	handler facade.MessageHandler, wsServer *facade.Server,
) *http.Server {
	return newHealthServer(cfg, readyzHandler(store, handler, wsServer))
}

// startAndServe starts all servers and blocks until shutdown signal or error.
func startAndServe(log logr.Logger, set *facadeServerSet) {
	errChan := make(chan error, 6)

	go serveHTTP(log, set.facadeServer, "facade server", errChan)
	go serveHTTP(log, set.healthServer, "health server", errChan)
	if set.internalFacadeServer != nil {
		go serveHTTP(log, set.internalFacadeServer, "internal facade server", errChan)
	}
	if set.a2aHTTPServer != nil {
		go serveHTTP(log, set.a2aHTTPServer, "a2a server", errChan)
	}
	if set.internalA2AHTTPServer != nil {
		go serveHTTP(log, set.internalA2AHTTPServer, "internal a2a server", errChan)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Info("received shutdown signal", "signal", sig)
	case err := <-errChan:
		log.Error(err, "server error")
	}

	shutdownAll(log, set)
}

// serveHTTP runs srv.ListenAndServe and reports a non-graceful error on errChan.
func serveHTTP(log logr.Logger, srv *http.Server, name string, errChan chan<- error) {
	log.Info("starting "+name, "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		errChan <- fmt.Errorf("%s error: %w", name, err)
	}
}

// shutdownAll gracefully shuts down all servers.
func shutdownAll(log logr.Logger, set *facadeServerSet) {
	log.Info("shutting down...")

	// Drain active realtime sessions before closing connections. This gives
	// in-progress calls up to DrainTimeout to finish naturally; the load
	// balancer already sees 503 on /readyz (IsDraining flipped by Drain).
	drainCtx, drainCancel := context.WithTimeout(context.Background(), set.wsServer.DrainTimeoutForShutdown())
	defer drainCancel()
	remaining := set.wsServer.Drain(drainCtx)
	log.Info("facade drained before shutdown", "remaining", remaining)

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := set.wsServer.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down websocket server")
	}
	if set.internalWSServer != nil {
		if err := set.internalWSServer.Shutdown(ctx); err != nil {
			log.Error(err, "error shutting down internal websocket server")
		}
	}
	if set.a2aSrv != nil {
		if err := set.a2aSrv.Shutdown(ctx); err != nil {
			log.Error(err, "error shutting down A2A server")
		}
	}
	if set.a2aCleanup != nil {
		set.a2aCleanup()
	}
	shutdownHTTP(ctx, log, set.a2aHTTPServer, "A2A HTTP server")
	shutdownHTTP(ctx, log, set.internalA2AHTTPServer, "internal A2A HTTP server")
	shutdownHTTP(ctx, log, set.facadeServer, "facade server")
	shutdownHTTP(ctx, log, set.internalFacadeServer, "internal facade server")
	shutdownHTTP(ctx, log, set.healthServer, "health server")

	log.Info("shutdown complete")
}

// shutdownHTTP gracefully shuts down a possibly-nil HTTP server, logging any
// error. No-op when srv is nil so optional listeners (internal twin, A2A) don't
// need a guard at each call site.
func shutdownHTTP(ctx context.Context, log logr.Logger, srv *http.Server, name string) {
	if srv == nil {
		return
	}
	if err := srv.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down "+name)
	}
}

// startA2AServer creates and configures the A2A server for dual-protocol mode.
// Returns the A2A server (for shutdown) and the HTTP server (for ListenAndServe).
func startA2AServer(
	cfg *agent.Config,
	log logr.Logger,
	tracingProvider *tracing.Provider,
) (*facadea2a.Server, *http.Server, *http.Server, func()) {
	log.Info("dual-protocol mode: starting A2A alongside WebSocket",
		"a2aPort", cfg.A2APort,
		"taskTTL", cfg.A2ATaskTTL,
		"conversationTTL", cfg.A2AConversationTTL,
	)

	// Build the auth chain for this A2A endpoint. In dual-protocol mode
	// the WebSocket side has already built its chain in
	// buildWebSocketServer; we rebuild here rather than plumb it across
	// because the cost is a single AgentRuntime k8s Get at startup and
	// keeping buildA2AHandler's signature consistent with the standalone
	// runA2AFacade path is worth more than the saved lookup.
	mgmtPlane, mgmtErr := loadMgmtPlaneValidator(log, cfg.AgentName, cfg.WorkspaceName)
	if mgmtErr != nil {
		log.Error(mgmtErr, "mgmt-plane validator load failed")
		os.Exit(1)
	}
	a2aChain, chainErr := buildExternalChain(
		context.Background(), buildK8sClient(), log, cfg.AgentName, cfg.Namespace)
	if chainErr != nil {
		log.Error(chainErr, "external auth chain build failed")
		os.Exit(1)
	}

	// Build card provider
	cardProvider := buildCardProvider(cfg, log)

	// Build task store
	taskStore, storeCleanup := buildTaskStore(cfg, log)

	// Pack path: for A2A, the SDK reads the pack directly
	packPath := cfg.PromptPackPath + "/pack.json"

	a2aSrv := facadea2a.NewServer(facadea2a.ServerConfig{
		PackPath:        packPath,
		PromptName:      "default",
		Port:            cfg.A2APort,
		TaskTTL:         cfg.A2ATaskTTL,
		ConversationTTL: cfg.A2AConversationTTL,
		CardProvider:    cardProvider,
		TaskStore:       taskStore,
		Log:             log,
	})

	// Create A2A metrics (shared by both the external and internal listeners —
	// metrics are per-agent, not per-listener).
	a2aMetrics := facadea2a.NewMetrics(cfg.AgentName, cfg.Namespace)
	inner := a2aSrv.Handler()

	// External listener: data-plane validators + mgmt-plane (Milestone A keeps
	// mgmt on the external chain). buildA2AHandler is shared with standalone
	// mode so both paths get the same auth + metrics + tracing wrapping.
	a2aHTTPServer := newA2AHTTPServer(fmt.Sprintf(":%d", cfg.A2APort),
		inner, a2aMetrics, tracingProvider, a2aChain, log)

	// Internal twin listener: management-plane-only chain on the internal port.
	// Built only when the controller has allocated an internal A2A port.
	var internalA2AHTTPServer *http.Server
	if cfg.InternalA2APort != 0 {
		internalA2AHTTPServer = newA2AHTTPServer(fmt.Sprintf(":%d", cfg.InternalA2APort),
			inner, a2aMetrics, tracingProvider, buildMgmtChain(mgmtPlane), log)
	}

	return a2aSrv, a2aHTTPServer, internalA2AHTTPServer, storeCleanup
}

// newA2AHTTPServer wraps the A2A handler with the given auth chain (plus metrics
// and tracing) and returns an HTTP server bound to addr. Shared by the external
// and internal twin listeners so they differ only in chain + address.
func newA2AHTTPServer(
	addr string,
	inner http.Handler,
	metrics *facadea2a.Metrics,
	tracingProvider *tracing.Provider,
	chain auth.Chain,
	log logr.Logger,
) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      buildA2AHandler(inner, metrics, tracingProvider, chain, log),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}
}

// graceWindowDuration converts an integer number of seconds to a time.Duration.
// Kept as a named function so it is easily testable and the call site in
// buildWebSocketServer stays readable.
func graceWindowDuration(secs int) time.Duration {
	return time.Duration(secs) * time.Second
}
