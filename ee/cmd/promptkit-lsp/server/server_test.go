/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewServer tests creating a new server.
func TestNewServer(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.NotNil(t, s.documents)
	assert.NotNil(t, s.validator)
	assert.NotNil(t, s.connections)
}

// TestHandleHealthz tests the liveness probe handler.
func TestHandleHealthz(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	s.handleHealthz(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

// TestHandleReadyz tests the readiness probe handler.
func TestHandleReadyz(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	s.handleReadyz(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

// TestHandleReadyzShuttingDown tests readiness probe during shutdown.
func TestHandleReadyzShuttingDown(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	// Set shutdown flag
	s.mu.Lock()
	s.shutdown = true
	s.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	s.handleReadyz(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// TestHandleWebSocketMissingParams tests WebSocket handler with missing parameters.
func TestHandleWebSocketMissingParams(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	// Missing both workspace and project
	req := httptest.NewRequest(http.MethodGet, "/lsp", nil)
	w := httptest.NewRecorder()

	s.handleWebSocket(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "workspace and project parameters are required")
}

// TestHandleWebSocketMissingProject tests WebSocket handler with missing project.
func TestHandleWebSocketMissingProject(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/lsp?workspace=test", nil)
	w := httptest.NewRecorder()

	s.handleWebSocket(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "workspace and project parameters are required")
}

// TestHandleWebSocketShuttingDown tests WebSocket handler during shutdown.
func TestHandleWebSocketShuttingDown(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	// Set shutdown flag
	s.mu.Lock()
	s.shutdown = true
	s.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/lsp?workspace=test&project=proj", nil)
	w := httptest.NewRecorder()

	s.handleWebSocket(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// TestConnectionClosed tests that operations on closed connection don't panic.
func TestConnectionClosed(t *testing.T) {
	c := &Connection{
		closed: true,
	}

	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	// These should not panic even with closed connection and nil conn
	s.sendResponse(c, 1, "test")
	s.sendError(c, 1, -32600, "test error", nil)
	s.sendNotification(c, "test", nil)
}

// TestServerShutdownEmpty tests shutdown with no connections.
func TestServerShutdownEmpty(t *testing.T) {
	cfg := Config{
		Addr:            ":0", // Random port
		HealthAddr:      ":0", // Random port
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	// Create minimal HTTP servers for shutdown test
	s.httpServer = &http.Server{Addr: cfg.Addr}
	s.healthSrv = &http.Server{Addr: cfg.HealthAddr}

	// Shutdown should not error with empty connections
	ctx := context.Background()
	// Note: This will error because servers aren't started, but that's expected
	_ = s.Shutdown(ctx)

	// Verify shutdown flag is set
	s.mu.RLock()
	assert.True(t, s.shutdown)
	s.mu.RUnlock()
}

// TestConfigFields tests that Config struct fields are accessible.
func TestConfigFields(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
		DevMode:         true,
	}

	assert.Equal(t, ":8080", cfg.Addr)
	assert.Equal(t, ":8081", cfg.HealthAddr)
	assert.Equal(t, "http://localhost:3000", cfg.DashboardAPIURL)
	assert.True(t, cfg.DevMode)
}

// TestConnectionFields tests that Connection struct fields are accessible.
func TestConnectionFields(t *testing.T) {
	c := &Connection{
		workspace:  "test-workspace",
		projectID:  "test-project",
		pendingReq: make(map[int]chan *Response),
	}

	assert.Equal(t, "test-workspace", c.workspace)
	assert.Equal(t, "test-project", c.projectID)
	assert.NotNil(t, c.pendingReq)
}
