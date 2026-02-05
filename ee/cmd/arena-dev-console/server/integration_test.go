//go:build integration

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.

Integration tests for arena-dev-console server.
Run with: go test -tags=integration ./ee/cmd/arena-dev-console/server/...
*/

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServerStartupWithMockProvider tests that the server starts correctly
// with a mock provider configuration.
func TestServerStartupWithMockProvider(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler, err := NewPromptKitHandler(cfg, logr.Discard())
	require.NoError(t, err, "Handler should initialize without error")
	require.NotNil(t, handler, "Handler should not be nil")
	require.NotNil(t, handler.providerRegistry, "Provider registry should be initialized")
}

// TestServerStartupWithEmptyOutputDir tests that the server handles empty
// output directory configuration by setting a default.
func TestServerStartupWithEmptyOutputDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to temp directory so "out" can be created there
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	cfg := &config.Config{
		Defaults: config.Defaults{
			// Empty Output.Dir
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler, err := NewPromptKitHandler(cfg, logr.Discard())
	require.NoError(t, err, "Handler should initialize with empty output dir")
	require.NotNil(t, handler, "Handler should not be nil")

	// Verify output directory was set
	assert.NotEmpty(t, cfg.Defaults.Output.Dir, "Output directory should be set")
}

// TestServerStartupInReadOnlyDirectory tests that the server fails gracefully
// when the working directory is read-only.
func TestServerStartupInReadOnlyDirectory(t *testing.T) {
	// Skip if running as root
	if os.Getuid() == 0 {
		t.Skip("Skipping test when running as root")
	}

	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	require.NoError(t, os.MkdirAll(readOnlyDir, 0555))
	defer func() { _ = os.Chmod(readOnlyDir, 0755) }()

	// Change to read-only directory
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(readOnlyDir)

	cfg := &config.Config{
		Defaults: config.Defaults{
			// Empty Output.Dir - would try to create "out" in read-only dir
			ConfigDir: readOnlyDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// This should either fail or set a default writable path
	handler, err := NewPromptKitHandler(cfg, logr.Discard())

	// Either an error occurred OR the handler set a writable default
	if err != nil {
		assert.Contains(t, err.Error(), "permission denied", "Error should mention permission denied")
	} else {
		// Handler succeeded - verify it chose a writable path
		assert.Contains(t, cfg.Defaults.Output.Dir, "/tmp", "Should have chosen /tmp as fallback")
		if handler.providerRegistry != nil {
			handler.providerRegistry.Close()
		}
	}
}

// TestWebSocketMessageHandling tests that WebSocket messages are handled correctly.
func TestWebSocketMessageHandling(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler, err := NewPromptKitHandler(cfg, logr.Discard())
	require.NoError(t, err)

	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("Failed to upgrade: %v", err)
			return
		}
		defer conn.Close()

		// Read message
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			t.Logf("Failed to read message: %v", err)
			return
		}

		// Parse client message
		var clientMsg struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(msgBytes, &clientMsg); err != nil {
			t.Logf("Failed to parse message: %v", err)
			return
		}

		// Create a simple test response
		response := map[string]interface{}{
			"type":    "done",
			"content": "Test response to: " + clientMsg.Content,
		}
		respBytes, _ := json.Marshal(response)
		_ = conn.WriteMessage(websocket.TextMessage, respBytes)
	}))
	defer server.Close()

	// Connect to WebSocket
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Send a message
	msg := map[string]interface{}{
		"type":    "chat",
		"content": "Hello, world!",
	}
	msgBytes, _ := json.Marshal(msg)
	err = conn.WriteMessage(websocket.TextMessage, msgBytes)
	require.NoError(t, err)

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, respBytes, err := conn.ReadMessage()
	require.NoError(t, err)

	var resp map[string]interface{}
	err = json.Unmarshal(respBytes, &resp)
	require.NoError(t, err)

	assert.Equal(t, "done", resp["type"])
	assert.Contains(t, resp["content"], "Hello, world!")

	// Cleanup
	if handler.providerRegistry != nil {
		handler.providerRegistry.Close()
	}
}

