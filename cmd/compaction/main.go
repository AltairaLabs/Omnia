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

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/altairalabs/omnia/internal/compaction"
	"github.com/altairalabs/omnia/internal/session/providers/cold"
	"github.com/altairalabs/omnia/internal/session/providers/postgres"
	"github.com/altairalabs/omnia/internal/session/providers/redis"
	"github.com/altairalabs/omnia/pkg/metrics"
)

// flags groups all CLI flags for the compaction binary.
type flags struct {
	retentionConfigPath string
	batchSize           int
	maxRetries          int
	compression         string
	dryRun              bool
	metricsAddr         string
	postgresConn        string
	redisAddrs          string
	coldBackend         string
	coldBucket          string
	coldRegion          string
	coldEndpoint        string
}

func parseFlags() *flags {
	f := &flags{}
	flag.StringVar(&f.retentionConfigPath, "retention-config",
		"/etc/omnia/retention/retention.yaml", "Path to retention config YAML")
	flag.IntVar(&f.batchSize, "batch-size", 1000, "Sessions per batch")
	flag.IntVar(&f.maxRetries, "max-retries", 3, "Max retry attempts per op")
	flag.StringVar(&f.compression, "compression", "snappy", "Parquet codec")
	flag.BoolVar(&f.dryRun, "dry-run", false, "Log without writing")
	flag.StringVar(&f.metricsAddr, "metrics-addr", ":9090", "Metrics address")
	flag.StringVar(&f.postgresConn, "postgres-conn", "", "Postgres conn string")
	flag.StringVar(&f.redisAddrs, "redis-addrs", "", "Redis addresses (csv)")
	flag.StringVar(&f.coldBackend, "cold-backend", "s3", "Cold backend type")
	flag.StringVar(&f.coldBucket, "cold-bucket", "", "Cold bucket name")
	flag.StringVar(&f.coldRegion, "cold-region", "", "Cold region (S3)")
	flag.StringVar(&f.coldEndpoint, "cold-endpoint", "", "Cold endpoint (S3)")
	flag.Parse()

	// Env var fallbacks for secrets.
	if f.postgresConn == "" {
		f.postgresConn = os.Getenv("POSTGRES_CONN")
	}
	if f.redisAddrs == "" {
		f.redisAddrs = os.Getenv("REDIS_ADDRS")
	}
	if f.coldBackend == "s3" && os.Getenv("COLD_BACKEND") != "" {
		f.coldBackend = os.Getenv("COLD_BACKEND")
	}
	if f.coldBucket == "" {
		f.coldBucket = os.Getenv("COLD_BUCKET")
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
	logger, err := zapCfg.Build()
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()
	log := logger.Sugar()

	// --- Signal context ---
	ctx, cancel := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	// --- Metrics server (goroutine) ---
	compactionMetrics := metrics.NewCompactionMetrics()
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: f.metricsAddr, Handler: mux}
	go func() {
		log.Infow("starting metrics server", "addr", f.metricsAddr)
		if srvErr := srv.ListenAndServe(); srvErr != nil && srvErr != http.ErrServerClosed {
			log.Errorw("metrics server error", "error", srvErr)
		}
	}()
	defer func() {
		shutCtx, shutCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer shutCancel()
		_ = srv.Shutdown(shutCtx)
	}()

	// --- Retention config ---
	retentionCfg, err := compaction.LoadRetentionConfig(f.retentionConfigPath)
	if err != nil {
		return fmt.Errorf("loading retention config: %w", err)
	}
	if !retentionCfg.ColdArchiveEnabled() {
		log.Info("cold archive is not enabled; exiting")
		return nil
	}

	// --- Providers ---
	warmProvider, coldProvider, hotProvider, cleanup, err := initProviders(ctx, f)
	if err != nil {
		return err
	}
	defer cleanup()

	// --- Engine ---
	engineCfg := compaction.Config{
		BatchSize:   f.batchSize,
		MaxRetries:  f.maxRetries,
		RetryDelay:  5 * time.Second,
		Compression: f.compression,
		DryRun:      f.dryRun,
	}
	engine := compaction.NewEngine(
		warmProvider, coldProvider, hotProvider,
		retentionCfg, engineCfg, compactionMetrics, log,
	)

	log.Info("starting compaction run")
	result, err := engine.Run(ctx)
	if err != nil {
		log.Errorw("compaction failed", "error", err)
		return err
	}

	log.Infow("compaction complete",
		"sessionsCompacted", result.SessionsCompacted,
		"batchesProcessed", result.BatchesProcessed,
		"coldPurged", result.ColdPurged,
		"errors", len(result.Errors),
	)
	for _, e := range result.Errors {
		log.Warnw("non-fatal error", "error", e)
	}
	return nil
}

// initProviders creates the storage providers and returns a cleanup function.
func initProviders(
	ctx context.Context, f *flags,
) (*postgres.Provider, *cold.Provider, *redis.Provider, func(), error) {
	var cleanups []func()
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	// Postgres (required)
	if f.postgresConn == "" {
		return nil, nil, nil, nil,
			fmt.Errorf("--postgres-conn or POSTGRES_CONN is required")
	}
	pgCfg := postgres.DefaultConfig()
	pgCfg.ConnString = f.postgresConn
	warmProvider, err := postgres.New(pgCfg)
	if err != nil {
		return nil, nil, nil, nil,
			fmt.Errorf("creating postgres provider: %w", err)
	}
	cleanups = append(cleanups, func() { _ = warmProvider.Close() })

	// Cold archive (required)
	if f.coldBucket == "" {
		cleanup()
		return nil, nil, nil, nil,
			fmt.Errorf("--cold-bucket or COLD_BUCKET is required")
	}
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
	default:
		cleanup()
		return nil, nil, nil, nil,
			fmt.Errorf("unsupported cold backend: %s", f.coldBackend)
	}
	coldProvider, err := cold.New(ctx, coldCfg)
	if err != nil {
		cleanup()
		return nil, nil, nil, nil,
			fmt.Errorf("creating cold archive provider: %w", err)
	}
	cleanups = append(cleanups, func() { _ = coldProvider.Close() })

	// Redis (optional)
	var hotProvider *redis.Provider
	if f.redisAddrs != "" {
		redisCfg := redis.DefaultConfig()
		redisCfg.Addrs = strings.Split(f.redisAddrs, ",")
		hotProvider, err = redis.New(redisCfg)
		if err != nil {
			cleanup()
			return nil, nil, nil, nil,
				fmt.Errorf("creating redis provider: %w", err)
		}
		cleanups = append(cleanups, func() { _ = hotProvider.Close() })
	}

	return warmProvider, coldProvider, hotProvider, cleanup, nil
}
