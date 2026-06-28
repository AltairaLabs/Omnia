/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/internal/serviceauth"
)

func TestGetPreferences_200_Decodes(t *testing.T) {
	want := privacy.Preferences{
		UserID:           "u1",
		OptOutAll:        true,
		OptOutWorkspaces: []string{"ws1"},
		OptOutAgents:     []string{"agent1"},
		ConsentGrants:    []privacy.ConsentCategory{privacy.ConsentMemoryHealth},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/privacy/preferences/u1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard())
	got, err := c.GetPreferences(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.UserID != want.UserID {
		t.Errorf("userId: got %q, want %q", got.UserID, want.UserID)
	}
	if !got.OptOutAll {
		t.Error("optOutAll: got false, want true")
	}
	if len(got.OptOutWorkspaces) != 1 || got.OptOutWorkspaces[0] != "ws1" {
		t.Errorf("optOutWorkspaces: got %v", got.OptOutWorkspaces)
	}
	if len(got.OptOutAgents) != 1 || got.OptOutAgents[0] != "agent1" {
		t.Errorf("optOutAgents: got %v", got.OptOutAgents)
	}
	if len(got.ConsentGrants) != 1 || got.ConsentGrants[0] != privacy.ConsentMemoryHealth {
		t.Errorf("consentGrants: got %v", got.ConsentGrants)
	}
}

func TestGetPreferences_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard())
	_, err := c.GetPreferences(context.Background(), "u1")
	if !errors.Is(err, privacy.ErrPreferencesNotFound) {
		t.Fatalf("want ErrPreferencesNotFound, got %v", err)
	}
}

func TestGetPreferences_ServerErrorFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard())
	_, err := c.GetPreferences(context.Background(), "u1")
	if err == nil || errors.Is(err, privacy.ErrPreferencesNotFound) {
		t.Fatalf("5xx must be a non-not-found error (fail-closed), got %v", err)
	}
}

func TestGetPreferences_TransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close() // close immediately so transport fails

	c := New(srv.URL, logr.Discard())
	_, err := c.GetPreferences(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
}

func TestGetConsentGrants_FromPreferences(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(privacy.Preferences{
			UserID:        "u1",
			ConsentGrants: []privacy.ConsentCategory{privacy.ConsentMemoryHealth},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard(), WithCacheTTL(time.Minute))
	if _, err := c.GetPreferences(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	grants, err := c.GetConsentGrants(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 1 || grants[0] != privacy.ConsentMemoryHealth {
		t.Errorf("got %v", grants)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("consent grant should reuse cached prefs, server hits=%d", n)
	}
}

func TestGetConsentGrants_NotFound_ReturnsEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard())
	grants, err := c.GetConsentGrants(context.Background(), "u1")
	if err != nil {
		t.Fatalf("expected nil error for not-found user, got %v", err)
	}
	if grants == nil || len(grants) != 0 {
		t.Errorf("expected empty slice, got %v", grants)
	}
}

func TestCache_TwoCallsWithinTTL_OneServerHit(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(privacy.Preferences{UserID: "u1"})
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard(), WithCacheTTL(time.Minute))
	if _, err := c.GetPreferences(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetPreferences(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("expected 1 server hit within TTL, got %d", n)
	}
}

func TestCache_ExpiresAfterTTL(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(privacy.Preferences{UserID: "u1"})
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard(), WithCacheTTL(1*time.Millisecond))
	if _, err := c.GetPreferences(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := c.GetPreferences(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("expected 2 server hits after TTL expiry, got %d", n)
	}
}

func TestSetOptOut_PostsBodyAndInvalidatesCache(t *testing.T) {
	var getHits atomic.Int32
	var setBody privacy.OptOutRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			getHits.Add(1)
			_ = json.NewEncoder(w).Encode(privacy.Preferences{UserID: "u1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/privacy/opt-out":
			if err := json.NewDecoder(r.Body).Decode(&setBody); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard(), WithCacheTTL(time.Minute))

	// Prime the cache.
	if _, err := c.GetPreferences(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if n := getHits.Load(); n != 1 {
		t.Fatalf("expected 1 GET after prime, got %d", n)
	}

	// SetOptOut should POST the right body.
	if err := c.SetOptOut(context.Background(), "u1", privacy.ScopeWorkspace, "ws1"); err != nil {
		t.Fatalf("SetOptOut error: %v", err)
	}
	if setBody.UserID != "u1" || setBody.Scope != privacy.ScopeWorkspace || setBody.Target != "ws1" {
		t.Errorf("unexpected body: %+v", setBody)
	}

	// Cache should be evicted: next GET must hit the server.
	if _, err := c.GetPreferences(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if n := getHits.Load(); n != 2 {
		t.Errorf("expected 2 GET hits after cache eviction, got %d", n)
	}
}

func TestRemoveOptOut_DeletesBodyAndInvalidatesCache(t *testing.T) {
	var getHits atomic.Int32
	var delBody privacy.OptOutRequest
	var delMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			getHits.Add(1)
			_ = json.NewEncoder(w).Encode(privacy.Preferences{UserID: "u2"})
		case r.URL.Path == "/api/v1/privacy/opt-out":
			delMethod = r.Method
			if err := json.NewDecoder(r.Body).Decode(&delBody); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard(), WithCacheTTL(time.Minute))

	// Prime the cache.
	if _, err := c.GetPreferences(context.Background(), "u2"); err != nil {
		t.Fatal(err)
	}

	// RemoveOptOut should send DELETE.
	if err := c.RemoveOptOut(context.Background(), "u2", privacy.ScopeAll, ""); err != nil {
		t.Fatalf("RemoveOptOut error: %v", err)
	}
	if delMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", delMethod)
	}
	if delBody.UserID != "u2" || delBody.Scope != privacy.ScopeAll {
		t.Errorf("unexpected delete body: %+v", delBody)
	}

	// Cache evicted — next GET hits server.
	if _, err := c.GetPreferences(context.Background(), "u2"); err != nil {
		t.Fatal(err)
	}
	if n := getHits.Load(); n != 2 {
		t.Errorf("expected 2 GET hits after cache eviction, got %d", n)
	}
}

func TestWithHTTPClient_UsesCustomClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(privacy.Preferences{UserID: "u1"})
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard(), WithHTTPClient(srv.Client()))
	prefs, err := c.GetPreferences(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prefs.UserID != "u1" {
		t.Errorf("got userID %q", prefs.UserID)
	}
}

func TestWithTokenSource_Applied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(privacy.Preferences{UserID: "u1"})
	}))
	defer srv.Close()

	// Pass an explicit (no-op) token source — verifies WithTokenSource is wired.
	ts := serviceauth.NewTokenSource("/nonexistent-token-path", 0)
	c := New(srv.URL, logr.Discard(), WithTokenSource(ts))
	prefs, err := c.GetPreferences(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prefs.UserID != "u1" {
		t.Errorf("got userID %q", prefs.UserID)
	}
}

