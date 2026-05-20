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
// runtime so the mux mounting, auth gate, response envelope, and
// health probe are all proven before merge. Without these tests B1
// (Validate rejecting facade.type=grpc) would have shipped silently.

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
)

// buildFacadeWithStubRuntime stands up an in-process function-mode
// facade hooked to a TCP-backed stub gRPC runtime. Returns the
// httptest server (the caller closes it) and the runtime stopper.
func buildFacadeWithStubRuntime(t *testing.T) (*httptest.Server, func()) {
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
	mux.Handle("POST /functions/{name}", functionAuthGate(handler, logr.Discard()))

	srv := httptest.NewServer(mux)
	return srv, func() {
		srv.Close()
		_ = rc.Close()
		stopRuntime()
	}
}

func TestRunFunctionsFacade_AuthGateBlocksByDefault(t *testing.T) {
	t.Setenv(envAllowUnauthFunction, "") // explicit: no bypass

	srv, stop := buildFacadeWithStubRuntime(t)
	defer stop()

	// The function-mode pod name is "summarizer" (from validFunctionConfig).
	resp, err := http.Post(srv.URL+"/functions/summarizer", "application/json",
		strings.NewReader(`{"q":"hi"}`))
	if err != nil {
		t.Fatalf("http.Post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("StatusCode = %d, want %d; body=%s",
			resp.StatusCode, http.StatusForbidden, body)
	}
}

func TestRunFunctionsFacade_HappyPathEndToEnd(t *testing.T) {
	// Opt into the bypass so the auth gate doesn't 403 us; PR 5+ wires
	// the real chain. This is the only way to exercise the full
	// handler → runtime → echo round-trip in PR 4.
	t.Setenv(envAllowUnauthFunction, "true")

	srv, stop := buildFacadeWithStubRuntime(t)
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
	t.Setenv(envAllowUnauthFunction, "true")

	srv, stop := buildFacadeWithStubRuntime(t)
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

func TestFunctionAuthGate_AllowsWhenEnvIsSet(t *testing.T) {
	t.Setenv(envAllowUnauthFunction, "true")
	called := false
	gated := functionAuthGate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), logr.Discard())

	rec := httptest.NewRecorder()
	gated.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if !called {
		t.Errorf("downstream handler was not called when bypass env is set")
	}
}

func TestFunctionAuthGate_RefusesWhenEnvIsUnset(t *testing.T) {
	t.Setenv(envAllowUnauthFunction, "")
	called := false
	gated := functionAuthGate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), logr.Discard())

	rec := httptest.NewRecorder()
	gated.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if called {
		t.Errorf("downstream handler must NOT be called when bypass env is unset")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("auth-gate refused requests must be 403; got %d", rec.Code)
	}
}
