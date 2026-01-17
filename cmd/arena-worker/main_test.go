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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
		content := "test content"
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
		if string(content) != "test content" {
			t.Errorf("extracted content = %v, want 'test content'", string(content))
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

	content := []byte("test content")
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
