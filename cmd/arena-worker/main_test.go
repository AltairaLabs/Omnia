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
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altairalabs/omnia/pkg/arena/queue"
)

const testContent = "test content"

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		want         string
	}{
		{
			name:         "returns default when env not set",
			key:          "TEST_NOT_SET",
			defaultValue: "default",
			envValue:     "",
			want:         "default",
		},
		{
			name:         "returns env value when set",
			key:          "TEST_SET",
			defaultValue: "default",
			envValue:     "custom",
			want:         "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			}

			got := getEnvOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDurationEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue time.Duration
		envValue     string
		want         time.Duration
	}{
		{
			name:         "returns default when env not set",
			key:          "TEST_DURATION_NOT_SET",
			defaultValue: 5 * time.Second,
			envValue:     "",
			want:         5 * time.Second,
		},
		{
			name:         "returns parsed duration when set",
			key:          "TEST_DURATION_SET",
			defaultValue: 5 * time.Second,
			envValue:     "10s",
			want:         10 * time.Second,
		},
		{
			name:         "returns default on invalid duration",
			key:          "TEST_DURATION_INVALID",
			defaultValue: 5 * time.Second,
			envValue:     "invalid",
			want:         5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			}

			got := getDurationEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getDurationEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("returns error when ARENA_JOB_NAME is missing", func(t *testing.T) {
		// Clear required env vars
		t.Setenv("ARENA_JOB_NAME", "")
		t.Setenv("ARENA_ARTIFACT_URL", "")

		_, err := loadConfig()
		if err == nil {
			t.Error("loadConfig() should return error when ARENA_JOB_NAME is missing")
		}
	})

	t.Run("returns error when ARENA_ARTIFACT_URL is missing", func(t *testing.T) {
		t.Setenv("ARENA_JOB_NAME", "test-job")
		t.Setenv("ARENA_ARTIFACT_URL", "")

		_, err := loadConfig()
		if err == nil {
			t.Error("loadConfig() should return error when ARENA_ARTIFACT_URL is missing")
		}
	})

	t.Run("returns config when required fields are set", func(t *testing.T) {
		t.Setenv("ARENA_JOB_NAME", "test-job")
		t.Setenv("ARENA_ARTIFACT_URL", "http://example.com/bundle.tar.gz")

		cfg, err := loadConfig()
		if err != nil {
			t.Errorf("loadConfig() error = %v", err)
		}
		if cfg.JobName != "test-job" {
			t.Errorf("JobName = %v, want test-job", cfg.JobName)
		}
		if cfg.ArtifactURL != "http://example.com/bundle.tar.gz" {
			t.Errorf("ArtifactURL = %v, want http://example.com/bundle.tar.gz", cfg.ArtifactURL)
		}
		// Check defaults
		if cfg.RedisAddr != "redis:6379" {
			t.Errorf("RedisAddr = %v, want redis:6379", cfg.RedisAddr)
		}
	})
}

func TestDownloadFile(t *testing.T) {
	t.Run("downloads file successfully", func(t *testing.T) {
		content := testContent
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(content))
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		destPath := filepath.Join(tmpDir, "downloaded.txt")

		err := downloadFile(context.Background(), server.URL, destPath)
		if err != nil {
			t.Fatalf("downloadFile() error = %v", err)
		}

		got, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("failed to read downloaded file: %v", err)
		}
		if string(got) != content {
			t.Errorf("downloaded content = %v, want %v", string(got), content)
		}
	})

	t.Run("returns error on non-200 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		destPath := filepath.Join(tmpDir, "downloaded.txt")

		err := downloadFile(context.Background(), server.URL, destPath)
		if err == nil {
			t.Error("downloadFile() should return error on 404")
		}
	})
}

