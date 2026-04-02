/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestMemoryHTTPDeleter_DeleteAllMemories(t *testing.T) {
	t.Run("single batch returns zero — terminates immediately", func(t *testing.T) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			mustWriteJSON(t, w, memoryBatchDeleteResponse{Deleted: 0})
		}))
		defer srv.Close()

		d := NewMemoryHTTPDeleter(srv.URL, zap.New())
		if err := d.DeleteAllMemories(context.Background(), "user-1", "ws-a"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 1 {
			t.Fatalf("expected 1 call, got %d", calls)
		}
	})

	t.Run("multiple batches — loops until deleted is zero", func(t *testing.T) {
		responses := []int{500, 200, 0}
		idx := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mustWriteJSON(t, w, memoryBatchDeleteResponse{Deleted: responses[idx]})
			idx++
		}))
		defer srv.Close()

		d := NewMemoryHTTPDeleter(srv.URL, zap.New())
		if err := d.DeleteAllMemories(context.Background(), "user-2", "ws-b"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if idx != 3 {
			t.Fatalf("expected 3 calls, got %d", idx)
		}
	})

	t.Run("HTTP transport error returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		srv.Close() // close immediately so the request fails

		d := NewMemoryHTTPDeleter(srv.URL, zap.New())
		err := d.DeleteAllMemories(context.Background(), "user-3", "ws-c")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("non-200 status code returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		d := NewMemoryHTTPDeleter(srv.URL, zap.New())
		err := d.DeleteAllMemories(context.Background(), "user-4", "ws-d")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid JSON response returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
		}))
		defer srv.Close()

		d := NewMemoryHTTPDeleter(srv.URL, zap.New())
		err := d.DeleteAllMemories(context.Background(), "user-5", "ws-e")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("request uses correct method and query parameters", func(t *testing.T) {
		var gotMethod, gotPath, gotQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			gotQuery = r.URL.RawQuery
			mustWriteJSON(t, w, memoryBatchDeleteResponse{Deleted: 0})
		}))
		defer srv.Close()

		d := NewMemoryHTTPDeleter(srv.URL, zap.New())
		if err := d.DeleteAllMemories(context.Background(), "user-6", "ws-f"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if gotMethod != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", gotMethod)
		}
		if gotPath != "/api/v1/memories/batch" {
			t.Errorf("unexpected path: %s", gotPath)
		}
		if gotQuery == "" {
			t.Error("expected query parameters, got none")
		}
	})
}

// mustWriteJSON writes v as JSON to w, failing t on error.
func mustWriteJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("writing JSON response: %v", err)
	}
}
