/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// revocationPolicy returns a minimal MemoryPolicy configured
// with the given consentRevocation action, and no per-tier modes so
// the TTL/LRU branches stay out of the way.
func revocationPolicy(action omniav1alpha1.ConsentRevocationAction) *omniav1alpha1.MemoryPolicy {
	grace := int32(7)
	return &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			Tiers:    omniav1alpha1.MemoryRetentionTierSet{},
			Schedule: "@every 1m",
			ConsentRevocation: &omniav1alpha1.MemoryConsentRevocationConfig{
				Action:    action,
				GraceDays: &grace,
			},
		},
	}
}

func TestRetentionWorker_ConsentCascade_SoftDeleteAction(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	userID := "user-cascade-soft"
	revokedID := saveUserMemWithCategory(t, store, userID, "memory:health")
	keptID := saveUserMemWithCategory(t, store, userID, "memory:context")

	// userID grants memory:context (absent from non-grantors) but not memory:health.
	src := &fakeConsentRevocationSource{
		nonGrantors: map[string][]string{
			"memory:health": {userID},
		},
	}

	w := NewRetentionWorker(store,
		&StaticPolicyLoader{Policy: revocationPolicy(omniav1alpha1.ConsentRevocationSoftDelete)},
		zap.New(zap.UseDevMode(true)))
	w.SetConsentRevocationSource(src)
	w.runOnce(ctx)

	assert.True(t, mustFetchEntityForgotten(t, store, revokedID),
		"revoked-category row must be soft-deleted")
	assert.False(t, mustFetchEntityForgotten(t, store, keptID),
		"still-granted row must be untouched")
}

func TestRetentionWorker_ConsentCascade_HardDeleteAction(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	userID := "user-cascade-hard"
	id := saveUserMemWithCategory(t, store, userID, "memory:location")

	// userID has no grants — appears in non-grantors for memory:location.
	src := &fakeConsentRevocationSource{
		nonGrantors: map[string][]string{
			"memory:location": {userID},
		},
	}

	w := NewRetentionWorker(store,
		&StaticPolicyLoader{Policy: revocationPolicy(omniav1alpha1.ConsentRevocationHardDelete)},
		zap.New(zap.UseDevMode(true)))
	w.SetConsentRevocationSource(src)
	w.runOnce(ctx)

	assert.False(t, mustFetchEntityExists(t, store, id),
		"HardDelete action must remove the row immediately")
}

func TestRetentionWorker_ConsentCascade_StopAction(t *testing.T) {
	// Stop is the escape hatch: existing rows stay as-is, only future
	// writes are blocked (by the privacy middleware, not the worker).
	store := newStore(t)
	ctx := context.Background()

	userID := "user-cascade-stop"
	id := saveUserMemWithCategory(t, store, userID, "memory:location")

	// No source needed: Stop action short-circuits before the source is used.
	w := NewRetentionWorker(store,
		&StaticPolicyLoader{Policy: revocationPolicy(omniav1alpha1.ConsentRevocationStop)},
		zap.New(zap.UseDevMode(true)))
	w.runOnce(ctx)

	assert.False(t, mustFetchEntityForgotten(t, store, id),
		"Stop action must not touch existing rows")
	assert.True(t, mustFetchEntityExists(t, store, id))
}

func TestRetentionWorker_ConsentCascade_NilSourceIsNoOp(t *testing.T) {
	// When no consent source is configured, the revocation pass must
	// skip entirely — never delete without verified consent state.
	store := newStore(t)
	ctx := context.Background()

	userID := "user-cascade-nil-src"
	id := saveUserMemWithCategory(t, store, userID, "memory:health")

	w := NewRetentionWorker(store,
		&StaticPolicyLoader{Policy: revocationPolicy(omniav1alpha1.ConsentRevocationSoftDelete)},
		zap.New(zap.UseDevMode(true)))
	// Deliberately do not call SetConsentRevocationSource — worker defaults to nil.
	w.runOnce(ctx)

	assert.False(t, mustFetchEntityForgotten(t, store, id),
		"nil source must not soft-delete any rows")
}

func TestRetentionWorker_ConsentCascade_SoftDeleteGraceCleanup(t *testing.T) {
	// A row already soft-deleted by the cascade, with forgotten_at
	// backdated beyond grace, should be hard-deleted on the same
	// pass (the cascade calls the grace cleanup after soft-delete).
	store := newStore(t)
	ctx := context.Background()

	userID := "user-cascade-grace"
	expired := saveUserMemWithCategory(t, store, userID, "memory:health")
	_, err := store.pool.Exec(ctx,
		`UPDATE memory_entities
		 SET forgotten = true,
		     forgotten_at = now() - interval '30 days'
		 WHERE id = $1`, expired)
	require.NoError(t, err)

	// Source returns userID as non-granter for health — but the row is already
	// forgotten=true so SoftDelete won't touch it. The grace-cleanup pass then
	// hard-deletes it (forgotten_at 30 days > 7-day grace).
	src := &fakeConsentRevocationSource{
		nonGrantors: map[string][]string{
			"memory:health": {userID},
		},
	}

	w := NewRetentionWorker(store,
		&StaticPolicyLoader{Policy: revocationPolicy(omniav1alpha1.ConsentRevocationSoftDelete)},
		zap.New(zap.UseDevMode(true)))
	w.SetConsentRevocationSource(src)
	w.runOnce(ctx)

	assert.False(t, mustFetchEntityExists(t, store, expired),
		"soft-deleted row past grace must be hard-deleted")
}

func TestResolveConsentAction(t *testing.T) {
	// Nil config defaults to SoftDelete so a policy with consentRevocation
	// unset still cascades safely rather than silently skipping.
	assert.Equal(t, omniav1alpha1.ConsentRevocationSoftDelete, resolveConsentAction(nil))

	// Empty Action defaults to SoftDelete.
	assert.Equal(t, omniav1alpha1.ConsentRevocationSoftDelete,
		resolveConsentAction(&omniav1alpha1.MemoryConsentRevocationConfig{}))

	// Explicit action round-trips.
	assert.Equal(t, omniav1alpha1.ConsentRevocationHardDelete,
		resolveConsentAction(&omniav1alpha1.MemoryConsentRevocationConfig{
			Action: omniav1alpha1.ConsentRevocationHardDelete,
		}))
}
