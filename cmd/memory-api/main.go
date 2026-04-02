/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	eeaudit "github.com/altairalabs/omnia/ee/pkg/audit"
	eemetrics "github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
	"github.com/altairalabs/omnia/internal/memory"
	memoryapi "github.com/altairalabs/omnia/internal/memory/api"
	sessionapi "github.com/altairalabs/omnia/internal/session/api"
	sessionpg "github.com/altairalabs/omnia/internal/session/postgres"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logctx"
	"github.com/altairalabs/omnia/pkg/logging"
)

// auditLoggerAdapter adapts ee/pkg/audit.Logger to memoryapi.MemoryAuditLogger.
// It converts MemoryAuditEntry fields into the session/api.AuditEntry shape,
// placing memory-specific fields (MemoryID, Kind) in Metadata.
type auditLoggerAdapter struct {
	inner *eeaudit.Logger
}

func (a *auditLoggerAdapter) LogEvent(ctx context.Context, entry *memoryapi.MemoryAuditEntry) {
	meta := entry.Metadata
	if meta == nil {
		meta = make(map[string]string)
	}
	if entry.MemoryID != "" {
		meta["memoryId"] = entry.MemoryID
	}
	if entry.Kind != "" {
		meta["kind"] = entry.Kind
	}
	a.inner.LogEvent(ctx, &sessionapi.AuditEntry{
		EventType: entry.EventType,
		Workspace: entry.WorkspaceID,
		IPAddress: entry.IPAddress,
		UserAgent: entry.UserAgent,
		Metadata:  meta,
	})
}

// flags groups all CLI flags for the memory-api binary.
type flags struct {
	apiAddr           string
	healthAddr        string
	metricsAddr       string
	postgresConn      string
	redisAddrs        string
	enterprise        bool
	tracingEnabled    bool
	tracingEndpoint   string
	tracingSample     float64
	tracingInsecure   bool
	embeddingProvider string // openai, gemini, voyageai
	embeddingModel    string // model override
	defaultTTL        string // env: DEFAULT_TTL, e.g. "720h"
	purpose           string // env: MEMORY_PURPOSE, e.g. "support_continuity"
	retentionInterval string // env: RETENTION_INTERVAL, e.g. "1h"
}

func parseFlags() *flags {
	f := &flags{}
	flag.StringVar(&f.apiAddr, "api-addr", ":8080", "API server listen address")
	flag.StringVar(&f.healthAddr, "health-addr", ":8081", "Health probe listen address")
	flag.StringVar(&f.metricsAddr, "metrics-addr", ":9090", "Metrics server listen address")
	flag.StringVar(&f.postgresConn, "postgres-conn", "", "Postgres connection string")
	flag.StringVar(&f.redisAddrs, "redis-addrs", "", "Redis addresses (comma-separated)")
	flag.BoolVar(&f.enterprise, "enterprise", false, "Enable enterprise features (audit logging)")
	flag.BoolVar(&f.tracingEnabled, "tracing-enabled", false, "Enable OpenTelemetry tracing")
	flag.StringVar(&f.tracingEndpoint, "tracing-endpoint", "", "OTel collector endpoint")
	flag.Float64Var(&f.tracingSample, "tracing-sample", 0, "Tracing sample rate (0.0-1.0)")
	flag.BoolVar(&f.tracingInsecure, "tracing-insecure", false, "Use insecure gRPC for tracing")
	flag.StringVar(&f.embeddingProvider, "embedding-provider", "", "Embedding provider (openai, gemini, voyageai)")
	flag.StringVar(&f.embeddingModel, "embedding-model", "", "Embedding model override")
	flag.StringVar(&f.defaultTTL, "default-ttl", "", "Default memory TTL duration (e.g. 720h)")
	flag.StringVar(&f.purpose, "purpose", "", "Default memory purpose tag (e.g. support_continuity)")
	flag.StringVar(&f.retentionInterval, "retention-interval", "", "Interval for retention worker (e.g. 1h)")
	flag.Parse()

	f.applyEnvFallbacks()
	return f
}

