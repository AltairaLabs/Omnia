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

package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoHTTPRequest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"answer":42}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "POST"}
	result, callResult, err := doHTTPRequest(context.Background(), srv.Client(), cfg, nil, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callResult.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", callResult.StatusCode)
	}
	if callResult.Err != nil {
		t.Errorf("expected no transport error, got: %v", callResult.Err)
	}
	var got map[string]any
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if got["answer"] != float64(42) {
		t.Errorf("expected answer=42, got %v", got["answer"])
	}
}

func TestDoHTTPRequest_NonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("plain text response"))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "POST"}
	result, callResult, err := doHTTPRequest(context.Background(), srv.Client(), cfg, nil, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callResult.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", callResult.StatusCode)
	}
	var got map[string]any
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("result should be wrapped JSON: %v", err)
	}
	if got["result"] != "plain text response" {
		t.Errorf("expected wrapped result, got %v", got["result"])
	}
}

func TestDoHTTPRequest_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "POST"}
	result, callResult, err := doHTTPRequest(context.Background(), srv.Client(), cfg, nil, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for 502 response")
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %s", result)
	}
	if callResult.StatusCode != http.StatusBadGateway {
		t.Errorf("expected StatusCode=502, got %d", callResult.StatusCode)
	}
	if callResult.Err != nil {
		t.Errorf("expected no transport error (HTTP error, not network error), got: %v", callResult.Err)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("expected error to mention status code 502, got: %v", err)
	}
}

func TestDoHTTPRequest_POSTWithBody(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "POST"}
	args := json.RawMessage(`{"key":"value"}`)
	_, _, err := doHTTPRequest(context.Background(), srv.Client(), cfg, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(receivedBody) != `{"key":"value"}` {
		t.Errorf("expected body %q, got %q", `{"key":"value"}`, receivedBody)
	}
}

func TestDoHTTPRequest_Headers(t *testing.T) {
	var receivedAuthHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "POST"}
	headers := map[string]string{"Authorization": "Bearer test-token"}
	_, _, err := doHTTPRequest(context.Background(), srv.Client(), cfg, headers, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedAuthHeader != "Bearer test-token" {
		t.Errorf("expected Authorization header 'Bearer test-token', got %q", receivedAuthHeader)
	}
}

func TestDoHTTPRequest_ConnectionRefused(t *testing.T) {
	// Port 1 is reserved and will always refuse connections.
	cfg := &HTTPCfg{Endpoint: "http://127.0.0.1:1", Method: "POST"}
	result, callResult, err := doHTTPRequest(context.Background(), &http.Client{}, cfg, nil, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected transport error for connection refused")
	}
	if result != nil {
		t.Errorf("expected nil result on transport error")
	}
	if callResult.Err == nil {
		t.Error("expected callResult.Err to be set for transport error")
	}
	if callResult.StatusCode != 0 {
		t.Errorf("expected StatusCode=0 for transport error, got %d", callResult.StatusCode)
	}
}

func TestDoHTTPRequest_GETWithQueryParams(t *testing.T) {
	var receivedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "GET"}
	args := json.RawMessage(`{"foo":"bar","n":1}`)
	_, _, err := doHTTPRequest(context.Background(), srv.Client(), cfg, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(receivedQuery, "foo=bar") {
		t.Errorf("expected query to contain 'foo=bar', got %q", receivedQuery)
	}
	if !strings.Contains(receivedQuery, "n=1") {
		t.Errorf("expected query to contain 'n=1', got %q", receivedQuery)
	}
}