func TestExtractTarGz(t *testing.T) {
	t.Run("extracts tar.gz successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		tarPath := filepath.Join(tmpDir, "test.tar.gz")
		destDir := filepath.Join(tmpDir, "extracted")

		// Create test tar.gz
		if err := createTestTarGz(tarPath); err != nil {
			t.Fatalf("failed to create test tar.gz: %v", err)
		}

		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatalf("failed to create dest dir: %v", err)
		}

		err := extractTarGz(tarPath, destDir)
		if err != nil {
			t.Fatalf("extractTarGz() error = %v", err)
		}

		// Verify extracted file exists
		extractedFile := filepath.Join(destDir, "test.txt")
		if _, err := os.Stat(extractedFile); os.IsNotExist(err) {
			t.Error("expected file was not extracted")
		}

		// Verify content
		content, err := os.ReadFile(extractedFile)
		if err != nil {
			t.Fatalf("failed to read extracted file: %v", err)
		}
		if string(content) != testContent {
			t.Errorf("extracted content = %v, want %q", string(content), testContent)
		}
	})

	t.Run("rejects path traversal attempts", func(t *testing.T) {
		tmpDir := t.TempDir()
		tarPath := filepath.Join(tmpDir, "malicious.tar.gz")
		destDir := filepath.Join(tmpDir, "extracted")

		// Create malicious tar.gz with path traversal
		if err := createMaliciousTarGz(tarPath); err != nil {
			t.Fatalf("failed to create malicious tar.gz: %v", err)
		}

		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatalf("failed to create dest dir: %v", err)
		}

		err := extractTarGz(tarPath, destDir)
		if err == nil {
			t.Error("extractTarGz() should reject path traversal")
		}
	})
}

