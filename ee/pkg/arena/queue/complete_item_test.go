/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package queue

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
)

const (
	completeTestJobID = "complete-job-1"
)

func TestCompleteItemUpdatesAccumulators(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scen-a", ProviderID: "prov-x"},
	}
	mustPush(t, q, ctx, items)
	mustPop(t, q, ctx)

	result := &ItemResult{
		Status:     "pass",
		DurationMs: 150.5,
		Metrics:    map[string]float64{"tokens": 100, "cost": 0.05},
	}

	if err := q.CompleteItem(ctx, completeTestJobID, "item-1", result); err != nil {
		t.Fatalf("CompleteItem() error = %v", err)
	}

	stats, err := q.GetStats(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	assertInt64(t, "Total", stats.Total, 1)
	assertInt64(t, "Passed", stats.Passed, 1)
	assertInt64(t, "Failed", stats.Failed, 0)
	assertFloat64(t, "TotalDurationMs", stats.TotalDurationMs, 150.5)
	assertInt64(t, "TotalTokens", stats.TotalTokens, 100)
	assertFloat64(t, "TotalCost", stats.TotalCost, 0.05)
}

func TestCompleteItemDoesAckBookkeeping(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scen-a", ProviderID: "prov-x"},
	}
	mustPush(t, q, ctx, items)
	mustPop(t, q, ctx)

	result := &ItemResult{Status: "pass", DurationMs: 100}
	if err := q.CompleteItem(ctx, completeTestJobID, "item-1", result); err != nil {
		t.Fatalf("CompleteItem() error = %v", err)
	}

	// Verify item moved to completed set
	completed, err := q.GetCompletedItems(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("GetCompletedItems() error = %v", err)
	}
	if len(completed) != 1 {
		t.Fatalf("Expected 1 completed item, got %d", len(completed))
	}
	if completed[0].ID != "item-1" {
		t.Errorf("Completed item ID = %s, want item-1", completed[0].ID)
	}

	// Verify progress reflects completion
	progress, err := q.Progress(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("Progress() error = %v", err)
	}
	if progress.Completed != 1 {
		t.Errorf("Progress.Completed = %d, want 1", progress.Completed)
	}
	if progress.Processing != 0 {
		t.Errorf("Progress.Processing = %d, want 0", progress.Processing)
	}
}

func TestFailItemUpdatesCounters(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scen-a", ProviderID: "prov-x"},
	}
	mustPush(t, q, ctx, items)
	mustPop(t, q, ctx)

	testErr := errors.New("execution timeout")
	if err := q.FailItem(ctx, completeTestJobID, "item-1", testErr); err != nil {
		t.Fatalf("FailItem() error = %v", err)
	}

	stats, err := q.GetStats(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	assertInt64(t, "Total", stats.Total, 1)
	assertInt64(t, "Passed", stats.Passed, 0)
	assertInt64(t, "Failed", stats.Failed, 1)
	assertFloat64(t, "TotalDurationMs", stats.TotalDurationMs, 0)

	// Verify item is in failed set
	failed, err := q.GetFailedItems(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("GetFailedItems() error = %v", err)
	}
	if len(failed) != 1 {
		t.Fatalf("Expected 1 failed item, got %d", len(failed))
	}
	if failed[0].Error != "execution timeout" {
		t.Errorf("Failed item error = %s, want 'execution timeout'", failed[0].Error)
	}
}

func TestAccumulatorsPerScenarioAndProvider(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scen-a", ProviderID: "prov-x"},
		{ID: "item-2", ScenarioID: "scen-a", ProviderID: "prov-y"},
		{ID: "item-3", ScenarioID: "scen-b", ProviderID: "prov-x"},
	}
	mustPush(t, q, ctx, items)

	// Pop and complete all items
	for i := 0; i < 3; i++ {
		mustPop(t, q, ctx)
	}

	results := []*ItemResult{
		{Status: "pass", DurationMs: 100, Metrics: map[string]float64{"tokens": 50, "cost": 0.01}},
		{Status: "fail", DurationMs: 200, Metrics: map[string]float64{"tokens": 80, "cost": 0.02}},
		{Status: "pass", DurationMs: 150, Metrics: map[string]float64{"tokens": 60, "cost": 0.015}},
	}

	for i, itemID := range []string{"item-1", "item-2", "item-3"} {
		if err := q.CompleteItem(ctx, completeTestJobID, itemID, results[i]); err != nil {
			t.Fatalf("CompleteItem(%s) error = %v", itemID, err)
		}
	}

	stats, err := q.GetStats(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	// Check global stats
	assertInt64(t, "Total", stats.Total, 3)
	assertInt64(t, "Passed", stats.Passed, 2)
	assertInt64(t, "Failed", stats.Failed, 1)

	// Check scenario breakdown
	scenA := stats.ByScenario["scen-a"]
	if scenA == nil {
		t.Fatal("Missing ByScenario[scen-a]")
	}
	assertInt64(t, "scen-a.Total", scenA.Total, 2)
	assertInt64(t, "scen-a.Passed", scenA.Passed, 1)
	assertInt64(t, "scen-a.Failed", scenA.Failed, 1)
	assertInt64(t, "scen-a.TotalTokens", scenA.TotalTokens, 130)

	scenB := stats.ByScenario["scen-b"]
	if scenB == nil {
		t.Fatal("Missing ByScenario[scen-b]")
	}
	assertInt64(t, "scen-b.Total", scenB.Total, 1)
	assertInt64(t, "scen-b.Passed", scenB.Passed, 1)

	// Check provider breakdown
	provX := stats.ByProvider["prov-x"]
	if provX == nil {
		t.Fatal("Missing ByProvider[prov-x]")
	}
	assertInt64(t, "prov-x.Total", provX.Total, 2)
	assertInt64(t, "prov-x.Passed", provX.Passed, 2)
	assertFloat64(t, "prov-x.TotalDurationMs", provX.TotalDurationMs, 250)

	provY := stats.ByProvider["prov-y"]
	if provY == nil {
		t.Fatal("Missing ByProvider[prov-y]")
	}
	assertInt64(t, "prov-y.Total", provY.Total, 1)
	assertInt64(t, "prov-y.Failed", provY.Failed, 1)
}

