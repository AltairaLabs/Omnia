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
	"os"
	"os/signal"
	"syscall"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
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
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Arena Worker starting\n")
	fmt.Printf("  Job: %s/%s\n", cfg.JobNamespace, cfg.JobName)
	fmt.Printf("  Type: %s\n", cfg.JobType)
	fmt.Printf("  Content: %s (version: %s)\n", cfg.ContentPath, cfg.ContentVersion)

	if cfg.ExecutionMode == executionModeFleet {
		fmt.Printf("  Execution mode: fleet (target: %s)\n", cfg.FleetWSURL)
	} else {
		fmt.Printf("  Execution mode: direct\n")
	}

	// Log override config if present
	if cfg.OverridesPath != "" {
		logOverrideConfig(cfg.OverridesPath)
	} else {
		// Log legacy tool registry overrides (deprecated)
		logToolOverrides(cfg)
	}

	// Log provider credential overrides (detected from environment)
	logProviderOverrides()

	// Get content path (mounted from PVC)
	bundlePath, err := getContentPath(cfg)
	if err != nil {
		return fmt.Errorf("failed to get content path: %w", err)
	}
	fmt.Printf("  Using mounted content at: %s\n", bundlePath)

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
		if err := q.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close queue: %v\n", err)
		}
	}()

	fmt.Printf("  Connected to Redis at %s\n", cfg.RedisAddr)

	// Process work items
	return processWorkItems(ctx, cfg, q, bundlePath)
}

// logToolOverrides logs tool registry overrides that will be used.
func logToolOverrides(cfg *Config) {
	if len(cfg.ToolOverrides) == 0 {
		fmt.Printf("  Tool overrides: none\n")
		return
	}

	fmt.Printf("  Tool overrides: %d tool(s) from ToolRegistry CRDs\n", len(cfg.ToolOverrides))
	for name, override := range cfg.ToolOverrides {
		fmt.Printf("    - %s: registry=%s handler=%s", name, override.RegistryName, override.HandlerName)
		if override.Endpoint != "" {
			fmt.Printf(" endpoint=%s", override.Endpoint)
		}
		if override.HandlerType != "" {
			fmt.Printf(" type=%s", override.HandlerType)
		}
		fmt.Printf("\n")
	}
}

// logOverrideConfig logs details about the override config loaded from ConfigMap.
func logOverrideConfig(path string) {
	cfg, err := loadOverrides(path)
	if err != nil {
		fmt.Printf("  Override config: error loading from %s: %v\n", path, err)
		return
	}
	if cfg == nil {
		fmt.Printf("  Override config: none (file not found at %s)\n", path)
		return
	}

	fmt.Printf("  Override config: %s\n", path)

	// Log provider overrides by group
	totalProviders := 0
	for group, providers := range cfg.Providers {
		fmt.Printf("    Provider group '%s': %d provider(s)\n", group, len(providers))
		for _, p := range providers {
			totalProviders++
			credStatus := "no credentials required"
			if p.SecretEnvVar != "" {
				if os.Getenv(p.SecretEnvVar) != "" {
					credStatus = fmt.Sprintf("✓ %s set", p.SecretEnvVar)
				} else {
					credStatus = fmt.Sprintf("✗ %s MISSING", p.SecretEnvVar)
				}
			}
			model := p.Model
			if model == "" {
				model = "default"
			}
			fmt.Printf("      - %s (%s/%s) [%s]\n", p.ID, p.Type, model, credStatus)
		}
	}
	if totalProviders > 0 {
		fmt.Printf("    Total override providers: %d\n", totalProviders)
	}

	// Log tool overrides
	if len(cfg.Tools) > 0 {
		fmt.Printf("    Tool overrides: %d tool(s)\n", len(cfg.Tools))
		for _, t := range cfg.Tools {
			fmt.Printf("      - %s -> %s\n", t.Name, t.Endpoint)
		}
	}
}

// logProviderOverrides logs provider credential overrides detected from environment.
// Provider overrides are resolved by the controller and passed as environment variables.
func logProviderOverrides() {
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

	if len(detected) == 0 {
		fmt.Printf("  Provider credentials: none detected\n")
		return
	}

	fmt.Printf("  Provider credentials: %d provider(s) configured\n", len(detected))
	for _, provider := range detected {
		fmt.Printf("    - %s\n", provider)
	}
}
