/*
Copyright 2026.

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

package content

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/api/authz"
)

// routePrefix is the content API path prefix; {workspace} is consumed by the
// authz middleware and {path...} by the handlers.
const routePrefix = "/api/v1/workspaces/{workspace}/content"

// Server hosts the operator's workspace-content API. Every content route is
// wrapped by the authz middleware, so an unauthenticated request never reaches
// a handler.
type Server struct {
	addr   string
	log    logr.Logger
	server *http.Server
}

// NewServer builds a content API server. handler performs the filesystem work;
// authorizer verifies the identity token and recomputes the workspace role.
func NewServer(addr string, handler *Handler, authorizer *authz.Authorizer, log logr.Logger) *Server {
	mux := http.NewServeMux()
	registerRoutes(mux, handler, authorizer)
	return &Server{
		addr: addr,
		log:  log,
		server: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 90 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
}

// registerRoutes mounts every content verb behind the authz middleware. The
// bare-prefix GET serves a root listing (the {path...} pattern only matches a
// trailing slash and beyond).
func registerRoutes(mux *http.ServeMux, h *Handler, a *authz.Authorizer) {
	guard := a.Middleware
	mux.Handle("GET "+routePrefix, guard(http.HandlerFunc(h.Get)))
	mux.Handle("GET "+routePrefix+"/{path...}", guard(http.HandlerFunc(h.Get)))
	mux.Handle("PUT "+routePrefix+"/{path...}", guard(http.HandlerFunc(h.Put)))
	mux.Handle("POST "+routePrefix+"/{path...}", guard(http.HandlerFunc(h.MkDir)))
	mux.Handle("DELETE "+routePrefix+"/{path...}", guard(http.HandlerFunc(h.Delete)))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// Start runs the server until ctx is cancelled or ListenAndServe fails.
func (s *Server) Start(ctx context.Context) error {
	s.log.Info("starting content API server", "addr", s.addr)
	errCh := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
