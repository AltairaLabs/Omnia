/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
)

// multiTierStoreStub extends mockMemoryStore-like behavior with configurable
// multi-tier responses. Inline to avoid leaking test-only state into the
// shared mockMemoryStore type.
type multiTierStoreStub struct {
	mockMemoryStore
	mu       sync.Mutex
	mtCalls  []memory.MultiTierRequest
	mtResult *memory.MultiTierResult
	mtErr    error
	// Hybrid-path capture: embeddings passed to RetrieveMultiTierHybrid.
	mthEmbeddings [][]float32
	mthResult     *memory.MultiTierResult
	mthErr        error
}

func (m *multiTierStoreStub) RetrieveMultiTier(_ context.Context, req memory.MultiTierRequest) (*memory.MultiTierResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mtCalls = append(m.mtCalls, req)
	if m.mtErr != nil {
		return nil, m.mtErr
	}
	return m.mtResult, nil
}

func (m *multiTierStoreStub) RetrieveMultiTierHybrid(_ context.Context, _ memory.MultiTierRequest, emb []float32) (*memory.MultiTierResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mthEmbeddings = append(m.mthEmbeddings, emb)
	if m.mthErr != nil {
		return nil, m.mthErr
	}
	if m.mthResult != nil {
		return m.mthResult, nil
	}
	return &memory.MultiTierResult{}, nil
}

// fakeEmbedProvider is a memory.EmbeddingProvider that returns a fixed vector
// (or an error) for every input — enough to drive the service routing tests
// without a real embedding backend.
type fakeEmbedProvider struct {
	vec []float32
	err error
}

func (f fakeEmbedProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = f.vec
	}
	return out, nil
}

func (f fakeEmbedProvider) Dimensions() int { return len(f.vec) }

func newEmbedder(vec []float32, err error) *memory.EmbeddingService {
	return memory.NewEmbeddingService(nil, fakeEmbedProvider{vec: vec, err: err}, logr.Discard())
}

// TestRetrieveMultiTier_RoutesToHybridWhenEmbedderPresent proves that with an
// embedder configured and a non-empty query, the service embeds the query and
// routes to the hybrid store path — not the FTS-only RetrieveMultiTier.
func TestRetrieveMultiTier_RoutesToHybridWhenEmbedderPresent(t *testing.T) {
	store := &multiTierStoreStub{
		mthResult: &memory.MultiTierResult{
			Memories: []*memory.MultiTierMemory{{Memory: &memory.Memory{ID: "hit"}}},
			Total:    1,
		},
	}
	svc := NewMemoryService(store, newEmbedder([]float32{0.1, 0.2}, nil), MemoryServiceConfig{}, logr.Discard())

	res, err := svc.RetrieveMultiTier(context.Background(), memory.MultiTierRequest{
		WorkspaceID: testWS, Query: "hello",
	})
	require.NoError(t, err)
	require.Equal(t, 1, res.Total)

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.mthEmbeddings, 1, "hybrid path must be called once")
	assert.NotEmpty(t, store.mthEmbeddings[0], "hybrid must receive the query embedding")
	assert.Empty(t, store.mtCalls, "FTS multi-tier must not be called when embedder present")
}

// TestRetrieveMultiTier_FallsBackToFTSOnEmbedError proves an embedder failure
// degrades to FTS multi-tier rather than failing the recall.
func TestRetrieveMultiTier_FallsBackToFTSOnEmbedError(t *testing.T) {
	store := &multiTierStoreStub{mtResult: &memory.MultiTierResult{Total: 0}}
	svc := NewMemoryService(store, newEmbedder(nil, errors.New("boom")), MemoryServiceConfig{}, logr.Discard())

	_, err := svc.RetrieveMultiTier(context.Background(), memory.MultiTierRequest{
		WorkspaceID: testWS, Query: "hi",
	})
	require.NoError(t, err, "embed failure must not fail the recall")

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Len(t, store.mtCalls, 1, "must fall back to FTS multi-tier on embed error")
	assert.Empty(t, store.mthEmbeddings, "hybrid must not be used when embed failed")
}

