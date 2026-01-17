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
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/altairalabs/omnia/pkg/arena/queue"
)

// Status constants for execution results.
const (
	statusPass = "pass"
	statusFail = "fail"
)

// Config holds the worker configuration from environment variables.
type Config struct {
	// Job identification
	JobName      string
	JobNamespace string
	ConfigName   string
	JobType      string

	// Artifact configuration
	ArtifactURL      string
	ArtifactRevision string

	// Redis configuration
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Worker configuration
	WorkDir        string
	PromptArenaBin string
	PollInterval   time.Duration
	ShutdownDelay  time.Duration
}

// ExecutionResult represents the result of running a scenario.
type ExecutionResult struct {
	Status     string             `json:"status"`
	DurationMs float64            `json:"durationMs"`
	Error      string             `json:"error,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Assertions []AssertionResult  `json:"assertions,omitempty"`
}

// AssertionResult represents a single assertion result.
type AssertionResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

func loadConfig() (*Config, error) {
	cfg := &Config{
		JobName:          os.Getenv("ARENA_JOB_NAME"),
		JobNamespace:     os.Getenv("ARENA_JOB_NAMESPACE"),
		ConfigName:       os.Getenv("ARENA_CONFIG_NAME"),
		JobType:          os.Getenv("ARENA_JOB_TYPE"),
		ArtifactURL:      os.Getenv("ARENA_ARTIFACT_URL"),
		ArtifactRevision: os.Getenv("ARENA_ARTIFACT_REVISION"),
		RedisAddr:        getEnvOrDefault("REDIS_ADDR", "redis:6379"),
		RedisPassword:    os.Getenv("REDIS_PASSWORD"),
		RedisDB:          0,
		WorkDir:          getEnvOrDefault("ARENA_WORK_DIR", "/tmp/arena"),
		PromptArenaBin:   getEnvOrDefault("PROMPTARENA_BIN", "promptarena"),
		PollInterval:     getDurationEnv("ARENA_POLL_INTERVAL", 100*time.Millisecond),
		ShutdownDelay:    getDurationEnv("ARENA_SHUTDOWN_DELAY", 5*time.Second),
	}

	if cfg.JobName == "" {
		return nil, errors.New("ARENA_JOB_NAME is required")
	}
	if cfg.ArtifactURL == "" {
		return nil, errors.New("ARENA_ARTIFACT_URL is required")
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}

func downloadAndExtract(ctx context.Context, cfg *Config) (string, error) {
	// Create work directory
	bundleDir := filepath.Join(cfg.WorkDir, "bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create work directory: %w", err)
	}

	// Download artifact
	tarPath := filepath.Join(cfg.WorkDir, "bundle.tar.gz")
	if err := downloadFile(ctx, cfg.ArtifactURL, tarPath); err != nil {
		return "", fmt.Errorf("failed to download artifact: %w", err)
	}

	// Extract tarball
	if err := extractTarGz(tarPath, bundleDir); err != nil {
		return "", fmt.Errorf("failed to extract artifact: %w", err)
	}

	// Clean up tarball (ignore error, non-critical)
	_ = os.Remove(tarPath)

	return bundleDir, nil
}

func downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractTarGz(tarPath, destDir string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Sanitize path to prevent directory traversal
		target := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := extractRegularFile(target, tr, header.Mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Validate symlink target to prevent symlink escape attacks
			// Reject absolute symlinks
			if filepath.IsAbs(header.Linkname) {
				return fmt.Errorf("invalid symlink target (absolute path): %s", header.Linkname)
			}
			// Resolve symlink target relative to the symlink's directory
			linkDir := filepath.Dir(target)
			resolvedLink := filepath.Join(linkDir, header.Linkname)
			resolvedLink = filepath.Clean(resolvedLink)
			// Verify resolved path stays within destDir
			if !strings.HasPrefix(resolvedLink, filepath.Clean(destDir)+string(os.PathSeparator)) &&
				resolvedLink != filepath.Clean(destDir) {
				return fmt.Errorf("invalid symlink target (escapes destination): %s -> %s",
					header.Name, header.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}

	return nil
}

func extractRegularFile(target string, tr *tar.Reader, mode int64) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return err
	}
	defer func() { _ = outFile.Close() }()

	_, err = io.Copy(outFile, tr)
	return err
}

func processWorkItems(ctx context.Context, cfg *Config, q queue.WorkQueue, bundlePath string) error {
	jobID := cfg.JobName
	emptyCount := 0
	maxEmptyPolls := 10 // Exit after this many consecutive empty polls

	fmt.Printf("Processing work items for job: %s\n", jobID)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Shutdown signal received, exiting...")
			return nil
		default:
		}

		// Pop next work item
		item, err := q.Pop(ctx, jobID)
		if err != nil {
			if errors.Is(err, queue.ErrQueueEmpty) {
				emptyCount++
				if emptyCount >= maxEmptyPolls {
					fmt.Printf("Queue empty after %d polls, checking if job is complete...\n", emptyCount)

					// Check job progress
					progress, err := q.Progress(ctx, jobID)
					if err != nil {
						return fmt.Errorf("failed to get job progress: %w", err)
					}

					if progress.IsComplete() {
						fmt.Printf("Job complete: %d/%d items processed\n",
							progress.Completed+progress.Failed, progress.Total)
						return nil
					}

					// Items still processing, wait and retry
					emptyCount = 0
				}

				time.Sleep(cfg.PollInterval)
				continue
			}
			return fmt.Errorf("failed to pop work item: %w", err)
		}

		emptyCount = 0 // Reset on successful pop

		// Execute the work item
		result, execErr := executeWorkItem(ctx, cfg, item, bundlePath)

		// Report result
		if execErr != nil {
			fmt.Printf("  [FAIL] %s: %v\n", item.ID, execErr)
			if err := q.Nack(ctx, jobID, item.ID, execErr); err != nil {
				fmt.Printf("  Warning: failed to nack item %s: %v\n", item.ID, err)
			}
		} else {
			resultJSON, _ := json.Marshal(result)
			fmt.Printf("  [%s] %s (%.0fms)\n", result.Status, item.ID, result.DurationMs)
			if err := q.Ack(ctx, jobID, item.ID, resultJSON); err != nil {
				fmt.Printf("  Warning: failed to ack item %s: %v\n", item.ID, err)
			}
		}
	}
}

func executeWorkItem(
	ctx context.Context,
	cfg *Config,
	item *queue.WorkItem,
	bundlePath string,
) (*ExecutionResult, error) {
	start := time.Now()

	// Build command arguments
	args := []string{
		"run",
		"--bundle", bundlePath,
		"--scenario", item.ScenarioID,
		"--provider", item.ProviderID,
		"--output-format", "json",
	}

	// Add config if present
	if len(item.Config) > 0 {
		configPath := filepath.Join(cfg.WorkDir, fmt.Sprintf("config-%s.json", item.ID))
		if err := os.WriteFile(configPath, item.Config, 0644); err != nil {
			return nil, fmt.Errorf("failed to write config: %w", err)
		}
		args = append(args, "--config", configPath)
		defer func() { _ = os.Remove(configPath) }()
	}

	// Execute promptarena
	cmd := exec.CommandContext(ctx, cfg.PromptArenaBin, args...)
	cmd.Dir = bundlePath

	output, err := cmd.Output()
	duration := time.Since(start)

	result := &ExecutionResult{
		DurationMs: float64(duration.Milliseconds()),
		Metrics:    make(map[string]float64),
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.Status = statusFail
			result.Error = string(exitErr.Stderr)
			return result, nil
		}
		return nil, fmt.Errorf("failed to execute promptarena: %w", err)
	}

	// Parse output JSON
	if len(output) > 0 {
		if err := json.Unmarshal(output, result); err != nil {
			// If output isn't valid JSON, treat as pass with raw output
			result.Status = statusPass
		}
	} else {
		result.Status = statusPass
	}

	if result.Status == "" {
		result.Status = statusPass
	}

	return result, nil
}
