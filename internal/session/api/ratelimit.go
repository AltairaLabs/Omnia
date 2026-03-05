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
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"golang.org/x/time/rate"

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/altairalabs/omnia/pkg/ratelimit"
)

// Default rate limit values for session-api.
const (
	defaultRateLimitRPS   = 100
	defaultRateLimitBurst = 200
	envRateLimitRPS       = "RATE_LIMIT_RPS"
	envRateLimitBurst     = "RATE_LIMIT_BURST"
)

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	// RPS is the sustained requests per second allowed per client IP.
	RPS float64
	// Burst is the maximum burst size allowed per client IP.
	Burst int
}

// RateLimitConfigFromEnv reads rate limit settings from environment variables,
// falling back to defaults.
func RateLimitConfigFromEnv() RateLimitConfig {
	cfg := RateLimitConfig{
		RPS:   defaultRateLimitRPS,
		Burst: defaultRateLimitBurst,
	}
	if v := os.Getenv(envRateLimitRPS); v != "" {
		if rps, err := strconv.ParseFloat(v, 64); err == nil && rps > 0 {
			cfg.RPS = rps
		}
	}
	if v := os.Getenv(envRateLimitBurst); v != "" {
		if burst, err := strconv.Atoi(v); err == nil && burst > 0 {
			cfg.Burst = burst
		}
	}
	return cfg
}

// NewRateLimitMiddleware creates HTTP middleware that enforces per-client-IP
// rate limiting. When the rate is exceeded, it responds with 429 Too Many
// Requests. The returned stop function must be called on shutdown to clean
// up the background goroutine.
func NewRateLimitMiddleware(cfg RateLimitConfig) (func(http.Handler) http.Handler, func()) {
	limiter := ratelimit.NewKeyedLimiter(rate.Limit(cfg.RPS), cfg.Burst)

	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !limiter.Allow(ip) {
				w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	return middleware, limiter.Stop
}

// clientIP extracts the client IP from the request, preferring X-Forwarded-For.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}
