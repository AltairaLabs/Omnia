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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip" // Register gzip decompressor for facade→runtime channel
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/stats"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	_ "github.com/AltairaLabs/PromptKit/runtime/evals/handlers" // Register default eval type handlers
	"github.com/AltairaLabs/PromptKit/runtime/logger"
	sdkmetrics "github.com/AltairaLabs/PromptKit/runtime/metrics"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"

	memoryhttpclient "github.com/altairalabs/omnia/internal/memory/httpclient"
	pkruntime "github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/internal/runtime/tools"
	"github.com/altairalabs/omnia/internal/session/httpclient"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/k8s"
	"github.com/altairalabs/omnia/pkg/logging"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

func main() {
	// Initialize global OpenTelemetry text map propagator for trace context propagation.
	// This must be set before any gRPC operations to ensure trace context flows through gRPC calls.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Initialize logger (respects LOG_LEVEL env var).
	// Create the Zap logger directly so we can derive both logr (for Omnia)
	// and slog (for PromptKit SDK) from the same Zap core without a lossy bridge.
	zapLog, err := logging.NewZapLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = zapLog.Sync() }()
	log := zapr.NewLogger(zapLog)
	sdkLogger := logging.SlogFromZap(zapLog)
	logger.SetLogger(sdkLogger) // Set immediately so all PromptKit logging uses the Zap backend

	// Load configuration — prefer CRD reading, fall back to env vars
	cfg, err := pkruntime.LoadConfigWithContext(context.Background())
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
		"tracingEnabled", cfg.TracingEnabled,
		"evalEnabled", cfg.EvalEnabled,
		"sessionAPIURL", cfg.SessionAPIURL,
		"memoryEnabled", cfg.MemoryEnabled)

	// Create PromptKit Collector for all pipeline + eval metrics.
	// This replaces the old hand-rolled LLMMetrics, RuntimeMetrics, and EvalMetrics.
	collectorRegistry := prometheus.NewRegistry()
	collector := sdkmetrics.NewCollector(sdkmetrics.CollectorOpts{
		Registerer: collectorRegistry,
		Namespace:  "omnia",
		ConstLabels: prometheus.Labels{
			"agent":           cfg.AgentName,
			"namespace":       cfg.Namespace,
			"promptpack_name": cfg.PromptPackName,
		},
	})

	// Load eval definitions and create collector if evals are enabled
	var evalDefs []evals.EvalDef
	if cfg.EvalEnabled {
		// Load ALL eval definitions (pack-level + prompt-level) so that
		// per-turn evals defined inside prompts are also executed.
		defs, err := pkruntime.LoadAllEvalDefs(cfg.PromptPackPath)
		if err != nil {
			log.Error(err, "failed to load eval definitions from pack, continuing without evals")
		} else {
			evalDefs = defs
			log.Info("evals enabled", "evalCount", len(evalDefs))
		}

		// Validate all eval types have registered handlers.
		// This surfaces misconfigured eval types at startup rather than silently
		// failing when conversations run.
		if missing := pkruntime.ValidateEvalDefs(evalDefs); len(missing) > 0 {
			log.Error(fmt.Errorf("unregistered eval types: %v", missing),
				"some eval types in the pack have no registered handler and will fail at runtime",
				"missingTypes", missing, "evalCount", len(evalDefs))
		}
	}

	// Validate pack content and report to AgentRuntime status.
	// This runs regardless of whether evals are enabled — it validates
	// the pack file itself and any eval definitions found.
	packValidationWarnings := validatePackContent(cfg.PromptPackPath, evalDefs, log)
	if cfg.AgentName != "" && cfg.Namespace != "" {
		patchCtx, patchCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer patchCancel()
		k8sClient, k8sErr := k8s.NewClient()
		if k8sErr != nil {
			log.Error(k8sErr, "failed to create k8s client for pack validation reporting")
		} else if patchErr := reportPackValidation(patchCtx, k8sClient,
			cfg.AgentName, cfg.Namespace, packValidationWarnings); patchErr != nil {
			log.Error(patchErr, "failed to patch PackContentValid condition")
		}
	}

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
		if err := redisotel.InstrumentTracing(client); err != nil {
			log.Error(err, "failed to instrument redis tracing")
		}

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
			tracingProvider = tracingProvider.WithLogger(log)
			// Set as global provider so PromptKit SDK can use it.
			// This is safe because runtime is isolated in its own container.
			otel.SetTracerProvider(tracingProvider.TracerProvider())
			log.Info("tracing initialized",
				"endpoint", cfg.TracingEndpoint,
				"sampleRate", cfg.TracingSampleRate)
		}
	}

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

	// Create runtime server. The config-derived options are factored out into
	// configDerivedServerOpts so wiring tests can assert that every config
	// field that should reach the runtime server actually does (see #728
	// item 2 and the wiring test in this package).
	serverOpts := []pkruntime.ServerOption{
		pkruntime.WithLogger(log),
		pkruntime.WithSlogLogger(sdkLogger),
		pkruntime.WithStateStore(store),
	}
	serverOpts = append(serverOpts, configDerivedServerOpts(cfg)...)
	if tracingProvider != nil {
		serverOpts = append(serverOpts, pkruntime.WithTracingProvider(tracingProvider))
	}
	// Wire skills if the operator emitted a manifest. Empty path or missing
	// file is a no-op — skills are optional.
	if path := os.Getenv("OMNIA_PROMPTPACK_MANIFEST_PATH"); path != "" {
		serverOpts = append(serverOpts, pkruntime.WithSkillManifest(path))
	}
	if cfg.PromptPackVersion != "" {
		serverOpts = append(serverOpts, pkruntime.WithPromptPackVersion(cfg.PromptPackVersion))
	}
	// Wire session recording via session-api when URL is configured
	if cfg.SessionAPIURL != "" {
		sessionStore := httpclient.NewStore(cfg.SessionAPIURL, log)
		serverOpts = append(serverOpts, pkruntime.WithSessionStore(sessionStore))
		log.Info("session recording enabled", "sessionAPIURL", cfg.SessionAPIURL)
	}

	// Wire memory store for cross-session memory via memory-api HTTP
	if cfg.MemoryEnabled && cfg.MemoryAPIURL != "" {
		memStore := memoryhttpclient.NewStore(cfg.MemoryAPIURL, log)
		serverOpts = append(serverOpts, pkruntime.WithMemoryStore(memStore))
		if cfg.WorkspaceUID != "" {
			serverOpts = append(serverOpts, pkruntime.WithWorkspaceUID(cfg.WorkspaceUID))
		}
		log.Info("memory store wired", "memoryAPIURL", cfg.MemoryAPIURL, "workspaceUID", cfg.WorkspaceUID)
	} else if cfg.MemoryEnabled {
		log.Info("memory enabled but no memory-api URL configured, skipping")
	}

	// Always wire the Collector so pipeline metrics (provider, tool, validation)
	// are recorded for every conversation, not just eval runs.
	serverOpts = append(serverOpts, pkruntime.WithEvalCollector(collector))
	if len(evalDefs) > 0 {
		serverOpts = append(serverOpts, pkruntime.WithEvalDefs(evalDefs))
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

			// Enrich tool metadata from the ToolRegistry CRD (best-effort)
			if cfg.ToolRegistryName != "" {
				enrichToolRegistryMeta(cfg, runtimeServer, log)
			}
		}
	} else {
		log.V(1).Info("tools disabled (no config path specified)")
	}

	// Create gRPC server with policy interceptors and optional tracing. Factored
	// out so wiring tests can assert the real server has the interceptors
	// installed.
	grpcServer := buildGRPCServer(tracingProvider)
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
	// Merge default Prometheus metrics with the Collector's isolated registry.
	gatherers := prometheus.Gatherers{prometheus.DefaultGatherer, collectorRegistry}
	healthMux.Handle("/metrics", promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{}))

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

	// Stop gRPC server with deadline — fall back to hard stop if streams don't finish.
	grpcDone := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcDone)
	}()
	select {
	case <-grpcDone:
		// Graceful shutdown completed
	case <-time.After(10 * time.Second):
		log.Info("gRPC graceful stop timed out, forcing stop")
		grpcServer.Stop()
	}

	log.Info("shutdown complete")
}

