/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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
	"context"
	"net"
	"net/http"
	"strings"
)

// requestMetaKey is the context key for RequestMeta.
type requestMetaKey struct{}

// RequestMeta holds request metadata extracted from HTTP headers.
type RequestMeta struct {
	IPAddress string
	UserAgent string
}

// withRequestMeta returns a new context carrying the given RequestMeta.
func withRequestMeta(ctx context.Context, meta RequestMeta) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, meta)
}

// requestMetaFromCtx extracts RequestMeta from the context.
// Returns a zero value and false if not present.
func requestMetaFromCtx(ctx context.Context) (RequestMeta, bool) {
	meta, ok := ctx.Value(requestMetaKey{}).(RequestMeta)
	return meta, ok
}

// AuditMiddleware extracts the client IP address and User-Agent from each
// incoming HTTP request and injects them into the request context as
// RequestMeta so that downstream handlers can include them in audit entries.
func AuditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meta := RequestMeta{
			IPAddress: extractIP(r),
			UserAgent: r.Header.Get("User-Agent"),
		}
		ctx := withRequestMeta(r.Context(), meta)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractIP returns the best-effort client IP address from the request.
// Priority: X-Forwarded-For (first entry) > X-Real-IP > RemoteAddr.
func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may be a comma-separated list; take the first entry.
		parts := strings.SplitN(xff, ",", 2)
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}
	// Strip port from RemoteAddr (format: "host:port").
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
