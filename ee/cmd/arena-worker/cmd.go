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

	"github.com/AltairaLabs/PromptKit/runtime/logger"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/pkg/logging"
)

func main() {
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
		"executionMode", cfg.ExecutionMode,
	)

	if cfg.ExecutionMode == executionModeFleet {
		log.Info("fleet mode configured", "wsURL", cfg.FleetWSURL)
	}

	// Configure PromptKit SDK logging via slog bridge
	configureSDKLogging(cfg, sdkLogger)

	// Log override config if present
	if cfg.OverridesPath != "" {
		logOverrideConfig(log, cfg.OverridesPath)
	} else {
		logToolOverrides(log, cfg)
	}

	// Log provider credential overrides (detected from environment)
	logProviderOverrides(log)

	// Get content path (mounted from PVC)
	bundlePath, err := getContentPath(cfg)
	if err != nil {
		return fmt.Errorf("failed to get content path: %w", err)
	}
	log.V(1).Info("content path resolved", "bundlePath", bundlePath)

	// Connect to Redis queue
	q, err := queue.NewRedisQueue(queue.RedisOptions{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
		Options:  queue.DefaultOptions(),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to queue: %w", err)
	}
	defer func() {
		if closeErr := q.Close(); closeErr != nil {
			log.Error(closeErr, "failed to close queue")
		}
	}()

	log.Info("connected to redis", "addr", cfg.RedisAddr)

	// Process work items
	return processWorkItems(ctx, log, cfg, q, bundlePath)
}

// configureSDKLogging sets up PromptKit SDK logging via the slog bridge.
func configureSDKLogging(cfg *Config, sdkLogger *slog.Logger) {
	logger.SetLogger(sdkLogger)
	if cfg.Verbose {
		logger.SetVerbose(true)
	}
}

// logToolOverrides logs tool registry overrides that will be used.
func logToolOverrides(log logr.Logger, cfg *Config) {
	if len(cfg.ToolOverrides) == 0 {
		log.V(1).Info("tool overrides", "count", 0)
		return
	}

	log.Info("tool overrides loaded", "count", len(cfg.ToolOverrides))
	for name, override := range cfg.ToolOverrides {
		log.V(1).Info("tool override",
			"tool", name,
			"registry", override.RegistryName,
			"handler", override.HandlerName,
			"endpoint", override.Endpoint,
			"handlerType", override.HandlerType,
		)
	}
}

// logOverrideConfig logs details about the override config loaded from ConfigMap.
func logOverrideConfig(log logr.Logger, path string) {
	cfg, err := loadOverrides(path)
	if err != nil {
		log.Error(err, "failed to load override config", "path", path)
		return
	}
	if cfg == nil {
		log.V(1).Info("override config not found", "path", path)
		return
	}

	// Count providers across all groups
	totalProviders := 0
	for group, providers := range cfg.Providers {
		log.V(1).Info("provider group loaded", "group", group, "count", len(providers))
		for _, p := range providers {
			totalProviders++
			hasCreds := p.SecretEnvVar != "" && os.Getenv(p.SecretEnvVar) != ""
			log.V(1).Info("provider override",
				"providerID", p.ID,
				"providerType", p.Type,
				"model", p.Model,
				"group", group,
				"hasCreds", hasCreds,
			)
		}
	}

	toolCount := len(cfg.Tools)
	log.Info("override config loaded",
		"path", path,
		"providerCount", totalProviders,
		"toolCount", toolCount,
	)

	for _, t := range cfg.Tools {
		log.V(1).Info("tool override", "tool", t.Name, "endpoint", t.Endpoint)
	}
}

// logProviderOverrides logs provider credential overrides detected from environment.
func logProviderOverrides(log logr.Logger) {
	// Known provider credential environment variables
	providerEnvVars := map[string]string{
		"OPENAI_API_KEY":      "OpenAI",
		"ANTHROPIC_API_KEY":   "Anthropic",
		"AZURE_OPENAI_KEY":    "Azure OpenAI",
		"GOOGLE_API_KEY":      "Google AI",
		"COHERE_API_KEY":      "Cohere",
		"MISTRAL_API_KEY":     "Mistral",
		"AWS_ACCESS_KEY_ID":   "AWS Bedrock",
		"GROQ_API_KEY":        "Groq",
		"TOGETHER_API_KEY":    "Together AI",
		"FIREWORKS_API_KEY":   "Fireworks",
		"DEEPSEEK_API_KEY":    "DeepSeek",
		"REPLICATE_API_TOKEN": "Replicate",
	}

	var detected []string
	for envVar, provider := range providerEnvVars {
		if os.Getenv(envVar) != "" {
			detected = append(detected, provider)
		}
	}

	log.Info("provider credentials detected", "count", len(detected), "providers", detected)
}
