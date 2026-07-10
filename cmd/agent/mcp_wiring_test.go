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

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/facade/auth"
	"github.com/altairalabs/omnia/pkg/policy"
)

// validSchemaJSON is a minimum JSON Schema accepted by facade.CompileSchema,
// used as the input/output schema in buildMCPServer tests.
const validSchemaJSON = `{"type":"object","additionalProperties":false}`

// testNamespace is the namespace used in mcp_wiring_test fixtures.
const testNamespace = "default"

func newMCPServerTestConfig(enabled bool) *agent.Config {
	cfg := &agent.Config{
		AgentName:                "test-fn",
		Namespace:                testNamespace,
		PromptPackName:           "p",
		MCPEnabled:               enabled,
		MCPPort:                  9998,
		FunctionInputSchemaJSON:  []byte(validSchemaJSON),
		FunctionOutputSchemaJSON: []byte(validSchemaJSON),
	}
	return cfg
}

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

func TestBuildMCPServer_ReturnsNilWhenDisabled(t *testing.T) {
	// When cfg.MCPEnabled is false, buildMCPServer must return nil so
	// startFunctionsAndServe doesn't spin up an unwanted listener.
	cfg := newMCPServerTestConfig(false)
	srv := buildMCPServer(cfg, nil, nil, nil, logr.Discard(), int32(cfg.MCPPort))
	if srv != nil {
		t.Errorf("buildMCPServer(disabled) = %+v, want nil", srv)
	}
}

func TestBuildMCPServer_ReturnsServerWhenEnabled(t *testing.T) {
	// When MCP is enabled and the function schemas compile cleanly,
	// buildMCPServer returns an *http.Server listening on cfg.MCPPort
	// with a non-nil handler. We don't ListenAndServe — just verify
	// the construction shape.
	cfg := newMCPServerTestConfig(true)
	srv := buildMCPServer(cfg, nil, nil, nil, logr.Discard(), int32(cfg.MCPPort))
	if srv == nil {
		t.Fatal("buildMCPServer(enabled) returned nil")
	}
	if srv.Addr != ":9998" {
		t.Errorf("Addr = %q, want :9998", srv.Addr)
	}
	if srv.Handler == nil {
		t.Error("Handler must be non-nil")
	}
}

func TestBuildMCPServer_ReturnsNilOnBadInputSchema(t *testing.T) {
	// A function with an invalid input schema is operator misconfig.
	// buildMCPServer logs the error and returns nil rather than
	// crashing — the HTTP route is still served.
	cfg := newMCPServerTestConfig(true)
	cfg.FunctionInputSchemaJSON = []byte("not json at all {")
	srv := buildMCPServer(cfg, nil, nil, nil, logr.Discard(), int32(cfg.MCPPort))
	if srv != nil {
		t.Errorf("buildMCPServer(bad-input-schema) = %+v, want nil", srv)
	}
}

func TestBuildMCPServer_CustomPort(t *testing.T) {
	// Operators can override the listener port via spec.facade.mcp.port.
	cfg := newMCPServerTestConfig(true)
	cfg.MCPPort = 9500
	srv := buildMCPServer(cfg, nil, nil, nil, logr.Discard(), int32(cfg.MCPPort))
	if srv == nil {
		t.Fatal("buildMCPServer(custom-port) returned nil")
	}
	if srv.Addr != ":9500" {
		t.Errorf("Addr = %q, want :9500", srv.Addr)
	}
}

func TestBuildMCPServer_InternalTwinPort(t *testing.T) {
	// The internal management-plane MCP twin is built on the internal port
	// (with a mgmt-plane-only chain supplied by the caller).
	cfg := newMCPServerTestConfig(true)
	srv := buildMCPServer(cfg, nil, nil, nil, logr.Discard(), int32(agent.DefaultInternalMCPPort))
	if srv == nil {
		t.Fatal("buildMCPServer(internal port) returned nil")
	}
	if srv.Addr != ":19998" {
		t.Errorf("Addr = %q, want :19998 (internal MCP twin)", srv.Addr)
	}
}

func TestBuildMCPServer_NilWhenPortZero(t *testing.T) {
	// A zero port (management plane disabled / no internal port allocated)
	// yields no server even when MCP is enabled, so the start helper skips it.
	cfg := newMCPServerTestConfig(true)
	srv := buildMCPServer(cfg, nil, nil, nil, logr.Discard(), 0)
	if srv != nil {
		t.Errorf("buildMCPServer(port=0) = %+v, want nil", srv)
	}
}
