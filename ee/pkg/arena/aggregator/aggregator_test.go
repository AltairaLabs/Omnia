/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package aggregator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

func TestAggregator_New(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	agg := New(q)

	if agg == nil {
		t.Fatal("New() returned nil")
	}
	if agg.queue != q {
		t.Error("Aggregator queue not set correctly")
	}
}

func TestAggregator_Aggregate_EmptyJob(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	agg := New(q)
	ctx := context.Background()

	// Push an item to create the job
	_ = q.Push(ctx, "job-1", []queue.WorkItem{{ID: "item-1"}})

	result, err := agg.Aggregate(ctx, "job-1")
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	if result.TotalItems != 0 {
		t.Errorf("TotalItems = %d, want 0", result.TotalItems)
	}
	if result.PassRate != 0 {
		t.Errorf("PassRate = %f, want 0", result.PassRate)
	}
}

func TestAggregator_Aggregate_AllPassed(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	agg := New(q)
	ctx := context.Background()

	// Push and complete items
	items := []queue.WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-2", ScenarioID: "scenario-1", ProviderID: "provider-2"},
		{ID: "item-3", ScenarioID: "scenario-2", ProviderID: "provider-1"},
	}
	_ = q.Push(ctx, "job-1", items)

	// Pop and ack all items
	for range 3 {
		item, _ := q.Pop(ctx, "job-1")
		result := []byte(`{"status": "pass", "durationMs": 100, "metrics": {"tokens": 50, "cost": 0.01}}`)
		_ = q.Ack(ctx, "job-1", item.ID, result)
	}

	result, err := agg.Aggregate(ctx, "job-1")
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	if result.TotalItems != 3 {
		t.Errorf("TotalItems = %d, want 3", result.TotalItems)
	}
	if result.PassedItems != 3 {
		t.Errorf("PassedItems = %d, want 3", result.PassedItems)
	}
	if result.FailedItems != 0 {
		t.Errorf("FailedItems = %d, want 0", result.FailedItems)
	}
	if result.PassRate != 100 {
		t.Errorf("PassRate = %f, want 100", result.PassRate)
	}
	if result.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", result.TotalTokens)
	}
	if result.TotalCost != 0.03 {
		t.Errorf("TotalCost = %f, want 0.03", result.TotalCost)
	}
}

func TestAggregator_Aggregate_WithFailures(t *testing.T) {
	q := queue.NewMemoryQueue(queue.Options{MaxRetries: 1})
	agg := New(q)
	ctx := context.Background()

	// Push items
	items := []queue.WorkItem{
		{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-2", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-3", ScenarioID: "scenario-1", ProviderID: "provider-1"},
		{ID: "item-4", ScenarioID: "scenario-1", ProviderID: "provider-1"},
	}
	_ = q.Push(ctx, "job-1", items)

	// Complete 3, fail 1
	for i := range 4 {
		item, _ := q.Pop(ctx, "job-1")
		if i == 2 {
			_ = q.Nack(ctx, "job-1", item.ID, nil)
		} else {
			_ = q.Ack(ctx, "job-1", item.ID, []byte(`{"status": "pass"}`))
		}
	}

	result, err := agg.Aggregate(ctx, "job-1")
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	if result.TotalItems != 4 {
		t.Errorf("TotalItems = %d, want 4", result.TotalItems)
	}
	if result.PassedItems != 3 {
		t.Errorf("PassedItems = %d, want 3", result.PassedItems)
	}
	if result.FailedItems != 1 {
		t.Errorf("FailedItems = %d, want 1", result.FailedItems)
	}
	if result.PassRate != 75 {
		t.Errorf("PassRate = %f, want 75", result.PassRate)
	}
}

