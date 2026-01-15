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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	pkruntime "github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/internal/runtime/tracing"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

func main() {
	// Create logger with configurable log level
	var zapLog *zap.Logger
	var err error

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "debug" || logLevel == "trace" {
		cfg := zap.NewDevelopmentConfig()
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		zapLog, err = cfg.Build()
	} else {
		zapLog, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = zapLog.Sync() }()
	log := zapr.NewLogger(zapLog)

	// Load configuration
	cfg, err := pkruntime.LoadConfig()
	if err != nil {
		log.Error(err, "failed to load configuration")
		os.Exit(1)
	}

	log.Info("starting runtime",
		"agent", cfg.AgentName,
		"namespace", cfg.Namespace,
		"grpcPort", cfg.GRPCPort,
		"healthPort", cfg.HealthPort,
		"packPath", cfg.PromptPackPath,
		"promptName", cfg.PromptName,
		"providerType", cfg.ProviderType,
		"model", cfg.Model,
		"baseURL", cfg.BaseURL,
		"mockProvider", cfg.MockProvider,
		"toolsConfigPath", cfg.ToolsConfigPath,
		"tracingEnabled", cfg.TracingEnabled)

	// Create state store for conversation persistence
	var store statestore.Store
	switch cfg.SessionType {
	case pkruntime.SessionTypeMemory:
		store = statestore.NewMemoryStore()
		log.Info("using in-memory state store")
	case pkruntime.SessionTypeRedis:
		// Parse Redis URL
		opts, err := redis.ParseURL(cfg.SessionURL)
		if err != nil {
			log.Error(err, "failed to parse Redis URL")
			os.Exit(1)
		}
		client := redis.NewClient(opts)

		// Test connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := client.Ping(ctx).Err(); err != nil {
			cancel()
			log.Error(err, "failed to connect to Redis")
			os.Exit(1)
		}
		cancel()

		store = statestore.NewRedisStore(client)
		log.Info("using Redis state store", "url", cfg.SessionURL)
	}

	// Initialize tracing if enabled
	var tracingProvider *tracing.Provider
	if cfg.TracingEnabled {
		tracingCfg := tracing.Config{
			Enabled:        true,
			Endpoint:       cfg.TracingEndpoint,
			ServiceName:    fmt.Sprintf("omnia-runtime-%s", cfg.AgentName),
			ServiceVersion: "1.0.0",
			Environment:    cfg.Namespace,
			SampleRate:     cfg.TracingSampleRate,
			Insecure:       cfg.TracingInsecure,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		tracingProvider, err = tracing.NewProvider(ctx, tracingCfg)
		cancel()
		if err != nil {
			log.Error(err, "failed to initialize tracing")
			// Continue without tracing - it's optional
		} else {
			log.Info("tracing initialized",
				"endpoint", cfg.TracingEndpoint,
				"sampleRate", cfg.TracingSampleRate)
		}
	}

	// Create Prometheus metrics
	metrics := pkruntime.NewMetrics(pkruntime.MetricsConfig{
		AgentName:            cfg.AgentName,
		Namespace:            cfg.Namespace,
		PromptPackName:       cfg.PromptPackName,
		PromptPackNamespace:  cfg.PromptPackNamespace,
		ProviderRefName:      cfg.ProviderRefName,
		ProviderRefNamespace: cfg.ProviderRefNamespace,
	})
	runtimeMetrics := pkruntime.NewRuntimeMetrics(cfg.AgentName, cfg.Namespace)

	// Debug: Log metric creation and create a test gauge
	log.Info("prometheus metrics created",
		"metricsNil", metrics == nil,
		"runtimeMetricsNil", runtimeMetrics == nil)

	// Test gauge to verify Prometheus registration is working
	testGauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: "omnia_runtime_info",
		Help: "Runtime information gauge (always 1)",
		ConstLabels: prometheus.Labels{
			"agent":     cfg.AgentName,
			"namespace": cfg.Namespace,
		},
	})
	testGauge.Set(1)

	// Create runtime server
	serverOpts := []pkruntime.ServerOption{
		pkruntime.WithLogger(log),
		pkruntime.WithPackPath(cfg.PromptPackPath),
		pkruntime.WithPromptName(cfg.PromptName),
		pkruntime.WithStateStore(store),
		pkruntime.WithModel(cfg.Model),
		pkruntime.WithMockProvider(cfg.MockProvider),
		pkruntime.WithMockConfigPath(cfg.MockConfigPath),
		pkruntime.WithToolsConfig(cfg.ToolsConfigPath),
		pkruntime.WithMetrics(metrics),
		pkruntime.WithRuntimeMetrics(runtimeMetrics),
		pkruntime.WithProviderInfo(cfg.ProviderType, cfg.Model),
		pkruntime.WithBaseURL(cfg.BaseURL),
		pkruntime.WithContextWindow(cfg.ContextWindow),
		pkruntime.WithTruncationStrategy(cfg.TruncationStrategy),
	}
	if tracingProvider != nil {
		serverOpts = append(serverOpts, pkruntime.WithTracingProvider(tracingProvider))
	}
	runtimeServer := pkruntime.NewServer(serverOpts...)
	defer func() { _ = runtimeServer.Close() }()

	// Initialize tools from config (optional - no tools config means tools are disabled)
	if cfg.ToolsConfigPath != "" {
		initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := runtimeServer.InitializeTools(initCtx); err != nil {
			initCancel()
			log.Error(err, "failed to initialize tools", "configPath", cfg.ToolsConfigPath)
			// Continue without tools - they're optional
		} else {
			initCancel()
			log.Info("tools initialized", "configPath", cfg.ToolsConfigPath)
		}
	} else {
		log.V(1).Info("tools disabled (no config path specified)")
	}

	// Create gRPC server with increased message size for multimodal content
	const maxMsgSize = 16 * 1024 * 1024 // 16MB to support base64-encoded images
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
	)
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

	// Create HTTP health server with metrics
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	healthMux.Handle("/metrics", promhttp.Handler())

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
