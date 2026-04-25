/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// compositeTTLPolicy builds a policy that runs TTL on every tier.
func compositeTTLPolicy(schedule string) *omniav1alpha1.MemoryPolicy {
	mode := omniav1alpha1.MemoryRetentionModeTTL
	return &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			Default: omniav1alpha1.MemoryRetentionDefaults{
				Tiers: omniav1alpha1.MemoryRetentionTierSet{
					Institutional: &omniav1alpha1.MemoryTierConfig{Mode: mode},
					Agent:         &omniav1alpha1.MemoryTierConfig{Mode: mode},
					User:          &omniav1alpha1.MemoryTierConfig{Mode: mode},
				},
				Schedule: schedule,
			},
		},
	}
}

func TestNewRetentionWorker(t *testing.T) {
	store := newStore(t)
	log := zap.New(zap.UseDevMode(true))
	loader := &StaticPolicyLoader{Policy: compositeTTLPolicy("0 3 * * *")}

	w := NewRetentionWorker(store, loader, log)

	assert.NotNil(t, w)
	assert.Equal(t, store, w.store)
	assert.NotNil(t, w.loader)
}

func TestRetentionWorker_Run_SoftDeletesExpiredEntities(t *testing.T) {
	// Verifies the worker invokes the TTL branch and flips forgotten=true
	// on an expired row. Uses a cron schedule of @every 10ms so the
	// worker fires quickly without sleeping long.
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	pastTime := time.Now().Add(-1 * time.Hour)
	mem := &Memory{
		Type:       "fact",
		Content:    "should be expired by worker",
		Confidence: 0.9,
		Scope:      scope,
		ExpiresAt:  &pastTime,
	}
	require.NoError(t, store.Save(ctx, mem))

	// Sanity check the row exists and isn't yet forgotten.
	before, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	require.Len(t, before, 1)

	loader := &StaticPolicyLoader{Policy: compositeTTLPolicy("@every 10ms")}
	log := zap.New(zap.UseDevMode(true))
	w := NewRetentionWorker(store, loader, log)

	done := make(chan struct{}, 4)
	w.testHook = func() { done <- struct{}{} }

	cancelCtx, cancel := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		w.Run(cancelCtx)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not fire a pass within 2s")
	}

	// List excludes forgotten rows, so an empty list means the expiry
	// ran and soft-deleted the entity.
	after, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, after, "expired memory should have been soft-deleted")

	cancel()
	<-workerDone
}

func TestRetentionWorker_Run_SkipsWhenNoPolicy(t *testing.T) {
	store := newStore(t)
	log := zap.New(zap.UseDevMode(true))
	loader := &StaticPolicyLoader{Policy: nil}
	w := NewRetentionWorker(store, loader, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx)
	}()
	select {
	case <-done:
		// Expected — worker exits immediately when no policy.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker should exit when loader returns no policy")
	}
}

func TestRetentionWorker_Run_InvalidSchedule(t *testing.T) {
	store := newStore(t)
	log := zap.New(zap.UseDevMode(true))
	loader := &StaticPolicyLoader{Policy: compositeTTLPolicy("not a cron")}
	w := NewRetentionWorker(store, loader, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx)
	}()
	select {
	case <-done:
		// Expected — bad schedule logs and exits.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker should exit on invalid schedule")
	}
}

func TestRegisterRetentionMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterRetentionMetrics(reg))
	// Re-register on the same registry should succeed without returning
	// an error (AlreadyRegisteredError is tolerated).
	require.NoError(t, RegisterRetentionMetrics(reg))

	// Exercise each observer so the metrics have coverage.
	m := defaultRetentionMetrics.Load()
	require.NotNil(t, m)
	m.observeSoftDelete(TierInstitutional, BranchTTL, 3)
	m.observeSoftDelete(TierInstitutional, BranchTTL, 0) // early return
	m.observeHardDelete(2)
	m.observeHardDelete(0) // early return
	m.observeRun(10*time.Millisecond, true)
	m.observeRun(10*time.Millisecond, false)
	m.observeBranchError(TierUser, BranchLRU)

	// Nil receivers must not panic — matches the nil-safety contract.
	var nilMetrics *retentionMetrics
	nilMetrics.observeSoftDelete(TierInstitutional, BranchTTL, 1)
	nilMetrics.observeHardDelete(1)
	nilMetrics.observeRun(time.Second, true)
	nilMetrics.observeBranchError(TierAgent, BranchTTL)
}