// applyEnvFallbacks applies environment variable overrides to flag defaults.
func (f *flags) applyEnvFallbacks() {
	envFallback(&f.postgresConn, "", "POSTGRES_CONN")
	envFallback(&f.redisAddrs, "", "REDIS_ADDRS")
	envFallback(&f.apiAddr, ":8080", "API_ADDR")
	envFallback(&f.healthAddr, ":8081", "HEALTH_ADDR")
	envFallback(&f.metricsAddr, ":9090", "METRICS_ADDR")

	envBoolFallback(&f.enterprise, "ENTERPRISE_ENABLED")
	envBoolFallback(&f.tracingEnabled, "TRACING_ENABLED")
	envBoolFallback(&f.tracingInsecure, "TRACING_INSECURE")
	envFallback(&f.tracingEndpoint, "", "TRACING_ENDPOINT")
	envFallback(&f.embeddingProvider, "", "EMBEDDING_PROVIDER")
	envFallback(&f.embeddingModel, "", "EMBEDDING_MODEL")
	envFallback(&f.defaultTTL, "", "DEFAULT_TTL")
	envFallback(&f.purpose, "", "MEMORY_PURPOSE")
	envFallback(&f.retentionInterval, "", "RETENTION_INTERVAL")
	if v := os.Getenv("TRACING_SAMPLE_RATE"); v != "" && f.tracingSample == 0 {
		if rate, err := strconv.ParseFloat(v, 64); err == nil {
			f.tracingSample = rate
		}
	}
}

// envFallback sets *dst from the environment variable envKey when *dst still
// equals the default value and the environment variable is non-empty.
func envFallback(dst *string, defaultVal, envKey string) {
	if *dst == defaultVal {
		if v := os.Getenv(envKey); v != "" {
			*dst = v
		}
	}
}

