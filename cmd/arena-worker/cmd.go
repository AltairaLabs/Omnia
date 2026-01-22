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
