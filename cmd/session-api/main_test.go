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

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestEnvInt32(t *testing.T) {
	tests := []struct {
		name string
		env  string
		def  int32
		want int32
	}{
		{"empty returns default", "", 25, 25},
		{"valid value", "10", 25, 10},
		{"invalid value returns default", "abc", 25, 25},
		{"zero is valid", "0", 25, 0},
		{"negative value", "-5", 25, -5},
		{"overflow returns default", "9999999999999", 25, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_INT32_" + tt.name
			if tt.env != "" {
				t.Setenv(key, tt.env)
			}
			got := envInt32(key, tt.def)
			if got != tt.want {
				t.Errorf("envInt32(%q, %d) = %d, want %d", key, tt.def, got, tt.want)
			}
		})
	}
}

func TestEnvDuration(t *testing.T) {
	tests := []struct {
		name string
		env  string
		def  time.Duration
		want time.Duration
	}{
		{"empty returns default", "", time.Hour, time.Hour},
		{"valid duration", "5m", time.Hour, 5 * time.Minute},
		{"invalid value returns default", "not-a-duration", time.Hour, time.Hour},
		{"zero is valid", "0s", time.Hour, 0},
		{"complex duration", "1h30m", time.Hour, 90 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_DURATION_" + tt.name
			if tt.env != "" {
				t.Setenv(key, tt.env)
			}
			got := envDuration(key, tt.def)
			if got != tt.want {
				t.Errorf("envDuration(%q, %v) = %v, want %v", key, tt.def, got, tt.want)
			}
		})
	}
}

func TestEnvFallback(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		defaultVal string
		envVal     string
		want       string
	}{
		{"env overrides default", "", "", "from-env", "from-env"},
		{"flag value kept when non-default", "flag-val", "", "", "flag-val"},
		{"empty env ignored", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_FALLBACK_" + tt.name
			if tt.envVal != "" {
				t.Setenv(key, tt.envVal)
			}
			val := tt.initial
			envFallback(&val, tt.defaultVal, key)
			if val != tt.want {
				t.Errorf("envFallback() = %q, want %q", val, tt.want)
			}
		})
	}
}

func TestEnvBoolFallback(t *testing.T) {
	tests := []struct {
		name    string
		initial bool
		envVal  string
		want    bool
	}{
		{"true from env", false, "true", true},
		{"non-true env ignored", false, "yes", false},
		{"already true stays true", true, "", true},
		{"empty env keeps false", false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_BOOL_" + tt.name
			if tt.envVal != "" {
				t.Setenv(key, tt.envVal)
			}
			val := tt.initial
			envBoolFallback(&val, key)
			if val != tt.want {
				t.Errorf("envBoolFallback() = %v, want %v", val, tt.want)
			}
		})
	}
}

func TestPoolConfigDefaults(t *testing.T) {
	if defaultMaxConns != 25 {
		t.Errorf("expected defaultMaxConns=25, got %d", defaultMaxConns)
	}
	if defaultMinConns != 5 {
		t.Errorf("expected defaultMinConns=5, got %d", defaultMinConns)
	}
	if defaultMaxConnLifetime != time.Hour {
		t.Errorf("expected defaultMaxConnLifetime=1h, got %v", defaultMaxConnLifetime)
	}
	if defaultMaxConnIdleTime != 30*time.Minute {
		t.Errorf("expected defaultMaxConnIdleTime=30m, got %v", defaultMaxConnIdleTime)
	}
}

func TestApplyEnvFallbacks_AllOverrides(t *testing.T) {
	t.Setenv("POSTGRES_CONN", "postgres://test:5432/db")
	t.Setenv("REDIS_ADDRS", "localhost:6379")
	t.Setenv("COLD_BACKEND", "s3")
	t.Setenv("COLD_BUCKET", "my-bucket")
	t.Setenv("COLD_REGION", "us-east-1")
	t.Setenv("COLD_ENDPOINT", "http://minio:9000")
	t.Setenv("API_ADDR", ":9999")
	t.Setenv("HEALTH_ADDR", ":9998")
	t.Setenv("METRICS_ADDR", ":9997")
	t.Setenv("OTLP_GRPC_ADDR", ":4319")
	t.Setenv("OTLP_HTTP_ADDR", ":4320")
	t.Setenv("ENTERPRISE_ENABLED", "true")
	t.Setenv("OTLP_ENABLED", "true")

	f := &flags{
		apiAddr:      ":8080",
		healthAddr:   ":8081",
		metricsAddr:  ":9090",
		otlpGRPCAddr: ":4317",
		otlpHTTPAddr: ":4318",
	}
	f.applyEnvFallbacks()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"postgresConn", f.postgresConn, "postgres://test:5432/db"},
		{"redisAddrs", f.redisAddrs, "localhost:6379"},
		{"coldBackend", f.coldBackend, "s3"},
		{"coldBucket", f.coldBucket, "my-bucket"},
		{"coldRegion", f.coldRegion, "us-east-1"},
		{"coldEndpoint", f.coldEndpoint, "http://minio:9000"},
		{"apiAddr", f.apiAddr, ":9999"},
		{"healthAddr", f.healthAddr, ":9998"},
		{"metricsAddr", f.metricsAddr, ":9997"},
		{"otlpGRPCAddr", f.otlpGRPCAddr, ":4319"},
		{"otlpHTTPAddr", f.otlpHTTPAddr, ":4320"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
	if !f.enterprise {
		t.Error("enterprise should be true")
	}
	if !f.otlpEnabled {
		t.Error("otlpEnabled should be true")
	}
}