// envBoolFallback enables a boolean flag from an environment variable when the
// flag is still false and the env var is "true".
func envBoolFallback(dst *bool, envKey string) {
	if !*dst && os.Getenv(envKey) == "true" {
		*dst = true
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	f := parseFlags()

	// --- Logger ---
	log, syncLog, err := logging.NewLogger()
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer syncLog()

	// --- Validate ---
	if f.postgresConn == "" {
		return fmt.Errorf("--postgres-conn or POSTGRES_CONN is required")
	}

	// --- Signal context ---
	ctx, cancel := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	// --- Postgres pool ---
	pool, err := initPool(ctx, f.postgresConn)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.V(1).Info("postgres pool created",
		"maxConns", envInt32("PG_MAX_CONNS", defaultMaxConns),
		"minConns", envInt32("PG_MIN_CONNS", defaultMinConns),
	)

	// --- Migrations ---
	if err := runMigrations(f.postgresConn, log); err != nil {
		return err
	}
	log.V(1).Info("migrations complete")

	// --- Memory store ---
	store := memory.NewPostgresMemoryStore(pool)

	// --- Service config: TTL + purpose ---
	var defaultTTL time.Duration
	if f.defaultTTL != "" {
		if d, err := time.ParseDuration(f.defaultTTL); err == nil {
			defaultTTL = d
		} else {
			log.Error(err, "invalid DEFAULT_TTL, defaulting to no TTL", "value", f.defaultTTL)
		}
	}
	svcCfg := memoryapi.MemoryServiceConfig{
		DefaultTTL: defaultTTL,
		Purpose:    f.purpose,
	}

	// --- Retention worker ---
	if f.retentionInterval != "" {
		if interval, err := time.ParseDuration(f.retentionInterval); err == nil && interval > 0 {
			worker := memory.NewRetentionWorker(store, interval, log)
			go worker.Run(ctx)
			log.Info("retention worker started", "interval", interval)
		} else if err != nil {
			log.Error(err, "invalid RETENTION_INTERVAL, retention worker disabled", "value", f.retentionInterval)
		}
	}

	// --- Embedding service ---
	var embeddingSvc *memory.EmbeddingService
	if f.embeddingProvider != "" {
		// Provider creation (OpenAI/Gemini/Voyage) will be wired when
		// PromptKit embedding providers are imported.
		log.Info("embedding provider configured", "provider", f.embeddingProvider, "model", f.embeddingModel)
	}

	// --- Tracing ---
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	if f.tracingEnabled {
		tracingCfg := tracing.Config{
			Enabled:     true,
			Endpoint:    f.tracingEndpoint,
			ServiceName: "omnia-memory-api",
			SampleRate:  f.tracingSample,
			Insecure:    f.tracingInsecure,
		}
		tp, tpErr := tracing.NewProvider(ctx, tracingCfg)
		if tpErr != nil {
			log.Error(tpErr, "tracing provider creation failed")
		} else {
			otel.SetTracerProvider(tp.TracerProvider())
			defer func() { _ = tp.Shutdown(ctx) }()
			log.Info("tracing enabled", "endpoint", f.tracingEndpoint, "sampleRate", f.tracingSample)
		}
	}

	// --- Event publisher (optional) ---
	var eventPublisher memoryapi.MemoryEventPublisher
	if f.redisAddrs != "" {
		redisClient := goredis.NewClient(&goredis.Options{Addr: f.redisAddrs})
		eventPublisher = memoryapi.NewRedisMemoryEventPublisher(redisClient, log)
		log.Info("memory event publisher enabled", "redisAddrs", f.redisAddrs)
	}

	// --- Build API mux ---
	apiMux, cleanup := buildAPIMux(store, embeddingSvc, svcCfg, eventPublisher, f.enterprise, pool, log)
	defer cleanup()

	// --- Servers ---
	healthSrv := newHealthServer(f.healthAddr, pool)
	metricsSrv := newMetricsServer(f.metricsAddr)
	apiSrv := &http.Server{
		Addr:         f.apiAddr,
		Handler:      apiMux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	startHTTPServer(log, "health", f.healthAddr, healthSrv)
	startHTTPServer(log, "metrics", f.metricsAddr, metricsSrv)
	startHTTPServer(log, "memory API", f.apiAddr, apiSrv)

	log.Info("memory-api ready",
		"api", f.apiAddr,
		"health", f.healthAddr,
		"metrics", f.metricsAddr,
		"enterprise", f.enterprise,
	)

	// --- Wait for shutdown ---
	<-ctx.Done()
	log.Info("shutting down")

	shutdownServers(log, apiSrv, healthSrv, metricsSrv)
	return nil
}

// buildAPIMux assembles the HTTP handler with all memory-api routes, wrapped
// with rate limiting, privacy (enterprise), metrics, and tracing middleware.
// Returns the handler and a cleanup function.
func buildAPIMux(
	store memory.Store,
	embeddingSvc *memory.EmbeddingService,
	cfg memoryapi.MemoryServiceConfig,
	publisher memoryapi.MemoryEventPublisher,
	enterprise bool,
	pool *pgxpool.Pool,
	log logr.Logger,
) (http.Handler, func()) {
	httpMetrics := memoryapi.NewHTTPMetrics(nil)

	svc := memoryapi.NewMemoryService(store, embeddingSvc, cfg, log)
	if publisher != nil {
		svc.SetEventPublisher(publisher)
	}

	// Enterprise audit logging.
	var auditClose func() error
	if enterprise {
		auditMetrics := eemetrics.NewAuditMetrics()
		auditLogger := eeaudit.NewLogger(pool, log, auditMetrics, eeaudit.LoggerConfig{})
		auditClose = auditLogger.Close
		svc.SetAuditLogger(&auditLoggerAdapter{inner: auditLogger})
		log.Info("memory audit logging enabled")
	}

	handler := memoryapi.NewHandler(svc, log)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// AuditMiddleware always applied — populates request context with IP/UA.
	// The service only emits events when an audit logger is configured.
	apiHandler := memoryapi.AuditMiddleware(mux)

	// Enterprise privacy middleware (opt-out + PII redaction).
	if enterprise {
		apiHandler = wrapPrivacyMiddleware(apiHandler, pool, log)
	}

	// Rate limiting middleware (per-client-IP token bucket).
	rlCfg := sessionapi.RateLimitConfigFromEnv()
	rlMiddleware, rlStop := sessionapi.NewRateLimitMiddleware(rlCfg)
	log.V(1).Info("rate limiter initialized", "rps", rlCfg.RPS, "burst", rlCfg.Burst)

	traced := otelhttp.NewHandler(traceLogMiddleware(apiHandler), "memory-api",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/healthz"
		}),
	)
	cleanup := func() {
		rlStop()
		if auditClose != nil {
			_ = auditClose()
		}
	}
	return rlMiddleware(httpMetrics.MetricsMiddleware(traced)), cleanup
}