func createTestTarGz(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gw := gzip.NewWriter(file)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	content := []byte(testContent)
	hdr := &tar.Header{
		Name: "test.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}

	return nil
}

func createMaliciousTarGz(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gw := gzip.NewWriter(file)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	content := []byte("malicious content")
	hdr := &tar.Header{
		Name: "../../../etc/passwd",
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}

	return nil
}

func TestProcessWorkItems(t *testing.T) {
	t.Run("processes items from queue until empty", func(t *testing.T) {
		q := queue.NewMemoryQueueWithDefaults()
		jobID := "test-job"

		// Push work items
		items := []queue.WorkItem{
			{ID: "item-1", ScenarioID: "scenario1", ProviderID: "provider1"},
			{ID: "item-2", ScenarioID: "scenario2", ProviderID: "provider1"},
		}
		if err := q.Push(context.Background(), jobID, items); err != nil {
			t.Fatalf("failed to push items: %v", err)
		}

		// Create a mock executable that returns valid JSON
		tmpDir := t.TempDir()
		mockBin := createMockPromptArena(t, tmpDir, `{"status":"pass","durationMs":100}`)

		cfg := &Config{
			JobName:        jobID,
			WorkDir:        tmpDir,
			PromptArenaBin: mockBin,
			PollInterval:   10 * time.Millisecond,
		}

		// Create a context that will cancel after short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := processWorkItems(ctx, cfg, q, tmpDir)
		if err != nil {
			t.Fatalf("processWorkItems() error = %v", err)
		}

		// Verify all items were processed
		progress, err := q.Progress(context.Background(), jobID)
		if err != nil {
			t.Fatalf("failed to get progress: %v", err)
		}
		if progress.Completed != 2 {
			t.Errorf("expected 2 completed items, got %d", progress.Completed)
		}
		if progress.Pending != 0 {
			t.Errorf("expected 0 pending items, got %d", progress.Pending)
		}
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		q := queue.NewMemoryQueueWithDefaults()
		jobID := "test-job-cancel"

		// Push a work item
		items := []queue.WorkItem{
			{ID: "item-1", ScenarioID: "scenario1", ProviderID: "provider1"},
		}
		if err := q.Push(context.Background(), jobID, items); err != nil {
			t.Fatalf("failed to push items: %v", err)
		}

		tmpDir := t.TempDir()
		cfg := &Config{
			JobName:        jobID,
			WorkDir:        tmpDir,
			PromptArenaBin: "nonexistent-binary",
			PollInterval:   10 * time.Millisecond,
		}

		// Cancel context immediately
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := processWorkItems(ctx, cfg, q, tmpDir)
		if err != nil {
			t.Fatalf("processWorkItems() with cancelled context should return nil, got %v", err)
		}
	})

	t.Run("handles failed items", func(t *testing.T) {
		q := queue.NewMemoryQueueWithDefaults()
		jobID := "test-job-fail"

		// Push work items
		items := []queue.WorkItem{
			{ID: "item-fail", ScenarioID: "scenario1", ProviderID: "provider1", MaxAttempts: 1},
		}
		if err := q.Push(context.Background(), jobID, items); err != nil {
			t.Fatalf("failed to push items: %v", err)
		}

		tmpDir := t.TempDir()
		// Create a mock that always exits with error
		mockBin := createMockPromptArena(t, tmpDir, "")

		cfg := &Config{
			JobName:        jobID,
			WorkDir:        tmpDir,
			PromptArenaBin: mockBin + "-nonexistent", // Force failure
			PollInterval:   10 * time.Millisecond,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := processWorkItems(ctx, cfg, q, tmpDir)
		if err != nil {
			t.Fatalf("processWorkItems() error = %v", err)
		}

		// Verify item was marked as failed
		progress, err := q.Progress(context.Background(), jobID)
		if err != nil {
			t.Fatalf("failed to get progress: %v", err)
		}
		if progress.Failed != 1 {
			t.Errorf("expected 1 failed item, got %d", progress.Failed)
		}
	})
}

func TestExecuteWorkItem(t *testing.T) {
	t.Run("executes successfully with JSON output", func(t *testing.T) {
		tmpDir := t.TempDir()
		mockBin := createMockPromptArena(t, tmpDir, `{"status":"pass","durationMs":150,"metrics":{"tokens":100}}`)

		cfg := &Config{
			WorkDir:        tmpDir,
			PromptArenaBin: mockBin,
		}

		item := &queue.WorkItem{
			ID:         "test-item",
			ScenarioID: "test-scenario",
			ProviderID: "test-provider",
		}

		result, err := executeWorkItem(context.Background(), cfg, item, tmpDir)
		if err != nil {
			t.Fatalf("executeWorkItem() error = %v", err)
		}

		if result.Status != statusPass {
			t.Errorf("expected status 'pass', got '%s'", result.Status)
		}
	})

	t.Run("handles non-JSON output", func(t *testing.T) {
		tmpDir := t.TempDir()
		mockBin := createMockPromptArena(t, tmpDir, "not valid json")

		cfg := &Config{
			WorkDir:        tmpDir,
			PromptArenaBin: mockBin,
		}

		item := &queue.WorkItem{
			ID:         "test-item",
			ScenarioID: "test-scenario",
			ProviderID: "test-provider",
		}

		result, err := executeWorkItem(context.Background(), cfg, item, tmpDir)
		if err != nil {
			t.Fatalf("executeWorkItem() error = %v", err)
		}

		// Non-JSON output should be treated as pass
		if result.Status != statusPass {
			t.Errorf("expected status 'pass' for non-JSON output, got '%s'", result.Status)
		}
	})

	t.Run("handles binary not found", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &Config{
			WorkDir:        tmpDir,
			PromptArenaBin: "/nonexistent/binary",
		}

		item := &queue.WorkItem{
			ID:         "test-item",
			ScenarioID: "test-scenario",
			ProviderID: "test-provider",
		}

		_, err := executeWorkItem(context.Background(), cfg, item, tmpDir)
		if err == nil {
			t.Error("executeWorkItem() should return error for missing binary")
		}
	})

	t.Run("handles item with config", func(t *testing.T) {
		tmpDir := t.TempDir()
		mockBin := createMockPromptArena(t, tmpDir, `{"status":"pass"}`)

		cfg := &Config{
			WorkDir:        tmpDir,
			PromptArenaBin: mockBin,
		}

		item := &queue.WorkItem{
			ID:         "test-item-config",
			ScenarioID: "test-scenario",
			ProviderID: "test-provider",
			Config:     []byte(`{"key":"value"}`),
		}

		result, err := executeWorkItem(context.Background(), cfg, item, tmpDir)
		if err != nil {
			t.Fatalf("executeWorkItem() error = %v", err)
		}

		if result.Status != statusPass {
			t.Errorf("expected status 'pass', got '%s'", result.Status)
		}

		// Verify config file was created and cleaned up
		configPath := filepath.Join(tmpDir, "config-test-item-config.json")
		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Error("config file should be cleaned up after execution")
		}
	})

	t.Run("handles command exit error", func(t *testing.T) {
		tmpDir := t.TempDir()
		mockBin := createFailingMockPromptArena(t, tmpDir)

		cfg := &Config{
			WorkDir:        tmpDir,
			PromptArenaBin: mockBin,
		}

		item := &queue.WorkItem{
			ID:         "test-item-exit",
			ScenarioID: "test-scenario",
			ProviderID: "test-provider",
		}

		result, err := executeWorkItem(context.Background(), cfg, item, tmpDir)
		if err != nil {
			t.Fatalf("executeWorkItem() should return result for exit error, got error: %v", err)
		}

		if result.Status != statusFail {
			t.Errorf("expected status 'fail' for exit error, got '%s'", result.Status)
		}
	})
}

