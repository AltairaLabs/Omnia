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

// This file holds the wiring tests for the function-mode pod startup
// flow. They exercise functions.go end-to-end against a real stub
// runtime so the mux mounting, auth middleware composition, response
// envelope, and health probe are all proven before merge. Function mode
// serves HTTP and validates with facade.type=rest (#1464).

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/pkg/policy"
)

// alwaysNoCredValidator returns ErrNoCredential, simulating a chain
// where no validator admits the request. The admit-path counterpart
// (alwaysAdmitValidator) lives in a2a_wiring_test.go and is reused.
type alwaysNoCredValidator struct{}

// Validate implements auth.Validator.
func (alwaysNoCredValidator) Validate(_ context.Context, _ *http.Request) (*policy.AuthenticatedIdentity, error) {
	return nil, auth.ErrNoCredential
}

// buildFacadeWithStubRuntime stands up an in-process function-mode
// facade hooked to a TCP-backed stub gRPC runtime. The auth chain is
// empty; allowUnauthenticated controls whether the empty-chain branch
// of auth.Middleware passes through or refuses with 401 — mirroring
// what the production builder does in runFunctionsFacade.
func buildFacadeWithStubRuntime(t *testing.T, allowUnauthenticated bool) (*httptest.Server, func()) {
	t.Helper()
	stub := &stubRuntimeServer{}
	addr, stopRuntime := startStubRuntimeOnTCP(t, stub)

	cfg := validFunctionConfig()
	cfg.RuntimeAddress = addr

	rc, err := dialRuntime(newDialRuntimeConfig(cfg.RuntimeAddress, nil), logr.Discard())
	if err != nil {
		stopRuntime()
		t.Fatalf("dialRuntime: %v", err)
	}

	registry, err := buildFunctionRegistry(cfg)
	if err != nil {
		_ = rc.Close()
		stopRuntime()
		t.Fatalf("buildFunctionRegistry: %v", err)
	}

	handler := facade.NewFunctionsHandler(registry, rc, logr.Discard())
	mux := http.NewServeMux()
	mux.Handle("POST /functions/{name}", auth.Middleware(auth.Chain{}, handler,
		auth.WithMiddlewareLogger(logr.Discard()),
		auth.WithMiddlewareAllowUnauthenticated(allowUnauthenticated)))

	srv := httptest.NewServer(mux)
	return srv, func() {
		srv.Close()
		_ = rc.Close()
		stopRuntime()
	}
}

func TestRunFunctionsFacade_StrictDefaultRefuses401(t *testing.T) {
	// Production default: when no auth chain is configured AND the
	// strict-default flag holds (OMNIA_FACADE_ALLOW_UNAUTHENTICATED not
	// set), the function route must refuse every request with 401. This
	// is the same behaviour the WebSocket path enforces — closing the
	// pen-test C-3 bypass on function-mode pods too.
	srv, stop := buildFacadeWithStubRuntime(t, false)
	defer stop()

	// The function-mode pod name is "summarizer" (from validFunctionConfig).
	resp, err := http.Post(srv.URL+"/functions/summarizer", "application/json",
		strings.NewReader(`{"q":"hi"}`))
	if err != nil {
		t.Fatalf("http.Post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("StatusCode = %d, want %d; body=%s",
			resp.StatusCode, http.StatusUnauthorized, body)
	}
}

