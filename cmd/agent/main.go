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
	"time"

	"github.com/go-logr/zapr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/session"
)

const (
	shutdownTimeout = 30 * time.Second
	readTimeout     = 10 * time.Second
	writeTimeout    = 10 * time.Second
	idleTimeout     = 120 * time.Second
)

func main() {
	// Initialize logger
	zapLog, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = zapLog.Sync() }()
	log := zapr.NewLogger(zapLog)

	// Load configuration from environment
	cfg, err := agent.LoadFromEnv()
	if err != nil {
		log.Error(err, "failed to load configuration")
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Error(err, "invalid configuration")
		os.Exit(1)
	}

	log.Info("starting agent",
		"agent", cfg.AgentName,
		"namespace", cfg.Namespace,
		"facade", cfg.FacadeType,
		"port", cfg.FacadePort,
		"handler", cfg.HandlerMode,
	)

	// Initialize session store
	store, err := initSessionStore(cfg, log)
	if err != nil {
		log.Error(err, "failed to initialize session store")
		os.Exit(1)
	}
	defer closeStore(store, log)

	// Create message handler based on mode
	handler, handlerCleanup := createHandler(cfg, log)
	if handlerCleanup != nil {
		defer handlerCleanup()
	}

	// Create Prometheus metrics
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)

	// Create WebSocket server with metrics
	wsConfig := facade.DefaultServerConfig()
	wsConfig.SessionTTL = cfg.SessionTTL
	wsServer := facade.NewServer(wsConfig, store, handler, log, facade.WithMetrics(metrics))

	// Create HTTP mux for routing
	mux := http.NewServeMux()
	mux.Handle("/ws", wsServer)
	mux.Handle("/metrics", promhttp.Handler())

	// Create facade HTTP server
	facadeServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.FacadePort),
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
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

func initSessionStore(cfg *agent.Config, log interface{ Info(string, ...any) }) (session.Store, error) {
	switch cfg.SessionType {
	case agent.SessionTypeMemory:
		log.Info("using in-memory session store")
		return session.NewMemoryStore(), nil
	case agent.SessionTypeRedis:
		log.Info("using Redis session store", "url", redactURL(cfg.SessionStoreURL))
		redisCfg, err := session.ParseRedisURL(cfg.SessionStoreURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
		}
		return session.NewRedisStore(redisCfg)
	default:
		return nil, fmt.Errorf("unsupported session type: %s", cfg.SessionType)
	}
}

func closeStore(store session.Store, log interface{ Error(error, string, ...any) }) {
	if closer, ok := store.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			log.Error(err, "error closing session store")
		}
	}
}

func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func readyzHandler(store session.Store, handler facade.MessageHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Check session store connectivity
		_, err := store.GetSession(ctx, "health-check-probe")
		if err != nil && err != session.ErrSessionNotFound {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w, "session store unavailable: %v", err)
			return
		}

		// Check runtime health if using runtime handler
		if runtimeHandler, ok := handler.(*agent.RuntimeHandler); ok {
			resp, err := runtimeHandler.Client().Health(ctx)
			if err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprintf(w, "runtime unavailable: %v", err)
				return
			}
			if !resp.Healthy {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprintf(w, "runtime unhealthy: %s", resp.Status)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// redactURL redacts sensitive parts of URLs for logging.
func redactURL(url string) string {
	// Simple redaction - in production, use a proper URL parser
	if len(url) > 20 {
		return url[:10] + "..." + url[len(url)-5:]
	}
	return "***"
}

// createHandler creates the appropriate message handler based on configuration.
// Returns the handler and an optional cleanup function.
func createHandler(cfg *agent.Config, log interface {
	Info(string, ...any)
	Error(error, string, ...any)
}) (facade.MessageHandler, func()) {
	switch cfg.HandlerMode {
	case agent.HandlerModeEcho:
		log.Info("using echo handler mode")
		return agent.NewEchoHandler(), nil
	case agent.HandlerModeDemo:
		log.Info("using demo handler mode with LLM metrics")
		return agent.NewDemoHandlerWithMetrics(cfg.AgentName, cfg.Namespace), nil
	case agent.HandlerModeRuntime:
		log.Info("using runtime handler mode", "address", cfg.RuntimeAddress)

		// Retry connection with exponential backoff (runtime container may still be starting)
		var client *facade.RuntimeClient
		var err error
		maxRetries := 10
		backoff := 500 * time.Millisecond

		for i := 0; i < maxRetries; i++ {
			client, err = facade.NewRuntimeClient(facade.RuntimeClientConfig{
				Address:     cfg.RuntimeAddress,
				DialTimeout: 5 * time.Second,
			})
			if err == nil {
				log.Info("connected to runtime", "address", cfg.RuntimeAddress, "attempt", i+1)
				break
			}

			log.Info("waiting for runtime to be ready", "address", cfg.RuntimeAddress, "attempt", i+1, "error", err.Error())
			time.Sleep(backoff)
			backoff = min(backoff*2, 5*time.Second) // Cap at 5 seconds
		}

		if err != nil {
			log.Error(err, "failed to connect to runtime after retries, falling back to nil handler")
			return nil, nil
		}

		handler := agent.NewRuntimeHandler(client)
		cleanup := func() {
			if err := client.Close(); err != nil {
				log.Error(err, "error closing runtime client")
			}
		}
		return handler, cleanup
	default:
		log.Info("unknown handler mode, using nil handler", "mode", cfg.HandlerMode)
		return nil, nil
	}
}
