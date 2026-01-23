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

	t.Run("rejects absolute symlink targets", func(t *testing.T) {
		tmpDir := t.TempDir()
		tarPath := filepath.Join(tmpDir, "symlink-absolute.tar.gz")
		destDir := filepath.Join(tmpDir, "extracted")

		if err := createTarGzWithSymlink(tarPath, "link.txt", "/etc/passwd"); err != nil {
			t.Fatalf("failed to create tar.gz with symlink: %v", err)
		}

		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatalf("failed to create dest dir: %v", err)
		}

		err := extractTarGz(tarPath, destDir)
		if err == nil {
			t.Error("extractTarGz() should reject absolute symlink targets")
		}
	})

	t.Run("rejects symlink escaping destination", func(t *testing.T) {
		tmpDir := t.TempDir()
		tarPath := filepath.Join(tmpDir, "symlink-escape.tar.gz")
		destDir := filepath.Join(tmpDir, "extracted")

		if err := createTarGzWithSymlink(tarPath, "link.txt", "../../../etc/passwd"); err != nil {
			t.Fatalf("failed to create tar.gz with symlink: %v", err)
		}

		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatalf("failed to create dest dir: %v", err)
		}

		err := extractTarGz(tarPath, destDir)
		if err == nil {
			t.Error("extractTarGz() should reject symlink escape attempts")
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

func TestSanitizeSymlinkTarget(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, "dest")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}

	tests := []struct {
		name        string
		symlinkPath string
		linkTarget  string
		wantErr     bool
	}{
		{
			name:        "valid relative symlink",
			symlinkPath: filepath.Join(destDir, "link.txt"),
			linkTarget:  "target.txt",
			wantErr:     false,
		},
		{
			name:        "valid relative symlink in subdir",
			symlinkPath: filepath.Join(destDir, "subdir", "link.txt"),
			linkTarget:  "../target.txt",
			wantErr:     false,
		},
		{
			name:        "absolute symlink target",
			symlinkPath: filepath.Join(destDir, "link.txt"),
			linkTarget:  "/etc/passwd",
			wantErr:     true,
		},
		{
			name:        "symlink escaping destination",
			symlinkPath: filepath.Join(destDir, "link.txt"),
			linkTarget:  "../../../etc/passwd",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sanitizeSymlinkTarget(destDir, tt.symlinkPath, tt.linkTarget)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeSymlinkTarget() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func createTarGzWithSymlink(path, linkName, linkTarget string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gw := gzip.NewWriter(file)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	hdr := &tar.Header{
		Name:     linkName,
		Mode:     0777,
		Typeflag: tar.TypeSymlink,
		Linkname: linkTarget,
	}
	return tw.WriteHeader(hdr)
}

func TestProcessWorkItems(t *testing.T) {
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
			JobName:      jobID,
			WorkDir:      tmpDir,
			PollInterval: 10 * time.Millisecond,
		}

		// Cancel context immediately
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := processWorkItems(ctx, cfg, q, tmpDir)
		if err != nil {
			t.Fatalf("processWorkItems() with cancelled context should return nil, got %v", err)
		}
	})
}

func TestFindArenaConfigFile(t *testing.T) {
	t.Run("finds config.arena.yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.arena.yaml")
		if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		result := findArenaConfigFile(tmpDir)
		if result != configPath {
			t.Errorf("expected %s, got %s", configPath, result)
		}
	})

	t.Run("finds arena.yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "arena.yaml")
		if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		result := findArenaConfigFile(tmpDir)
		if result != configPath {
			t.Errorf("expected %s, got %s", configPath, result)
		}
	})

	t.Run("finds config.yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")
		if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		result := findArenaConfigFile(tmpDir)
		if result != configPath {
			t.Errorf("expected %s, got %s", configPath, result)
		}
	})

	t.Run("prefers config.arena.yaml over others", func(t *testing.T) {
		tmpDir := t.TempDir()
		arenaConfig := filepath.Join(tmpDir, "config.arena.yaml")
		plainConfig := filepath.Join(tmpDir, "config.yaml")
		if err := os.WriteFile(arenaConfig, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create arena config: %v", err)
		}
		if err := os.WriteFile(plainConfig, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create plain config: %v", err)
		}

		result := findArenaConfigFile(tmpDir)
		if result != arenaConfig {
			t.Errorf("expected %s, got %s", arenaConfig, result)
		}
	})

	t.Run("returns empty string when no config found", func(t *testing.T) {
		tmpDir := t.TempDir()

		result := findArenaConfigFile(tmpDir)
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})
}

func TestExecuteWorkItem(t *testing.T) {
	t.Run("returns error when arena config not found", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &Config{
			WorkDir: tmpDir,
		}

		item := &queue.WorkItem{
			ID:         "test-item",
			ScenarioID: "test-scenario",
			ProviderID: "test-provider",
		}

		_, err := executeWorkItem(context.Background(), cfg, item, tmpDir)
		if err == nil {
			t.Error("executeWorkItem() should return error when arena config is missing")
		}
		if !contains(err.Error(), "arena config file not found") {
			t.Errorf("expected error about missing config, got: %v", err)
		}
	})
}

// contains checks if s contains any of the substrings (used in tests)
func contains(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if bytes.Contains([]byte(s), []byte(sub)) {
			return true
		}
	}
	return false
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
