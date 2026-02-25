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
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/ee/pkg/audit"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/internal/session/otlp"
	sessionpg "github.com/altairalabs/omnia/internal/session/postgres"
	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/altairalabs/omnia/internal/session/providers/cold"
	pgprovider "github.com/altairalabs/omnia/internal/session/providers/postgres"
	"github.com/altairalabs/omnia/internal/session/providers/redis"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// redisClientProvider is implemented by providers that expose the underlying Redis client.
type redisClientProvider interface {
	RedisClient() goredis.UniversalClient
}

// flags groups all CLI flags for the session-api binary.
type flags struct {
	apiAddr      string
	healthAddr   string
	metricsAddr  string
	postgresConn string
	redisAddrs   string
	coldBackend  string
	coldBucket   string
	coldRegion   string
	coldEndpoint string
	enterprise   bool
	otlpEnabled  bool
	otlpGRPCAddr string
	otlpHTTPAddr string
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
	zapCfg := zap.NewProductionConfig()
	zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	zapLogger, err := zapCfg.Build()
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer func() { _ = zapLogger.Sync() }()
	log := zapr.NewLogger(zapLogger)

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

	// --- Migrations ---
	if err := runMigrations(f.postgresConn, log); err != nil {
		return err
	}

	// --- Providers ---
	registry, providerCleanup, err := initProviders(ctx, f, pool)
	if err != nil {
		return err
	}
	defer providerCleanup()

	// --- Build API mux ---
	apiMux, auditCleanup := buildAPIMux(pool, registry, f, log)
	defer auditCleanup()

	// --- Servers ---
	healthSrv := newHealthServer(f.healthAddr, pool)
	apiSrv := &http.Server{Addr: f.apiAddr, Handler: apiMux}

	go func() {
		log.Info("starting health server", "addr", f.healthAddr)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "health server error")
		}
	}()
	go func() {
		log.Info("starting session API server", "addr", f.apiAddr)
		if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "API server error")
		}
	}()

	// --- OTLP servers (optional) ---
	var grpcSrv *grpc.Server
	var otlpHTTPSrv *http.Server
	if f.otlpEnabled {
		grpcSrv, otlpHTTPSrv = startOTLPServers(f, registry, log)
	}

	log.Info("session-api ready",
		"api", f.apiAddr,
		"health", f.healthAddr,
		"enterprise", f.enterprise,
		"otlp", f.otlpEnabled,
	)

	// --- Wait for shutdown ---
	<-ctx.Done()
	log.Info("shutting down")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()

	if grpcSrv != nil {
		grpcSrv.GracefulStop()
	}
	if otlpHTTPSrv != nil {
		if err := otlpHTTPSrv.Shutdown(shutCtx); err != nil {
			log.Error(err, "OTLP HTTP server shutdown error")
		}
	}
	if err := apiSrv.Shutdown(shutCtx); err != nil {
		log.Error(err, "API server shutdown error")
	}
	if err := healthSrv.Shutdown(shutCtx); err != nil {
		log.Error(err, "health server shutdown error")
	}

	return nil
}

// initPool creates and returns a pgxpool connection pool.
func initPool(ctx context.Context, connStr string) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres connection string: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating postgres pool: %w", err)
	}
	return pool, nil
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

// buildAPIMux assembles the HTTP mux with all API routes. Returns the mux and
// a cleanup function for the audit logger (no-op when enterprise is disabled).
func buildAPIMux(pool *pgxpool.Pool, registry *providers.Registry, f *flags, log logr.Logger) (*http.ServeMux, func()) {
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

	// Event publisher (reuses the same Redis used for hot cache, if configured).
	svcCfg.EventPublisher = initEventPublisher(registry, log)

	sessionService := api.NewSessionService(registry, svcCfg, log)
	handler := api.NewHandler(sessionService, log)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	registerEnterpriseRoutes(mux, pool, registry, auditLogger, f, log)
	mux.Handle("GET /metrics", promhttp.Handler())

	return mux, cleanup
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
		deletionHandler := privacy.NewDeletionHandler(deletionSvc, log)
		deletionHandler.RegisterRoutes(mux)
	}

	if f.enterprise {
		privacyStore := privacy.NewPreferencesStore(pool)
		optOutHandler := privacy.NewOptOutHandler(privacyStore, log)
		optOutHandler.RegisterRoutes(mux)
	}
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
func initProviders(ctx context.Context, f *flags, pool *pgxpool.Pool) (*providers.Registry, func(), error) {
	registry := providers.NewRegistry()
	var cleanups []func()

	// Warm store (postgres, using shared pool).
	warmProvider := pgprovider.NewFromPool(pool)
	registry.SetWarmStore(warmProvider)
	cleanups = append(cleanups, func() { _ = warmProvider.Close() })

	// Hot cache (redis, optional).
	if f.redisAddrs != "" {
		redisCfg := redis.DefaultConfig()
		redisCfg.Addrs = strings.Split(f.redisAddrs, ",")
		hotProvider, err := redis.New(redisCfg)
		if err != nil {
			return nil, nil, fmt.Errorf("creating redis provider: %w", err)
		}
		registry.SetHotCache(hotProvider)
		cleanups = append(cleanups, func() { _ = hotProvider.Close() })
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
func initEventPublisher(registry *providers.Registry, log logr.Logger) api.EventPublisher {
	hot, err := registry.HotCache()
	if err != nil {
		return nil
	}
	rp, ok := hot.(redisClientProvider)
	if !ok {
		return nil
	}
	log.Info("event publisher enabled (Redis Streams)")
	return api.NewRedisEventPublisher(rp.RedisClient(), log)
}

// startOTLPServers creates and starts the OTLP gRPC and HTTP servers.
// Returns the servers for graceful shutdown.
func startOTLPServers(f *flags, registry *providers.Registry, log logr.Logger) (*grpc.Server, *http.Server) {
	sessionService := api.NewSessionService(registry, api.ServiceConfig{}, log)
	transformer := otlp.NewTransformer(sessionService, log)

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