func TestAggregator_Aggregate_ByScenario(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	agg := New(q)
	ctx := context.Background()

	// Push items with different scenarios
	items := []queue.WorkItem{
		{ID: "item-1", ScenarioID: "scenario-a"},
		{ID: "item-2", ScenarioID: "scenario-a"},
		{ID: "item-3", ScenarioID: "scenario-b"},
	}
	_ = q.Push(ctx, "job-1", items)

	// Complete all
	for range 3 {
		item, _ := q.Pop(ctx, "job-1")
		_ = q.Ack(ctx, "job-1", item.ID, []byte(`{"status": "pass"}`))
	}

	result, err := agg.Aggregate(ctx, "job-1")
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	if len(result.ByScenario) != 2 {
		t.Errorf("ByScenario count = %d, want 2", len(result.ByScenario))
	}

	scenarioA := result.ByScenario["scenario-a"]
	if scenarioA == nil {
		t.Fatal("scenario-a stats not found")
	}
	if scenarioA.Total != 2 {
		t.Errorf("scenario-a Total = %d, want 2", scenarioA.Total)
	}
	if scenarioA.PassRate != 100 {
		t.Errorf("scenario-a PassRate = %f, want 100", scenarioA.PassRate)
	}

	scenarioB := result.ByScenario["scenario-b"]
	if scenarioB == nil {
		t.Fatal("scenario-b stats not found")
	}
	if scenarioB.Total != 1 {
		t.Errorf("scenario-b Total = %d, want 1", scenarioB.Total)
	}
}

func TestAggregator_Aggregate_ByProvider(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	agg := New(q)
	ctx := context.Background()

	// Push items with different providers
	items := []queue.WorkItem{
		{ID: "item-1", ProviderID: "openai"},
		{ID: "item-2", ProviderID: "anthropic"},
		{ID: "item-3", ProviderID: "openai"},
	}
	_ = q.Push(ctx, "job-1", items)

	// Complete all
	for range 3 {
		item, _ := q.Pop(ctx, "job-1")
		_ = q.Ack(ctx, "job-1", item.ID, []byte(`{"status": "pass"}`))
	}

	result, err := agg.Aggregate(ctx, "job-1")
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	if len(result.ByProvider) != 2 {
		t.Errorf("ByProvider count = %d, want 2", len(result.ByProvider))
	}

	openai := result.ByProvider["openai"]
	if openai == nil {
		t.Fatal("openai stats not found")
	}
	if openai.Total != 2 {
		t.Errorf("openai Total = %d, want 2", openai.Total)
	}

	anthropic := result.ByProvider["anthropic"]
	if anthropic == nil {
		t.Fatal("anthropic stats not found")
	}
	if anthropic.Total != 1 {
		t.Errorf("anthropic Total = %d, want 1", anthropic.Total)
	}
}

func TestAggregator_Aggregate_ErrorGrouping(t *testing.T) {
	q := queue.NewMemoryQueue(queue.Options{MaxRetries: 1})
	agg := New(q)
	ctx := context.Background()

	// Push items
	items := []queue.WorkItem{
		{ID: "item-1"},
		{ID: "item-2"},
		{ID: "item-3"},
	}
	_ = q.Push(ctx, "job-1", items)

	// Fail all with same error
	for range 3 {
		item, _ := q.Pop(ctx, "job-1")
		_ = q.Nack(ctx, "job-1", item.ID, &testError{msg: "connection timeout"})
	}

	result, err := agg.Aggregate(ctx, "job-1")
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	if len(result.Errors) != 1 {
		t.Errorf("Errors count = %d, want 1", len(result.Errors))
	}
	if result.Errors[0].Message != "connection timeout" {
		t.Errorf("Error message = %s, want 'connection timeout'", result.Errors[0].Message)
	}
	if result.Errors[0].Count != 3 {
		t.Errorf("Error count = %d, want 3", result.Errors[0].Count)
	}
	if len(result.Errors[0].WorkItemIDs) != 3 {
		t.Errorf("WorkItemIDs count = %d, want 3", len(result.Errors[0].WorkItemIDs))
	}
}

