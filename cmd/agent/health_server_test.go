package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/agent"
)

func TestNewHealthServer_RegistersEndpoints(t *testing.T) {
	t.Parallel()

	srv := newHealthServer(&agent.Config{HealthPort: 8081}, readyzOKHandler)
	if got, want := srv.Addr, ":8081"; got != want {
		t.Fatalf("addr = %q, want %q", got, want)
	}
	if got, want := srv.ReadTimeout, readTimeout; got != want {
		t.Fatalf("read timeout = %v, want %v", got, want)
	}
	if got, want := srv.WriteTimeout, writeTimeout; got != want {
		t.Fatalf("write timeout = %v, want %v", got, want)
	}

	tests := []struct {
		name   string
		path   string
		status int
	}{
		{name: "healthz", path: "/healthz", status: http.StatusOK},
		{name: "readyz", path: "/readyz", status: http.StatusOK},
		{name: "metrics", path: "/metrics", status: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rr, req)
			if rr.Code != tc.status {
				t.Fatalf("status = %d, want %d", rr.Code, tc.status)
			}
		})
	}
}

func TestRunPrimaryAndHealthServers_ReturnsOnServerError(t *testing.T) {
	t.Parallel()

	primary := &http.Server{Addr: ":-1", Handler: http.NewServeMux()}
	health := &http.Server{Addr: ":-2", Handler: http.NewServeMux()}

	done := make(chan struct{})
	go func() {
		runPrimaryAndHealthServers(logr.Discard(), "primary server", primary, health)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runPrimaryAndHealthServers did not return after server error")
	}
}

func TestShutdownPrimaryAndHealthServers_NoPanic(t *testing.T) {
	t.Parallel()

	primary := &http.Server{Addr: ":0", Handler: http.NewServeMux()}
	health := &http.Server{Addr: ":0", Handler: http.NewServeMux()}

	shutdownPrimaryAndHealthServers(logr.Discard(), "primary server", primary, health)
}
