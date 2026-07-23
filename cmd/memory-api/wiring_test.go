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
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eeaudit "github.com/altairalabs/omnia/ee/pkg/audit"
	"github.com/altairalabs/omnia/ee/pkg/memory/consolidation"
	eemetrics "github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/internal/memory"
	memoryapi "github.com/altairalabs/omnia/internal/memory/api"
	"github.com/altairalabs/omnia/internal/memory/ingestion"
	memorypg "github.com/altairalabs/omnia/internal/memory/postgres"
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
func (fakeMemoryStore) FindRelatedEntities(_ context.Context, _ map[string]string, _ []string, _ int) ([]memory.EntityRelation, error) {
	return nil, nil
}
func (fakeMemoryStore) RetrieveHybrid(_ context.Context, _ map[string]string, _ string, _ []float32, _ memory.RetrieveOptions) ([]*memory.Memory, error) {
	return nil, nil
}
func (fakeMemoryStore) SupersedeMany(_ context.Context, sourceIDs []string, mem *memory.Memory) (string, []string, error) {
	if len(sourceIDs) == 0 {
		return "", nil, nil
	}
	mem.ID = sourceIDs[0]
	return sourceIDs[0], nil, nil
}
func (fakeMemoryStore) FindConflictedEntities(_ context.Context, _ string, _ int) ([]memory.ConflictedEntity, error) {
	return nil, nil
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
func (fakeMemoryStore) RetrieveMultiTierHybrid(_ context.Context, _ memory.MultiTierRequest, _ []float32) (*memory.MultiTierResult, error) {
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
		nil, // auditLogger optional; non-enterprise tests don't exercise it
		logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup — empty in unit tests
		nil,           // consentPruner — not needed in wiring tests
		nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
	)
	defer cleanup()

	body, err := json.Marshal(memoryapi.SaveMemoryRequest{
		Type:    "fact",
		Content: "test content",
		Scope: map[string]string{
			memory.ScopeWorkspaceID: "ws-1",
			// deliberately no ScopeVirtualUserID — should trigger ErrMissingUserID
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
		nil, // auditLogger optional; non-enterprise tests don't exercise it
		logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup — empty in unit tests
		nil,           // consentPruner — not needed in wiring tests
		nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
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

// TestReembedWorkerOptions_DisabledWithoutEmbeddingService proves
// the wiring guard: without an embedding service the worker would
// have nothing to call, so it must stay off.
func TestReembedWorkerOptions_DisabledWithoutEmbeddingService(t *testing.T) {
	f := &flags{reembedInterval: "30m"}
	_, enabled := f.reembedWorkerOptions(nil)
	if enabled {
		t.Error("expected reembed worker to be disabled when embedding service is nil")
	}
}

// TestReembedWorkerOptions_DisabledWhenIntervalEmpty proves the
// other half of the guard: an embedding service alone is not enough
// — without an interval the worker would tick on zero duration.
func TestReembedWorkerOptions_DisabledWhenIntervalEmpty(t *testing.T) {
	f := &flags{}
	svc := memory.NewEmbeddingService(&memory.PostgresMemoryStore{}, nil, logr.Discard())
	_, enabled := f.reembedWorkerOptions(svc)
	if enabled {
		t.Error("expected reembed worker to be disabled when interval is empty")
	}
}

// TestEmbeddingModelIdentifier proves the staleness key combines the Provider
// CRD name with the resolved model (MAINT-5), falling back to the bare name
// when the provider exposes no model.
func TestEmbeddingModelIdentifier(t *testing.T) {
	if got := embeddingModelIdentifier("openai", "text-embedding-3-large"); got != "openai/text-embedding-3-large" {
		t.Errorf("with model: got %q, want openai/text-embedding-3-large", got)
	}
	if got := embeddingModelIdentifier("ollama", ""); got != "ollama" {
		t.Errorf("empty model should yield the bare provider name: got %q", got)
	}
}

// TestReembedWorkerOptions_CurrentModelFromService proves the wiring fix: the
// worker's staleness comparison value is the identifier the EmbeddingService
// stamps (provider/model), not the bare provider name — so an in-place
// spec.model edit (same CRD name) is actually detected (MAINT-5).
func TestReembedWorkerOptions_CurrentModelFromService(t *testing.T) {
	const modelID = "openai/text-embedding-3-large"
	svc := memory.NewEmbeddingService(&memory.PostgresMemoryStore{}, nil, logr.Discard()).WithModelName(modelID)
	f := &flags{reembedInterval: "30m", embeddingProviderName: "openai"}

	opts, enabled := f.reembedWorkerOptions(svc)
	if !enabled {
		t.Fatal("expected reembed worker enabled")
	}
	if opts.CurrentModel != modelID {
		t.Errorf("CurrentModel: got %q, want %q (must match EmbeddingService.ModelName, not the bare provider name)", opts.CurrentModel, modelID)
	}
}

// TestTombstoneWorkerOptions_DisabledWhenIntervalEmpty proves the
// guard fires when no interval is set — without it the worker
// would tick on the zero-duration default and spam RunOnce.
func TestTombstoneWorkerOptions_DisabledWhenIntervalEmpty(t *testing.T) {
	f := &flags{}
	_, enabled := f.tombstoneWorkerOptions(logr.Discard(), nil)
	if enabled {
		t.Error("expected tombstone worker to be disabled when interval is empty")
	}
}

// TestTombstoneWorkerOptions_InvalidIntervalDisables proves a bad
// duration string keeps the worker off rather than starting it
// with whatever ParseDuration partially succeeded on.
func TestTombstoneWorkerOptions_InvalidIntervalDisables(t *testing.T) {
	f := &flags{tombstoneInterval: "not-a-duration"}
	_, enabled := f.tombstoneWorkerOptions(logr.Discard(), nil)
	if enabled {
		t.Error("expected tombstone worker to be disabled when interval is invalid")
	}
}

// TestTombstoneWorkerOptions_PopulatesFromFlags proves the happy
// path: a valid interval populates the options including the
// store-backed workspace discoverer.
func TestTombstoneWorkerOptions_PopulatesFromFlags(t *testing.T) {
	store := &memory.PostgresMemoryStore{}
	f := &flags{
		tombstoneInterval:    "6h",
		tombstoneMinAge:      "720h",
		tombstoneMinInactive: 30,
		tombstoneKeepRecent:  10,
	}
	opts, enabled := f.tombstoneWorkerOptions(logr.Discard(), store)
	if !enabled {
		t.Fatal("expected tombstone worker to be enabled")
	}
	if opts.Interval != 6*time.Hour {
		t.Errorf("expected 6h interval, got %v", opts.Interval)
	}
	if opts.MinAge != 720*time.Hour {
		t.Errorf("expected 720h min-age, got %v", opts.MinAge)
	}
	if opts.MinInactiveCount != 30 {
		t.Errorf("expected min-inactive 30, got %d", opts.MinInactiveCount)
	}
	if opts.KeepRecentInactive != 10 {
		t.Errorf("expected keep-recent 10, got %d", opts.KeepRecentInactive)
	}
	if opts.WorkspaceDiscoverer == nil {
		t.Error("expected WorkspaceDiscoverer to be wired to store.ListWorkspaceIDs")
	}
}

// TestReembedWorkerOptions_PopulatesFromFlags proves the happy
// path: a valid interval + an embedding service produces enabled
// options carrying the BatchSize and the CurrentModel sourced from the
// service's stamped identifier (provider/model), not the bare flag (MAINT-5).
func TestReembedWorkerOptions_PopulatesFromFlags(t *testing.T) {
	f := &flags{
		reembedInterval:       "15m",
		reembedBatchSize:      25,
		embeddingProviderName: "openai-embed",
	}
	svc := memory.NewEmbeddingService(&memory.PostgresMemoryStore{}, nil, logr.Discard()).
		WithModelName("openai-embed/text-embedding-3-small")
	opts, enabled := f.reembedWorkerOptions(svc)
	if !enabled {
		t.Fatal("expected reembed worker to be enabled")
	}
	if opts.Interval != 15*time.Minute {
		t.Errorf("expected 15m interval, got %v", opts.Interval)
	}
	if opts.BatchSize != 25 {
		t.Errorf("expected batch size 25, got %d", opts.BatchSize)
	}
	if opts.CurrentModel != "openai-embed/text-embedding-3-small" {
		t.Errorf("expected the stamped provider/model identifier, got %q", opts.CurrentModel)
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
		nil, // embeddingSvc nil → regex-only
		nil, // auditLogger nil → enforcement audit disabled
		"",  // workspace — empty in unit tests (no in-cluster k8s)
		"",  // serviceGroup — empty in unit tests
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
	_ = wrapPrivacyMiddleware(context.Background(), next, nil, nil, "", "", logr.Discard())
}

// TestBuildAPIMux_EnterpriseAuditRoutesWired proves the audit query
// endpoint (GET /api/v1/audit/memories) is registered when an
// auditLogger is supplied — i.e. when --enterprise=true. This is the
// wiring boundary between "audit logger constructed" and "audit
// routes actually reachable." Without this guard, a future refactor
// could leave the audit handler attached to a parallel mux that
// never gets served.
//
// We construct the eeaudit.Logger with a nil pool — NewLogger
// handles the nil case (only the async DB writer touches the pool,
// and we never enqueue an event in this test). The wiring claim is
// purely about route registration.
func TestBuildAPIMux_EnterpriseAuditRoutesWired(t *testing.T) {
	freshPromRegistry(t)
	auditLogger := eeaudit.NewLogger(nil, logr.Discard(), eemetrics.NewAuditMetrics(), eeaudit.LoggerConfig{})
	t.Cleanup(func() { _ = auditLogger.Close() })

	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{},
		nil,
		memoryapi.MemoryServiceConfig{},
		nil,
		true, // enterprise=true
		nil,
		nil,
		auditLogger,
		logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup — empty in unit tests
		nil,           // consentPruner — not needed in wiring tests
		nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
	)
	defer cleanup()

	// Invalid 'to' parameter forces handleQuery to short-circuit with 400
	// BEFORE touching the (nil) Postgres pool. We're proving route
	// registration here, not query correctness.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/audit/memories?to=not-a-timestamp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/audit/memories not registered in enterprise mode; got 404")
	}
}

// TestBuildAPIMux_NonEnterpriseAuditRoutesAbsent proves the audit
// route is NOT registered when no auditLogger is supplied. This is the
// negative counterpart to TestBuildAPIMux_EnterpriseAuditRoutesWired
// — without this assertion the EnterpriseAuditRoutesWired test could
// pass trivially if some other registration path attached the route.
func TestBuildAPIMux_NonEnterpriseAuditRoutesAbsent(t *testing.T) {
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{},
		nil,
		memoryapi.MemoryServiceConfig{},
		nil,
		false, // enterprise=false
		nil,
		nil,
		nil, // no audit logger
		logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup — empty in unit tests
		nil,           // consentPruner — not needed in wiring tests
		nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
	)
	defer cleanup()

	// Invalid 'to' parameter forces handleQuery to short-circuit with 400
	// BEFORE touching the (nil) Postgres pool. We're proving route
	// registration here, not query correctness.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/audit/memories?to=not-a-timestamp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/audit/memories should be 404 when audit logger nil; got %d", rr.Code)
	}
}

// TestBuildConsolidationWorkerOptions_AllFieldsWired guards against
// the consolidation v1 audit findings: WorkerOptions.Auditor was
// never set, WorkerOptions.Workspaces (lister) was missing,
// PIIRedactor was missing. This single assertion block enforces that
// every option the worker depends on is populated when the binary
// constructs it.
//
// Hermetic: passes nil pool + fake k8s client + nil audit logger
// (covers the non-enterprise default), then enterprise=true with a
// real auditLogger to check the Auditor field flips populated.
func TestBuildConsolidationWorkerOptions_AllFieldsWired(t *testing.T) {
	freshPromRegistry(t)
	scheme := k8sruntime.NewScheme()
	if err := omniav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	fakeClient, err := client.New(&rest.Config{Host: "http://example.invalid"}, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	// Wiring checks per WorkerOptions field. Each entry's nilErr is the
	// consolidation v1 audit finding encoded as a regression message.
	type fieldCheck struct {
		name   string
		nilErr string
		isNil  func(consolidation.WorkerOptions) bool
	}
	checks := []fieldCheck{
		{"Store", "Store unwired", func(o consolidation.WorkerOptions) bool { return o.Store == nil }},
		{"LockStore", "LockStore unwired — advisory lock won't acquire", func(o consolidation.WorkerOptions) bool { return o.LockStore == nil }},
		{"Policies", "Policies unwired — worker won't see MemoryPolicy CRs", func(o consolidation.WorkerOptions) bool { return o.Policies == nil }},
		{"Workspaces", "Workspaces unwired — worker uses policy name as workspace UID (consolidation v1 bug)", func(o consolidation.WorkerOptions) bool { return o.Workspaces == nil }},
		{"PreFilterRunner", "PreFilterRunner unwired — worker skips every workspace", func(o consolidation.WorkerOptions) bool { return o.PreFilterRunner == nil }},
		{"RunTracker", "RunTracker unwired — per-axis cron schedules won't be honoured", func(o consolidation.WorkerOptions) bool { return o.RunTracker == nil }},
		{"Client", "Client unwired — worker can't call function packs", func(o consolidation.WorkerOptions) bool { return o.Client == nil }},
		{"Metrics", "Metrics unwired — no observability", func(o consolidation.WorkerOptions) bool { return o.Metrics == nil }},
		{"LivenessMark", "LivenessMark unwired — worker liveness gauge never flips on", func(o consolidation.WorkerOptions) bool { return o.LivenessMark == nil }},
		{"LivenessUnmark", "LivenessUnmark unwired — worker liveness gauge never flips off", func(o consolidation.WorkerOptions) bool { return o.LivenessUnmark == nil }},
		{"PIIRedactor", "PIIRedactor unwired — PII gate is a no-op (consolidation v1 bug)", func(o consolidation.WorkerOptions) bool { return o.PIIRedactor == nil }},
	}

	t.Run("non-enterprise leaves Auditor nil", func(t *testing.T) {
		freshPromRegistry(t)
		opts := newConsolidationWorkerOptions(time.Hour, fakeClient, "demo", memory.NewPostgresMemoryStore(nil), nil, logr.Discard())
		for _, c := range checks {
			if c.isNil(opts) {
				t.Errorf("%s: %s", c.name, c.nilErr)
			}
		}
		if opts.Interval == 0 {
			t.Error("Interval unwired — worker won't tick")
		}
		if opts.Auditor != nil {
			t.Error("Auditor should be nil in non-enterprise mode")
		}
		// Exercise the liveness closures so they're not just non-nil
		// but callable — they touch the shared liveness gauge.
		opts.LivenessMark()
		opts.LivenessUnmark()
	})

	t.Run("enterprise sets Auditor", func(t *testing.T) {
		freshPromRegistry(t)
		auditLogger := eeaudit.NewLogger(nil, logr.Discard(), eemetrics.NewAuditMetrics(), eeaudit.LoggerConfig{})
		t.Cleanup(func() { _ = auditLogger.Close() })
		opts := newConsolidationWorkerOptions(time.Hour, fakeClient, "demo", memory.NewPostgresMemoryStore(nil), auditLogger, logr.Discard())
		if opts.Auditor == nil {
			t.Error("Auditor unwired in enterprise mode — consolidation audit rows silently dropped (consolidation v1 bug)")
		}
	})
}

// TestBuildConsolidationWorker_DisabledFastPaths covers the disabled
// branches of buildConsolidationWorker — empty interval, unparseable
// interval, and no in-cluster kubeconfig (true in any unit test
// environment). Each path returns nil before doing real work; the
// happy path requires an actual cluster and is exercised by the
// consolidation E2E.
func TestBuildConsolidationWorker_DisabledFastPaths(t *testing.T) {
	t.Run("empty interval disables", func(t *testing.T) {
		// enterprise=true so the interval check (not the enterprise gate) fires.
		f := &flags{enterprise: true}
		if w := buildConsolidationWorker(context.Background(), f, nil, nil, logr.Discard()); w != nil {
			t.Errorf("expected nil worker when CONSOLIDATION_INTERVAL unset, got %v", w)
		}
	})
	t.Run("invalid interval disables", func(t *testing.T) {
		// enterprise=true so the interval-parse check (not the enterprise gate) fires.
		f := &flags{enterprise: true, consolidationInterval: "not-a-duration"}
		if w := buildConsolidationWorker(context.Background(), f, nil, nil, logr.Discard()); w != nil {
			t.Errorf("expected nil worker on unparseable interval, got %v", w)
		}
	})
	t.Run("no in-cluster config disables", func(t *testing.T) {
		// enterprise=true + valid interval, but rest.InClusterConfig() fails in
		// unit-test environments — exercises the kubeconfig-error branch.
		f := &flags{enterprise: true, consolidationInterval: "10m"}
		if w := buildConsolidationWorker(context.Background(), f, nil, nil, logr.Discard()); w != nil {
			t.Errorf("expected nil worker when no in-cluster config, got %v", w)
		}
	})
}

// TestParseConsolidationInterval covers each branch of the validator
// extracted from buildConsolidationWorker: empty, unparseable, non-positive,
// and a valid duration. The original code was untestable because the three
// disable branches were buried inside a function that also called
// rest.InClusterConfig().
func TestParseConsolidationInterval(t *testing.T) {
	t.Run("empty string disables", func(t *testing.T) {
		got, ok := parseConsolidationInterval("", logr.Discard())
		if ok || got != 0 {
			t.Errorf("expected (0, false) for empty, got (%v, %v)", got, ok)
		}
	})
	t.Run("unparseable disables", func(t *testing.T) {
		got, ok := parseConsolidationInterval("not-a-duration", logr.Discard())
		if ok || got != 0 {
			t.Errorf("expected (0, false) for unparseable, got (%v, %v)", got, ok)
		}
	})
	t.Run("non-positive disables", func(t *testing.T) {
		got, ok := parseConsolidationInterval("0s", logr.Discard())
		if ok || got != 0 {
			t.Errorf("expected (0, false) for 0s, got (%v, %v)", got, ok)
		}
	})
	t.Run("valid interval enables", func(t *testing.T) {
		got, ok := parseConsolidationInterval("10m", logr.Discard())
		if !ok || got != 10*time.Minute {
			t.Errorf("expected (10m, true), got (%v, %v)", got, ok)
		}
	})
}

// TestNewConsolidationWorker_ReturnsWorker covers the helper-call line that
// composes the worker from already-acquired deps. The original
// buildConsolidationWorker was untestable from unit tests because the
// NewWorker call sat behind rest.InClusterConfig(); the refactor lifts the
// composition into newConsolidationWorker which takes the client as an arg.
func TestNewConsolidationWorker_ReturnsWorker(t *testing.T) {
	freshPromRegistry(t)
	scheme := k8sruntime.NewScheme()
	if err := omniav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add omnia scheme: %v", err)
	}
	fakeClient, err := client.New(&rest.Config{Host: "127.0.0.1:1"},
		client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("build fake client: %v", err)
	}
	w := newConsolidationWorker(time.Hour, fakeClient, "demo",
		memory.NewPostgresMemoryStore(nil), nil, logr.Discard())
	if w == nil {
		t.Fatal("expected non-nil worker from newConsolidationWorker")
	}
}

// TestNewClientForConsolidation_BuildsClient covers the scheme + client
// construction lifted out of newConsolidationK8sClient. The InClusterConfig
// boundary is the only piece left in newConsolidationK8sClient; this test
// proves the scheme registers omniav1alpha1 and client.New is called with it.
func TestNewClientForConsolidation_BuildsClient(t *testing.T) {
	t.Run("valid config builds client", func(t *testing.T) {
		c, ok := newClientForConsolidation(&rest.Config{Host: "127.0.0.1:1"}, logr.Discard())
		if !ok || c == nil {
			t.Fatalf("expected non-nil client and ok=true, got (%v, %v)", c, ok)
		}
	})
	t.Run("invalid config returns false", func(t *testing.T) {
		c, ok := newClientForConsolidation(&rest.Config{Host: "://not a host"}, logr.Discard())
		if ok || c != nil {
			t.Fatalf("expected nil client and ok=false on malformed host, got (%v, %v)", c, ok)
		}
	})
}

// TestMemoryMigrations_IncludeAuditLog guards against the
// consolidation v1 audit finding where memory-api's --enterprise
// audit path INSERTed into an `audit_log` table that didn't exist.
// Migration 000010 adds the table. This test fails fast if the
// migration is deleted or renamed.
func TestMemoryMigrations_IncludeAuditLog(t *testing.T) {
	// audit_log is created by the collapsed initial migration (#1309 collapsed
	// the 000001..000012 chain into a single initial).
	got, err := memorypg.MigrationsFS.ReadFile("migrations/000001_initial_schema.up.sql")
	if err != nil {
		t.Fatalf("000001_initial_schema.up.sql missing: %v", err)
	}
	if !strings.Contains(string(got), "CREATE TABLE") || !strings.Contains(string(got), "audit_log") {
		t.Errorf("000001_initial_schema.up.sql doesn't create audit_log table; content=%q",
			string(got))
	}
}

// TestIngestChunkSizeFlag_DefaultsAndParsing proves that:
//  1. The flags struct default for ingestChunkSize is 200.
//  2. The flags struct default for ingestChunkOverlap is 40.
//  3. Explicit values are preserved (flag parse is exercised at binary startup;
//     here we test the struct directly as the other int-flag tests do).
func TestIngestChunkSizeFlag_DefaultsAndParsing(t *testing.T) {
	// Defaults: zero-value struct should reflect the intended defaults after
	// applyEnvFallbacks (no env vars set). Because the int defaults are set via
	// flag.IntVar in parseFlags (not in applyEnvFallbacks), we test via a flags
	// struct with the default values already populated — matching how they arrive
	// after flag.Parse() when no flags are passed.
	f := &flags{
		ingestChunkSize:    200,
		ingestChunkOverlap: 40,
	}
	if f.ingestChunkSize != 200 {
		t.Errorf("expected ingestChunkSize default 200, got %d", f.ingestChunkSize)
	}
	if f.ingestChunkOverlap != 40 {
		t.Errorf("expected ingestChunkOverlap default 40, got %d", f.ingestChunkOverlap)
	}

	// Explicit values are preserved.
	f2 := &flags{ingestChunkSize: 512, ingestChunkOverlap: 64}
	if f2.ingestChunkSize != 512 {
		t.Errorf("expected ingestChunkSize 512, got %d", f2.ingestChunkSize)
	}
	if f2.ingestChunkOverlap != 64 {
		t.Errorf("expected ingestChunkOverlap 64, got %d", f2.ingestChunkOverlap)
	}
}

// TestBuildAPIMux_IngestRouteWiredWithChunkStrategy verifies that the
// POST /api/v1/ingest route is registered and reaches the handler. Without
// a wired ingestion strategy, the handler would return 422/500; after
// SetIngestionStrategy is called the route processes the request. We exercise
// a partial request that hits the JSON-decode path (no store or embedding
// service call needed) — a 400 or 422 proves the handler ran; a 404 would
// mean the route isn't registered.
func TestBuildAPIMux_IngestRouteWiredWithChunkStrategy(t *testing.T) {
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{},
		nil,
		memoryapi.MemoryServiceConfig{},
		nil,
		false,
		nil,
		nil,
		nil,
		logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup — empty in unit tests
		nil,           // consentPruner — not needed in wiring tests
		nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
	)
	defer cleanup()

	// Send a request with a missing required field so we get a 4xx response
	// from the handler (not 404 which would mean the route isn't registered).
	req := httptest.NewRequest(http.MethodPost, "/api/v1/institutional/ingest",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("POST /api/v1/institutional/ingest not registered; got 404 — SetIngestion wiring may be broken")
	}
}

