package deploy

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/api/authz"
)

// routePrefix is the deploy API path prefix; {workspace} is consumed by the
// authz middleware.
const routePrefix = "/api/v1/workspaces/{workspace}/deployments"

// Server hosts the operator's deploy-intent API. The single POST route is
// wrapped by the authz middleware (editor required), so an unauthenticated or
// under-privileged request never reaches the handler.
type Server struct {
	addr   string
	log    logr.Logger
	server *http.Server
}

// NewServer builds a deploy API server.
func NewServer(addr string, handler *Handler, authorizer *authz.Authorizer, log logr.Logger) *Server {
	mux := http.NewServeMux()
	guard := authorizer.Middleware
	mux.Handle("POST "+routePrefix, guard(http.HandlerFunc(handler.Deploy)))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
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

// Start runs the server until ctx is cancelled or ListenAndServe fails.
func (s *Server) Start(ctx context.Context) error {
	s.log.Info("starting deploy API server", "addr", s.addr)
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
