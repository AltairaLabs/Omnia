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
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/zapr"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/altairalabs/omnia/ee/pkg/audit"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/internal/session/api"
	sessionpg "github.com/altairalabs/omnia/internal/session/postgres"
	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/altairalabs/omnia/internal/session/providers/cold"
	pgprovider "github.com/altairalabs/omnia/internal/session/providers/postgres"
	"github.com/altairalabs/omnia/internal/session/providers/redis"
)

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
	flag.Parse()

	// Env var fallbacks.
	if f.postgresConn == "" {
		f.postgresConn = os.Getenv("POSTGRES_CONN")
	}
	if f.redisAddrs == "" {
		f.redisAddrs = os.Getenv("REDIS_ADDRS")
	}
	if f.coldBackend == "" {
		f.coldBackend = os.Getenv("COLD_BACKEND")
	}
	if f.coldBucket == "" {
		f.coldBucket = os.Getenv("COLD_BUCKET")
	}
	if f.coldRegion == "" {
		f.coldRegion = os.Getenv("COLD_REGION")
	}
	if f.coldEndpoint == "" {
		f.coldEndpoint = os.Getenv("COLD_ENDPOINT")
	}
	if !f.enterprise && os.Getenv("ENTERPRISE_ENABLED") == "true" {
		f.enterprise = true
	}
	if f.apiAddr == ":8080" && os.Getenv("API_ADDR") != "" {
		f.apiAddr = os.Getenv("API_ADDR")
	}
	if f.healthAddr == ":8081" && os.Getenv("HEALTH_ADDR") != "" {
		f.healthAddr = os.Getenv("HEALTH_ADDR")
	}
	if f.metricsAddr == ":9090" && os.Getenv("METRICS_ADDR") != "" {
		f.metricsAddr = os.Getenv("METRICS_ADDR")
	}

	return f
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
	poolCfg, err := pgxpool.ParseConfig(f.postgresConn)
	if err != nil {
		return fmt.Errorf("parsing postgres connection string: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("creating postgres pool: %w", err)
	}
	defer pool.Close()

	// --- Migrations ---
	migrator, err := sessionpg.NewMigrator(f.postgresConn, log)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	if err := migrator.Up(); err != nil {
		_ = migrator.Close()
		return fmt.Errorf("running migrations: %w", err)
	}
	_ = migrator.Close()

	// --- Providers ---
	registry, providerCleanup, err := initProviders(ctx, f, pool)
	if err != nil {
		return err
	}
	defer providerCleanup()

	// --- Session service ---
	svcCfg := api.ServiceConfig{}

	// Enterprise: audit logger.
	var auditLogger *audit.Logger
	if f.enterprise {
		auditMetrics := metrics.NewAuditMetrics()
		auditLogger = audit.NewLogger(pool, log, auditMetrics, audit.LoggerConfig{})
		defer func() { _ = auditLogger.Close() }()
		svcCfg.AuditLogger = auditLogger
	}

	sessionService := api.NewSessionService(registry, svcCfg, log)
	handler := api.NewHandler(sessionService, log)

	// --- API mux ---
	apiMux := http.NewServeMux()
	handler.RegisterRoutes(apiMux)

	if f.enterprise && auditLogger != nil {
		ah := audit.NewHandler(auditLogger, log)
		ah.RegisterRoutes(apiMux)
	}

	if f.enterprise {
		privacyStore := privacy.NewPreferencesStore(pool)
		optOutHandler := privacy.NewOptOutHandler(privacyStore, log)
		optOutHandler.RegisterRoutes(apiMux)
	}

	apiMux.Handle("GET /metrics", promhttp.Handler())

	// --- Health server ---
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

	healthSrv := &http.Server{Addr: f.healthAddr, Handler: healthMux}
	go func() {
		log.Info("starting health server", "addr", f.healthAddr)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "health server error")
		}
	}()

	// --- API server ---
	apiSrv := &http.Server{Addr: f.apiAddr, Handler: apiMux}
	go func() {
		log.Info("starting session API server", "addr", f.apiAddr)
		if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "API server error")
		}
	}()

	log.Info("session-api ready",
		"api", f.apiAddr,
		"health", f.healthAddr,
		"enterprise", f.enterprise,
	)

	// --- Wait for shutdown ---
	<-ctx.Done()
	log.Info("shutting down")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()

	// Graceful shutdown: API first, then health.
	if err := apiSrv.Shutdown(shutCtx); err != nil {
		log.Error(err, "API server shutdown error")
	}
	if err := healthSrv.Shutdown(shutCtx); err != nil {
		log.Error(err, "health server shutdown error")
	}

	return nil
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