func TestDownloadAndExtract(t *testing.T) {
	t.Run("downloads and extracts artifact", func(t *testing.T) {
		// Create a test tar.gz and serve it
		tmpDir := t.TempDir()
		tarPath := filepath.Join(tmpDir, "bundle.tar.gz")
		if err := createTestTarGz(tarPath); err != nil {
			t.Fatalf("failed to create test tar.gz: %v", err)
		}

		tarContent, err := os.ReadFile(tarPath)
		if err != nil {
			t.Fatalf("failed to read tar.gz: %v", err)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(tarContent)
		}))
		defer server.Close()

		workDir := filepath.Join(tmpDir, "work")
		cfg := &Config{
			ArtifactURL: server.URL,
			WorkDir:     workDir,
		}

		bundlePath, err := downloadAndExtract(context.Background(), cfg)
		if err != nil {
			t.Fatalf("downloadAndExtract() error = %v", err)
		}

		// Verify extracted file exists
		extractedFile := filepath.Join(bundlePath, "test.txt")
		if _, err := os.Stat(extractedFile); os.IsNotExist(err) {
			t.Error("expected file was not extracted")
		}
	})
}

func TestExtractRegularFile(t *testing.T) {
	t.Run("extracts file with correct permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		target := filepath.Join(tmpDir, "subdir", "test.txt")
		content := testContent

		// Create a tar reader with our content
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		hdr := &tar.Header{
			Name: "test.txt",
			Mode: 0755,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write content: %v", err)
		}
		if err := tw.Close(); err != nil {
			t.Fatalf("failed to close tar writer: %v", err)
		}

		tr := tar.NewReader(&buf)
		_, err := tr.Next()
		if err != nil {
			t.Fatalf("failed to read header: %v", err)
		}

		err = extractRegularFile(target, tr, 0755)
		if err != nil {
			t.Fatalf("extractRegularFile() error = %v", err)
		}

		// Verify file exists and has content
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("failed to read extracted file: %v", err)
		}
		if string(got) != content {
			t.Errorf("content = %q, want %q", string(got), content)
		}
	})
}

// createMockPromptArena creates a mock promptarena binary that outputs the given JSON
func createMockPromptArena(t *testing.T, dir, output string) string {
	t.Helper()

	// Create a simple shell script that outputs the JSON
	scriptPath := filepath.Join(dir, "mock-promptarena.sh")
	script := fmt.Sprintf("#!/bin/sh\necho '%s'", output)

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	return scriptPath
}

// createFailingMockPromptArena creates a mock that exits with error
func createFailingMockPromptArena(t *testing.T, dir string) string {
	t.Helper()

	scriptPath := filepath.Join(dir, "mock-promptarena-fail.sh")
	script := "#!/bin/sh\necho 'error message' >&2\nexit 1"

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	return scriptPath
}
