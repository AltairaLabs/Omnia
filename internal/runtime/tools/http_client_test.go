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

func TestDoHTTPRequest_URLTemplate(t *testing.T) {
	var receivedPath string
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{
		Endpoint:    srv.URL, // fallback, should not be used
		URLTemplate: srv.URL + "/users/{id}/posts",
		Method:      "POST",
	}
	args := json.RawMessage(`{"id":"123","title":"Hello"}`)
	_, _, err := doHTTPRequest(context.Background(), srv.Client(), cfg, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedPath != "/users/123/posts" {
		t.Errorf("expected path /users/123/posts, got %q", receivedPath)
	}
	// "id" should be consumed; only "title" should be in the body.
	var body map[string]any
	if err := json.Unmarshal(receivedBody, &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}
	if _, ok := body["id"]; ok {
		t.Error("expected 'id' to be consumed by URL template, but it was in the body")
	}
	if body["title"] != "Hello" {
		t.Errorf("expected title=Hello in body, got %v", body["title"])
	}
}

func TestDoHTTPRequest_StaticQuery(t *testing.T) {
	var receivedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{
		Endpoint: srv.URL,
		Method:   "POST",
		StaticQuery: map[string]string{
			"api_key": "secret123",
			"format":  "json",
		},
	}
	_, _, err := doHTTPRequest(context.Background(), srv.Client(), cfg, nil, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(receivedQuery, "api_key=secret123") {
		t.Errorf("expected query to contain 'api_key=secret123', got %q", receivedQuery)
	}
	if !strings.Contains(receivedQuery, "format=json") {
		t.Errorf("expected query to contain 'format=json', got %q", receivedQuery)
	}
}

func TestDoHTTPRequest_QueryParams_POST(t *testing.T) {
	var receivedQuery string
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{
		Endpoint:    srv.URL,
		Method:      "POST",
		QueryParams: []string{"page", "limit"},
	}
	args := json.RawMessage(`{"page":"1","limit":"10","data":"value"}`)
	_, _, err := doHTTPRequest(context.Background(), srv.Client(), cfg, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(receivedQuery, "page=1") {
		t.Errorf("expected query to contain 'page=1', got %q", receivedQuery)
	}
	if !strings.Contains(receivedQuery, "limit=10") {
		t.Errorf("expected query to contain 'limit=10', got %q", receivedQuery)
	}
	var body map[string]any
	if err := json.Unmarshal(receivedBody, &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}
	if body["data"] != "value" {
		t.Errorf("expected data=value in body, got %v", body["data"])
	}
	if _, ok := body["page"]; ok {
		t.Error("expected 'page' to be extracted as query param, but it was in the body")
	}
	if _, ok := body["limit"]; ok {
		t.Error("expected 'limit' to be extracted as query param, but it was in the body")
	}
}

func TestDoHTTPRequest_HeaderParams(t *testing.T) {
	var receivedUserID string
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserID = r.Header.Get("X-User-ID")
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{
		Endpoint:     srv.URL,
		Method:       "POST",
		HeaderParams: map[string]string{"user_id": "X-User-ID"},
	}
	args := json.RawMessage(`{"user_id":"abc","query":"hello"}`)
	_, _, err := doHTTPRequest(context.Background(), srv.Client(), cfg, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedUserID != "abc" {
		t.Errorf("expected X-User-ID header 'abc', got %q", receivedUserID)
	}
	var body map[string]any
	if err := json.Unmarshal(receivedBody, &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}
	if _, ok := body["user_id"]; ok {
		t.Error("expected 'user_id' to be consumed by HeaderParams, but it was in the body")
	}
	if body["query"] != "hello" {
		t.Errorf("expected query=hello in body, got %v", body["query"])
	}
}

func TestDoHTTPRequest_StaticBody(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{
		Endpoint: srv.URL,
		Method:   "POST",
		StaticBody: map[string]any{
			"version":  "2.0",
			"default":  "static_val",
			"override": "should_be_overridden",
		},
	}
	args := json.RawMessage(`{"override":"from_args","extra":"dynamic"}`)
	_, _, err := doHTTPRequest(context.Background(), srv.Client(), cfg, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(receivedBody, &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}
	if body["version"] != "2.0" {
		t.Errorf("expected version=2.0 from static body, got %v", body["version"])
	}
	if body["default"] != "static_val" {
		t.Errorf("expected default=static_val from static body, got %v", body["default"])
	}
	if body["override"] != "from_args" {
		t.Errorf("expected override=from_args (args win), got %v", body["override"])
	}
	if body["extra"] != "dynamic" {
		t.Errorf("expected extra=dynamic from args, got %v", body["extra"])
	}
}

func TestDoHTTPRequest_Redact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"Alice","ssn":"123-45-6789","age":30}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{
		Endpoint: srv.URL,
		Method:   "GET",
		Redact:   []string{"ssn"},
	}
	result, _, err := doHTTPRequest(context.Background(), srv.Client(), cfg, nil, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if got["ssn"] != "[REDACTED]" {
		t.Errorf("expected ssn=[REDACTED], got %v", got["ssn"])
	}
	if got["name"] != "Alice" {
		t.Errorf("expected name=Alice (not redacted), got %v", got["name"])
	}
	if got["age"] != float64(30) {
		t.Errorf("expected age=30 (not redacted), got %v", got["age"])
	}
}

func TestDoHTTPRequest_BodyMapping(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	// BodyMapping extracts just the "query" field from the args
	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "POST", BodyMapping: "query"}
	_, _, err := doHTTPRequest(context.Background(), http.DefaultClient, cfg, nil,
		json.RawMessage(`{"query":"hello","page":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// JMESPath "query" extracts the string value → "hello"
	if strings.TrimSpace(string(receivedBody)) != `"hello"` {
		t.Errorf("expected body to be \"hello\" (JMESPath-extracted), got %s", receivedBody)
	}
}

func TestDoHTTPRequest_ResponseMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"items":[1,2,3]},"meta":{"page":1}}`))
	}))
	defer srv.Close()

	// ResponseMapping extracts just data.items from the response
	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "GET", ResponseMapping: "data.items"}
	result, _, err := doHTTPRequest(context.Background(), http.DefaultClient, cfg, nil,
		json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != `[1,2,3]` {
		t.Errorf("expected [1,2,3] from response mapping, got %s", result)
	}
}
