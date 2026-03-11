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

package tooltest

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/altairalabs/omnia/internal/httputil"
)

// Server provides HTTP endpoints for tool testing.
type Server struct {
	addr   string
	log    logr.Logger
	tester *Tester
	server *http.Server
}

// NewServer creates a new tool test API server.
func NewServer(addr string, c client.Client, log logr.Logger) *Server {
	return &Server{
		addr:   addr,
		log:    log.WithName("tooltest-server"),
		tester: NewTester(c, log),
	}
}

// Start starts the HTTP server.
func (s *Server) Start(_ context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/namespaces/{namespace}/toolregistries/{registry}/test", s.handleTestTool)
	mux.HandleFunc("/healthz", s.handleHealthz)

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.log.Info("starting tool test API server", "addr", s.addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// handleTestTool handles POST /api/v1/namespaces/{namespace}/toolregistries/{registry}/test.
func (s *Server) handleTestTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	namespace := r.PathValue("namespace")
	registry := r.PathValue("registry")

	if namespace == "" || registry == "" {
		http.Error(w, "namespace and registry are required", http.StatusBadRequest)
		return
	}

	var req TestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.HandlerName == "" {
		http.Error(w, "handlerName is required", http.StatusBadRequest)
		return
	}

	resp := s.tester.Test(r.Context(), namespace, registry, &req)
	status := http.StatusOK
	if !resp.Success {
		status = http.StatusUnprocessableEntity
	}

	if err := httputil.WriteJSON(w, status, resp); err != nil {
		s.log.Error(err, "failed to write response")
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
