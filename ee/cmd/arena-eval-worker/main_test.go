/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"testing"
	"time"

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
			name:    "missing REDIS_ADDR",
			env:     map[string]string{},
			wantErr: "REDIS_ADDR",
		},
		{
			name:    "missing NAMESPACE",
			env:     map[string]string{"REDIS_ADDR": "localhost:6379"},
			wantErr: "NAMESPACE",
		},
		{
			// SESSION_API_URL is now resolved separately after the k8s
			// client is built (see resolveSessionAPIURL), so loadConfig no
			// longer enforces its presence. The empty-env case is covered
			// by TestResolveSessionAPIURL_MissingEverything.
			name: "no session api url — loadConfig still passes",
			env:  map[string]string{"REDIS_ADDR": "localhost:6379", "NAMESPACE": "ns"},
		},
		{
			name: "all required present",
			env: map[string]string{
				"REDIS_ADDR":      "localhost:6379",
				"NAMESPACE":       "ns",
				"SESSION_API_URL": "http://session-api:8080",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Save and restore env vars.
			envKeys := []string{
				envRedisAddr, envRedisPass, envRedisDB,
				envNamespace, envNamespaces, envSessionAPI, envMetricsAddr,
			}
			saved := make(map[string]string)
			for _, k := range envKeys {
				saved[k] = ""
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
				assert.Equal(t, "localhost:6379", cfg.RedisAddr)
				assert.Equal(t, []string{"ns"}, cfg.Namespaces)
				assert.Equal(t, defaultMetrics, cfg.MetricsAddr)
			}
		})
	}
}

func TestLoadConfig_RedisDB(t *testing.T) {
	t.Setenv(envRedisAddr, "localhost:6379")
	t.Setenv(envNamespace, "ns")
	t.Setenv(envSessionAPI, "http://session-api:8080")
	t.Setenv(envRedisDB, "3")
	t.Setenv(envMetricsAddr, "")
	t.Setenv(envRedisPass, "")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, 3, cfg.RedisDB)
}

func TestLoadConfig_InvalidRedisDB(t *testing.T) {
	t.Setenv(envRedisAddr, "localhost:6379")
	t.Setenv(envNamespace, "ns")
	t.Setenv(envSessionAPI, "http://session-api:8080")
	t.Setenv(envRedisDB, "notanumber")
	t.Setenv(envMetricsAddr, "")
	t.Setenv(envRedisPass, "")

	_, err := loadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "REDIS_DB")
}

func TestLoadConfig_CustomMetricsAddr(t *testing.T) {
	t.Setenv(envRedisAddr, "localhost:6379")
	t.Setenv(envNamespace, "ns")
	t.Setenv(envSessionAPI, "http://session-api:8080")
	t.Setenv(envMetricsAddr, ":9091")
	t.Setenv(envRedisDB, "")
	t.Setenv(envRedisPass, "")

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

func TestLoadConfig_NAMESPACES(t *testing.T) {
	t.Setenv(envRedisAddr, "localhost:6379")
	t.Setenv(envNamespaces, "ns1,ns2,ns3")
	t.Setenv(envNamespace, "")
	t.Setenv(envSessionAPI, "http://session-api:8080")
	t.Setenv(envMetricsAddr, "")
	t.Setenv(envRedisDB, "")
	t.Setenv(envRedisPass, "")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, []string{"ns1", "ns2", "ns3"}, cfg.Namespaces)
}

func TestLoadConfig_NAMESPACES_OverridesNAMESPACE(t *testing.T) {
	t.Setenv(envRedisAddr, "localhost:6379")
	t.Setenv(envNamespaces, "ns1,ns2")
	t.Setenv(envNamespace, "old-ns")
	t.Setenv(envSessionAPI, "http://session-api:8080")
	t.Setenv(envMetricsAddr, "")
	t.Setenv(envRedisDB, "")
	t.Setenv(envRedisPass, "")

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

// TestResolveSessionAPIURL_EnvOverride verifies that an explicit
// SESSION_API_URL env var takes precedence over service discovery.
func TestResolveSessionAPIURL_EnvOverride(t *testing.T) {
	url, err := resolveSessionAPIURL(context.Background(), nil, "http://explicit:8080")
	require.NoError(t, err)
	assert.Equal(t, "http://explicit:8080", url)
}

// TestResolveSessionAPIURL_MissingEverything verifies we return a clear
// error when neither the env var nor a resolver is available.
func TestResolveSessionAPIURL_MissingEverything(t *testing.T) {
	_, err := resolveSessionAPIURL(context.Background(), nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), envSessionAPI)
}

// TestResolveSessionAPIURL_FromWorkspaceCRD verifies that the resolver
// returns a session-api URL from the Workspace CRD's status when no env var
// override is set. This is the post-#717 path: the operator no longer injects
// SESSION_API_URL; services read it from Workspace.status.services[*].
func TestResolveSessionAPIURL_FromWorkspaceCRD(t *testing.T) {
	// Tell the resolver which namespace this "process" runs in (the
	// eval-worker pod's workspace namespace in a real deployment).
	t.Setenv("OMNIA_NAMESPACE", "ws-ns")

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

	url, err := resolveSessionAPIURL(context.Background(), resolver, "")
	require.NoError(t, err)
	assert.Equal(t, "http://session-ws.ws-ns.svc:8080", url)
}
