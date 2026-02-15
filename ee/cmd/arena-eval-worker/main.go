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
	"os"
	"os/signal"
	"strconv"
	"syscall"

	goredis "github.com/redis/go-redis/v9"

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
	defaultLogLevel = "info"
)

func main() {
	logger := buildLogger()

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

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

	logger.Info("starting arena-eval-worker",
		"namespace", cfg.Namespace,
		"redisAddr", cfg.RedisAddr,
		"sessionAPI", cfg.SessionAPIURL,
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
}

// loadConfig reads and validates environment variables.
func loadConfig() (*workerEnvConfig, error) {
	cfg := &workerEnvConfig{
		RedisAddr:     os.Getenv(envRedisAddr),
		RedisPassword: os.Getenv(envRedisPass),
		Namespace:     os.Getenv(envNamespace),
		SessionAPIURL: os.Getenv(envSessionAPI),
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

	if dbStr := os.Getenv(envRedisDB); dbStr != "" {
		db, err := strconv.Atoi(dbStr)
		if err != nil {
			return nil, fmt.Errorf("invalid %s value %q: %w", envRedisDB, dbStr, err)
		}
		cfg.RedisDB = db
	}

	return cfg, nil
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
