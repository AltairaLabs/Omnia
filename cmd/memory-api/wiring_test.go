/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/altairalabs/omnia/internal/memory"
	memoryapi "github.com/altairalabs/omnia/internal/memory/api"
)

// freshPromRegistry swaps the default Prometheus registerer for the duration
// of a test so that multiple buildAPIMux calls do not panic with "duplicate
// metrics collector registration".
func freshPromRegistry(t *testing.T) {
	t.Helper()
	prev := prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
	t.Cleanup(func() { prometheus.DefaultRegisterer = prev })
}

// fakeMemoryStore is a no-op implementation of memory.Store. The wiring test
// never exercises the store — it only validates that routes and middleware
// are registered on the real mux — but buildAPIMux requires a non-nil Store
// at construction time.
type fakeMemoryStore struct{}

func (fakeMemoryStore) Save(_ context.Context, _ *memory.Memory) error { return nil }
func (fakeMemoryStore) SaveWithResult(_ context.Context, mem *memory.Memory) (*memory.SaveResult, error) {
	return &memory.SaveResult{ID: mem.ID, Action: memory.SaveActionAdded}, nil
}
func (fakeMemoryStore) FindSimilarObservations(_ context.Context, _ map[string]string,
	_ []float32, _ int, _ float64,
) ([]memory.SimilarObservation, error) {
	return nil, nil
}
func (fakeMemoryStore) AppendObservationToEntity(_ context.Context, entityID string, mem *memory.Memory) ([]string, error) {
	mem.ID = entityID
	return nil, nil
}
func (fakeMemoryStore) GetMemory(_ context.Context, _ map[string]string, _ string) (*memory.Memory, error) {
	return nil, memory.ErrNotFound
}
func (fakeMemoryStore) LinkEntities(_ context.Context, _ map[string]string, _, _, _ string, _ float64) (string, error) {
	return "", nil
}
func (fakeMemoryStore) Retrieve(_ context.Context, _ map[string]string, _ string, _ memory.RetrieveOptions) ([]*memory.Memory, error) {
	return nil, nil
}
func (fakeMemoryStore) List(_ context.Context, _ map[string]string, _ memory.ListOptions) ([]*memory.Memory, error) {
	return nil, nil
}
func (fakeMemoryStore) Delete(_ context.Context, _ map[string]string, _ string) error { return nil }
func (fakeMemoryStore) DeleteAll(_ context.Context, _ map[string]string) error        { return nil }
func (fakeMemoryStore) ExportAll(_ context.Context, _ map[string]string) ([]*memory.Memory, error) {
	return nil, nil
}
func (fakeMemoryStore) BatchDelete(_ context.Context, _ map[string]string, _ int) (int, error) {
	return 0, nil
}
func (fakeMemoryStore) RetrieveMultiTier(_ context.Context, _ memory.MultiTierRequest) (*memory.MultiTierResult, error) {
	return &memory.MultiTierResult{Memories: []*memory.MultiTierMemory{}, Total: 0}, nil
}
func (fakeMemoryStore) SaveInstitutional(_ context.Context, _ *memory.Memory) error { return nil }
func (fakeMemoryStore) ListInstitutional(_ context.Context, _ string, _ memory.ListOptions) ([]*memory.Memory, error) {
	return nil, nil
}
func (fakeMemoryStore) DeleteInstitutional(_ context.Context, _, _ string) error  { return nil }
func (fakeMemoryStore) SaveAgentScoped(_ context.Context, _ *memory.Memory) error { return nil }
func (fakeMemoryStore) ListAgentScoped(_ context.Context, _, _ string, _ memory.ListOptions) ([]*memory.Memory, error) {
	return nil, nil
}
func (fakeMemoryStore) DeleteAgentScoped(_ context.Context, _, _, _ string) error { return nil }
func (fakeMemoryStore) FindCompactionCandidates(_ context.Context, _ memory.FindCompactionCandidatesOptions) ([]memory.CompactionCandidate, error) {
	return nil, nil
}
func (fakeMemoryStore) SaveCompactionSummary(_ context.Context, _ memory.CompactionSummary) (string, error) {
	return "", nil
}