func TestAggregator_ToJobResult(t *testing.T) {
	agg := &Aggregator{}

	result := &AggregatedResult{
		TotalItems:    100,
		PassedItems:   85,
		FailedItems:   15,
		PassRate:      85.0,
		TotalDuration: 100 * time.Second,
		AvgDuration:   1 * time.Second,
		TotalTokens:   50000,
		TotalCost:     0.75,
	}

	jobResult := agg.ToJobResult(result)

	if jobResult == nil {
		t.Fatal("ToJobResult() returned nil")
	}

	tests := map[string]string{
		"passRate":      "85.0",
		"totalItems":    "100",
		"passedItems":   "85",
		"failedItems":   "15",
		"avgDurationMs": "1000",
		"totalTokens":   "50000",
		"totalCost":     "0.7500",
	}

	for key, expected := range tests {
		if jobResult.Summary[key] != expected {
			t.Errorf("Summary[%s] = %s, want %s", key, jobResult.Summary[key], expected)
		}
	}
}

func TestAggregator_ToJobResult_Nil(t *testing.T) {
	agg := &Aggregator{}

	jobResult := agg.ToJobResult(nil)
	if jobResult != nil {
		t.Error("ToJobResult(nil) should return nil")
	}
}

func TestAggregator_ToJobResult_NoOptionalMetrics(t *testing.T) {
	agg := &Aggregator{}

	result := &AggregatedResult{
		TotalItems:  10,
		PassedItems: 10,
		PassRate:    100,
		AvgDuration: 500 * time.Millisecond,
	}

	jobResult := agg.ToJobResult(result)

	// Optional metrics should not be present
	if _, ok := jobResult.Summary["totalTokens"]; ok {
		t.Error("totalTokens should not be present when zero")
	}
	if _, ok := jobResult.Summary["totalCost"]; ok {
		t.Error("totalCost should not be present when zero")
	}
}

func TestAggregator_ToJobResult_IncludesDetails(t *testing.T) {
	agg := &Aggregator{}

	result := &AggregatedResult{
		TotalItems:  3,
		PassedItems: 2,
		FailedItems: 1,
		PassRate:    66.7,
		AvgDuration: 500 * time.Millisecond,
		ByScenario: map[string]*ScenarioStats{
			"greeting":  {Total: 2, Passed: 2, PassRate: 100, AvgDuration: 300 * time.Millisecond, TotalTokens: 100},
			"complaint": {Total: 1, Passed: 0, Failed: 1, PassRate: 0, AvgDuration: 700 * time.Millisecond},
		},
		ByProvider: map[string]*ProviderStats{
			"fleet-agent": {Total: 3, Passed: 2, Failed: 1, PassRate: 66.7, AvgDuration: 500 * time.Millisecond},
		},
		Assertions: []AssertionResult{
			{Name: "response_contains_greeting", Passed: true},
			{Name: "response_is_polite", Passed: false, Message: "Response was rude"},
		},
		Errors: []ErrorSummary{
			{Message: "assertion [response_is_polite] failed", Count: 1, WorkItemIDs: []string{"item-3"}},
		},
	}

	jobResult := agg.ToJobResult(result)

	detailsJSON, ok := jobResult.Summary["details"]
	if !ok {
		t.Fatal("Summary should contain 'details' key")
	}

	var details resultDetails
	if err := json.Unmarshal([]byte(detailsJSON), &details); err != nil {
		t.Fatalf("Failed to parse details JSON: %v", err)
	}

	if len(details.Scenarios) != 2 {
		t.Errorf("expected 2 scenarios, got %d", len(details.Scenarios))
	}
	if len(details.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(details.Providers))
	}
	if len(details.Assertions) != 2 {
		t.Errorf("expected 2 assertion summaries, got %d", len(details.Assertions))
	}
	// Verify assertion summaries have correct fields
	if len(details.Assertions) >= 2 {
		a0 := details.Assertions[0]
		if a0.Name != "response_contains_greeting" || a0.Total != 1 || a0.Passed != 1 ||
			a0.Failed != 0 || a0.PassRate != 100 {
			t.Errorf("unexpected first assertion summary: %+v", a0)
		}
		a1 := details.Assertions[1]
		if a1.Name != "response_is_polite" || a1.Total != 1 || a1.Passed != 0 || a1.Failed != 1 || a1.PassRate != 0 {
			t.Errorf("unexpected second assertion summary: %+v", a1)
		}
		if len(a1.Failures) != 1 || a1.Failures[0] != "Response was rude" {
			t.Errorf("expected failure message 'Response was rude', got %v", a1.Failures)
		}
	}
	if len(details.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(details.Errors))
	}
}

