/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session/api"
)

const (
	fiTestNamespaceA = "ns-a"
	fiTestFunction   = "summarizer"
)

// newFunctionInvocationsStore stands up a fresh postgres-backed store
// via the shared testcontainer. Skipped when -short is set (no
// testcontainer is started in that mode).
func newFunctionInvocationsStore(t *testing.T) *FunctionInvocationsStoreImpl {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	return NewFunctionInvocationsStore(freshDB(t))
}

// newTestInvocation builds a success-status row. Every test that needs
// a different status sets it on the returned struct.
func newTestInvocation(namespace, function string, createdAt time.Time) *api.FunctionInvocation {
	return &api.FunctionInvocation{
		ID:           uuid.New().String(),
		Namespace:    namespace,
		FunctionName: function,
		InputHash:    "abc123",
		OutputJSON:   json.RawMessage(`{"a":1}`),
		Status:       api.FunctionInvocationStatusSuccess,
		DurationMs:   42,
		CostUSD:      0.001,
		TraceID:      "trace-1",
		CreatedAt:    createdAt,
	}
}

func TestFunctionInvocationsStore_CreateAndGet(t *testing.T) {
	store := newFunctionInvocationsStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	inv := newTestInvocation(fiTestNamespaceA, fiTestFunction, now)
	require.NoError(t, store.CreateFunctionInvocation(ctx, inv))

	got, err := store.GetFunctionInvocation(ctx, fiTestNamespaceA, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, inv.ID, got.ID)
	assert.Equal(t, "summarizer", got.FunctionName)
	assert.Equal(t, api.FunctionInvocationStatusSuccess, got.Status)
	assert.Equal(t, int32(42), got.DurationMs)
	assert.Equal(t, "trace-1", got.TraceID)
	assert.JSONEq(t, `{"a":1}`, string(got.OutputJSON))
}

func TestFunctionInvocationsStore_Get_NotFound(t *testing.T) {
	store := newFunctionInvocationsStore(t)
	_, err := store.GetFunctionInvocation(context.Background(), fiTestNamespaceA, uuid.NewString())
	assert.ErrorIs(t, err, api.ErrFunctionInvocationNotFound)
}

func TestFunctionInvocationsStore_Get_CrossTenantHidden(t *testing.T) {
	store := newFunctionInvocationsStore(t)
	ctx := context.Background()

	inv := newTestInvocation(fiTestNamespaceA, fiTestFunction, time.Now().UTC())
	require.NoError(t, store.CreateFunctionInvocation(ctx, inv))

	// Cross-tenant Get must return ErrNotFound, not the row.
	_, err := store.GetFunctionInvocation(ctx, "ns-b", inv.ID)
	assert.ErrorIs(t, err, api.ErrFunctionInvocationNotFound)
}

func TestFunctionInvocationsStore_ListByFunctionAndTimeWindow(t *testing.T) {
	store := newFunctionInvocationsStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	old := now.Add(-2 * time.Hour)
	mid := now.Add(-30 * time.Minute)
	recent := now.Add(-1 * time.Minute)

	for _, ts := range []time.Time{old, mid, recent} {
		require.NoError(t, store.CreateFunctionInvocation(ctx,
			newTestInvocation(fiTestNamespaceA, fiTestFunction, ts)))
	}
	// Different function in same namespace; must be excluded by FunctionName filter.
	require.NoError(t, store.CreateFunctionInvocation(ctx,
		newTestInvocation(fiTestNamespaceA, "classifier", recent)))

	rows, err := store.ListFunctionInvocations(ctx, api.FunctionInvocationListOpts{
		Namespace:    fiTestNamespaceA,
		FunctionName: fiTestFunction,
		From:         now.Add(-1 * time.Hour),
	})
	require.NoError(t, err)
	// Should pick up mid + recent, but not old (outside window) or
	// classifier (different function).
	assert.Len(t, rows, 2)
	// DESC order.
	assert.True(t, rows[0].CreatedAt.After(rows[1].CreatedAt),
		"rows must be ordered by created_at DESC")
}

func TestFunctionInvocationsStore_ListNamespaceScoped(t *testing.T) {
	store := newFunctionInvocationsStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, store.CreateFunctionInvocation(ctx,
		newTestInvocation(fiTestNamespaceA, "x", now)))
	require.NoError(t, store.CreateFunctionInvocation(ctx,
		newTestInvocation("ns-b", "x", now)))

	rows, err := store.ListFunctionInvocations(ctx, api.FunctionInvocationListOpts{
		Namespace: fiTestNamespaceA,
	})
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.Equal(t, fiTestNamespaceA, rows[0].Namespace)
}

func TestFunctionInvocationsStore_LimitClamps(t *testing.T) {
	assert.Equal(t, api.DefaultFunctionInvocationListLimit, clampFunctionInvocationListLimit(0))
	assert.Equal(t, api.DefaultFunctionInvocationListLimit, clampFunctionInvocationListLimit(-5))
	assert.Equal(t, 42, clampFunctionInvocationListLimit(42))
	assert.Equal(t, api.MaxFunctionInvocationListLimit,
		clampFunctionInvocationListLimit(api.MaxFunctionInvocationListLimit+1000))
}

func TestFunctionInvocationsStore_RejectsMissingRequiredFields(t *testing.T) {
	// Pure validation test — the early-returns trigger before any pool
	// access, so this works fine with a nil pool and skips the
	// integration-only freshDB.
	store := &FunctionInvocationsStoreImpl{}
	ctx := context.Background()
	now := time.Now().UTC()

	tt := []struct {
		name string
		inv  *api.FunctionInvocation
	}{
		{
			name: "nil",
			inv:  nil,
		},
		{
			name: "missing namespace",
			inv: &api.FunctionInvocation{
				ID:           uuid.NewString(),
				FunctionName: "x",
				Status:       api.FunctionInvocationStatusSuccess,
				CreatedAt:    now,
			},
		},
		{
			name: "missing function name",
			inv: &api.FunctionInvocation{
				ID:        uuid.NewString(),
				Namespace: fiTestNamespaceA,
				Status:    api.FunctionInvocationStatusSuccess,
				CreatedAt: now,
			},
		},
		{
			name: "missing status",
			inv: &api.FunctionInvocation{
				ID:           uuid.NewString(),
				Namespace:    fiTestNamespaceA,
				FunctionName: "x",
				CreatedAt:    now,
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := store.CreateFunctionInvocation(ctx, tc.inv)
			assert.Error(t, err)
		})
	}
}
