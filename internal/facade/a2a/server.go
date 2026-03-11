/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package a2a

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	"github.com/AltairaLabs/PromptKit/sdk"
	a2aserver "github.com/AltairaLabs/PromptKit/server/a2a"
)

// ServerConfig holds configuration for creating an A2A facade server.
type ServerConfig struct {
	// PackPath is the path to the PromptPack JSON file.
	PackPath string

	// PromptName is the default prompt name to use.
	PromptName string

	// Port is the TCP port to serve on.
	Port int

	// TaskTTL is how long completed/failed tasks are retained.
	TaskTTL time.Duration

	// ConversationTTL is how long idle conversations are retained.
	ConversationTTL time.Duration

	// CardProvider provides the Agent Card for discovery.
	CardProvider a2aserver.AgentCardProvider

	// Authenticator validates incoming requests. Nil means no auth.
	Authenticator a2aserver.Authenticator

	// TaskStore is an optional custom task store (e.g., Redis-backed).
	// If nil, the PromptKit default in-memory store is used.
	TaskStore a2aserver.TaskStore

	// SDKOptions are additional options passed to each SDK conversation.
	SDKOptions []sdk.Option

	// Log is the logger.
	Log logr.Logger
}

// Server wraps the PromptKit A2A server with Omnia-specific lifecycle management.
type Server struct {
	inner *a2aserver.Server
	log   logr.Logger
}

// NewServer creates a new A2A facade server.
func NewServer(cfg ServerConfig) *Server {
	opener := sdk.A2AOpener(cfg.PackPath, cfg.PromptName, cfg.SDKOptions...)

	opts := []a2aserver.Option{
		a2aserver.WithPort(cfg.Port),
		a2aserver.WithTaskTTL(cfg.TaskTTL),
		a2aserver.WithConversationTTL(cfg.ConversationTTL),
	}

	if cfg.CardProvider != nil {
		opts = append(opts, a2aserver.WithCardProvider(cfg.CardProvider))
	}

	if cfg.Authenticator != nil {
		opts = append(opts, a2aserver.WithAuthenticator(cfg.Authenticator))
	}

	if cfg.TaskStore != nil {
		opts = append(opts, a2aserver.WithTaskStore(cfg.TaskStore))
	}

	inner := a2aserver.NewServer(opener, opts...)

	return &Server{
		inner: inner,
		log:   cfg.Log,
	}
}

// Handler returns the HTTP handler for the A2A server.
// This includes all routes: /.well-known/agent.json, /a2a, /healthz, /readyz.
func (s *Server) Handler() http.Handler {
	return s.inner.Handler()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info("shutting down A2A server")
	if err := s.inner.Shutdown(ctx); err != nil {
		return fmt.Errorf("a2a server shutdown: %w", err)
	}
	return nil
}
