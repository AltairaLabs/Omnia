/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
	"github.com/altairalabs/omnia/pkg/servicediscovery"
)

func TestNewK8sClient_Success(t *testing.T) {
	// NewClientWithConfig succeeds even if the server is unreachable — it's lazy.
	cfg := &rest.Config{Host: "https://127.0.0.1:0"}

	c, err := k8s.NewClientWithConfig(cfg)
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestLoadConfig_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "missing REDIS_URL",
			env:     map[string]string{},
			wantErr: "REDIS_URL",
		},
		{
			name:    "missing NAMESPACE",
			env:     map[string]string{"REDIS_URL": "redis://localhost:6379/0"},
			wantErr: "NAMESPACE",
		},
		{
			// SESSION_API_URL is now resolved separately after the k8s
			// client is built (see resolveSessionAPIURL), so loadConfig no
			// longer enforces its presence. The empty-env case is covered
			// by TestResolveSessionAPIURL_MissingEverything.
			name: "no session api url — loadConfig still passes",
			env:  map[string]string{"REDIS_URL": "redis://localhost:6379/0", "NAMESPACE": "ns"},
		},
		{
			name: "all required present",
			env: map[string]string{
				"REDIS_URL":       "redis://localhost:6379/0",
				"NAMESPACE":       "ns",
				"SESSION_API_URL": "http://session-api:8080",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			envKeys := []string{
				envRedisURL,
				envNamespace, envNamespaces, envSessionAPI, envMetricsAddr,
			}
			for _, k := range envKeys {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			cfg, err := loadConfig()
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "redis://localhost:6379/0", cfg.RedisURL)
				assert.Equal(t, []string{"ns"}, cfg.Namespaces)
				assert.Equal(t, defaultMetrics, cfg.MetricsAddr)
			}
		})
	}
}

func TestLoadConfig_CustomMetricsAddr(t *testing.T) {
	t.Setenv(envRedisURL, "redis://localhost:6379/0")
	t.Setenv(envNamespace, "ns")
	t.Setenv(envSessionAPI, "http://session-api:8080")
	t.Setenv(envMetricsAddr, ":9091")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, ":9091", cfg.MetricsAddr)
}

func TestBuildLogger(t *testing.T) {
	tests := []struct {
		level string
	}{
		{"debug"},
		{"info"},
		{"warn"},
		{"error"},
		{""},
	}

	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			t.Setenv(envLogLevel, tc.level)
			logger := buildLogger()
			assert.NotNil(t, logger)
		})
	}
}

func TestStartHTTPServer(t *testing.T) {
	t.Setenv(envLogLevel, "error")
	logger := buildLogger()

	go startHTTPServer("127.0.0.1:0", logger, nil)
	time.Sleep(50 * time.Millisecond)
}

// TestBuildMetricsHealthMux is the route-registration wiring contract for
// startHTTPServer. The original TestStartHTTPServer spawned the server in a
// goroutine without verifying any route was actually registered — a
// regression that removed mux.Handle("/metrics", ...) would pass that test.
// This test builds the same mux startHTTPServer assembles and ServeHTTP-tests
// each documented route.
func TestBuildMetricsHealthMux(t *testing.T) {
	mux := buildMetricsHealthMux(prometheus.NewRegistry())
	tests := []struct {
		name string
		path string
	}{
		{"metrics route", "/metrics"},
		{"healthz route", "/healthz"},
		{"readyz route", "/readyz"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code == http.StatusNotFound {
				t.Errorf("%s should be registered, got 404", tc.path)
			}
		})
	}
}

// TestBuildMetricsHealthMux_NilRegistry guards against the regression where the
// metrics server was started with a nil *prometheus.Registry (startHTTPServer
// was called with nil for early liveness). A nil registry in the gatherers set
// panics at scrape time inside (*Registry).Gather, closing the connection so
// Prometheus records an EOF / down target. /metrics must return 200, not panic.
func TestBuildMetricsHealthMux_NilRegistry(t *testing.T) {
	mux := buildMetricsHealthMux(nil)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code, "/metrics must serve 200 with a nil eval registry")
}