// wrapPrivacyMiddleware creates and wires the enterprise privacy middleware.
// When the K8s API is unreachable (e.g., in tests), the middleware is skipped
// and the original handler is returned unchanged.
func wrapPrivacyMiddleware(next http.Handler, pool *pgxpool.Pool, log logr.Logger) http.Handler {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Info("memory privacy middleware skipped", "reason", "no in-cluster kubeconfig")
		return next
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(eev1alpha1.AddToScheme(scheme))
	k8sClient, err := client.New(kubeConfig, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "memory privacy middleware skipped", "reason", "k8s client creation failed")
		return next
	}

	watcher := privacy.NewPolicyWatcher(k8sClient, log)

	// Start the watcher asynchronously; it syncs the policy cache in the background.
	go func() {
		if startErr := watcher.Start(context.Background()); startErr != nil {
			log.Error(startErr, "memory policy watcher start failed")
		}
	}()

	prefStore := privacy.NewPreferencesStore(pool)
	redactor := redaction.NewRedactor()

	checkOptOut := memoryapi.OptOutChecker(func(ctx context.Context, userID, workspace string) bool {
		return privacy.ShouldRemember(ctx, prefStore, userID, workspace, "")
	})

	contentRedactor := memoryapi.ContentRedactor(func(ctx context.Context, workspace, content string) (string, error) {
		policy := watcher.GetEffectivePolicy(workspace, "")
		if policy == nil || policy.Recording.PII == nil || !policy.Recording.PII.Redact {
			return content, nil
		}
		redacted, _, err := redactor.Redact(ctx, content, policy.Recording.PII)
		return redacted, err
	})

	mw := memoryapi.NewMemoryPrivacyMiddleware(checkOptOut, contentRedactor, log)
	log.Info("memory privacy middleware enabled")
	return mw.Wrap(next)
}

// traceLogMiddleware injects the OTel trace ID into the logging context.
func traceLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			ctx = logctx.WithTraceID(ctx, sc.TraceID().String())
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// startHTTPServer starts an HTTP server in a background goroutine.
func startHTTPServer(log logr.Logger, name, addr string, srv *http.Server) {
	go func() {
		log.Info("starting server", "server", name, "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "server error", "server", name)
		}
	}()
}

// shutdownServers gracefully stops all HTTP servers with a 30-second timeout.
func shutdownServers(log logr.Logger, servers ...*http.Server) {
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()

	for _, srv := range servers {
		if srv == nil {
			continue
		}
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Error(err, "server shutdown error", "addr", srv.Addr)
		}
	}
}

// Pool configuration defaults.
const (
	defaultMaxConns        = 50
	defaultMinConns        = 5
	defaultMaxConnLifetime = time.Hour
	defaultMaxConnIdleTime = 30 * time.Minute
)

// initPool creates and returns a pgxpool connection pool with configured limits.
func initPool(ctx context.Context, connStr string) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres connection string: %w", err)
	}

	poolCfg.MaxConns = envInt32("PG_MAX_CONNS", defaultMaxConns)
	poolCfg.MinConns = envInt32("PG_MIN_CONNS", defaultMinConns)
	poolCfg.MaxConnLifetime = envDuration("PG_MAX_CONN_LIFETIME", defaultMaxConnLifetime)
	poolCfg.MaxConnIdleTime = envDuration("PG_MAX_CONN_IDLE_TIME", defaultMaxConnIdleTime)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating postgres pool: %w", err)
	}
	return pool, nil
}

// envInt32 reads an environment variable as int32, returning def on missing/invalid values.
func envInt32(key string, def int32) int32 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return def
	}
	return int32(n)
}

// envDuration reads an environment variable as a time.Duration, returning def on missing/invalid.
func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// runMigrations applies database schema migrations.
func runMigrations(connStr string, log logr.Logger) error {
	migrator, err := sessionpg.NewMigrator(connStr, log)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	if err := migrator.Up(); err != nil {
		_ = migrator.Close()
		return fmt.Errorf("running migrations: %w", err)
	}
	_ = migrator.Close()
	return nil
}

// newMetricsServer creates a dedicated HTTP server for Prometheus metrics.
func newMetricsServer(addr string) *http.Server {
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", promhttp.Handler())
	return &http.Server{Addr: addr, Handler: metricsMux}
}

// newHealthServer creates an HTTP server for health and readiness probes.
func newHealthServer(addr string, pool *pgxpool.Pool) *http.Server {
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	healthMux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("postgres unavailable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return &http.Server{Addr: addr, Handler: healthMux}
}