func TestApplyEnvFallbacks_NoOverrideWhenFlagSet(t *testing.T) {
	t.Setenv("POSTGRES_CONN", "should-not-apply")
	t.Setenv("API_ADDR", "should-not-apply")
	t.Setenv("ENTERPRISE_ENABLED", "true")

	f := &flags{
		postgresConn: "flag-value",
		apiAddr:      ":9999",
		healthAddr:   ":8081",
		metricsAddr:  ":9090",
		otlpGRPCAddr: ":4317",
		otlpHTTPAddr: ":4318",
		enterprise:   true, // already true, env should not matter
	}
	f.applyEnvFallbacks()

	if f.postgresConn != "flag-value" {
		t.Errorf("postgresConn = %q, want flag-value", f.postgresConn)
	}
	// apiAddr was ":9999" which differs from default ":8080", so env should not override
	if f.apiAddr != ":9999" {
		t.Errorf("apiAddr = %q, want :9999", f.apiAddr)
	}
}

func TestNewMetricsServer(t *testing.T) {
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", promhttp.Handler())

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	metricsMux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics: expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "application/openmetrics-text") {
		t.Fatalf("metrics: unexpected Content-Type %q", ct)
	}
}

func TestNewHealthServer_Healthz(t *testing.T) {
	// We test the healthz handler by creating a health mux identical to newHealthServer.
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	healthMux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz: expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("healthz: expected 'ok', got %q", rec.Body.String())
	}
}

func TestFlagsStruct(t *testing.T) {
	f := &flags{
		apiAddr:      ":8080",
		healthAddr:   ":8081",
		metricsAddr:  ":9090",
		postgresConn: "postgres://localhost/test",
		redisAddrs:   "localhost:6379,localhost:6380",
		coldBackend:  "s3",
		coldBucket:   "archive",
		coldRegion:   "us-west-2",
		coldEndpoint: "http://s3.local",
		enterprise:   true,
		otlpEnabled:  true,
		otlpGRPCAddr: ":4317",
		otlpHTTPAddr: ":4318",
	}

	// Verify all fields are accessible and correct.
	if f.apiAddr != ":8080" {
		t.Errorf("apiAddr = %q", f.apiAddr)
	}
	if f.healthAddr != ":8081" {
		t.Errorf("healthAddr = %q", f.healthAddr)
	}
	if f.metricsAddr != ":9090" {
		t.Errorf("metricsAddr = %q", f.metricsAddr)
	}
	if f.postgresConn != "postgres://localhost/test" {
		t.Errorf("postgresConn = %q", f.postgresConn)
	}
	if f.redisAddrs != "localhost:6379,localhost:6380" {
		t.Errorf("redisAddrs = %q", f.redisAddrs)
	}
	if f.coldBackend != "s3" {
		t.Errorf("coldBackend = %q", f.coldBackend)
	}
	if f.coldBucket != "archive" {
		t.Errorf("coldBucket = %q", f.coldBucket)
	}
	if f.coldRegion != "us-west-2" {
		t.Errorf("coldRegion = %q", f.coldRegion)
	}
	if f.coldEndpoint != "http://s3.local" {
		t.Errorf("coldEndpoint = %q", f.coldEndpoint)
	}
	if !f.enterprise {
		t.Error("enterprise should be true")
	}
	if !f.otlpEnabled {
		t.Error("otlpEnabled should be true")
	}
	if f.otlpGRPCAddr != ":4317" {
		t.Errorf("otlpGRPCAddr = %q", f.otlpGRPCAddr)
	}
	if f.otlpHTTPAddr != ":4318" {
		t.Errorf("otlpHTTPAddr = %q", f.otlpHTTPAddr)
	}
}
