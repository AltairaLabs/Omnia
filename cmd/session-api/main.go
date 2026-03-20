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
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	gcsstorage "cloud.google.com/go/storage"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/audit"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
	"github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/internal/session/otlp"
	sessionpg "github.com/altairalabs/omnia/internal/session/postgres"
	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/altairalabs/omnia/internal/session/providers/cold"
	pgprovider "github.com/altairalabs/omnia/internal/session/providers/postgres"
	"github.com/altairalabs/omnia/internal/session/providers/redis"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logging"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// redisClientProvider is implemented by providers that expose the underlying Redis client.
type redisClientProvider interface {
	RedisClient() goredis.UniversalClient
}

// flags groups all CLI flags for the session-api binary.
type flags struct {
	apiAddr         string
	healthAddr      string
	metricsAddr     string
	postgresConn    string
	redisAddrs      string
	coldBackend     string
	coldBucket      string
	coldRegion      string
	coldEndpoint    string
	enterprise      bool
	otlpEnabled     bool
	otlpGRPCAddr    string
	otlpHTTPAddr    string
	tracingEnabled  bool
	tracingEndpoint string
	tracingSample   float64
	tracingInsecure bool
}

func parseFlags() *flags {
	f := &flags{}
	flag.StringVar(&f.apiAddr, "api-addr", ":8080", "API server listen address")
	flag.StringVar(&f.healthAddr, "health-addr", ":8081", "Health probe listen address")
	flag.StringVar(&f.metricsAddr, "metrics-addr", ":9090", "Metrics server listen address")
	flag.StringVar(&f.postgresConn, "postgres-conn", "", "Postgres connection string")
	flag.StringVar(&f.redisAddrs, "redis-addrs", "", "Redis addresses (comma-separated)")
	flag.StringVar(&f.coldBackend, "cold-backend", "", "Cold archive backend (s3, gcs, azure)")
	flag.StringVar(&f.coldBucket, "cold-bucket", "", "Cold archive bucket name")
	flag.StringVar(&f.coldRegion, "cold-region", "", "Cold archive region (S3)")
	flag.StringVar(&f.coldEndpoint, "cold-endpoint", "", "Cold archive endpoint (S3)")
	flag.BoolVar(&f.enterprise, "enterprise", false, "Enable enterprise features (audit)")
	flag.BoolVar(&f.otlpEnabled, "otlp-enabled", false, "Enable OTLP ingestion endpoint")
	flag.StringVar(&f.otlpGRPCAddr, "otlp-grpc-addr", ":4317", "OTLP gRPC listen address")
	flag.StringVar(&f.otlpHTTPAddr, "otlp-http-addr", ":4318", "OTLP HTTP listen address")
	flag.Parse()

	f.applyEnvFallbacks()
	return f
}

