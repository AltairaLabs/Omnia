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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func TestHandleGetPrivacyPolicy_ReturnsEffective(t *testing.T) {
	resolver := PolicyResolverFunc(func(namespace, agent string) (json.RawMessage, bool) {
		return json.RawMessage(`{"recording":{"enabled":true,"facadeData":true,"richData":false}}`), true
	})

	handler := NewHandler(nil, logr.Discard(), DefaultMaxBodySize)
	handler.SetPolicyResolver(resolver)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy-policy?namespace=default&agent=my-agent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"enabled":true`) {
		t.Errorf("expected enabled:true in body, got: %s", body)
	}
	if !strings.Contains(body, `"richData":false`) {
		t.Errorf("expected richData:false in body, got: %s", body)
	}
}

func TestHandleGetPrivacyPolicy_NoPolicyReturns204(t *testing.T) {
	resolver := PolicyResolverFunc(func(_, _ string) (json.RawMessage, bool) {
		return nil, false
	})

	handler := NewHandler(nil, logr.Discard(), DefaultMaxBodySize)
	handler.SetPolicyResolver(resolver)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy-policy?namespace=default&agent=my-agent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestHandleGetPrivacyPolicy_NoResolverReturns204(t *testing.T) {
	handler := NewHandler(nil, logr.Discard(), DefaultMaxBodySize)
	// no SetPolicyResolver call — non-enterprise mode

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy-policy?namespace=default&agent=my-agent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}