// enrichToolRegistryMeta reads the ToolRegistry CRD and sets handler metadata on the tool manager.
// This is best-effort — if the CRD read fails, tools still work without provenance metadata.
func enrichToolRegistryMeta(cfg *pkruntime.Config, server *pkruntime.Server, log logr.Logger) {
	trCtx, trCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer trCancel()

	k8sClient, err := k8s.NewClient()
	if err != nil {
		log.Error(err, "failed to create k8s client for ToolRegistry metadata")
		return
	}

	tr, err := k8s.GetToolRegistry(trCtx, k8sClient, cfg.ToolRegistryName, cfg.ToolRegistryNamespace)
	if err != nil {
		log.Error(err, "failed to read ToolRegistry, continuing without registry metadata",
			"name", cfg.ToolRegistryName, "namespace", cfg.ToolRegistryNamespace)
		return
	}

	// Load the tools config to get handler entries for metadata mapping
	toolsCfg, err := tools.LoadConfig(cfg.ToolsConfigPath)
	if err != nil {
		log.Error(err, "failed to reload tools config for metadata mapping")
		return
	}

	server.SetToolRegistryInfo(tr.Name, tr.Namespace, toolsCfg.Handlers)
	log.Info("tool registry metadata enriched",
		"registryName", tr.Name, "registryNamespace", tr.Namespace)
}