func TestGetStatsReturnsZeroForNewJob(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	stats, err := q.GetStats(ctx, "nonexistent-job")
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	assertInt64(t, "Total", stats.Total, 0)
	assertInt64(t, "Passed", stats.Passed, 0)
	assertInt64(t, "Failed", stats.Failed, 0)
	assertFloat64(t, "TotalDurationMs", stats.TotalDurationMs, 0)

	if stats.ByScenario == nil {
		t.Error("ByScenario should be non-nil empty map")
	}
	if stats.ByProvider == nil {
		t.Error("ByProvider should be non-nil empty map")
	}
}

func TestConcurrentCompleteItem(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	const numItems = 50
	items := make([]WorkItem, numItems)
	for i := 0; i < numItems; i++ {
		items[i] = WorkItem{
			ID:         fmt.Sprintf("item-%d", i),
			ScenarioID: "scen-a",
			ProviderID: "prov-x",
		}
	}

	mustPush(t, q, ctx, items)

	// Pop all items first
	for i := 0; i < numItems; i++ {
		mustPop(t, q, ctx)
	}

	// Complete all items concurrently
	var wg sync.WaitGroup
	errs := make(chan error, numItems)

	for i := 0; i < numItems; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result := &ItemResult{
				Status:     "pass",
				DurationMs: 100,
				Metrics:    map[string]float64{"tokens": 10, "cost": 0.001},
			}
			if err := q.CompleteItem(ctx, completeTestJobID, fmt.Sprintf("item-%d", idx), result); err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("Concurrent CompleteItem() error = %v", err)
	}

	stats, err := q.GetStats(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	assertInt64(t, "Total", stats.Total, numItems)
	assertInt64(t, "Passed", stats.Passed, numItems)
	assertFloat64(t, "TotalDurationMs", stats.TotalDurationMs, float64(numItems)*100)
	assertInt64(t, "TotalTokens", stats.TotalTokens, int64(numItems)*10)
}

func TestExtractTokensAndCost(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scen-a"},
		{ID: "item-2", ScenarioID: "scen-b"},
	}
	mustPush(t, q, ctx, items)
	mustPop(t, q, ctx)
	mustPop(t, q, ctx)

	// First item uses "totalTokens" and "totalCost" keys
	result1 := &ItemResult{
		Status:     "pass",
		DurationMs: 100,
		Metrics:    map[string]float64{"totalTokens": 200, "totalCost": 0.10},
	}
	if err := q.CompleteItem(ctx, completeTestJobID, "item-1", result1); err != nil {
		t.Fatalf("CompleteItem() error = %v", err)
	}

	// Second item uses "tokens" and "cost" keys
	result2 := &ItemResult{
		Status:     "pass",
		DurationMs: 100,
		Metrics:    map[string]float64{"tokens": 100, "cost": 0.05},
	}
	if err := q.CompleteItem(ctx, completeTestJobID, "item-2", result2); err != nil {
		t.Fatalf("CompleteItem() error = %v", err)
	}

	stats, err := q.GetStats(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	// Both token formats should be accumulated
	assertInt64(t, "TotalTokens", stats.TotalTokens, 300)
	assertFloat64(t, "TotalCost", stats.TotalCost, 0.15)
}

