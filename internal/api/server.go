// Package api provides a minimal HTTP server for serving Arena artifacts.
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
)

// Server provides an HTTP server for serving Arena artifacts.
// The CRD API has been removed - the dashboard now uses workspace-scoped
// K8s API access with ServiceAccount tokens instead.
type Server struct {
	log         logr.Logger
	artifactDir string
}

// NewServer creates a new artifact server.
func NewServer(log logr.Logger, artifactDir string) *Server {
	return &Server{
		log:         log.WithName("artifact-server"),
		artifactDir: artifactDir,
	}
}

// Handler returns an http.Handler for the artifact server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// CORS middleware for artifact downloads
	corsHandler := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			h.ServeHTTP(w, r)
		})
	}

	// Arena artifacts file server
	if s.artifactDir != "" {
		fileServer := http.FileServer(http.Dir(s.artifactDir))
		mux.Handle("/artifacts/", corsHandler(http.StripPrefix("/artifacts/", fileServer)))
	}

	// Health check endpoint
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return mux
}

// Run starts the artifact server. It blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: s.Handler(),
	}

	// Graceful shutdown with timeout
	go func() {
		<-ctx.Done()
		s.log.Info("shutting down artifact server")
		shutdownCtx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			s.log.Error(err, "error shutting down artifact server")
		}
	}()

	s.log.Info("starting artifact server", "addr", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
