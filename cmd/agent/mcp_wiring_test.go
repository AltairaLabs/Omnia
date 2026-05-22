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

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/policy"
)

const testMCPResourceMetadataURL = "https://example.com/.well-known/oauth-protected-resource"

// mcpAuthValidatorStub is a programmable Validator used to drive
// buildMCPHandler's auth-chain branches.
type mcpAuthValidatorStub struct {
	id  *policy.AuthenticatedIdentity
	err error
}

func (s *mcpAuthValidatorStub) Validate(_ context.Context, _ *http.Request) (*policy.AuthenticatedIdentity, error) {
	return s.id, s.err
}

func TestBuildMCPHandler_RejectsWithWWWAuthenticate(t *testing.T) {
	// A rejected request must carry WWW-Authenticate per the MCP
	// 2025-03-26 spec so clients can discover the auth challenge.
	chain := auth.Chain{&mcpAuthValidatorStub{err: auth.ErrNoCredential}}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := buildMCPHandler(inner, nil, chain, testMCPResourceMetadataURL, logr.Discard())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("Code: got %d want 401", rr.Code)
	}
	got := rr.Header().Get("WWW-Authenticate")
	if got == "" {
		t.Fatalf("WWW-Authenticate header missing")
	}
	if !strings.Contains(got, `realm="omnia"`) {
		t.Errorf("WWW-Authenticate missing realm: %q", got)
	}
	if !strings.Contains(got, "resource_metadata=") || !strings.Contains(got, testMCPResourceMetadataURL) {
		t.Errorf("WWW-Authenticate missing resource_metadata: %q", got)
	}
}

func TestBuildMCPHandler_AdmitsWithValidCredential(t *testing.T) {
	// An admitted request must reach the inner handler. Identity
	// propagation is the auth.Middleware's responsibility (covered
	// in its own tests) — here we just verify the chain wraps cleanly.
	identity := &policy.AuthenticatedIdentity{Origin: policy.OriginManagementPlane}
	chain := auth.Chain{&mcpAuthValidatorStub{id: identity}}
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := buildMCPHandler(inner, nil, chain, testMCPResourceMetadataURL, logr.Discard())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if !called {
		t.Error("inner handler not invoked despite valid credential")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("Code: got %d want 200", rr.Code)
	}
}

func TestBuildMCPHandler_EmptyChainWithAllowUnauthenticated(t *testing.T) {
	// Dev/CI clusters without an externalAuth chain rely on
	// OMNIA_FACADE_ALLOW_UNAUTHENTICATED=true to pass through. The wiring
	// must honour that flag the same way the WebSocket and HTTP routes do.
	t.Setenv(envFacadeAllowUnauthenticated, "true")
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := buildMCPHandler(inner, nil, nil, testMCPResourceMetadataURL, logr.Discard())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if rr.Code != http.StatusOK {
		t.Errorf("Code: got %d want 200 (allowUnauthenticated=true should pass empty chain)", rr.Code)
	}
}

func TestBuildMCPHandler_WiresTracingProvider(t *testing.T) {
	// Parallel to TestBuildA2AHandler_WiresTracingProvider: when a non-nil
	// tracing.Provider is passed, the handler emits OpenTelemetry spans.
	// A regression that drops the otelhttp wrapper — or fails to thread
	// the provider through buildMCPHandler — is caught here.
	t.Setenv(envFacadeAllowUnauthenticated, "true")

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	provider := tracing.NewTestProvider(tp)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := buildMCPHandler(inner, provider, nil, testMCPResourceMetadataURL, logr.Discard())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("inner handler not invoked: status=%d", rr.Code)
	}

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Error("no spans recorded — otelhttp middleware is not wired")
	}
}
