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

package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestHTTPCredentialValidator_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	v := &httpCredentialValidator{
		url:     srv.URL,
		addAuth: func(req *http.Request, c string) { req.Header.Set("Authorization", "Bearer "+c) },
		client:  srv.Client(),
	}
	if err := v.Validate(context.Background(), "good-key"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestHTTPCredentialValidator_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	v := &httpCredentialValidator{
		url:     srv.URL,
		addAuth: func(req *http.Request, c string) { req.Header.Set("Authorization", "Bearer "+c) },
		client:  srv.Client(),
	}
	err := v.Validate(context.Background(), "bad-key")
	if !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("expected ErrCredentialInvalid, got %v", err)
	}
}

func TestHTTPCredentialValidator_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()

	v := &httpCredentialValidator{
		url:     srv.URL,
		addAuth: func(req *http.Request, c string) { req.Header.Set("x-api-key", c) },
		client:  srv.Client(),
	}
	err := v.Validate(context.Background(), "bad-key")
	if !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("expected ErrCredentialInvalid, got %v", err)
	}
}

func TestHTTPCredentialValidator_Gemini400WithMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"API key not valid","status":"INVALID_ARGUMENT","details":[{"reason":"API_KEY_INVALID"}]}}`))
	}))
	defer srv.Close()

	v := &httpCredentialValidator{
		url:     srv.URL,
		addAuth: func(req *http.Request, c string) { req.Header.Set("x-goog-api-key", c) },
		client:  srv.Client(),
	}
	err := v.Validate(context.Background(), "bad-key")
	if !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("expected ErrCredentialInvalid, got %v", err)
	}
}

func TestHTTPCredentialValidator_400WithoutMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"missing required parameter"}`))
	}))
	defer srv.Close()

	v := &httpCredentialValidator{
		url:     srv.URL,
		addAuth: func(req *http.Request, c string) { req.Header.Set("x-goog-api-key", c) },
		client:  srv.Client(),
	}
	err := v.Validate(context.Background(), "any-key")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("400 without auth marker must NOT be classified as invalid: %v", err)
	}
}

func TestHTTPCredentialValidator_500NotInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	v := &httpCredentialValidator{
		url:     srv.URL,
		addAuth: func(req *http.Request, c string) { req.Header.Set("Authorization", "Bearer "+c) },
		client:  srv.Client(),
	}
	err := v.Validate(context.Background(), "key")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("5xx must NOT be classified as invalid: %v", err)
	}
}

func TestHTTPCredentialValidator_EmptyCredential(t *testing.T) {
	v := &httpCredentialValidator{
		url:     "http://unused",
		addAuth: func(_ *http.Request, _ string) {},
	}
	err := v.Validate(context.Background(), "")
	if !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("expected ErrCredentialInvalid for empty credential, got %v", err)
	}
}

func TestHTTPCredentialValidator_NetworkError(t *testing.T) {
	v := &httpCredentialValidator{
		url:     "http://127.0.0.1:1", // port 1 reliably refuses connections
		addAuth: func(req *http.Request, c string) { req.Header.Set("Authorization", "Bearer "+c) },
		client:  &http.Client{Timeout: 2 * time.Second},
	}
	err := v.Validate(context.Background(), "key")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("network error must NOT be classified as invalid: %v", err)
	}
}

func TestHTTPCredentialValidator_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	v := &httpCredentialValidator{
		url:     srv.URL,
		addAuth: func(req *http.Request, c string) { req.Header.Set("Authorization", "Bearer "+c) },
		client:  srv.Client(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := v.Validate(ctx, "key")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("context cancel must NOT be classified as invalid: %v", err)
	}
}

