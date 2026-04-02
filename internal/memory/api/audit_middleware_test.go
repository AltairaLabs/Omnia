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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractIP_XForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1, 192.168.1.1")
	assert.Equal(t, "203.0.113.5", extractIP(r))
}

func TestExtractIP_XForwardedFor_Single(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.5")
	assert.Equal(t, "203.0.113.5", extractIP(r))
}

func TestExtractIP_XRealIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Real-IP", "203.0.113.10")
	assert.Equal(t, "203.0.113.10", extractIP(r))
}

func TestExtractIP_RemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:54321"
	assert.Equal(t, "192.0.2.1", extractIP(r))
}

func TestExtractIP_RemoteAddrNoPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1"
	// net.SplitHostPort fails — falls back to raw RemoteAddr
	assert.Equal(t, "192.0.2.1", extractIP(r))
}

func TestExtractIP_XForwardedForHasPriority(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.5")
	r.Header.Set("X-Real-IP", "10.0.0.1")
	// X-Forwarded-For wins
	assert.Equal(t, "203.0.113.5", extractIP(r))
}

func TestAuditMiddleware_PopulatesContext(t *testing.T) {
	var capturedMeta RequestMeta
	var metaPresent bool

	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedMeta, metaPresent = requestMetaFromCtx(r.Context())
	})

	mw := AuditMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/v1/memories", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("User-Agent", "test-agent/1.0")

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	require.True(t, metaPresent, "RequestMeta should be present in context")
	assert.Equal(t, "1.2.3.4", capturedMeta.IPAddress)
	assert.Equal(t, "test-agent/1.0", capturedMeta.UserAgent)
}

func TestAuditMiddleware_NoHeaders(t *testing.T) {
	var capturedMeta RequestMeta
	var metaPresent bool

	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedMeta, metaPresent = requestMetaFromCtx(r.Context())
	})

	mw := AuditMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/v1/memories", nil)
	req.RemoteAddr = "192.0.2.99:1234"

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	require.True(t, metaPresent)
	assert.Equal(t, "192.0.2.99", capturedMeta.IPAddress)
	assert.Empty(t, capturedMeta.UserAgent)
}

func TestRequestMetaFromCtx_NotPresent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	meta, ok := requestMetaFromCtx(req.Context())
	assert.False(t, ok)
	assert.Empty(t, meta.IPAddress)
	assert.Empty(t, meta.UserAgent)
}
