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
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/altairalabs/omnia/pkg/arena/queue"
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
	if cfg.ContentPath != "" {
		// Filesystem mode: content mounted directly
		fmt.Printf("  Content: %s (version: %s)\n", cfg.ContentPath, cfg.ContentVersion)
	} else {
		// Legacy mode: tar.gz download
		fmt.Printf("  Artifact: %s (rev: %s)\n", cfg.ArtifactURL, cfg.ArtifactRevision)
	}

	// Log tool registry overrides
	logToolOverrides(cfg)

	// Log provider credential overrides (detected from environment)
	logProviderOverrides()

	// Get bundle path (filesystem mode or download/extract)
	bundlePath, err := getBundlePath(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to get bundle: %w", err)
	}
	if cfg.ContentPath != "" {
		fmt.Printf("  Using mounted content at: %s\n", bundlePath)
	} else {
		fmt.Printf("  Bundle extracted to: %s\n", bundlePath)
	}

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
	defer func() { _ = q.Close() }()

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