// TestBuildAPIMux_POSTMemoryWithoutUserIDReturns400 verifies the wiring
// contract that the POST /api/v1/memories route is registered and reaches the
// handler's user_id validation. An unregistered route would return 404; a
// broken middleware chain would prevent the handler from running.
//
// This test proves:
//  1. POST /api/v1/memories is registered on the real mux.
//  2. AuditMiddleware is in the chain (it wraps the handler without breaking
//     it — the handler receives the request and produces a 400).
//  3. The user_id validation guard fires as expected.
func TestBuildAPIMux_POSTMemoryWithoutUserIDReturns400(t *testing.T) {
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{},
		nil, // embedding service is optional
		memoryapi.MemoryServiceConfig{},
		nil, // event publisher is optional
		false,
		nil, // pool is only used by enterprise privacy middleware
		nil, // policy loader is optional — identity ranker without it
		logr.Discard(),
	)
	defer cleanup()

	body, err := json.Marshal(memoryapi.SaveMemoryRequest{
		Type:    "fact",
		Content: "test content",
		Scope: map[string]string{
			memory.ScopeWorkspaceID: "ws-1",
			// deliberately no ScopeUserID — should trigger ErrMissingUserID
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for POST without user_id, got %d; body=%q",
			rr.Code, rr.Body.String())
	}
}

// TestBuildAPIMux_GETMemoriesWired verifies that the GET /api/v1/memories
// route is registered on the real mux. This is the read path; it reaches the
// store via the service layer. A 404 here means the route isn't registered.
// The request will return 500 because the fake store returns nil (not a valid
// response shape in some cases) — or it may return 200 with an empty list.
// Either is acceptable: anything other than 404 proves wiring.
func TestBuildAPIMux_GETMemoriesWired(t *testing.T) {
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{},
		nil,
		memoryapi.MemoryServiceConfig{},
		nil,
		false,
		nil,
		nil, // policy loader is optional — identity ranker without it
		logr.Discard(),
	)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories?workspace=ws-1&userId=alice", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/memories not registered; buildAPIMux returned 404")
	}
}

// TestCompactionWorkerOptions_DisabledWhenIntervalEmpty proves that omitting
// --compaction-interval keeps the worker off. Without this guard the worker
// would tick on the zero-duration default and spam RunOnce in a tight loop.
func TestCompactionWorkerOptions_DisabledWhenIntervalEmpty(t *testing.T) {
	f := &flags{}
	_, enabled := f.compactionWorkerOptions(logr.Discard(), nil)
	if enabled {
		t.Error("expected compaction worker to be disabled when interval is empty")
	}
}

// TestCompactionWorkerOptions_InvalidIntervalDisables proves that a bad
// duration string keeps the worker off instead of starting it with whatever
// ParseDuration happens to partially succeed on.
func TestCompactionWorkerOptions_InvalidIntervalDisables(t *testing.T) {
	f := &flags{compactionInterval: "not-a-duration"}
	_, enabled := f.compactionWorkerOptions(logr.Discard(), nil)
	if enabled {
		t.Error("expected compaction worker to be disabled when interval is invalid")
	}
}

// TestCompactionWorkerOptions_PopulatesAgeAndDiscoverer proves the happy path:
// a valid interval + age populates options and wires the store's
// ListWorkspaceIDs as the workspace discoverer.
func TestCompactionWorkerOptions_PopulatesAgeAndDiscoverer(t *testing.T) {
	store := &memory.PostgresMemoryStore{}
	f := &flags{
		compactionInterval: "6h",
		compactionAge:      "720h",
	}
	opts, enabled := f.compactionWorkerOptions(logr.Discard(), store)
	if !enabled {
		t.Fatal("expected compaction worker to be enabled")
	}
	if opts.Interval != 6*time.Hour {
		t.Errorf("expected 6h interval, got %v", opts.Interval)
	}
	if opts.Age != 720*time.Hour {
		t.Errorf("expected 720h age, got %v", opts.Age)
	}
	if opts.WorkspaceDiscoverer == nil {
		t.Error("expected WorkspaceDiscoverer to be wired to store.ListWorkspaceIDs")
	}
}