// isNotHealthCheck filters out gRPC health check RPCs from tracing.
func isNotHealthCheck(info *stats.RPCTagInfo) bool {
	return info.FullMethodName != "/omnia.runtime.v1.RuntimeService/Health"
}

// configDerivedServerOpts returns the pkruntime.ServerOption slice derived
// solely from cfg fields (no logger, store, collector, etc.). Factored out of
// run so wiring tests can assert that cfg fields with real production impact
// — especially MediaBasePath — actually reach the runtime server. If you add
// a new cfg.Xxx field that the runtime needs, add the corresponding
// pkruntime.WithXxx here, not ad hoc in run. See #728.
func configDerivedServerOpts(cfg *pkruntime.Config) []pkruntime.ServerOption {
	return []pkruntime.ServerOption{
		pkruntime.WithPackPath(cfg.PromptPackPath),
		pkruntime.WithPromptName(cfg.PromptName),
		pkruntime.WithAgentIdentity(cfg.AgentName, cfg.Namespace),
		pkruntime.WithPromptPackName(cfg.PromptPackName),
		pkruntime.WithModel(cfg.Model),
		pkruntime.WithMockProvider(cfg.MockProvider),
		pkruntime.WithMockConfigPath(cfg.MockConfigPath),
		pkruntime.WithToolsConfig(cfg.ToolsConfigPath),
		pkruntime.WithProviderInfo(cfg.ProviderType, cfg.Model),
		pkruntime.WithBaseURL(cfg.BaseURL),
		pkruntime.WithHeaders(cfg.Headers),
		pkruntime.WithPlatform(pkruntime.PlatformConfig{
			Type:     cfg.PlatformType,
			Region:   cfg.PlatformRegion,
			Project:  cfg.PlatformProject,
			Endpoint: cfg.PlatformEndpoint,
		}),
		pkruntime.WithAuth(pkruntime.AuthConfig{
			Type:                       cfg.AuthType,
			RoleArn:                    cfg.AuthRoleArn,
			ServiceAccountEmail:        cfg.AuthServiceAccountEmail,
			CredentialsSecretName:      cfg.AuthCredentialsSecretName,
			CredentialsSecretKey:       cfg.AuthCredentialsSecretKey,
			CredentialsSecretNamespace: cfg.Namespace,
		}),
		pkruntime.WithProviderRequestTimeout(cfg.ProviderRequestTimeout),
		pkruntime.WithProviderStreamIdleTimeout(cfg.ProviderStreamIdleTimeout),
		pkruntime.WithPricing(cfg.InputCostPer1K, cfg.OutputCostPer1K),
		pkruntime.WithContextWindow(cfg.ContextWindow),
		pkruntime.WithTruncationStrategy(cfg.TruncationStrategy),
		pkruntime.WithMediaBasePath(cfg.MediaBasePath),
	}
}

// buildGRPCServer constructs the runtime gRPC server with the policy
// interceptors and, optionally, the OpenTelemetry stats handler. It is
// factored out of run so wiring tests can assert that the interceptors are
// installed on the real server. See issue #714.
func buildGRPCServer(tracingProvider *tracing.Provider) *grpc.Server {
	const maxMsgSize = 16 * 1024 * 1024 // 16MB to support base64-encoded images
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
		grpc.ChainUnaryInterceptor(pkruntime.PolicyUnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(pkruntime.PolicyStreamServerInterceptor()),
	}
	if tracingProvider != nil {
		opts = append(opts, grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithTracerProvider(tracingProvider.TracerProvider()),
			otelgrpc.WithFilter(isNotHealthCheck),
		)))
	}
	return grpc.NewServer(opts...)
}
