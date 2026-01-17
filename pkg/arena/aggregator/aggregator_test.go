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

package aggregator

import (
	"context"
	"testing"
	"time"

	"github.com/altairalabs/omnia/pkg/arena/queue"
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

func TestAggregator_Aggregate_JobNotFound(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	agg := New(q)
	ctx := context.Background()

	_, err := agg.Aggregate(ctx, "nonexistent-job")
	if err == nil {
		t.Error("Aggregate() should return error for nonexistent job")
	}
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
