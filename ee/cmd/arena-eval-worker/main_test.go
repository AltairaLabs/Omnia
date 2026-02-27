/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/altairalabs/omnia/pkg/k8s"
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
			name:    "missing SESSION_API_URL",
			env:     map[string]string{"REDIS_ADDR": "localhost:6379", "NAMESPACE": "ns"},
			wantErr: "SESSION_API_URL",
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
			envKeys := []string{envRedisAddr, envRedisPass, envRedisDB, envNamespace, envSessionAPI, envMetricsAddr}
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
				assert.Equal(t, "ns", cfg.Namespace)
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

	go startHTTPServer("127.0.0.1:0", logger)
	time.Sleep(50 * time.Millisecond)
}
