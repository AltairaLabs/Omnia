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

package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return NewServer(ServerConfig{
		Adapter:    &stubAdapter{},
		ServerInfo: ServerInfo{Name: testServerName, Version: testServerVersion},
		Resource:   "https://example.com/mcp",
		Log:        testr.New(t),
	})
}

func TestServer_RoutesMCPPathToTransport(t *testing.T) {
	srv := newTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, PathMCP,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Code: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"protocolVersion":"2025-03-26"`) {
		t.Errorf("body missing protocolVersion: %s", rr.Body.String())
	}
}

func TestServer_RoutesDiscoveryPath(t *testing.T) {
	srv := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, PathResourceMetadata, nil))

	if rr.Code != http.StatusOK {
		t.Errorf("Code: got %d want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"resource":"https://example.com/mcp"`) {
		t.Errorf("body missing resource: %s", rr.Body.String())
	}
}

func TestServer_UnknownPathReturns404(t *testing.T) {
	srv := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/unknown", nil))

	if rr.Code != http.StatusNotFound {
		t.Errorf("Code: got %d want 404", rr.Code)
	}
}

func TestServer_ResourceMetadataURL_StripsMCPSuffix(t *testing.T) {
	srv := NewServer(ServerConfig{
		Adapter:    &stubAdapter{},
		ServerInfo: ServerInfo{Name: testServerName, Version: testServerVersion},
		Resource:   "https://example.com/mcp",
	})
	want := "https://example.com" + PathResourceMetadata
	if got := srv.ResourceMetadataURL(); got != want {
		t.Errorf("ResourceMetadataURL: got %q want %q", got, want)
	}
}

func TestServer_ResourceMetadataURL_NoSuffixToStrip(t *testing.T) {
	srv := NewServer(ServerConfig{
		Adapter:    &stubAdapter{},
		ServerInfo: ServerInfo{Name: testServerName, Version: testServerVersion},
		Resource:   "https://example.com",
	})
	want := "https://example.com" + PathResourceMetadata
	if got := srv.ResourceMetadataURL(); got != want {
		t.Errorf("ResourceMetadataURL: got %q want %q", got, want)
	}
}

func TestServer_NoLoggerFallsBackToDiscard(t *testing.T) {
	srv := NewServer(ServerConfig{
		Adapter:    &stubAdapter{},
		ServerInfo: ServerInfo{Name: testServerName, Version: testServerVersion},
		Resource:   "https://example.com/mcp",
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, PathMCP,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Code: got %d want 200", rr.Code)
	}
}