func TestExtractTokensSeparateInputOutput(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scen-a", ProviderID: "prov-x"},
	}
	mustPush(t, q, ctx, items)
	mustPop(t, q, ctx)

	// Worker writes separate input/output token keys (no "totalTokens" key).
	result := &ItemResult{
		Status:     "pass",
		DurationMs: 100,
		Metrics:    map[string]float64{"totalInputTokens": 500, "totalOutputTokens": 200},
	}
	if err := q.CompleteItem(ctx, completeTestJobID, "item-1", result); err != nil {
		t.Fatalf("CompleteItem() error = %v", err)
	}

	stats, err := q.GetStats(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	// Should sum input + output tokens.
	assertInt64(t, "TotalTokens", stats.TotalTokens, 700)
}

func TestFailItemScenarioAndProviderStats(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scen-a", ProviderID: "prov-x"},
	}
	mustPush(t, q, ctx, items)
	mustPop(t, q, ctx)

	if err := q.FailItem(ctx, completeTestJobID, "item-1", errors.New("crash")); err != nil {
		t.Fatalf("FailItem() error = %v", err)
	}

	stats, err := q.GetStats(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	scenA := stats.ByScenario["scen-a"]
	if scenA == nil {
		t.Fatal("Missing ByScenario[scen-a]")
	}
	assertInt64(t, "scen-a.Total", scenA.Total, 1)
	assertInt64(t, "scen-a.Failed", scenA.Failed, 1)

	provX := stats.ByProvider["prov-x"]
	if provX == nil {
		t.Fatal("Missing ByProvider[prov-x]")
	}
	assertInt64(t, "prov-x.Total", provX.Total, 1)
	assertInt64(t, "prov-x.Failed", provX.Failed, 1)
}

func TestCompleteItemNotFound(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{{ID: "item-1"}}
	mustPush(t, q, ctx, items)

	// Try to complete without popping first
	result := &ItemResult{Status: "pass", DurationMs: 100}
	err := q.CompleteItem(ctx, completeTestJobID, "item-1", result)
	if err != ErrItemNotFound {
		t.Fatalf("CompleteItem() error = %v, want ErrItemNotFound", err)
	}
}

func TestFailItemNotFound(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{{ID: "item-1"}}
	mustPush(t, q, ctx, items)

	err := q.FailItem(ctx, completeTestJobID, "item-1", errors.New("crash"))
	if err != ErrItemNotFound {
		t.Fatalf("FailItem() error = %v, want ErrItemNotFound", err)
	}
}

func TestFailItemJobNotFound(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	err := q.FailItem(ctx, "nonexistent", "item-1", errors.New("crash"))
	if err != ErrJobNotFound {
		t.Fatalf("FailItem() error = %v, want ErrJobNotFound", err)
	}
}

func TestGetStatsOnClosedQueue(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	_ = q.Close()

	_, err := q.GetStats(context.Background(), completeTestJobID)
	if err != ErrQueueClosed {
		t.Fatalf("GetStats() error = %v, want ErrQueueClosed", err)
	}
}

func TestCompleteItemMixedPassFail(t *testing.T) {
	q := NewMemoryQueueWithDefaults()
	ctx := context.Background()

	items := []WorkItem{
		{ID: "item-1", ScenarioID: "scen-a"},
		{ID: "item-2", ScenarioID: "scen-a"},
		{ID: "item-3", ScenarioID: "scen-a"},
	}
	mustPush(t, q, ctx, items)

	for i := 0; i < 3; i++ {
		mustPop(t, q, ctx)
	}

	// Complete item-1 as pass
	if err := q.CompleteItem(ctx, completeTestJobID, "item-1", &ItemResult{
		Status: "pass", DurationMs: 100,
	}); err != nil {
		t.Fatal(err)
	}

	// Fail item-2 via FailItem
	if err := q.FailItem(ctx, completeTestJobID, "item-2", errors.New("timeout")); err != nil {
		t.Fatal(err)
	}

	// Complete item-3 as fail via CompleteItem (worker detected failure)
	if err := q.CompleteItem(ctx, completeTestJobID, "item-3", &ItemResult{
		Status: "fail", DurationMs: 300, Error: "assertion failed",
	}); err != nil {
		t.Fatal(err)
	}

	stats, err := q.GetStats(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	assertInt64(t, "Total", stats.Total, 3)
	assertInt64(t, "Passed", stats.Passed, 1)
	assertInt64(t, "Failed", stats.Failed, 2)
	assertFloat64(t, "TotalDurationMs", stats.TotalDurationMs, 400) // 100 + 0 + 300
}

// Test helpers

func mustPush(t *testing.T, q *MemoryQueue, ctx context.Context, items []WorkItem) {
	t.Helper()
	if err := q.Push(ctx, completeTestJobID, items); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
}

func mustPop(t *testing.T, q *MemoryQueue, ctx context.Context) *WorkItem {
	t.Helper()
	item, err := q.Pop(ctx, completeTestJobID)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	return item
}

func assertInt64(t *testing.T, name string, got, want int64) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}

func assertFloat64(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %f, want %f", name, got, want)
	}
}
