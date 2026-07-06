/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altairalabs/omnia/ee/pkg/policy"
)

// TestBuildHealthMux_RoutesRegistered asserts that buildHealthMux registers
// both /healthz and /readyz. The policy-broker is fronted by Kubernetes
// probes; a missing /readyz means the pod never becomes Ready (silently
// failing the rollout).
func TestBuildHealthMux_RoutesRegistered(t *testing.T) {
	mux := buildHealthMux()
	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code == http.StatusNotFound {
				t.Errorf("%s should be registered, got 404", path)
			}
		})
	}
}

// TestBuildDecisionMux_RouteRegistered asserts that the decision endpoint
// used by the runtime broker client is actually wired into the server —
// unit tests for policy.BrokerHandler pass regardless of whether main.go
// ever registers it, so this test is the only thing that would catch a
// broker binary that silently serves nothing on /v1/decision.
func TestBuildDecisionMux_RouteRegistered(t *testing.T) {
	eval, err := policy.NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := policy.NewBrokerHandler(eval, logger)

	mux := buildDecisionMux(handler)

	req := httptest.NewRequest(http.MethodPost, decisionPath, bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("%s should be registered, got 404", decisionPath)
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// TestGetEnvOrDefault verifies the fallback helper used for the broker's
// config env vars (LISTEN_ADDR, HEALTH_ADDR, NAMESPACE). A bug here
// misconfigures every deployment.
func TestGetEnvOrDefault(t *testing.T) {
	t.Run("returns default when env unset", func(t *testing.T) {
		t.Setenv("POLICY_BROKER_TEST_VAR", "")
		got := getEnvOrDefault("POLICY_BROKER_TEST_VAR", "fallback")
		if got != "fallback" {
			t.Errorf("expected fallback, got %q", got)
		}
	})
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("POLICY_BROKER_TEST_VAR", "actual")
		got := getEnvOrDefault("POLICY_BROKER_TEST_VAR", "actual")
		if got != "actual" {
			t.Errorf("expected actual, got %q", got)
		}
	})
}

// TestDefaultListenAddr_NotProxyPort asserts the broker's default port is
// distinct from the (retired) policy-proxy's :8082 — the whole point of
// P2.1 is a new decision endpoint, not resurrecting the dead proxy port.
func TestDefaultListenAddr_NotProxyPort(t *testing.T) {
	if defaultListenAddr == ":8082" {
		t.Error("defaultListenAddr must not reuse the retired policy-proxy port :8082")
	}
}
