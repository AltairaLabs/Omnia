/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestBuildHealthMux_RoutesRegistered asserts that buildHealthMux registers
// both /healthz and /readyz. The policy-proxy is fronted by Kubernetes
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

// TestGetEnvOrDefault verifies the fallback helper used for all four
// proxy config env vars (LISTEN_ADDR, HEALTH_ADDR, UPSTREAM_URL,
// NAMESPACE). A bug here misconfigures every deployment.
func TestGetEnvOrDefault(t *testing.T) {
	t.Run("returns default when env unset", func(t *testing.T) {
		t.Setenv("POLICY_PROXY_TEST_VAR", "")
		got := getEnvOrDefault("POLICY_PROXY_TEST_VAR", "fallback")
		if got != "fallback" {
			t.Errorf("expected fallback, got %q", got)
		}
	})
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("POLICY_PROXY_TEST_VAR", "actual")
		got := getEnvOrDefault("POLICY_PROXY_TEST_VAR", "fallback")
		if got != "actual" {
			t.Errorf("expected actual, got %q", got)
		}
	})
}
