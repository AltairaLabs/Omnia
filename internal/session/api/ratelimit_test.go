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

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testRemoteAddr = "10.0.0.1:12345"

func TestRateLimitMiddleware_AllowsUnderLimit(t *testing.T) {
	cfg := RateLimitConfig{RPS: 100, Burst: 10}
	mw, stop := NewRateLimitMiddleware(cfg)
	defer stop()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.RemoteAddr = testRemoteAddr
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRateLimitMiddleware_RejectsOverBurst(t *testing.T) {
	cfg := RateLimitConfig{RPS: 1, Burst: 2}
	mw, stop := NewRateLimitMiddleware(cfg)
	defer stop()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust burst
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = testRemoteAddr
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rr.Code)
		}
	}

	// Next request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = testRemoteAddr
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
}

func TestRateLimitMiddleware_PerIPIsolation(t *testing.T) {
	cfg := RateLimitConfig{RPS: 1, Burst: 1}
	mw, stop := NewRateLimitMiddleware(cfg)
	defer stop()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust IP1
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = testRemoteAddr
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("IP1 first: expected 200, got %d", rr.Code)
	}

	// IP2 should still be allowed
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.2:12345"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("IP2 first: expected 200, got %d", rr2.Code)
	}
}

func TestRateLimitMiddleware_UsesXForwardedFor(t *testing.T) {
	cfg := RateLimitConfig{RPS: 1, Burst: 1}
	mw, stop := NewRateLimitMiddleware(cfg)
	defer stop()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request with X-Forwarded-For
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.99:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first: expected 200, got %d", rr.Code)
	}

	// Same X-Forwarded-For client should be limited
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.99:54321"
	req2.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second: expected 429, got %d", rr2.Code)
	}
}

func TestRateLimitConfigFromEnv_Defaults(t *testing.T) {
	cfg := RateLimitConfigFromEnv()
	if cfg.RPS != defaultRateLimitRPS {
		t.Fatalf("expected default RPS %v, got %v", defaultRateLimitRPS, cfg.RPS)
	}
	if cfg.Burst != defaultRateLimitBurst {
		t.Fatalf("expected default burst %v, got %v", defaultRateLimitBurst, cfg.Burst)
	}
}

func TestRateLimitConfigFromEnv_CustomValues(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPS", "50")
	t.Setenv("RATE_LIMIT_BURST", "75")

	cfg := RateLimitConfigFromEnv()
	if cfg.RPS != 50 {
		t.Fatalf("expected RPS 50, got %v", cfg.RPS)
	}
	if cfg.Burst != 75 {
		t.Fatalf("expected burst 75, got %d", cfg.Burst)
	}
}

func TestRateLimitConfigFromEnv_InvalidValues(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPS", "notanumber")
	t.Setenv("RATE_LIMIT_BURST", "-5")

	cfg := RateLimitConfigFromEnv()
	if cfg.RPS != defaultRateLimitRPS {
		t.Fatalf("expected default RPS on invalid input, got %v", cfg.RPS)
	}
	if cfg.Burst != defaultRateLimitBurst {
		t.Fatalf("expected default burst on invalid input, got %d", cfg.Burst)
	}
}

func TestClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:8080"
	ip := clientIP(req)
	if ip != "192.168.1.1" {
		t.Fatalf("expected 192.168.1.1, got %s", ip)
	}
}

func TestClientIP_XForwardedForSingle(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	ip := clientIP(req)
	if ip != "203.0.113.50" {
		t.Fatalf("expected 203.0.113.50, got %s", ip)
	}
}

func TestClientIP_XForwardedForMultiple(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18, 150.172.238.178")
	ip := clientIP(req)
	if ip != "203.0.113.50" {
		t.Fatalf("expected 203.0.113.50, got %s", ip)
	}
}

func TestWriteError_RateLimited(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, ErrRateLimitExceeded)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
}