// TestRetrieveMultiTier_EmptyQueryUsesFTS proves an empty query skips embedding
// and uses the FTS-only path even when an embedder is configured.
func TestRetrieveMultiTier_EmptyQueryUsesFTS(t *testing.T) {
	store := &multiTierStoreStub{mtResult: &memory.MultiTierResult{Total: 0}}
	svc := NewMemoryService(store, newEmbedder([]float32{0.1}, nil), MemoryServiceConfig{}, logr.Discard())

	_, err := svc.RetrieveMultiTier(context.Background(), memory.MultiTierRequest{WorkspaceID: testWS})
	require.NoError(t, err)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Len(t, store.mtCalls, 1, "empty query must use FTS multi-tier")
	assert.Empty(t, store.mthEmbeddings)
}

func TestRetrieveMultiTier_PassesThroughAndEmitsAudit(t *testing.T) {
	store := &multiTierStoreStub{
		mtResult: &memory.MultiTierResult{
			Memories: []*memory.MultiTierMemory{
				{Memory: &memory.Memory{ID: "m-1"}, Tier: memory.TierUser},
			},
			Total: 1,
		},
	}
	audit := newMockAuditLogger()
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetAuditLogger(audit)

	got, err := svc.RetrieveMultiTier(context.Background(), memory.MultiTierRequest{
		WorkspaceID: testWS,
		UserID:      "u-1",
		AgentID:     "a-1",
		Query:       "dark",
		Limit:       10,
	})
	require.NoError(t, err)
	require.Equal(t, 1, got.Total)
	assert.Equal(t, "m-1", got.Memories[0].ID)

	store.mu.Lock()
	require.Len(t, store.mtCalls, 1)
	call := store.mtCalls[0]
	store.mu.Unlock()
	assert.Equal(t, testWS, call.WorkspaceID)
	assert.Equal(t, "a-1", call.AgentID)

	entry := audit.receiveEntry(t)
	assert.Equal(t, auditEventMemoryAccessed, entry.EventType)
	assert.Equal(t, "retrieve_multi_tier", entry.Metadata["operation"])
	assert.Equal(t, testWS, entry.WorkspaceID)
	assert.Equal(t, "u-1", entry.UserID)
}

func TestRetrieveMultiTier_PropagatesStoreError(t *testing.T) {
	store := &multiTierStoreStub{mtErr: errors.New("boom")}
	audit := newMockAuditLogger()
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetAuditLogger(audit)

	_, err := svc.RetrieveMultiTier(context.Background(), memory.MultiTierRequest{WorkspaceID: testWS})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")

	// No audit on failure.
	select {
	case e := <-audit.entries:
		t.Fatalf("unexpected audit entry on store error: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// expected — no event
	}
}

func TestRetrieveMultiTier_BuildsHalfLifeFromPolicyLoader(t *testing.T) {
	store := &multiTierStoreStub{mtResult: &memory.MultiTierResult{}}
	loader := &stubPolicyLoader{
		policy: &omniav1alpha1.MemoryPolicy{
			Spec: omniav1alpha1.MemoryPolicySpec{
				Recall: &omniav1alpha1.MemoryRecallConfig{
					HalfLife: &omniav1alpha1.MemoryRecallHalfLife{
						User:          "7d",
						Institutional: "365d",
					},
				},
			},
		},
	}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetPolicyLoader(loader)

	_, err := svc.RetrieveMultiTier(context.Background(), memory.MultiTierRequest{WorkspaceID: testWS})
	require.NoError(t, err)

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.mtCalls, 1)
	assert.Equal(t, 7*24*time.Hour, store.mtCalls[0].HalfLife.User, "service must populate User half-life from policy")
	assert.Equal(t, 365*24*time.Hour, store.mtCalls[0].HalfLife.Institutional)
	// Unset tier defaults to the baseline rather than zero.
	assert.Equal(t, 30*24*time.Hour, store.mtCalls[0].HalfLife.Agent)
}