func TestValidatorForProvider(t *testing.T) {
	tests := []struct {
		name       string
		provider   *omniav1alpha1.Provider
		wantNil    bool
		wantPath   string
		wantHeader string
	}{
		{
			name: "openai with default base url",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{Type: omniav1alpha1.ProviderTypeOpenAI},
			},
			wantPath:   "https://api.openai.com/v1/models",
			wantHeader: "Authorization",
		},
		{
			name: "claude with default base url",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{Type: omniav1alpha1.ProviderTypeClaude},
			},
			wantPath:   "https://api.anthropic.com/v1/models",
			wantHeader: "x-api-key",
		},
		{
			name: "gemini with default base url",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{Type: omniav1alpha1.ProviderTypeGemini},
			},
			wantPath:   "https://generativelanguage.googleapis.com/v1beta/models",
			wantHeader: "x-goog-api-key",
		},
		{
			name: "openai with custom base url",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type:    omniav1alpha1.ProviderTypeOpenAI,
					BaseURL: "https://openrouter.ai/api",
				},
			},
			wantPath:   "https://openrouter.ai/api/v1/models",
			wantHeader: "Authorization",
		},
		{
			name: "ollama returns nil (no auth)",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{Type: omniav1alpha1.ProviderTypeOllama},
			},
			wantNil: true,
		},
		{
			name: "mock returns nil",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{Type: omniav1alpha1.ProviderTypeMock},
			},
			wantNil: true,
		},
		{
			name: "platform-hosted returns nil",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type:     omniav1alpha1.ProviderTypeClaude,
					Platform: &omniav1alpha1.PlatformConfig{Type: omniav1alpha1.PlatformTypeBedrock},
				},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validatorForProvider(tt.provider, nil)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %#v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil validator")
			}
			httpV, ok := got.(*httpCredentialValidator)
			if !ok {
				t.Fatalf("expected *httpCredentialValidator, got %T", got)
			}
			if httpV.url != tt.wantPath {
				t.Errorf("url: want %q, got %q", tt.wantPath, httpV.url)
			}
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://x", nil)
			httpV.addAuth(req, "test-cred")
			if req.Header.Get(tt.wantHeader) == "" {
				t.Errorf("expected %q header to be set", tt.wantHeader)
			}
		})
	}
}

func TestCredentialValidationCache_HitAndMiss(t *testing.T) {
	c := newCredentialValidationCache()
	key := validationCacheKey("ns", "p", "s", "rv1")

	if _, ok := c.get(key); ok {
		t.Fatal("expected miss on empty cache")
	}

	c.put(key, nil)
	got, ok := c.get(key)
	if !ok {
		t.Fatal("expected hit")
	}
	if got != nil {
		t.Fatalf("expected nil error, got %v", got)
	}

	c.put(key, ErrCredentialInvalid)
	got, ok = c.get(key)
	if !ok {
		t.Fatal("expected hit")
	}
	if !errors.Is(got, ErrCredentialInvalid) {
		t.Fatalf("expected ErrCredentialInvalid, got %v", got)
	}
}

func TestCredentialValidationCache_Expiry(t *testing.T) {
	c := newCredentialValidationCache()
	key := validationCacheKey("ns", "p", "s", "rv1")
	c.entries[key] = credentialValidationEntry{
		err:      nil,
		cachedAt: time.Now().Add(-2 * validatorCacheTTL),
	}
	if _, ok := c.get(key); ok {
		t.Fatal("expected expired entry to miss")
	}
	if _, present := c.entries[key]; present {
		t.Fatal("expected expired entry to be evicted")
	}
}

func TestCredentialValidationCache_KeyIncludesResourceVersion(t *testing.T) {
	k1 := validationCacheKey("ns", "p", "s", "rv1")
	k2 := validationCacheKey("ns", "p", "s", "rv2")
	if k1 == k2 {
		t.Fatal("cache keys must differ when resourceVersion changes")
	}
}

func TestContainsAuthFailureMarker(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`{"error":{"details":[{"reason":"API_KEY_INVALID"}]}}`, true},
		{`api key not valid. Please pass a valid API key.`, true},
		{`{"error":{"type":"authentication_error"}}`, true},
		{`{"error":{"code":"invalid_api_key"}}`, true},
		{`{"ERROR":"INVALID_API_KEY"}`, true}, // case-insensitive
		{`{"error":"missing required parameter"}`, false},
		{`{}`, false},
		{``, false},
	}
	for _, tc := range cases {
		t.Run(tc.body, func(t *testing.T) {
			got := containsAuthFailureMarker([]byte(tc.body))
			if got != tc.want {
				t.Errorf("containsAuthFailureMarker(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

func TestContainsCaseInsensitive(t *testing.T) {
	cases := []struct {
		hay, needle string
		want        bool
	}{
		{"hello world", "WORLD", true},
		{"HELLO WORLD", "world", true},
		{"hello", "hello", true},
		{"hello", "h", true},
		{"hello", "x", false},
		{"hi", "hello", false},
		{"", "x", false},
		{"x", "", true},
	}
	for _, tc := range cases {
		got := containsCaseInsensitive([]byte(tc.hay), []byte(tc.needle))
		if got != tc.want {
			t.Errorf("containsCaseInsensitive(%q,%q) = %v, want %v", tc.hay, tc.needle, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("truncate short: got %q", got)
	}
	got := truncate(strings.Repeat("x", 50), 10)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncate long: missing ellipsis: %q", got)
	}
	if len(got) <= 10 {
		// "…" is 3 bytes in UTF-8 so total > 10 bytes is fine; just sanity check
		t.Errorf("truncate long: too short %q", got)
	}
}

func TestValidationCacheKey_Format(t *testing.T) {
	got := validationCacheKey("ns", "prov", "sec", "12345")
	want := "ns/prov|sec@12345"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
