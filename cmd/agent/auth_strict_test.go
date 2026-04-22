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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"

	facadea2a "github.com/altairalabs/omnia/internal/facade/a2a"
	"github.com/altairalabs/omnia/internal/facade/auth"
)

func TestAllowUnauthenticatedFallback_DefaultStrict(t *testing.T) {
	t.Setenv(envFacadeAllowUnauthenticated, "")
	if allowUnauthenticatedFallback(logr.Discard()) {
		t.Error("default must be strict (false)")
	}
}

func TestAllowUnauthenticatedFallback_TruthyValuesPermit(t *testing.T) {
	for _, v := range []string{"true", "TRUE", "1", "t", "T"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv(envFacadeAllowUnauthenticated, v)
			if !allowUnauthenticatedFallback(logr.Discard()) {
				t.Errorf("value %q must enable permissive mode", v)
			}
		})
	}
}

func TestAllowUnauthenticatedFallback_ExplicitFalseStaysStrict(t *testing.T) {
	t.Setenv(envFacadeAllowUnauthenticated, "false")
	if allowUnauthenticatedFallback(logr.Discard()) {
		t.Error("explicit false must stay strict")
	}
}

func TestAllowUnauthenticatedFallback_UnparseableFailsSafe(t *testing.T) {
	// A misspelled value must not silently downgrade to permissive —
	// operators rely on OMNIA_FACADE_ALLOW_UNAUTHENTICATED=1 being the
	// only way in.
	t.Setenv(envFacadeAllowUnauthenticated, "yes-please")
	if allowUnauthenticatedFallback(logr.Discard()) {
		t.Error("unparseable value must stay strict")
	}
}

// TestBuildA2AHandler_EmptyChainStrictRejects proves B2 is fixed: when
// the A2A handler is built with an empty chain and the strict default,
// every request must 401. Before the fix, the middleware wrap was
// skipped entirely for empty chains, which meant the A2A path served
// unauthenticated traffic whenever the boot race with the Workspace
// controller left us without a mgmt-plane validator.
func TestBuildA2AHandler_EmptyChainStrictRejects(t *testing.T) {
	t.Setenv(envFacadeAllowUnauthenticated, "")

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // should never run
	})
	metrics := facadea2a.NewMetrics("test-agent", "test-ns")
	handler := buildA2AHandler(inner, metrics, nil, auth.Chain{}, logr.Discard())

	req := httptest.NewRequest(http.MethodPost, "/tasks", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (strict default must reject empty-chain A2A)", rec.Code)
	}
}

// TestBuildA2AHandler_EmptyChainPermissiveAllows proves the dev escape
// hatch still works — setting OMNIA_FACADE_ALLOW_UNAUTHENTICATED=true
// lets the A2A path serve traffic with no validators, which is the
// pre-fix behaviour standalone binaries and CI smoke tests rely on.
func TestBuildA2AHandler_EmptyChainPermissiveAllows(t *testing.T) {
	t.Setenv(envFacadeAllowUnauthenticated, "true")

	var called bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	metrics := facadea2a.NewMetrics("test-agent-perm", "test-ns-perm")
	handler := buildA2AHandler(inner, metrics, nil, auth.Chain{}, logr.Discard())

	req := httptest.NewRequest(http.MethodPost, "/tasks", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 with permissive mode", rec.Code)
	}
	if !called {
		t.Error("inner handler must run under permissive mode")
	}
}
