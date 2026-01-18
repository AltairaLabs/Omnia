// Package api provides a REST API server for the Omnia dashboard.
// It uses the controller-runtime cached client to serve CRD data efficiently.
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

package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Error message constants.
const (
	errMethodNotAllowed    = "method not allowed"
	errFailedGetPromptPack = "failed to get promptpack"
)

// Server provides REST API endpoints for the Omnia dashboard.
type Server struct {
	client      client.Client
	clientset   kubernetes.Interface
	log         logr.Logger
	artifactDir string
	guardrails  GuardrailsConfig
}

// NewServer creates a new API server with the given cached client and clientset.
func NewServer(c client.Client, clientset kubernetes.Interface, log logr.Logger, artifactDir string, guardrails GuardrailsConfig) *Server {
	return &Server{
		client:      c,
		clientset:   clientset,
		log:         log.WithName("api-server"),
		artifactDir: artifactDir,
		guardrails:  guardrails,
	}
}

// Handler returns an http.Handler for the API server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// CORS middleware wrapper
	corsHandler := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Set CORS headers
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			h(w, r)
		}
	}

	// Combined middleware: CORS + guardrails
	wrap := func(h http.HandlerFunc) http.HandlerFunc {
		return corsHandler(s.guardrailsHandler(h))
	}

	// AgentRuntime endpoints
	mux.HandleFunc("/api/v1/agents", wrap(s.handleAgents))
	// Note: /api/v1/agents/ handled by handleAgentOrLogs below for both agent details and logs

	// PromptPack endpoints
	mux.HandleFunc("/api/v1/promptpacks", wrap(s.handlePromptPacks))
	mux.HandleFunc("/api/v1/promptpacks/", wrap(s.handlePromptPack))

	// ToolRegistry endpoints
	mux.HandleFunc("/api/v1/toolregistries", wrap(s.handleToolRegistries))
	mux.HandleFunc("/api/v1/toolregistries/", wrap(s.handleToolRegistry))

	// Provider endpoints
	mux.HandleFunc("/api/v1/providers", wrap(s.handleProviders))
	mux.HandleFunc("/api/v1/providers/", wrap(s.handleProvider))

	// Stats endpoint
	mux.HandleFunc("/api/v1/stats", wrap(s.handleStats))

	// Namespaces endpoint
	mux.HandleFunc("/api/v1/namespaces", wrap(s.handleNamespaces))

	// Logs endpoint
	mux.HandleFunc("/api/v1/agents/", wrap(s.handleAgentOrLogs))

	// Arena artifacts file server
	if s.artifactDir != "" {
		// Serve files from artifactDir at /artifacts/
		fileServer := http.FileServer(http.Dir(s.artifactDir))
		mux.Handle("/artifacts/", http.StripPrefix("/artifacts/", fileServer))
	}

	return mux
}

// Run starts the API server. It blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: s.Handler(),
	}

	// Graceful shutdown with timeout
	// Note: We use a fresh context because ctx is already cancelled when this runs
	go func() {
		<-ctx.Done()
		s.log.Info("shutting down API server")
		shutdownCtx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			s.log.Error(err, "error shutting down API server")
		}
	}()

	s.log.Info("starting API server", "addr", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