func TestGetPreferences_InvalidJSON_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not valid json"))
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard())
	_, err := c.GetPreferences(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if errors.Is(err, privacy.ErrPreferencesNotFound) {
		t.Fatal("decode error must not be ErrPreferencesNotFound")
	}
}

func TestSetOptOut_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(privacy.Preferences{UserID: "u1"})
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard())
	err := c.SetOptOut(context.Background(), "u1", privacy.ScopeAll, "")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestSetOptOut_TransportError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(privacy.Preferences{UserID: "u1"})
	}))
	srv.Close() // shut down immediately

	c := New(srv.URL, logr.Discard())
	err := c.SetOptOut(context.Background(), "u1", privacy.ScopeAll, "")
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
}

func TestGetConsentStats_DecodesResponse(t *testing.T) {
	want := privacy.ConsentStats{
		TotalUsers:       10,
		OptedOutAll:      2,
		GrantsByCategory: map[string]int64{"memory:health": 5},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard())
	got, err := c.GetConsentStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TotalUsers != want.TotalUsers {
		t.Errorf("TotalUsers: got %d, want %d", got.TotalUsers, want.TotalUsers)
	}
	if got.OptedOutAll != want.OptedOutAll {
		t.Errorf("OptedOutAll: got %d, want %d", got.OptedOutAll, want.OptedOutAll)
	}
	if got.GrantsByCategory["memory:health"] != 5 {
		t.Errorf("GrantsByCategory[memory:health]: got %d, want 5",
			got.GrantsByCategory["memory:health"])
	}
}

func TestGetConsentStats_NonOK_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard())
	_, err := c.GetConsentStats(context.Background())
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestListConsentUsers_DecodesUserIDs(t *testing.T) {
	want := []string{"u1", "u2", "u3"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"category": "memory:health",
			"granted":  true,
			"userIds":  want,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard())
	got, err := c.ListConsentUsers(context.Background(), privacy.ConsentMemoryHealth, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d users, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i] != id {
			t.Errorf("userIDs[%d]: got %q, want %q", i, got[i], id)
		}
	}
}

func TestListConsentUsers_Cache_TwoCallsOneServerHit(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"userIds": []string{"u1"}})
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard(), WithCacheTTL(time.Minute))
	if _, err := c.ListConsentUsers(context.Background(), privacy.ConsentMemoryHealth, true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListConsentUsers(context.Background(), privacy.ConsentMemoryHealth, true); err != nil {
		t.Fatal(err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("expected 1 server hit within TTL, got %d", n)
	}
}

func TestListConsentUsers_NonOK_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New(srv.URL, logr.Discard())
	_, err := c.ListConsentUsers(context.Background(), privacy.ConsentMemoryHealth, true)
	if err == nil {
		t.Fatal("expected error on 400, got nil")
	}
}
