/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// createSupersession seeds an entity with n old observations, runs
// compaction, then backdates the summary observation's created_at so
// the grace window has elapsed. Returns the summary entity id and the
// superseded observation IDs so tests can assert deletion state.
func createSupersession(
	t *testing.T, store *PostgresMemoryStore,
	workspace, userID string, n int, summaryAge time.Duration,
) (summaryEntityID string, superseded []string) {
	t.Helper()
	ctx := context.Background()

	mustInsertOldEntities(t, store, workspace, userID, "", n,
		"will be summarized", time.Now().Add(-90*24*time.Hour))

	candidates, err := store.FindCompactionCandidates(ctx, FindCompactionCandidatesOptions{
		WorkspaceID:  workspace,
		OlderThan:    time.Now().Add(-30 * 24 * time.Hour),
		MinGroupSize: 1,
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	superseded = candidates[0].ObservationIDs
	summaryEntityID, err = store.SaveCompactionSummary(ctx, CompactionSummary{
		WorkspaceID:            workspace,
		UserID:                 userID,
		Content:                "Summary: older notes.",
		SupersededObservations: superseded,
	})
	require.NoError(t, err)

	// Backdate the summary observation so the grace window has elapsed.
	if summaryAge > 0 {
		_, err := store.pool.Exec(ctx,
			`UPDATE memory_observations
			 SET created_at = now() - $1::interval
			 WHERE entity_id = $2`,
			durationToInterval(summaryAge), summaryEntityID)
		require.NoError(t, err)
	}
	return summaryEntityID, superseded
}

// durationToInterval renders a Go duration as a Postgres interval
// string. Testcontainers are fast; sub-second granularity isn't
// needed for retention tests.
func durationToInterval(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int64(d.Seconds()))
}

// mustSupersededObservationsCount returns how many observations have a
// non-null superseded_by pointer. Used to assert delete vs skip.
func mustSupersededObservationsCount(t *testing.T, store *PostgresMemoryStore) int {
	t.Helper()
	var n int
	err := store.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM memory_observations WHERE superseded_by IS NOT NULL").Scan(&n)
	require.NoError(t, err)
	return n
}

func TestHardDeleteSupersededObservations_RemovesOnlyPastGrace(t *testing.T) {
	store := newStore(t)
	ws := "cc000000-0000-0000-0000-000000000001"
	user := "cc000000-0000-0000-0000-000000000002"

	// Summary aged beyond grace; the 3 superseded observations should go.
	createSupersession(t, store, ws, user, 3, 30*24*time.Hour)
	require.Equal(t, 3, mustSupersededObservationsCount(t, store))

	n, err := store.HardDeleteSupersededObservations(context.Background(), 14, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	assert.Equal(t, 0, mustSupersededObservationsCount(t, store))
}

func TestHardDeleteSupersededObservations_RespectsGrace(t *testing.T) {
	store := newStore(t)
	ws := "cc000000-0000-0000-0000-000000000003"
	user := "cc000000-0000-0000-0000-000000000004"

	// Summary only 3 days old; grace is 14 days, so nothing should be
	// deleted yet.
	createSupersession(t, store, ws, user, 2, 3*24*time.Hour)
	require.Equal(t, 2, mustSupersededObservationsCount(t, store))

	n, err := store.HardDeleteSupersededObservations(context.Background(), 14, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.Equal(t, 2, mustSupersededObservationsCount(t, store))
}

func TestHardDeleteSupersededObservations_BatchSizeZeroIsNoOp(t *testing.T) {
	store := newStore(t)
	n, err := store.HardDeleteSupersededObservations(context.Background(), 14, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestHardDeleteSupersededObservations_NegativeGraceErrors(t *testing.T) {
	store := newStore(t)
	_, err := store.HardDeleteSupersededObservations(context.Background(), -1, 100)
	require.Error(t, err)
}

func TestRetentionWorker_Supersession_SkipsWhenDisabled(t *testing.T) {
	store := newStore(t)
	ws := "cc000000-0000-0000-0000-000000000005"
	user := "cc000000-0000-0000-0000-000000000006"
	createSupersession(t, store, ws, user, 2, 30*24*time.Hour)

	disabled := &omniav1alpha1.MemorySupersessionConfig{Enabled: false}
	policy := &omniav1alpha1.MemoryRetentionPolicy{
		Spec: omniav1alpha1.MemoryRetentionPolicySpec{
			Default: omniav1alpha1.MemoryRetentionDefaults{
				Tiers:        omniav1alpha1.MemoryRetentionTierSet{},
				Schedule:     "@every 1m",
				Supersession: disabled,
			},
		},
	}
	w := NewRetentionWorker(store, &StaticPolicyLoader{Policy: policy},
		zap.New(zap.UseDevMode(true)))
	w.runOnce(context.Background())

	assert.Equal(t, 2, mustSupersededObservationsCount(t, store),
		"disabled supersession must not touch rows")
}

func TestRetentionWorker_Supersession_DeletesWhenEnabled(t *testing.T) {
	store := newStore(t)
	ws := "cc000000-0000-0000-0000-000000000007"
	user := "cc000000-0000-0000-0000-000000000008"
	createSupersession(t, store, ws, user, 2, 30*24*time.Hour)

	grace := int32(14)
	policy := &omniav1alpha1.MemoryRetentionPolicy{
		Spec: omniav1alpha1.MemoryRetentionPolicySpec{
			Default: omniav1alpha1.MemoryRetentionDefaults{
				Tiers:    omniav1alpha1.MemoryRetentionTierSet{},
				Schedule: "@every 1m",
				Supersession: &omniav1alpha1.MemorySupersessionConfig{
					Enabled:   true,
					GraceDays: &grace,
				},
			},
		},
	}
	w := NewRetentionWorker(store, &StaticPolicyLoader{Policy: policy},
		zap.New(zap.UseDevMode(true)))
	w.runOnce(context.Background())

	assert.Equal(t, 0, mustSupersededObservationsCount(t, store),
		"enabled supersession must hard-delete rows past grace")
}

func TestResolveSupersessionGraceDays(t *testing.T) {
	assert.Equal(t, int32(14), resolveSupersessionGraceDays(nil))
	assert.Equal(t, int32(14), resolveSupersessionGraceDays(
		&omniav1alpha1.MemorySupersessionConfig{Enabled: true}))

	g := int32(30)
	cfg := &omniav1alpha1.MemorySupersessionConfig{Enabled: true, GraceDays: &g}
	assert.Equal(t, int32(30), resolveSupersessionGraceDays(cfg))
}