// stubPolicyLoader is a controllable PolicyLoader for service-layer tests.
type stubPolicyLoader struct {
	policy *omniav1alpha1.MemoryPolicy
	err    error
}

func (s *stubPolicyLoader) Load(_ context.Context) (*omniav1alpha1.MemoryPolicy, error) {
	return s.policy, s.err
}

func TestRetrieveMultiTier_BuildsRankerFromPolicyLoader(t *testing.T) {
	store := &multiTierStoreStub{
		mtResult: &memory.MultiTierResult{Memories: nil, Total: 0},
	}
	loader := &stubPolicyLoader{
		policy: &omniav1alpha1.MemoryPolicy{
			Spec: omniav1alpha1.MemoryPolicySpec{
				TierPrecedence: &omniav1alpha1.TierPrecedenceConfig{
					Multiplicative: &omniav1alpha1.MultiplicativeTierPrecedence{
						Institutional: "2.0",
						Agent:         "1.0",
						User:          "0.5",
					},
				},
			},
		},
	}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetPolicyLoader(loader)

	_, err := svc.RetrieveMultiTier(context.Background(), memory.MultiTierRequest{WorkspaceID: testWS})
	require.NoError(t, err)

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.mtCalls, 1)
	require.NotNil(t, store.mtCalls[0].Ranker, "service must populate Ranker from policy loader")
	mr, ok := store.mtCalls[0].Ranker.(memory.MultiplicativeTierRanker)
	require.True(t, ok, "expected MultiplicativeTierRanker, got %T", store.mtCalls[0].Ranker)
	assert.InDelta(t, 2.0, mr.Weights[memory.TierInstitutional], 1e-9)
	assert.InDelta(t, 0.5, mr.Weights[memory.TierUser], 1e-9)
}

func TestRetrieveMultiTier_PolicyLoaderErrorFallsBackToIdentity(t *testing.T) {
	store := &multiTierStoreStub{
		mtResult: &memory.MultiTierResult{Memories: nil, Total: 0},
	}
	loader := &stubPolicyLoader{err: errors.New("api outage")}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetPolicyLoader(loader)

	_, err := svc.RetrieveMultiTier(context.Background(), memory.MultiTierRequest{WorkspaceID: testWS})
	require.NoError(t, err, "loader errors must not fail retrieval")

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.mtCalls, 1)
	_, ok := store.mtCalls[0].Ranker.(memory.IdentityTierRanker)
	assert.True(t, ok, "loader error must fall back to identity ranker; got %T", store.mtCalls[0].Ranker)
}

func TestRetrieveMultiTier_PreservesCallerSuppliedRanker(t *testing.T) {
	store := &multiTierStoreStub{
		mtResult: &memory.MultiTierResult{Memories: nil, Total: 0},
	}
	loader := &stubPolicyLoader{
		policy: &omniav1alpha1.MemoryPolicy{
			Spec: omniav1alpha1.MemoryPolicySpec{
				TierPrecedence: &omniav1alpha1.TierPrecedenceConfig{
					Multiplicative: &omniav1alpha1.MultiplicativeTierPrecedence{
						Institutional: "5.0",
					},
				},
			},
		},
	}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetPolicyLoader(loader)

	supplied := memory.IdentityTierRanker{}
	_, err := svc.RetrieveMultiTier(context.Background(), memory.MultiTierRequest{
		WorkspaceID: testWS,
		Ranker:      supplied,
	})
	require.NoError(t, err)

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.mtCalls, 1)
	_, ok := store.mtCalls[0].Ranker.(memory.IdentityTierRanker)
	assert.True(t, ok, "caller-supplied ranker must take precedence over loader; got %T", store.mtCalls[0].Ranker)
}