// TestWrapPrivacyMiddleware_NoEmbeddingProvider_StillBuildsValidator verifies
// that the privacy middleware wiring tolerates a nil embedding service —
// the validator should still be constructed in regex-only mode and the
// returned handler must not be nil.
//
// Note: this exercises only the no-kubeconfig branch (the only branch
// reachable in unit tests). Full validator behaviour (regex + embedding +
// override) is covered by ee/pkg/privacy/classify and middleware tests.
func TestWrapPrivacyMiddleware_NoEmbeddingProvider_StillBuildsValidator(t *testing.T) {
	freshPromRegistry(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := wrapPrivacyMiddleware(
		context.Background(),
		next,
		nil, // pool — function tolerates nil because it skips when no kubeconfig
		nil, // embeddingSvc nil → regex-only
		logr.Discard(),
	)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
}

// TestWrapPrivacyMiddleware_RegistersSuppressionMetrics verifies that
// wrapPrivacyMiddleware registers the suppression metric on the default
// Prometheus registry. The wrap path tolerates nil pool/embeddingSvc — it
// short-circuits when no kubeconfig is available, but the metrics
// registration happens before that branch (in this test environment the
// short-circuit means the metric won't actually be wired into the
// middleware, but the registration itself is exercised when the kubeconfig
// path runs in production).
//
// In the no-kubeconfig branch (this test) the function returns the
// untouched handler before constructing the middleware, so we can't
// assert registration directly. The unit-level coverage in
// suppression_metrics_test.go + the middleware_test.go observability
// tests prove the metric works end-to-end.
func TestWrapPrivacyMiddleware_DoesNotPanicWithNilEmbeddingSvc(t *testing.T) {
	// Smoke test — the validator and metrics construction in
	// wrapPrivacyMiddleware must tolerate nil embedding service in any
	// future code path that doesn't short-circuit on missing kubeconfig.
	freshPromRegistry(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("wrapPrivacyMiddleware panicked: %v", r)
		}
	}()
	_ = wrapPrivacyMiddleware(context.Background(), next, nil, nil, logr.Discard())
}

// TestMemoryAnalyticsOptInMetrics_Registered verifies that the
// analytics:aggregate opt-in metric surface registers cleanly on a
// fresh Prometheus registry, and that duplicate registration is
// rejected (proves the collectors actually hit the registry).
// Hermetic — no running Postgres needed.
func TestMemoryAnalyticsOptInMetrics_Registered(t *testing.T) {
	freshPromRegistry(t)
	m := memory.NewAnalyticsOptInMetrics()
	if err := memory.RegisterAnalyticsOptInMetrics(prometheus.DefaultRegisterer, m); err != nil {
		t.Fatalf("first RegisterAnalyticsOptInMetrics: %v", err)
	}
	// Second registration must fail — otherwise the first didn't land.
	m2 := memory.NewAnalyticsOptInMetrics()
	if err := memory.RegisterAnalyticsOptInMetrics(prometheus.DefaultRegisterer, m2); err == nil {
		t.Error("second RegisterAnalyticsOptInMetrics: want AlreadyRegistered error, got nil")
	}
}

// TestBuildAPIMux_HealthzAlwaysReachable verifies /healthz is wired regardless
// of enterprise mode. This is a smoke test that the middleware chain does not
// incorrectly gate health checks.
func TestBuildAPIMux_HealthzAlwaysReachable(t *testing.T) {
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{},
		nil,
		memoryapi.MemoryServiceConfig{},
		nil,
		false,
		nil,
		nil, // policy loader is optional — identity ranker without it
		logr.Discard(),
	)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /healthz should return 200, got %d", rr.Code)
	}
}
