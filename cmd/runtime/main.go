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
	"syscall"
	"time"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/internal/runtime/pack"
	"github.com/altairalabs/omnia/internal/runtime/provider"
	"github.com/altairalabs/omnia/internal/session"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

func main() {
	// Create logger
	zapLog, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = zapLog.Sync() }()
	log := zapr.NewLogger(zapLog)

	// Load configuration
	cfg, err := runtime.LoadConfig()
	if err != nil {
		log.Error(err, "failed to load configuration")
		os.Exit(1)
	}

	log.Info("starting runtime",
		"agent", cfg.AgentName,
		"namespace", cfg.Namespace,
		"grpcPort", cfg.GRPCPort,
		"healthPort", cfg.HealthPort)

	// Create session store
	var sessionStore session.Store
	switch cfg.SessionType {
	case runtime.SessionTypeMemory:
		sessionStore = session.NewMemoryStore()
		log.Info("using in-memory session store")
	case runtime.SessionTypeRedis:
		redisCfg, err := session.ParseRedisURL(cfg.SessionURL)
		if err != nil {
			log.Error(err, "failed to parse Redis URL")
			os.Exit(1)
		}
		sessionStore, err = session.NewRedisStore(redisCfg)
		if err != nil {
			log.Error(err, "failed to create Redis session store")
			os.Exit(1)
		}
		log.Info("using Redis session store", "url", cfg.SessionURL)
	}
	defer func() { _ = sessionStore.Close() }()

	// Create session adapter
	sessionAdapter := runtime.NewSessionAdapter(sessionStore, cfg.SessionTTL)

	// Create LLM provider
	var llmProvider runtime.Provider
	switch cfg.ProviderType {
	case runtime.ProviderTypeOpenAI:
		llmProvider = provider.NewOpenAIProvider(cfg.ProviderAPIKey)
		log.Info("using OpenAI provider")
	case runtime.ProviderTypeAnthropic:
		log.Error(nil, "Anthropic provider not yet implemented")
		os.Exit(1)
	}

	// Create pack loader
	packLoader := pack.NewFileLoader(cfg.PromptPackPath)
	if packLoader.Exists() {
		log.Info("loaded PromptPack", "path", cfg.PromptPackPath)
	} else {
		log.Info("no PromptPack found", "path", cfg.PromptPackPath)
	}

	// Create runtime server
	runtimeServer := runtime.NewServer(
		runtime.WithLogger(log),
		runtime.WithProvider(llmProvider),
		runtime.WithSessionStore(sessionAdapter),
		runtime.WithPackLoader(packLoader),
		runtime.WithAgentInfo(cfg.AgentName, cfg.Namespace),
	)

	// Create gRPC server
	grpcServer := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(grpcServer, runtimeServer)

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Start gRPC server
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		log.Error(err, "failed to listen on gRPC port", "port", cfg.GRPCPort)
		os.Exit(1)
	}

	go func() {
		log.Info("gRPC server starting", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Error(err, "gRPC server error")
		}
	}()

	// Create HTTP health server
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Check if provider and session store are working
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HealthPort),
		Handler:           healthMux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("health server starting", "port", cfg.HealthPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "health server error")
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("shutting down...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop health server
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error(err, "failed to shutdown health server")
	}

	// Stop gRPC server
	grpcServer.GracefulStop()

	log.Info("shutdown complete")
}