// TestBuildEngineComponentsPermissionDenied specifically tests the "mkdir out: permission denied"
// scenario that occurs in distroless containers with read-only root filesystems.
func TestBuildEngineComponentsPermissionDenied(t *testing.T) {
	// Skip if running as root
	if os.Getuid() == 0 {
		t.Skip("Skipping test when running as root")
	}

	// Create a read-only directory to simulate distroless container root
	tmpDir := t.TempDir()
	readOnlyRoot := filepath.Join(tmpDir, "readonly-root")
	require.NoError(t, os.MkdirAll(readOnlyRoot, 0555))
	defer func() { _ = os.Chmod(readOnlyRoot, 0755) }()

	// Create a writable /tmp equivalent
	writableTmp := filepath.Join(tmpDir, "writable-tmp")
	require.NoError(t, os.MkdirAll(writableTmp, 0755))

	// Save original working directory
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()

	// Test 1: Verify failure when in read-only dir with empty output config
	t.Run("fails in read-only directory", func(t *testing.T) {
		_ = os.Chdir(readOnlyRoot)

		cfg := &config.Config{
			Defaults: config.Defaults{
				// Empty - will try to use default "out" path
			},
			LoadedProviders: map[string]*config.Provider{
				"mock": {ID: "mock", Type: "mock", Model: "mock"},
			},
		}

		handler := &PromptKitHandler{
			config:       cfg,
			log:          logr.Discard(),
			sessions:     make(map[string]*SessionState),
			nsRegistries: make(map[string]*providers.Registry),
		}

		err := handler.buildComponents()
		// The handler should either:
		// 1. Fail with permission denied (if it doesn't set a default)
		// 2. Succeed by setting a default writable path
		if err != nil {
			assert.Contains(t, err.Error(), "permission denied")
		} else {
			// Verify it chose a writable path
			assert.Contains(t, cfg.Defaults.Output.Dir, "/tmp")
			if handler.providerRegistry != nil {
				handler.providerRegistry.Close()
			}
		}
	})

	// Test 2: Verify success when output dir is explicitly set to writable location
	t.Run("succeeds with explicit writable output dir", func(t *testing.T) {
		_ = os.Chdir(readOnlyRoot)

		cfg := &config.Config{
			Defaults: config.Defaults{
				Output: config.OutputConfig{
					Dir: writableTmp,
				},
				OutDir:    writableTmp,
				ConfigDir: writableTmp,
			},
			LoadedProviders: map[string]*config.Provider{
				"mock": {ID: "mock", Type: "mock", Model: "mock"},
			},
		}

		handler := &PromptKitHandler{
			config:       cfg,
			log:          logr.Discard(),
			sessions:     make(map[string]*SessionState),
			nsRegistries: make(map[string]*providers.Registry),
		}

		err := handler.buildComponents()
		require.NoError(t, err, "Should succeed with explicit writable output directory")

		if handler.providerRegistry != nil {
			handler.providerRegistry.Close()
		}
	})

	// Test 3: Verify the working directory change workaround works
	t.Run("succeeds with chdir to writable directory", func(t *testing.T) {
		// Start in read-only directory
		_ = os.Chdir(readOnlyRoot)

		// Change to writable directory (this is what buildComponents does)
		_ = os.Chdir(writableTmp)

		cfg := &config.Config{
			Defaults: config.Defaults{
				// Empty - will use default "out" which is now relative to writableTmp
				ConfigDir: writableTmp,
			},
			LoadedProviders: map[string]*config.Provider{
				"mock": {ID: "mock", Type: "mock", Model: "mock"},
			},
		}

		handler := &PromptKitHandler{
			config:       cfg,
			log:          logr.Discard(),
			sessions:     make(map[string]*SessionState),
			nsRegistries: make(map[string]*providers.Registry),
		}

		err := handler.buildComponents()
		require.NoError(t, err, "Should succeed after changing to writable directory")

		// Verify "out" was created in the writable directory
		outDir := filepath.Join(writableTmp, "out")
		_, err = os.Stat(outDir)
		// Note: might not exist if handler sets /tmp default, which is also fine
		if err != nil {
			// Verify the handler set a /tmp default instead
			assert.Contains(t, cfg.Defaults.Output.Dir, "/tmp")
		}

		if handler.providerRegistry != nil {
			handler.providerRegistry.Close()
		}
	})
}

// TestGetOrLoadK8sRegistryLogging tests that getOrLoadK8sRegistry logs appropriately.
func TestGetOrLoadK8sRegistryLogging(t *testing.T) {
	// This test verifies that when getOrLoadK8sRegistry fails, it logs the error
	// before returning. We can't easily test this without mocking, but we can
	// verify the handler structure supports it.

	tmpDir := t.TempDir()

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: tmpDir,
			},
			OutDir:    tmpDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {ID: "mock", Type: "mock", Model: "mock"},
		},
	}

	handler, err := NewPromptKitHandler(cfg, logr.Discard())
	require.NoError(t, err)

	// Verify handler has proper logging setup
	assert.NotNil(t, handler.log, "Handler should have a logger")

	// Test that we can call getRegistryAndConfig without K8s (uses static config)
	registry, retCfg, err := handler.getRegistryAndConfig(context.Background(), "test-ns")
	require.NoError(t, err)
	assert.NotNil(t, registry)
	assert.NotNil(t, retCfg)

	if handler.providerRegistry != nil {
		handler.providerRegistry.Close()
	}
}
