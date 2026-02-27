/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

// arena-eval-worker is a long-running binary that consumes session events
// from Redis Streams and runs evals for non-PromptKit agents.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	goredis "github.com/redis/go-redis/v9"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/altairalabs/omnia/ee/pkg/evals"
)

// Environment variable names for worker configuration.
const (
	envRedisAddr    = "REDIS_ADDR"
	envRedisPass    = "REDIS_PASSWORD"
	envRedisDB      = "REDIS_DB"
	envNamespace    = "NAMESPACE"
	envSessionAPI   = "SESSION_API_URL"
	envLogLevel     = "LOG_LEVEL"
	envMetricsAddr  = "METRICS_ADDR"
	defaultLogLevel = "info"
	defaultMetrics  = ":9090"
)

func main() {
	logger := buildLogger()

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Set up K8s client for PromptPack ConfigMap reads.
	k8sCfg := ctrl.GetConfigOrDie()
	k8sClient, err := newK8sClient(k8sCfg)
	if err != nil {
		logger.Error("failed to create k8s client", "error", err)
		os.Exit(1)
	}

	packLoader := evals.NewPromptPackLoader(k8sClient)

	redisClient := goredis.NewClient(&goredis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer func() { _ = redisClient.Close() }()

	sessionClient := evals.NewHTTPSessionAPIClient(cfg.SessionAPIURL)

	worker := evals.NewEvalWorker(evals.WorkerConfig{
		RedisClient: redisClient,
		SessionAPI:  sessionClient,
		Namespace:   cfg.Namespace,
		Logger:      logger,
		PackLoader:  packLoader,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received shutdown signal", "signal", sig.String())
		cancel()
	}()

	// Start HTTP server for metrics and health probes.
	go startHTTPServer(cfg.MetricsAddr, logger)

	logger.Info("starting arena-eval-worker",
		"namespace", cfg.Namespace,
		"redisAddr", cfg.RedisAddr,
		"sessionAPI", cfg.SessionAPIURL,
		"metricsAddr", cfg.MetricsAddr,
	)

	if err := worker.Start(ctx); err != nil {
		logger.Error("worker exited with error", "error", err)
		os.Exit(1)
	}

	logger.Info("shutdown complete")
}

// workerEnvConfig holds parsed environment configuration.
type workerEnvConfig struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	Namespace     string
	SessionAPIURL string
	MetricsAddr   string
}

// loadConfig reads and validates environment variables.
func loadConfig() (*workerEnvConfig, error) {
	cfg := &workerEnvConfig{
		RedisAddr:     os.Getenv(envRedisAddr),
		RedisPassword: os.Getenv(envRedisPass),
		Namespace:     os.Getenv(envNamespace),
		SessionAPIURL: os.Getenv(envSessionAPI),
		MetricsAddr:   os.Getenv(envMetricsAddr),
	}

	if cfg.RedisAddr == "" {
		return nil, fmt.Errorf("%s is required", envRedisAddr)
	}
	if cfg.Namespace == "" {
		return nil, fmt.Errorf("%s is required", envNamespace)
	}
	if cfg.SessionAPIURL == "" {
		return nil, fmt.Errorf("%s is required", envSessionAPI)
	}
	if cfg.MetricsAddr == "" {
		cfg.MetricsAddr = defaultMetrics
	}

	if dbStr := os.Getenv(envRedisDB); dbStr != "" {
		db, err := strconv.Atoi(dbStr)
		if err != nil {
			return nil, fmt.Errorf("invalid %s value %q: %w", envRedisDB, dbStr, err)
		}
		cfg.RedisDB = db
	}

	return cfg, nil
}

// startHTTPServer starts the metrics and health probe HTTP server.
func startHTTPServer(addr string, logger *slog.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("starting metrics/health server", "addr", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("metrics server failed", "error", err)
	}
}

// newK8sClient creates a controller-runtime client with only the types needed
// by the eval worker (ConfigMaps for PromptPack data).
func newK8sClient(cfg *rest.Config) (client.Client, error) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add corev1 to scheme: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create k8s client: %w", err)
	}

	return c, nil
}

// buildLogger creates a structured logger from the LOG_LEVEL environment variable.
func buildLogger() *slog.Logger {
	levelStr := os.Getenv(envLogLevel)
	if levelStr == "" {
		levelStr = defaultLogLevel
	}

	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
