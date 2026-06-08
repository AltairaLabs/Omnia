package api

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

// aggregatingStore is a Store (via the embedded mockStore) that also implements
// memory.Aggregator, standing in for the Postgres store underneath a
// *CachedStore.
type aggregatingStore struct {
	*mockStore
	rows    []memory.AggregateRow
	gotOpts memory.AggregateOptions
}

func (s *aggregatingStore) Aggregate(_ context.Context, opts memory.AggregateOptions) ([]memory.AggregateRow, error) {
	s.gotOpts = opts
	return s.rows, nil
}

// TestAggregateMemories_ThroughCachedStore is the regression guard for #1253:
// with the store cache on (the default) s.store is a *CachedStore, so the
// service must reach Aggregate through the wrapper instead of type-asserting
// the concrete *PostgresMemoryStore — which 500'd every aggregate request and
// killed the dashboard Memory Analytics page.
func TestAggregateMemories_ThroughCachedStore(t *testing.T) {
	want := []memory.AggregateRow{
		{Key: "agg-row-a", Value: 10, Count: 10},
		{Key: "agg-row-b", Value: 2, Count: 2},
	}
	inner := &aggregatingStore{mockStore: &mockStore{}, rows: want}

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cached := memory.NewCachedStore(inner, rdb, 5*time.Minute, logr.Discard())

	svc := NewMemoryService(cached, nil, MemoryServiceConfig{}, logr.Discard())

	got, err := svc.AggregateMemories(context.Background(), memory.AggregateOptions{
		Workspace: "ws",
		GroupBy:   memory.AggregateGroupByCategory,
	})
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, "ws", inner.gotOpts.Workspace,
		"opts must reach the inner store through the cache wrapper")
}

// TestAggregateMemories_NonAggregatorStore_Errors keeps the property that a
// store with no Aggregate support surfaces a clear error (rather than a panic
// or silent success) — the same guarantee the old concrete-type assertion gave.
func TestAggregateMemories_NonAggregatorStore_Errors(t *testing.T) {
	svc := NewMemoryService(&mockStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	_, err := svc.AggregateMemories(context.Background(), memory.AggregateOptions{Workspace: "ws"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Aggregate")
}