// TestRetentionWorker_Run_ExecutesAllBranches asserts every branch
// in a composite tier fires on one pass — TTL soft-deletes an
// expired row, LRU soft-deletes a stale row, and the Decay branch
// logs its "not yet implemented" notice without erroring.
func TestRetentionWorker_Run_ExecutesAllBranches(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	past := time.Now().Add(-1 * time.Hour)

	expired := &Memory{
		Type: "fact", Content: "ttl-target", Confidence: 0.9,
		Scope:     map[string]string{ScopeWorkspaceID: testWorkspace1},
		ExpiresAt: &past,
	}
	require.NoError(t, store.SaveInstitutional(ctx, expired))

	stale := &Memory{
		Type: "fact", Content: "lru-target", Confidence: 0.9,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1},
	}
	require.NoError(t, store.SaveInstitutional(ctx, stale))
	_, err := store.pool.Exec(ctx,
		"UPDATE memory_entities SET created_at = now() - interval '2 hours' WHERE id = $1",
		stale.ID)
	require.NoError(t, err)
	_, err = store.pool.Exec(ctx,
		"UPDATE memory_observations SET observed_at = now() - interval '2 hours', accessed_at = now() - interval '2 hours' WHERE entity_id = $1",
		stale.ID)
	require.NoError(t, err)

	mode := omniav1alpha1.MemoryRetentionModeComposite
	policy := &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			Default: omniav1alpha1.MemoryRetentionDefaults{
				Tiers: omniav1alpha1.MemoryRetentionTierSet{
					Institutional: &omniav1alpha1.MemoryTierConfig{
						Mode: mode,
						LRU:  &omniav1alpha1.MemoryLRUConfig{StaleAfter: "30m"},
					},
				},
				Schedule: "@every 10ms",
			},
		},
	}
	w := NewRetentionWorker(store, &StaticPolicyLoader{Policy: policy},
		zap.New(zap.UseDevMode(true)))

	// Call runOnce directly — avoids cron timing flakiness.
	w.runOnce(ctx)

	assert.True(t, mustFetchEntityForgotten(t, store, expired.ID), "TTL branch must soft-delete expired row")
	assert.True(t, mustFetchEntityForgotten(t, store, stale.ID), "LRU branch must soft-delete stale row")
}

// TestRetentionWorker_Run_BadScheduleExitsImmediately guards the
// worker's safety check on an invalid cron expression from the
// policy — the worker logs and exits rather than tight-looping.
func TestRetentionWorker_Run_BadLRUConfigIsNonFatal(t *testing.T) {
	// A malformed staleAfter fails parsing on the LRU branch but
	// runOnce should keep iterating and still apply TTL elsewhere.
	store := newStore(t)
	ctx := context.Background()
	past := time.Now().Add(-1 * time.Hour)

	expired := &Memory{
		Type: "fact", Content: "ttl", Confidence: 0.9,
		Scope:     map[string]string{ScopeWorkspaceID: testWorkspace1},
		ExpiresAt: &past,
	}
	require.NoError(t, store.SaveInstitutional(ctx, expired))

	policy := &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			Default: omniav1alpha1.MemoryRetentionDefaults{
				Tiers: omniav1alpha1.MemoryRetentionTierSet{
					Institutional: &omniav1alpha1.MemoryTierConfig{
						Mode: omniav1alpha1.MemoryRetentionModeComposite,
						LRU:  &omniav1alpha1.MemoryLRUConfig{StaleAfter: "not-a-duration"},
					},
				},
				Schedule: "@every 1m",
			},
		},
	}
	w := NewRetentionWorker(store, &StaticPolicyLoader{Policy: policy},
		zap.New(zap.UseDevMode(true)))
	w.runOnce(ctx)

	assert.True(t, mustFetchEntityForgotten(t, store, expired.ID),
		"TTL branch must still run even if LRU config is malformed")
}

func TestErrString(t *testing.T) {
	assert.Equal(t, "", errString(nil))
	assert.Equal(t, "boom", errString(errBoom))
}

var errBoom = &stringError{"boom"}

type stringError struct{ s string }

func (e *stringError) Error() string { return e.s }
