/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session/api"
)

func TestRecordProviderUsage_PersistsRows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := freshDB(t)
	store := NewProviderUsageStore(pool)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	rows := []*api.ProviderUsage{
		{
			Namespace:     "omnia-demo",
			WorkspaceName: "demo",
			Provider:      "azure",
			ProviderName:  "azure-embed",
			Model:         "text-embedding-3-small",
			Source:        "embedding",
			InputTokens:   512,
			CostUSD:       0.0001,
			CallCount:     3,
			CreatedAt:     now,
		},
		{
			Namespace:    "omnia-demo",
			Provider:     pcProviderOpenAI,
			Source:       "judge",
			InputTokens:  100,
			OutputTokens: 20,
			CachedTokens: 10,
			CallCount:    1,
			CreatedAt:    now,
		},
	}
	require.NoError(t, store.RecordProviderUsage(ctx, rows))

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM provider_usage WHERE namespace = $1`, "omnia-demo").Scan(&count))
	assert.Equal(t, 2, count)

	// Verify the embedding row's columns round-trip, including the nullable
	// provider_name and the explicit call_count.
	var (
		providerName string
		source       string
		inputTokens  int64
		callCount    int32
	)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT provider_name, source, input_tokens, call_count
		 FROM provider_usage WHERE source = 'embedding'`).
		Scan(&providerName, &source, &inputTokens, &callCount))
	assert.Equal(t, "azure-embed", providerName)
	assert.Equal(t, "embedding", source)
	assert.Equal(t, int64(512), inputTokens)
	assert.Equal(t, int32(3), callCount)
}

func TestRecordProviderUsage_DefaultsCallCountAndCreatedAt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := freshDB(t)
	store := NewProviderUsageStore(pool)
	ctx := context.Background()

	// CallCount 0 and zero CreatedAt should default to 1 / now().
	require.NoError(t, store.RecordProviderUsage(ctx, []*api.ProviderUsage{
		{Namespace: "ns", Provider: pcProviderOpenAI, Source: "embedding"},
	}))

	var (
		callCount int32
		createdAt time.Time
	)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT call_count, created_at FROM provider_usage WHERE namespace = 'ns'`).
		Scan(&callCount, &createdAt))
	assert.Equal(t, int32(1), callCount)
	assert.WithinDuration(t, time.Now().UTC(), createdAt, time.Minute)
}

func TestRecordProviderUsage_EmptyIsNoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := freshDB(t)
	store := NewProviderUsageStore(pool)
	require.NoError(t, store.RecordProviderUsage(context.Background(), nil))
}