func TestSummarizeAssertions(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := summarizeAssertions(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		result := summarizeAssertions([]AssertionResult{})
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("groups by name with pass fail counts", func(t *testing.T) {
		assertions := []AssertionResult{
			{Name: "check_format", Passed: true},
			{Name: "check_format", Passed: true},
			{Name: "check_format", Passed: false, Message: "bad format"},
			{Name: "check_tone", Passed: false, Message: "too aggressive"},
			{Name: "check_tone", Passed: true},
		}
		result := summarizeAssertions(assertions)
		if len(result) != 2 {
			t.Fatalf("expected 2 summaries, got %d", len(result))
		}

		// Verify insertion order preserved
		if result[0].Name != "check_format" {
			t.Errorf("expected first summary to be check_format, got %s", result[0].Name)
		}
		if result[0].Total != 3 || result[0].Passed != 2 || result[0].Failed != 1 {
			t.Errorf("unexpected check_format counts: %+v", result[0])
		}
		if result[0].PassRate != float64(2)/float64(3)*100 {
			t.Errorf("unexpected check_format pass rate: %f", result[0].PassRate)
		}
		if len(result[0].Failures) != 1 || result[0].Failures[0] != "bad format" {
			t.Errorf("unexpected check_format failures: %v", result[0].Failures)
		}

		if result[1].Name != "check_tone" {
			t.Errorf("expected second summary to be check_tone, got %s", result[1].Name)
		}
		if result[1].Total != 2 || result[1].Passed != 1 || result[1].Failed != 1 {
			t.Errorf("unexpected check_tone counts: %+v", result[1])
		}
	})

	t.Run("deduplicates failure messages", func(t *testing.T) {
		assertions := []AssertionResult{
			{Name: "check_length", Passed: false, Message: "too long"},
			{Name: "check_length", Passed: false, Message: "too long"},
			{Name: "check_length", Passed: false, Message: "too short"},
		}
		result := summarizeAssertions(assertions)
		if len(result) != 1 {
			t.Fatalf("expected 1 summary, got %d", len(result))
		}
		if len(result[0].Failures) != 2 {
			t.Errorf("expected 2 unique failure messages, got %d: %v", len(result[0].Failures), result[0].Failures)
		}
	})

	t.Run("all pass has no failures slice", func(t *testing.T) {
		assertions := []AssertionResult{
			{Name: "check_ok", Passed: true},
			{Name: "check_ok", Passed: true},
		}
		result := summarizeAssertions(assertions)
		if len(result) != 1 {
			t.Fatalf("expected 1 summary, got %d", len(result))
		}
		if result[0].PassRate != 100 {
			t.Errorf("expected 100%% pass rate, got %f", result[0].PassRate)
		}
		if result[0].Failures != nil {
			t.Errorf("expected nil failures for all-pass, got %v", result[0].Failures)
		}
	})
}

func TestAggregator_Aggregate_JobNotFound(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	agg := New(q)
	ctx := context.Background()

	_, err := agg.Aggregate(ctx, "nonexistent-job")
	if err == nil {
		t.Error("Aggregate() should return error for nonexistent job")
	}
}

func TestStatsToResult_Nil(t *testing.T) {
	result := StatsToResult(nil)
	if result == nil {
		t.Fatal("StatsToResult(nil) returned nil")
	}
	if result.TotalItems != 0 {
		t.Errorf("TotalItems = %d, want 0", result.TotalItems)
	}
}

func TestStatsToResult_Basic(t *testing.T) {
	stats := &queue.JobStats{
		Total:           100,
		Passed:          85,
		Failed:          15,
		TotalDurationMs: 50000, // 50s total
		TotalTokens:     25000,
		TotalCost:       1.50,
	}

	result := StatsToResult(stats)

	if result.TotalItems != 100 {
		t.Errorf("TotalItems = %d, want 100", result.TotalItems)
	}
	if result.PassedItems != 85 {
		t.Errorf("PassedItems = %d, want 85", result.PassedItems)
	}
	if result.FailedItems != 15 {
		t.Errorf("FailedItems = %d, want 15", result.FailedItems)
	}
	if result.PassRate != 85 {
		t.Errorf("PassRate = %f, want 85", result.PassRate)
	}
	if result.TotalDuration != 50*time.Second {
		t.Errorf("TotalDuration = %v, want 50s", result.TotalDuration)
	}
	if result.AvgDuration != 500*time.Millisecond {
		t.Errorf("AvgDuration = %v, want 500ms", result.AvgDuration)
	}
	if result.TotalTokens != 25000 {
		t.Errorf("TotalTokens = %d, want 25000", result.TotalTokens)
	}
	if result.TotalCost != 1.50 {
		t.Errorf("TotalCost = %f, want 1.50", result.TotalCost)
	}
}

func TestStatsToResult_WithGroups(t *testing.T) {
	stats := &queue.JobStats{
		Total:           4,
		Passed:          3,
		Failed:          1,
		TotalDurationMs: 4000,
		ByScenario: map[string]*queue.GroupStats{
			"greeting": {Total: 2, Passed: 2, Failed: 0, TotalDurationMs: 2000, TotalTokens: 100},
			"billing":  {Total: 2, Passed: 1, Failed: 1, TotalDurationMs: 2000, TotalCost: 0.05},
		},
		ByProvider: map[string]*queue.GroupStats{
			"openai":    {Total: 2, Passed: 2, Failed: 0, TotalDurationMs: 1500},
			"anthropic": {Total: 2, Passed: 1, Failed: 1, TotalDurationMs: 2500},
		},
	}

	result := StatsToResult(stats)

	if len(result.ByScenario) != 2 {
		t.Fatalf("ByScenario count = %d, want 2", len(result.ByScenario))
	}
	greeting := result.ByScenario["greeting"]
	if greeting.Total != 2 || greeting.Passed != 2 || greeting.PassRate != 100 {
		t.Errorf("greeting stats = %+v", greeting)
	}
	if greeting.TotalTokens != 100 {
		t.Errorf("greeting TotalTokens = %d, want 100", greeting.TotalTokens)
	}

	billing := result.ByScenario["billing"]
	if billing.Total != 2 || billing.Passed != 1 || billing.Failed != 1 || billing.PassRate != 50 {
		t.Errorf("billing stats = %+v", billing)
	}

	if len(result.ByProvider) != 2 {
		t.Fatalf("ByProvider count = %d, want 2", len(result.ByProvider))
	}
	openai := result.ByProvider["openai"]
	if openai.Total != 2 || openai.PassRate != 100 {
		t.Errorf("openai stats = %+v", openai)
	}
	anthropic := result.ByProvider["anthropic"]
	if anthropic.Total != 2 || anthropic.PassRate != 50 {
		t.Errorf("anthropic stats = %+v", anthropic)
	}
}

func TestStatsToResult_ZeroItems(t *testing.T) {
	stats := &queue.JobStats{}
	result := StatsToResult(stats)
	if result.TotalItems != 0 {
		t.Errorf("TotalItems = %d, want 0", result.TotalItems)
	}
	if result.PassRate != 0 {
		t.Errorf("PassRate = %f, want 0", result.PassRate)
	}
	if result.ByScenario != nil {
		t.Error("ByScenario should be nil for empty stats")
	}
	if result.ByProvider != nil {
		t.Error("ByProvider should be nil for empty stats")
	}
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
