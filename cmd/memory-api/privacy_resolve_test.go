/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	coreomniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/ee/pkg/privacy/httpclient"
	memoryapi "github.com/altairalabs/omnia/internal/memory/api"
	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

// TestResolvePrivacyPrefStore_EnvURL verifies that when PRIVACY_API_URL is set,
// resolvePrivacyPrefStore returns an *httpclient.Client.
// PRIVACY_API_URL is no longer honoured: privacy-api is per-workspace, so its
// endpoint comes from the Workspace. Setting the var must not produce an HTTP
// client pointing at it.
func TestResolvePrivacyPrefStore_IgnoresEnvURL(t *testing.T) {
	t.Setenv("PRIVACY_API_URL", "http://privacy-api.omnia-system:8080")

	store := resolvePrivacyPrefStore(context.Background(), "", "", nil, logr.Discard())
	if _, ok := store.(*httpclient.Client); ok {
		t.Error("PRIVACY_API_URL still produces an httpclient store")
	}
}

// TestResolvePrivacyPrefStore_NoEnvEmptyWorkspace verifies that when no env var
// is set and workspace is empty, resolvePrivacyPrefStore returns the permissive
// store whose GetPreferences returns ErrPreferencesNotFound.
func TestResolvePrivacyPrefStore_NoEnvEmptyWorkspace(t *testing.T) {
	t.Setenv("PRIVACY_API_URL", "")

	store := resolvePrivacyPrefStore(context.Background(), "", "", nil, logr.Discard())
	_, err := store.GetPreferences(context.Background(), "some-user")
	if !errors.Is(err, privacy.ErrPreferencesNotFound) {
		t.Errorf("expected ErrPreferencesNotFound from permissive store, got %v", err)
	}
}

// TestResolvePrivacyPrefStore_WorkspaceStatus_HTTPClient verifies the in-cluster
// discovery path: no PRIVACY_API_URL env, a non-empty workspace and serviceGroup,
// and a Workspace CRD whose status.privacyURL is populated.
// resolvePrivacyPrefStore must return an *httpclient.Client built from that URL.
func TestResolvePrivacyPrefStore_WorkspaceStatus_HTTPClient(t *testing.T) {
	t.Setenv("PRIVACY_API_URL", "")
	// Clear SESSION_API_URL and MEMORY_API_URL so servicediscovery.resolveFromEnv
	// does not short-circuit before the k8s workspace lookup.
	t.Setenv("SESSION_API_URL", "")
	t.Setenv("MEMORY_API_URL", "")

	ws := &coreomniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "my-ws"},
		Status: coreomniav1alpha1.WorkspaceStatus{
			PrivacyURL: "http://privacy-ws.ns:8080",
			Services: []coreomniav1alpha1.ServiceGroupStatus{
				{
					Name:       "default",
					SessionURL: "http://session.svc",
					Ready:      true,
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(newPrivacyMiddlewareScheme()).
		WithObjects(ws).
		Build()

	store := resolvePrivacyPrefStore(context.Background(), "my-ws", "default", fakeClient, logr.Discard())
	if _, ok := store.(*httpclient.Client); !ok {
		t.Errorf("expected *httpclient.Client from workspace-status discovery path, got %T", store)
	}
}

// TestResolvePrivacyPrefStore_WorkspaceStatus_Permissive verifies that when
// PRIVACY_API_URL is not set and the workspace resolver fails (no matching
// Workspace CRD in-cluster), resolvePrivacyPrefStore falls through to the
// permissive store without crashing.
func TestResolvePrivacyPrefStore_WorkspaceStatus_Permissive(t *testing.T) {
	t.Setenv("PRIVACY_API_URL", "")
	t.Setenv("SESSION_API_URL", "")
	t.Setenv("MEMORY_API_URL", "")

	// Empty fake client — no Workspace object, so Get returns not-found and the
	// resolver returns an error, causing the fall-through to the permissive store.
	fakeClient := fake.NewClientBuilder().
		WithScheme(newPrivacyMiddlewareScheme()).
		Build()

	store := resolvePrivacyPrefStore(context.Background(), "my-ws", "default", fakeClient, logr.Discard())
	if _, ok := store.(*httpclient.Client); ok {
		t.Error("expected permissive store when workspace not found, got *httpclient.Client")
	}
	_, err := store.GetPreferences(context.Background(), "some-user")
	if !errors.Is(err, privacy.ErrPreferencesNotFound) {
		t.Errorf("expected ErrPreferencesNotFound from permissive store, got %v", err)
	}
}

// TestBuildAPIMux_ConsentStatsRouteGone verifies that
// GET /api/v1/privacy/consent/stats returns 404 on memory-api now that the
// endpoint has moved to privacy-api.
func TestBuildAPIMux_ConsentStatsRouteGone(t *testing.T) {
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{},
		nil,
		memoryapi.MemoryServiceConfig{},
		nil,
		true, // enterprise=true: was only registered under enterprise
		nil,
		nil,
		nil,
		logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup
		nil,           // consentPruner — not needed in this test
		nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
	)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/consent/stats?workspace=ws-1", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/privacy/consent/stats should return 404 (moved to privacy-api), got %d", rr.Code)
	}
}

// TestBuildAPIMux_EnforcementStatsRouteGone verifies that
// GET /api/v1/privacy/enforcement-stats returns 404 on memory-api now that the
// endpoint has moved to privacy-api.
func TestBuildAPIMux_EnforcementStatsRouteGone(t *testing.T) {
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{},
		nil,
		memoryapi.MemoryServiceConfig{},
		nil,
		true, // enterprise=true: was only registered under enterprise
		nil,
		nil,
		nil,
		logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup
		nil,           // consentPruner — not needed in this test
		nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
	)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/enforcement-stats?workspace=ws-1", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/privacy/enforcement-stats should return 404 (moved to privacy-api), got %d", rr.Code)
	}
}