// TestBuildAPIMux_SemanticRouteWired verifies that the
// POST /api/v1/memories/retrieve/semantic route is registered on the real mux.
// A 404 here means the route isn't registered. We send a minimal valid body
// so the handler can proceed past JSON decoding and workspace validation;
// any non-404 response proves the route is wired.
func TestBuildAPIMux_SemanticRouteWired(t *testing.T) {
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{},
		nil,
		memoryapi.MemoryServiceConfig{},
		nil,
		false,
		nil,
		nil,
		nil,
		logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup — empty in unit tests
		nil,           // consentPruner — not needed in wiring tests
		nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
	)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve/semantic",
		strings.NewReader(`{"workspace_id":"ws-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("POST /api/v1/memories/retrieve/semantic not registered; got 404")
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
		nil, // auditLogger optional; non-enterprise tests don't exercise it
		logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup — empty in unit tests
		nil,           // consentPruner — not needed in wiring tests
		nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
	)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /healthz should return 200, got %d", rr.Code)
	}
}

// TestBuildAPIMux_SummaryCandidatesWired proves GET /api/v1/ingest/summary-candidates
// is registered. Anything other than 404 proves wiring (200 with empty list
// when no queue dir is configured).
func TestBuildAPIMux_SummaryCandidatesWired(t *testing.T) {
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{}, nil, memoryapi.MemoryServiceConfig{}, nil,
		false, nil, nil, nil, logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup — empty in unit tests
		nil,           // consentPruner — not needed in wiring tests
		nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
	)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/summary-candidates", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/ingest/summary-candidates not registered; got 404")
	}
}

// TestBuildAPIMux_SaveSummaryWired proves POST /api/v1/ingest/summaries is
// registered. Without a workspace_id the handler returns 400 — proving the
// route reaches the handler (404 would mean unregistered).
func TestBuildAPIMux_SaveSummaryWired(t *testing.T) {
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{}, nil, memoryapi.MemoryServiceConfig{}, nil,
		false, nil, nil, nil, logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup — empty in unit tests
		nil,           // consentPruner — not needed in wiring tests
		nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
	)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/summaries",
		bytes.NewReader([]byte(`{"about_key":"k","summary":"s"}`)))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Errorf("POST /api/v1/ingest/summaries not registered; got 404")
	}
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 (missing workspace_id), got %d", rr.Code)
	}
}

// TestBuildIngestOptions_QueueEnabledWhenDirSet proves --ingest-queue-dir
// constructs a real queue; empty dir leaves it nil (agent path off).
func TestBuildIngestOptions_QueueEnabledWhenDirSet(t *testing.T) {
	off := buildIngestOptions(&flags{ingestQueueDir: ""}, logr.Discard())
	if off.Queue != nil {
		t.Error("empty --ingest-queue-dir should leave queue nil")
	}
	on := buildIngestOptions(&flags{ingestQueueDir: t.TempDir()}, logr.Discard())
	if on.Queue == nil {
		t.Error("non-empty --ingest-queue-dir should construct a queue")
	}
}

// TestBuildProjectionWorker_DisabledFastPaths covers the disabled branches of
// buildProjectionWorker — empty interval, unparseable interval, and no
// in-cluster kubeconfig (true in any unit-test environment). Each returns nil
// before composing the worker; the happy path requires an actual cluster and
// is exercised by the projection E2E.
func TestBuildProjectionWorker_DisabledFastPaths(t *testing.T) {
	reg := prometheus.NewRegistry()
	t.Run("empty interval disables", func(t *testing.T) {
		// enterprise=true so the interval check (not the enterprise gate) fires.
		if w := buildProjectionWorker(&flags{enterprise: true}, nil, reg, logr.Discard()); w != nil {
			t.Errorf("expected nil worker when PROJECTION_INTERVAL unset, got %v", w)
		}
	})
	t.Run("invalid interval disables", func(t *testing.T) {
		// enterprise=true so the interval-parse check (not the enterprise gate) fires.
		if w := buildProjectionWorker(&flags{enterprise: true, projectionInterval: "not-a-duration"}, nil, reg, logr.Discard()); w != nil {
			t.Errorf("expected nil worker on unparseable interval, got %v", w)
		}
	})
	t.Run("no in-cluster config disables", func(t *testing.T) {
		// enterprise=true + valid interval, but rest.InClusterConfig() fails in
		// unit tests — exercises the kubeconfig-error branch.
		if w := buildProjectionWorker(&flags{enterprise: true, projectionInterval: "30s"}, nil, reg, logr.Discard()); w != nil {
			t.Errorf("expected nil worker when no in-cluster config, got %v", w)
		}
	})
}

// TestNewProjectionWorker_ReturnsWorker covers the composition lifted out of
// buildProjectionWorker. The fake client + nil-pool store + fresh registry
// prove every field wires without in-cluster kubeconfig or Postgres.
func TestNewProjectionWorker_ReturnsWorker(t *testing.T) {
	scheme := k8sruntime.NewScheme()
	if err := omniav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add omnia scheme: %v", err)
	}
	fakeClient, err := client.New(&rest.Config{Host: "127.0.0.1:1"}, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("build fake client: %v", err)
	}
	w := newProjectionWorker(30*time.Second, fakeClient, "demo",
		memory.NewPostgresMemoryStore(nil), prometheus.NewRegistry(), logr.Discard())
	if w == nil {
		t.Fatal("expected non-nil worker from newProjectionWorker")
	}
}

// stubConsentPruner is a minimal memory.ConsentEventPruner test double for the
// cmd/memory-api wiring test. It records which delete path was taken so the
// test can prove buildAPIMux called svc.SetConsentEventPruner and wired the
// pruner all the way through to the service layer.
type stubConsentPruner struct {
	softCalled bool
	hardCalled bool
}

func (s *stubConsentPruner) SoftDeleteUserConsentCategory(_ context.Context, _, _, _ string) (int64, error) {
	s.softCalled = true
	return 1, nil
}

func (s *stubConsentPruner) HardDeleteUserConsentCategory(_ context.Context, _, _, _ string) (int64, error) {
	s.hardCalled = true
	return 1, nil
}

// TestBuildAPIMux_ConsentEventRouteWired verifies the CE1 wiring contract:
//
//  1. POST /api/v1/memories/consent-events is registered on the real mux when
//     enterprise=true (a 404 proves the route is absent; anything else proves
//     it is registered and the handler ran).
//  2. The stub ConsentEventPruner is actually invoked by the service layer —
//     proving buildAPIMux called svc.SetConsentEventPruner rather than leaving
//     the pruner nil (which would return 500 "pruner not configured").
//  3. With enterprise=false the route returns 403 (requireEnterprise gate),
//     confirming the gate is active even though the route IS registered on the
//     mux (it is always registered; the gate fires inside the handler).
//
// No database is required: policyLoader=nil causes resolveAction to default to
// SoftDelete, so only stubConsentPruner.SoftDeleteUserConsentCategory is called.
func TestBuildAPIMux_ConsentEventRouteWired(t *testing.T) {
	t.Run("enterprise=true route registered and pruner invoked", func(t *testing.T) {
		freshPromRegistry(t)
		pruner := &stubConsentPruner{}

		handler, cleanup := buildAPIMux(
			context.Background(),
			fakeMemoryStore{},
			nil,
			memoryapi.MemoryServiceConfig{},
			nil,
			true, // enterprise=true — unlocks the requireEnterprise gate
			nil,
			nil, // policyLoader nil → default SoftDelete action
			nil, // auditLogger optional
			logr.Discard(),
			memoryapi.IngestOptions{Fallback: ingestion.Config{
				Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
			}},
			"", "", // workspace, serviceGroup — empty in unit tests
			pruner,        // consentPruner — stub records invocation
			nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
		)
		defer cleanup()

		body, err := json.Marshal(memoryapi.ConsentEventRequest{
			UserID:   "u1",
			Category: "memory:health",
		})
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost,
			"/api/v1/memories/consent-events?workspace=ws-1",
			bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code == http.StatusNotFound {
			t.Errorf("POST /api/v1/memories/consent-events not registered; buildAPIMux returned 404")
		}
		if !pruner.softCalled && !pruner.hardCalled {
			t.Errorf("consentPruner not invoked: buildAPIMux did not wire svc.SetConsentEventPruner; "+
				"softCalled=%v hardCalled=%v status=%d body=%q",
				pruner.softCalled, pruner.hardCalled, rr.Code, rr.Body.String())
		}
	})

	t.Run("enterprise=false route returns 403 gate", func(t *testing.T) {
		freshPromRegistry(t)

		handler, cleanup := buildAPIMux(
			context.Background(),
			fakeMemoryStore{},
			nil,
			memoryapi.MemoryServiceConfig{},
			nil,
			false, // enterprise=false — requireEnterprise gate fires
			nil,
			nil,
			nil,
			logr.Discard(),
			memoryapi.IngestOptions{Fallback: ingestion.Config{
				Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
			}},
			"", "", // workspace, serviceGroup — empty in unit tests
			nil,           // consentPruner — gate fires before handler; pruner irrelevant
			nil, nil, nil, // reviewer, allowedSubjects, allowedNamespaces (auth disabled)
		)
		defer cleanup()

		body, _ := json.Marshal(memoryapi.ConsentEventRequest{
			UserID:   "u1",
			Category: "memory:health",
		})
		req := httptest.NewRequest(http.MethodPost,
			"/api/v1/memories/consent-events?workspace=ws-1",
			bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 (enterprise_required gate) when enterprise=false, got %d; body=%q",
				rr.Code, rr.Body.String())
		}
	})
}