// applyEnvFallbacks applies environment variable overrides to flag defaults.
func (f *flags) applyEnvFallbacks() {
	envFallback(&f.postgresConn, "", "POSTGRES_CONN")
	envFallback(&f.redisAddrs, "", "REDIS_ADDRS")
	envFallback(&f.coldBackend, "", "COLD_BACKEND")
	envFallback(&f.coldBucket, "", "COLD_BUCKET")
	envFallback(&f.coldRegion, "", "COLD_REGION")
	envFallback(&f.coldEndpoint, "", "COLD_ENDPOINT")
	envFallback(&f.apiAddr, ":8080", "API_ADDR")
	envFallback(&f.healthAddr, ":8081", "HEALTH_ADDR")
	envFallback(&f.metricsAddr, ":9090", "METRICS_ADDR")
	envFallback(&f.otlpGRPCAddr, ":4317", "OTLP_GRPC_ADDR")
	envFallback(&f.otlpHTTPAddr, ":4318", "OTLP_HTTP_ADDR")

	envBoolFallback(&f.enterprise, "ENTERPRISE_ENABLED")
	envBoolFallback(&f.otlpEnabled, "OTLP_ENABLED")

	envBoolFallback(&f.tracingEnabled, "TRACING_ENABLED")
	envBoolFallback(&f.tracingInsecure, "TRACING_INSECURE")
	envFallback(&f.tracingEndpoint, "", "TRACING_ENDPOINT")
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

	// --- Postgres pool (shared) ---
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

	// --- Providers ---
	registry, providerCleanup, err := initProviders(ctx, f, pool, log)
	if err != nil {
		return err
	}
	defer providerCleanup()

	// --- Tracing ---
	// Set propagator so incoming trace context (e.g. from facade httpclient)
	// is extracted and spans become children of the caller's trace.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	if f.tracingEnabled {
		tracingCfg := tracing.Config{
			Enabled:     true,
			Endpoint:    f.tracingEndpoint,
			ServiceName: "omnia-session-api",
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

	// --- Build API mux ---
	apiMux, sessionService, auditCleanup := buildAPIMux(pool, registry, f, log)
	defer auditCleanup()

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
	startHTTPServer(log, "session API", f.apiAddr, apiSrv)

	// --- OTLP servers (optional) ---
	var grpcSrv *grpc.Server
	var otlpHTTPSrv *http.Server
	if f.otlpEnabled {
		grpcSrv, otlpHTTPSrv = startOTLPServers(f, sessionService, log)
	}

	log.Info("session-api ready",
		"api", f.apiAddr,
		"health", f.healthAddr,
		"metrics", f.metricsAddr,
		"enterprise", f.enterprise,
		"otlp", f.otlpEnabled,
	)

	// --- Wait for shutdown ---
	<-ctx.Done()
	log.Info("shutting down")

	shutdownServers(log, apiSrv, healthSrv, metricsSrv, grpcSrv, otlpHTTPSrv)
	return nil
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

// shutdownServers gracefully stops all servers with a 30-second timeout.
func shutdownServers(log logr.Logger, apiSrv, healthSrv, metricsSrv *http.Server, grpcSrv *grpc.Server, otlpHTTPSrv *http.Server) {
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()

	if grpcSrv != nil {
		grpcDone := make(chan struct{})
		go func() {
			grpcSrv.GracefulStop()
			close(grpcDone)
		}()
		select {
		case <-grpcDone:
			// Graceful shutdown completed
		case <-time.After(10 * time.Second):
			log.Info("gRPC graceful stop timed out, forcing stop")
			grpcSrv.Stop()
		}
	}

	for _, s := range []struct {
		name string
		srv  *http.Server
	}{
		{"OTLP HTTP", otlpHTTPSrv},
		{"metrics", metricsSrv},
		{"API", apiSrv},
		{"health", healthSrv},
	} {
		if s.srv == nil {
			continue
		}
		if err := s.srv.Shutdown(shutCtx); err != nil {
			log.Error(err, "server shutdown error", "server", s.name)
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
// Pool settings are read from environment variables with sensible defaults:
//
//	PG_MAX_CONNS (default 25), PG_MIN_CONNS (default 5),
//	PG_MAX_CONN_LIFETIME (default 1h), PG_MAX_CONN_IDLE_TIME (default 30m).
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

// buildAPIMux assembles the HTTP handler with all API routes, wrapped with
// Prometheus metrics middleware. Returns the handler and a cleanup function
// for the audit logger (no-op when enterprise is disabled).
func buildAPIMux(pool *pgxpool.Pool, registry *providers.Registry, f *flags, log logr.Logger) (http.Handler, *api.SessionService, func()) {
	svcCfg := api.ServiceConfig{}
	cleanup := func() {}

	// Enterprise: audit logger.
	var auditLogger *audit.Logger
	if f.enterprise {
		auditMetrics := metrics.NewAuditMetrics()
		auditLogger = audit.NewLogger(pool, log, auditMetrics, audit.LoggerConfig{})
		svcCfg.AuditLogger = auditLogger
		cleanup = func() { _ = auditLogger.Close() }
	}

	httpMetrics := api.NewHTTPMetrics(nil)
	httpMetrics.Initialize()

	// Event publisher (reuses the same Redis used for hot cache, if configured).
	svcCfg.EventPublisher = initEventPublisher(registry, log, httpMetrics)

	sessionService := api.NewSessionService(registry, svcCfg, log)
	maxBody := int64(envInt32("MAX_BODY_SIZE", int32(api.DefaultMaxBodySize)))
	handler := api.NewHandler(sessionService, log, maxBody)

	// Wire up eval result endpoints when Postgres is available.
	if pool != nil {
		evalStore := pgprovider.NewEvalStore(pool)
		evalService := api.NewEvalService(evalStore, log)
		handler.SetEvalService(evalService)
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	registerEnterpriseRoutes(mux, pool, registry, auditLogger, f, log)

	// Privacy middleware (enterprise only): PII redaction + user opt-out.
	var apiHandler http.Handler = mux
	if f.enterprise {
		apiHandler = wrapPrivacyMiddleware(apiHandler, registry, pool, log)
	}

	// Rate limiting middleware (per-client-IP token bucket).
	rlCfg := api.RateLimitConfigFromEnv()
	rlMiddleware, rlStop := api.NewRateLimitMiddleware(rlCfg)
	origCleanup := cleanup
	cleanup = func() {
		rlStop()
		origCleanup()
	}
	log.V(1).Info("rate limiter initialized", "rps", rlCfg.RPS, "burst", rlCfg.Burst)

	traced := otelhttp.NewHandler(api.TraceLogMiddleware(apiHandler), "session-api",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/healthz"
		}),
	)
	return rlMiddleware(api.MetricsMiddleware(httpMetrics, traced)), sessionService, cleanup
}

// registerEnterpriseRoutes adds audit, GDPR deletion, and opt-out routes when
// enterprise mode is enabled.
func registerEnterpriseRoutes(mux *http.ServeMux, pool *pgxpool.Pool, registry *providers.Registry, auditLogger *audit.Logger, f *flags, log logr.Logger) {
	if f.enterprise && auditLogger != nil {
		ah := audit.NewHandler(auditLogger, log)
		ah.RegisterRoutes(mux)

		warm, _ := registry.WarmStore()
		deletionStore := privacy.NewPostgresDeletionStore(pool)
		deleter := privacy.NewWarmStoreSessionDeleter(warm)
		deletionSvc := privacy.NewDeletionService(deletionStore, deleter, auditLogger, log)
		deletionSvc.SetMediaDeleter(buildMediaDeleter(f, log))
		deletionHandler := privacy.NewDeletionHandler(deletionSvc, log)
		deletionHandler.RegisterRoutes(mux)
	}

	if f.enterprise {
		privacyStore := privacy.NewPreferencesStore(pool)
		optOutHandler := privacy.NewOptOutHandler(privacyStore, log)
		optOutHandler.RegisterRoutes(mux)
	}
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

// initProviders creates the tiered storage registry (warm/hot/cold) and returns
// a cleanup function that closes all providers in reverse order.
func initProviders(ctx context.Context, f *flags, pool *pgxpool.Pool, log logr.Logger) (*providers.Registry, func(), error) {
	registry := providers.NewRegistry()
	var cleanups []func()

	// Warm store (postgres, using shared pool).
	warmProvider := pgprovider.NewFromPool(pool)
	registry.SetWarmStore(warmProvider)
	cleanups = append(cleanups, func() { _ = warmProvider.Close() })
	log.V(1).Info("warm store initialized")

	// Hot cache (redis, optional).
	if f.redisAddrs != "" {
		redisCfg := redis.DefaultConfig()
		redisCfg.Addrs = strings.Split(f.redisAddrs, ",")
		redisCfg.MaxMessagesPerSession = int(envInt32("REDIS_MAX_MESSAGES", int32(redisCfg.MaxMessagesPerSession)))
		hotProvider, err := redis.New(redisCfg)
		if err != nil {
			return nil, nil, fmt.Errorf("creating redis provider: %w", err)
		}
		registry.SetHotCache(hotProvider)
		cleanups = append(cleanups, func() { _ = hotProvider.Close() })
		log.V(1).Info("hot cache initialized", "addrs", redisCfg.Addrs)
	}

	// Cold archive (optional).
	if f.coldBackend != "" && f.coldBucket != "" {
		coldCfg := cold.DefaultConfig()
		coldCfg.Backend = cold.BackendType(f.coldBackend)
		coldCfg.Bucket = f.coldBucket
		switch coldCfg.Backend {
		case cold.BackendS3:
			coldCfg.S3 = &cold.S3Config{
				Region:   f.coldRegion,
				Endpoint: f.coldEndpoint,
			}
		case cold.BackendGCS:
			coldCfg.GCS = &cold.GCSConfig{}
		case cold.BackendAzure:
			coldCfg.Azure = &cold.AzureConfig{}
		}
		coldProvider, err := cold.New(ctx, coldCfg)
		if err != nil {
			return nil, nil, fmt.Errorf("creating cold archive provider: %w", err)
		}
		registry.SetColdArchive(coldProvider)
		cleanups = append(cleanups, func() { _ = coldProvider.Close() })
		log.V(1).Info("cold archive initialized", "backend", f.coldBackend)
	}

	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	return registry, cleanup, nil
}

// initEventPublisher creates an EventPublisher backed by the Redis client from
// the hot cache provider, if available. Returns nil when Redis is not configured.
func initEventPublisher(registry *providers.Registry, log logr.Logger, httpMetrics ...*api.HTTPMetrics) api.EventPublisher {
	hot, err := registry.HotCache()
	if err != nil {
		log.V(1).Info("event publisher skipped", "reason", "no hot cache")
		return nil
	}
	rp, ok := hot.(redisClientProvider)
	if !ok {
		log.V(1).Info("event publisher skipped", "reason", "hot cache does not expose Redis client")
		return nil
	}
	log.V(1).Info("event publisher initialized")
	var m *api.HTTPMetrics
	if len(httpMetrics) > 0 {
		m = httpMetrics[0]
	}
	return api.NewRedisEventPublisher(rp.RedisClient(), log, m)
}

// startOTLPServers creates and starts the OTLP gRPC and HTTP servers.
// Returns the servers for graceful shutdown.
func startOTLPServers(f *flags, svc *api.SessionService, log logr.Logger) (*grpc.Server, *http.Server) {
	transformer := otlp.NewTransformer(svc, log)

	// gRPC server.
	grpcSrv := grpc.NewServer()
	receiver := otlp.NewReceiver(transformer, log)
	coltracepb.RegisterTraceServiceServer(grpcSrv, receiver)

	go func() {
		lis, err := net.Listen("tcp", f.otlpGRPCAddr)
		if err != nil {
			log.Error(err, "failed to listen for OTLP gRPC", "addr", f.otlpGRPCAddr)
			return
		}
		log.Info("starting OTLP gRPC server", "addr", f.otlpGRPCAddr)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Error(err, "OTLP gRPC server error")
		}
	}()

	// HTTP server.
	handler := otlp.NewHandler(transformer, log)
	otlpMux := http.NewServeMux()
	handler.RegisterRoutes(otlpMux)

	httpSrv := &http.Server{Addr: f.otlpHTTPAddr, Handler: otlpMux}
	go func() {
		log.Info("starting OTLP HTTP server", "addr", f.otlpHTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "OTLP HTTP server error")
		}
	}()

	return grpcSrv, httpSrv
}

// buildMediaDeleter returns a MediaDeleter based on the configured cold storage
// backend. When no object storage is configured, nil is returned and the
// deletion service retains its default NoOpMediaDeleter.
func buildMediaDeleter(f *flags, log logr.Logger) privacy.MediaDeleter {
	if f.coldBackend == "" || f.coldBucket == "" {
		log.V(1).Info("media deleter skipped", "reason", "no object storage configured")
		return nil
	}

	client, err := buildObjectStoreClient(f, log)
	if err != nil {
		log.Error(err, "media deleter creation failed, using no-op")
		return privacy.NoOpMediaDeleter{}
	}

	log.Info("media deleter enabled", "backend", f.coldBackend, "bucket", f.coldBucket)
	return privacy.NewObjectStoreMediaDeleter(client, f.coldBucket, "sessions/", log)
}

func buildObjectStoreClient(f *flags, log logr.Logger) (privacy.ObjectStoreClient, error) {
	switch cold.BackendType(f.coldBackend) {
	case cold.BackendS3:
		return buildS3ObjectStoreClient(f, log)
	case cold.BackendGCS:
		return buildGCSObjectStoreClient(log)
	case cold.BackendAzure:
		return buildAzureObjectStoreClient(f, log)
	default:
		return nil, fmt.Errorf("unsupported cold backend for media deleter: %s", f.coldBackend)
	}
}

func buildS3ObjectStoreClient(f *flags, log logr.Logger) (*privacy.S3ObjectStoreClient, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(f.coldRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	opts := []func(*s3sdk.Options){}
	if f.coldEndpoint != "" {
		opts = append(opts, func(o *s3sdk.Options) {
			o.BaseEndpoint = aws.String(f.coldEndpoint)
			o.UsePathStyle = true
		})
	}
	client := s3sdk.NewFromConfig(cfg, opts...)
	return privacy.NewS3ObjectStoreClient(client, log), nil
}

func buildGCSObjectStoreClient(log logr.Logger) (*privacy.GCSObjectStoreClient, error) {
	client, err := gcsstorage.NewClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("creating GCS client: %w", err)
	}
	return privacy.NewGCSObjectStoreClient(client, log), nil
}

func buildAzureObjectStoreClient(f *flags, log logr.Logger) (*privacy.AzureObjectStoreClient, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("creating Azure credential: %w", err)
	}
	endpoint := f.coldEndpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.blob.core.windows.net", f.coldBucket)
	}
	client, err := azblob.NewClient(endpoint, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Azure blob client: %w", err)
	}
	return privacy.NewAzureObjectStoreClient(client, log), nil
}

// wrapPrivacyMiddleware creates and returns the privacy middleware handler.
// When the K8s API is unreachable (e.g., in tests), the middleware is skipped
// and the original handler is returned unchanged.
func wrapPrivacyMiddleware(
	next http.Handler,
	registry *providers.Registry,
	pool *pgxpool.Pool,
	log logr.Logger,
) http.Handler {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Info("privacy middleware skipped", "reason", "no in-cluster kubeconfig")
		return next
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(omniav1alpha1.AddToScheme(scheme))
	k8sClient, err := client.New(kubeConfig, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "privacy middleware skipped", "reason", "k8s client creation failed")
		return next
	}

	watcher := privacy.NewPolicyWatcher(k8sClient, log)

	// Start the watcher asynchronously; it syncs the cache in the background.
	go func() {
		if startErr := watcher.Start(context.Background()); startErr != nil {
			log.Error(startErr, "policy watcher start failed")
		}
	}()

	sessionLookup := privacy.NewWarmStoreSessionLookup(registry)
	sessionCache := privacy.NewSessionMetadataCache(sessionLookup, 10000)
	redactor := redaction.NewRedactor()
	prefStore := privacy.NewPreferencesStore(pool)

	middleware := privacy.NewPrivacyMiddleware(watcher, sessionCache, redactor, prefStore, log)
	log.Info("privacy middleware enabled")
	return middleware.Wrap(next)
}