// TestBuildMetricsHealthMux_ExposesEvalRegistry is the wiring contract that the
// eval registry actually reaches /metrics. The original bug created the eval
// registry separately from the one handed to the server, so omnia_eval_* were
// registered but never exposed. A metric registered in the passed registry must
// appear in the scrape output.
func TestBuildMetricsHealthMux_ExposesEvalRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "omnia_eval_test_total",
		Help: "test metric for wiring assertion",
	})
	reg.MustRegister(c)
	c.Inc()

	mux := buildMetricsHealthMux(reg)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "omnia_eval_test_total",
		"metric registered in the eval registry must be exposed at /metrics")
}

func TestLoadConfig_NAMESPACES(t *testing.T) {
	t.Setenv(envRedisURL, "redis://localhost:6379/0")
	t.Setenv(envNamespaces, "ns1,ns2,ns3")
	t.Setenv(envNamespace, "")
	t.Setenv(envSessionAPI, "http://session-api:8080")
	t.Setenv(envMetricsAddr, "")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, []string{"ns1", "ns2", "ns3"}, cfg.Namespaces)
}

func TestLoadConfig_NAMESPACES_OverridesNAMESPACE(t *testing.T) {
	t.Setenv(envRedisURL, "redis://localhost:6379/0")
	t.Setenv(envNamespaces, "ns1,ns2")
	t.Setenv(envNamespace, "old-ns")
	t.Setenv(envSessionAPI, "http://session-api:8080")
	t.Setenv(envMetricsAddr, "")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, []string{"ns1", "ns2"}, cfg.Namespaces)
}

func TestParseNamespaces_TrimsWhitespace(t *testing.T) {
	t.Setenv(envNamespaces, " ns1 , ns2 , ")
	t.Setenv(envNamespace, "")

	result := parseNamespaces()
	assert.Equal(t, []string{"ns1", "ns2"}, result)
}

func TestParseNamespaces_FallbackToNAMESPACE(t *testing.T) {
	t.Setenv(envNamespaces, "")
	t.Setenv(envNamespace, "fallback")

	result := parseNamespaces()
	assert.Equal(t, []string{"fallback"}, result)
}

func TestParseNamespaces_NeitherSet(t *testing.T) {
	t.Setenv(envNamespaces, "")
	t.Setenv(envNamespace, "")

	result := parseNamespaces()
	assert.Nil(t, result)
}

// SESSION_API_URL is no longer an override: the worker resolves its own URL
// from the Workspace, so setting the var must not change the outcome.
func TestResolveSessionAPIURL_IgnoresEnvOverride(t *testing.T) {
	t.Setenv(envSessionAPI, "http://should-be-ignored:8080")

	_, err := resolveSessionAPIURL(context.Background(), nil)

	require.Error(t, err, "a set SESSION_API_URL must not satisfy resolution")
}

// TestResolveSessionAPIURL_NoResolver verifies we return a clear error when
// there is no client to resolve with.
func TestResolveSessionAPIURL_NoResolver(t *testing.T) {
	_, err := resolveSessionAPIURL(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Kubernetes client")
}

// TestResolveSessionAPIURL_FromWorkspaceCRD verifies that the resolver
// returns a session-api URL from the Workspace CRD's status when no env var
// override is set. This is the post-#717 path: the operator no longer injects
// SESSION_API_URL; services read it from Workspace.status.services[*].
func TestResolveSessionAPIURL_FromWorkspaceCRD(t *testing.T) {
	// Tell the resolver which namespace this "process" runs in (the
	// eval-worker pod's workspace namespace in a real deployment).
	t.Setenv("OMNIA_NAMESPACE", "ws-ns")
	// The operator injects the workspace NAME ("ws"); the pod no longer infers
	// it from its namespace ("ws-ns"), which is a different identifier (#1875).
	t.Setenv(k8s.EnvWorkspaceName, "ws")

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "ws",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-ns"},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: []omniav1alpha1.ServiceGroupStatus{
				{
					Name:       "default",
					Ready:      true,
					SessionURL: "http://session-ws.ws-ns.svc:8080",
					MemoryURL:  "http://memory-ws.ws-ns.svc:8080",
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	resolver := servicediscovery.NewResolver(c)

	url, err := resolveSessionAPIURL(context.Background(), resolver)
	require.NoError(t, err)
	assert.Equal(t, "http://session-ws.ws-ns.svc:8080", url)
}
