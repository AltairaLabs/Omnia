/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package a2a

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestNewServer_ReturnsHandler(t *testing.T) {
	// NewServer will fail to open the pack (file doesn't exist),
	// but the server itself is created and the handler is usable for
	// agent card discovery and health checks.
	cardProvider := NewCRDCardProvider(&omniav1alpha1.AgentCardSpec{
		Name:        "test-agent",
		Description: "A test agent",
	}, "http://localhost:9999")

	srv := NewServer(ServerConfig{
		PackPath:        "/nonexistent/pack.json",
		PromptName:      "default",
		Port:            9999,
		TaskTTL:         1 * time.Hour,
		ConversationTTL: 30 * time.Minute,
		CardProvider:    cardProvider,
		Log:             logr.Discard(),
	})
	require.NotNil(t, srv)

	handler := srv.Handler()
	require.NotNil(t, handler)

	// Agent card endpoint should work
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "test-agent")

	// Health endpoint should work
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Readiness endpoint should work
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestNewServer_WithBearerAuth(t *testing.T) {
	auth := NewBearerAuthenticator("secret")
	srv := NewServer(ServerConfig{
		PackPath:        "/nonexistent/pack.json",
		PromptName:      "default",
		Port:            9999,
		TaskTTL:         1 * time.Hour,
		ConversationTTL: 30 * time.Minute,
		Authenticator:   auth,
		Log:             logr.Discard(),
	})

	handler := srv.Handler()

	// A2A endpoint should reject unauthenticated requests
	req := httptest.NewRequest(http.MethodPost, "/a2a", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestServer_Shutdown(t *testing.T) {
	srv := NewServer(ServerConfig{
		PackPath:        "/nonexistent/pack.json",
		PromptName:      "default",
		Port:            0,
		TaskTTL:         1 * time.Hour,
		ConversationTTL: 30 * time.Minute,
		Log:             logr.Discard(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := srv.Shutdown(ctx)
	require.NoError(t, err)
}