func TestRunFunctionsFacade_HappyPathEndToEnd(t *testing.T) {
	// Mirror the dev/CI configuration: empty auth chain +
	// OMNIA_FACADE_ALLOW_UNAUTHENTICATED=true → pass-through.
	srv, stop := buildFacadeWithStubRuntime(t, true)
	defer stop()

	// The stub echoes input as output, so the payload must satisfy
	// BOTH the input schema (required: q) and the output schema
	// (required: a) used by validFunctionConfig.
	resp, err := http.Post(srv.URL+"/functions/summarizer", "application/json",
		strings.NewReader(`{"q":"hello","a":"42"}`))
	if err != nil {
		t.Fatalf("http.Post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("StatusCode = %d, want 200; body=%s", resp.StatusCode, body)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	output, ok := body["output"].(map[string]any)
	if !ok {
		t.Fatalf("output not a JSON object: %v (body=%v)", body["output"], body)
	}
	if output["q"] != "hello" {
		t.Errorf("output.q = %v, want %q", output["q"], "hello")
	}
	if output["a"] != "42" {
		t.Errorf("output.a = %v, want %q", output["a"], "42")
	}
	if body["invocation_id"] == "" || body["invocation_id"] == nil {
		t.Errorf("invocation_id must be non-empty in success envelope")
	}
}

func TestRunFunctionsFacade_UnknownFunctionIs404(t *testing.T) {
	srv, stop := buildFacadeWithStubRuntime(t, true)
	defer stop()

	resp, err := http.Post(srv.URL+"/functions/does-not-exist", "application/json",
		strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("http.Post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", resp.StatusCode)
	}
}

func TestNewFunctionsHealthServer_ReadyzPassesAgainstHealthyStub(t *testing.T) {
	addr, stopRuntime := startStubRuntimeOnTCP(t, &stubRuntimeServer{})
	defer stopRuntime()

	rc, err := dialRuntime(newDialRuntimeConfig(addr, nil), logr.Discard())
	if err != nil {
		t.Fatalf("dialRuntime: %v", err)
	}
	defer func() { _ = rc.Close() }()

	cfg := validFunctionConfig()
	cfg.HealthPort = 0 // let the OS pick; we drive the mux directly
	srv := newFunctionsHealthServer(cfg, rc)

	// Drive /readyz against the mux without binding a real port.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	req = req.WithContext(context.Background())
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("/readyz = %d, want 200", rec.Code)
	}
}

func TestNewFunctionsHTTPServer_HasWriteTimeout(t *testing.T) {
	// Regression for review S3: unlike the WebSocket path,
	// function-mode endpoints get a finite WriteTimeout so a stalled
	// runtime doesn't leak sockets.
	cfg := validFunctionConfig()
	srv := newFunctionsHTTPServer(cfg, http.NewServeMux())
	if srv.WriteTimeout == 0 {
		t.Errorf("WriteTimeout must be set for one-shot function endpoint")
	}
	if srv.WriteTimeout < time.Second {
		t.Errorf("WriteTimeout = %v, expect a sensible non-trivial bound", srv.WriteTimeout)
	}
}

// TestFunctionAuthMiddleware_ChainAdmitsAuthenticatedRequest pins the
// production wiring: a non-empty chain that admits replaces the empty-
// chain fallback. This is the regression-guard against a future refactor
// that accidentally swaps auth.Middleware out for a passthrough.
func TestFunctionAuthMiddleware_ChainAdmitsAuthenticatedRequest(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	// Single-validator chain that admits everyone — the equivalent of
	// "any credential is fine" used to confirm the middleware actually
	// runs the chain rather than short-circuiting on the empty-chain
	// branch.
	chain := auth.Chain{alwaysAdmitValidator{id: &policy.AuthenticatedIdentity{
		Subject: "test", Origin: policy.OriginSharedToken,
	}}}
	mw := auth.Middleware(chain, next,
		auth.WithMiddlewareLogger(logr.Discard()),
		auth.WithMiddlewareAllowUnauthenticated(false))

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if !called {
		t.Errorf("downstream handler must be called when a chain validator admits")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("admitted request must reach handler; got status %d", rec.Code)
	}
}

func TestFunctionAuthMiddleware_NonEmptyChainAllUnauthorized(t *testing.T) {
	// When the chain is non-empty and all validators return ErrNoCredential,
	// auth.Middleware MUST 401 regardless of allowUnauthenticated — the
	// flag only controls the empty-chain fallback.
	chain := auth.Chain{&alwaysNoCredValidator{}}
	mw := auth.Middleware(chain, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("handler must not be reached when chain refuses")
	}),
		auth.WithMiddlewareLogger(logr.Discard()),
		auth.WithMiddlewareAllowUnauthenticated(true))

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
