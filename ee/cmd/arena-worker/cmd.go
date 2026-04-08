/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/logger"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logging"
)

func main() {
	// Disable PromptKit JSON schema validation — the Go structs are the
	// source of truth and the remote schema may lag behind.  This also
	// avoids network fetches in air-gapped environments.
	config.SchemaValidationDisabled.Store(true)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// Initialize structured logger (same pattern as facade/runtime)
	zapLog, err := logging.NewZapLogger()
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() { _ = zapLog.Sync() }()

	log := zapr.NewLogger(zapLog)

	// Initialize global OpenTelemetry text map propagator so that trace context
	// is injected into outbound WebSocket upgrade requests (fleet mode).
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Initialize tracing provider (reads TRACING_* env vars, same as facade/runtime).
	tracingCfg := tracing.Config{
		Enabled:     os.Getenv("TRACING_ENABLED") == "true",
		Endpoint:    os.Getenv("TRACING_ENDPOINT"),
		ServiceName: "omnia-arena-worker",
		Insecure:    os.Getenv("TRACING_INSECURE") == "true",
	}
	if rate := os.Getenv("TRACING_SAMPLE_RATE"); rate != "" {
		if v, parseErr := fmt.Sscanf(rate, "%f", &tracingCfg.SampleRate); v == 0 || parseErr != nil {
			log.V(1).Info("invalid TRACING_SAMPLE_RATE, using default")
		}
	}
	tp, tpErr := tracing.NewProvider(ctx, tracingCfg)
	if tpErr != nil {
		log.Error(tpErr, "tracing provider creation failed")
	} else {
		otel.SetTracerProvider(tp.TracerProvider())
		defer func() { _ = tp.Shutdown(ctx) }()
		if tracingCfg.Enabled {
			log.Info("tracing enabled", "endpoint", tracingCfg.Endpoint, "sampleRate", tracingCfg.SampleRate)
		}
	}

	// Bridge PromptKit SDK logging to the same Zap core
	sdkLogger := logging.SlogFromZap(zapLog)

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	log.Info("arena worker starting",
		"job", cfg.JobName,
		"namespace", cfg.JobNamespace,
		"jobType", cfg.JobType,
		"contentPath", cfg.ContentPath,
		"contentVersion", cfg.ContentVersion,
	)

	// Configure PromptKit SDK logging via slog bridge
	configureSDKLogging(cfg, sdkLogger)

	// Get content path (mounted from PVC)
	bundlePath, err := getContentPath(cfg)
	if err != nil {
		return fmt.Errorf("failed to get content path: %w", err)
	}
	log.V(1).Info("content path resolved", "bundlePath", bundlePath)

	// Connect to Redis queue
	rawQ, err := queue.NewRedisQueue(queue.RedisOptions{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
		Options:  queue.DefaultOptions(),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to queue: %w", err)
	}
	defer func() {
		if closeErr := rawQ.Close(); closeErr != nil {
			log.Error(closeErr, "failed to close queue")
		}
	}()

	log.Info("connected to redis", "addr", cfg.RedisAddr)

	// Initialize metrics and wrap queue with instrumentation
	queueMetrics := queue.NewQueueMetrics(queue.QueueMetricsConfig{})
	queueMetrics.Initialize()
	q := queue.NewInstrumentedQueue(rawQ, queueMetrics)

	workerMetrics := NewWorkerMetrics()

	metricsAddr := getEnvOrDefault("METRICS_ADDR", defaultMetricsAddr)
	go startMetricsServer(metricsAddr, log)

	// Process work items
	err = processWorkItems(ctx, log, cfg, q, bundlePath, workerMetrics)

	// Persist output to S3 after all work items complete (best-effort, non-fatal).
	// For PVC output, the engine writes directly to the mounted path — no extra step needed.
	if err == nil {
		persistOutputToS3(ctx, log, cfg)
	}

	// Wait after processing completes so Prometheus can scrape final metrics.
	// Without this, the pod exits immediately and the last scrape never happens.
	if cfg.ShutdownDelay > 0 {
		log.Info("waiting for final metrics scrape", "delay", cfg.ShutdownDelay)
		time.Sleep(cfg.ShutdownDelay)
	}

	return err
}

// persistOutputToS3 uploads the engine output directory to S3 when the job's
// OutputConfig specifies S3. It is a no-op for PVC output (the engine writes
// directly to the mounted path) and when no output config is set.
// Errors are logged but do not fail the worker — results have already been
// recorded to Redis at this point.
func persistOutputToS3(ctx context.Context, log logr.Logger, cfg *Config) {
	if cfg.OutputConfig == nil ||
		cfg.OutputConfig.Type != omniav1alpha1.OutputTypeS3 ||
		cfg.OutputConfig.S3 == nil {
		return
	}
	outputDir := resolveOutputDir(cfg)
	s3Cfg := cfg.OutputConfig.S3
	log.Info("uploading output to S3",
		"bucket", s3Cfg.Bucket,
		"prefix", s3Cfg.Prefix,
		"outputDir", outputDir,
	)
	uploadFn, err := newS3UploadFunc(ctx, s3Cfg)
	if err != nil {
		log.Error(err, "failed to create S3 upload client — output not persisted")
		return
	}
	if err := uploadOutputToS3(ctx, log, outputDir, cfg.JobName, cfg.JobNamespace, s3Cfg, uploadFn); err != nil {
		log.Error(err, "failed to upload output to S3 — results may be incomplete")
		return
	}
	log.Info("output uploaded to S3", "bucket", s3Cfg.Bucket, "prefix", s3Cfg.Prefix)
}

// configureSDKLogging sets up PromptKit SDK logging via the slog bridge.
func configureSDKLogging(cfg *Config, sdkLogger *slog.Logger) {
	logger.SetLogger(sdkLogger)
	if cfg.Verbose {
		logger.SetVerbose(true)
	}
}
