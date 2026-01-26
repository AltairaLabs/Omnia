/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package aggregator

import (
	"context"
	"fmt"
	"time"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

// Aggregator collects and summarizes results from Arena job executions.
type Aggregator struct {
	queue queue.WorkQueue
}

// New creates a new Aggregator with the given queue.
func New(q queue.WorkQueue) *Aggregator {
	return &Aggregator{
		queue: q,
	}
}

// Aggregate collects and summarizes results for a completed job.
// It retrieves all completed and failed work items from the queue,
// parses their results, and produces an aggregated summary.
func (a *Aggregator) Aggregate(ctx context.Context, jobID string) (*AggregatedResult, error) {
	// Get all completed items
	completed, err := a.queue.GetCompletedItems(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get completed items: %w", err)
	}

	// Get all failed items
	failed, err := a.queue.GetFailedItems(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get failed items: %w", err)
	}

	// Parse results and aggregate
	result := &AggregatedResult{
		ByScenario: make(map[string]*ScenarioStats),
		ByProvider: make(map[string]*ProviderStats),
	}

	// Track errors for grouping
	errorCounts := make(map[string]*ErrorSummary)

	// Process completed items
	for _, item := range completed {
		execResult, err := ParseExecutionResult(item)
		if err != nil {
			continue
		}
		a.aggregateResult(result, execResult, errorCounts)
	}

	// Process failed items
	for _, item := range failed {
		execResult, err := ParseExecutionResult(item)
		if err != nil {
			// Even if parsing fails, count the failure
			result.TotalItems++
			result.FailedItems++
			a.trackError(errorCounts, item.Error, item.ID)
			continue
		}
		a.aggregateResult(result, execResult, errorCounts)
	}

	// Calculate averages and rates
	a.finalizeResult(result, errorCounts)

	return result, nil
}

// aggregateResult adds a single execution result to the aggregated result.
func (a *Aggregator) aggregateResult(
	result *AggregatedResult, execResult *ExecutionResult, errorCounts map[string]*ErrorSummary,
) {
	result.TotalItems++
	result.TotalDuration += execResult.Duration

	// Track pass/fail
	if execResult.Status == StatusPass {
		result.PassedItems++
	} else {
		result.FailedItems++
		if execResult.Error != "" {
			a.trackError(errorCounts, execResult.Error, execResult.WorkItemID)
		}
	}

	// Aggregate metrics
	if execResult.Metrics != nil {
		if tokens, ok := execResult.Metrics["tokens"]; ok {
			result.TotalTokens += int64(tokens)
		}
		if cost, ok := execResult.Metrics["cost"]; ok {
			result.TotalCost += cost
		}
	}

	// Update scenario stats
	if execResult.ScenarioID != "" {
		stats := result.ByScenario[execResult.ScenarioID]
		if stats == nil {
			stats = &ScenarioStats{}
			result.ByScenario[execResult.ScenarioID] = stats
		}
		a.updateScenarioStats(stats, execResult)
	}

	// Update provider stats
	if execResult.ProviderID != "" {
		stats := result.ByProvider[execResult.ProviderID]
		if stats == nil {
			stats = &ProviderStats{}
			result.ByProvider[execResult.ProviderID] = stats
		}
		a.updateProviderStats(stats, execResult)
	}
}

// updateScenarioStats updates statistics for a scenario.
func (a *Aggregator) updateScenarioStats(stats *ScenarioStats, execResult *ExecutionResult) {
	stats.Total++
	stats.TotalDuration += execResult.Duration

	if execResult.Status == StatusPass {
		stats.Passed++
	} else {
		stats.Failed++
	}

	if execResult.Metrics != nil {
		if tokens, ok := execResult.Metrics["tokens"]; ok {
			stats.TotalTokens += int64(tokens)
		}
		if cost, ok := execResult.Metrics["cost"]; ok {
			stats.TotalCost += cost
		}
	}
}

// updateProviderStats updates statistics for a provider.
func (a *Aggregator) updateProviderStats(stats *ProviderStats, execResult *ExecutionResult) {
	stats.Total++
	stats.TotalDuration += execResult.Duration

	if execResult.Status == StatusPass {
		stats.Passed++
	} else {
		stats.Failed++
	}

	if execResult.Metrics != nil {
		if tokens, ok := execResult.Metrics["tokens"]; ok {
			stats.TotalTokens += int64(tokens)
		}
		if cost, ok := execResult.Metrics["cost"]; ok {
			stats.TotalCost += cost
		}
	}
}

// trackError groups errors by message.
func (a *Aggregator) trackError(errorCounts map[string]*ErrorSummary, errorMsg string, workItemID string) {
	if errorMsg == "" {
		errorMsg = "unknown error"
	}

	summary := errorCounts[errorMsg]
	if summary == nil {
		summary = &ErrorSummary{
			Message:     errorMsg,
			WorkItemIDs: []string{},
		}
		errorCounts[errorMsg] = summary
	}
	summary.Count++
	summary.WorkItemIDs = append(summary.WorkItemIDs, workItemID)
}

// finalizeResult calculates averages and converts error map to slice.
func (a *Aggregator) finalizeResult(result *AggregatedResult, errorCounts map[string]*ErrorSummary) {
	// Calculate overall averages
	if result.TotalItems > 0 {
		result.PassRate = float64(result.PassedItems) / float64(result.TotalItems) * 100
		result.AvgDuration = result.TotalDuration / time.Duration(result.TotalItems)
	}

	// Calculate scenario averages
	for _, stats := range result.ByScenario {
		if stats.Total > 0 {
			stats.PassRate = float64(stats.Passed) / float64(stats.Total) * 100
			stats.AvgDuration = stats.TotalDuration / time.Duration(stats.Total)
		}
	}

	// Calculate provider averages
	for _, stats := range result.ByProvider {
		if stats.Total > 0 {
			stats.PassRate = float64(stats.Passed) / float64(stats.Total) * 100
			stats.AvgDuration = stats.TotalDuration / time.Duration(stats.Total)
		}
	}

	// Convert error map to slice
	result.Errors = make([]ErrorSummary, 0, len(errorCounts))
	for _, summary := range errorCounts {
		result.Errors = append(result.Errors, *summary)
	}

	// Clean up empty maps
	if len(result.ByScenario) == 0 {
		result.ByScenario = nil
	}
	if len(result.ByProvider) == 0 {
		result.ByProvider = nil
	}
	if len(result.Errors) == 0 {
		result.Errors = nil
	}
}

// ToJobResult converts an AggregatedResult to the CRD JobResult format.
// This is used to populate the ArenaJob.Status.Result field.
func (a *Aggregator) ToJobResult(result *AggregatedResult) *omniav1alpha1.JobResult {
	if result == nil {
		return nil
	}

	summary := make(map[string]string)

	// Add core metrics
	summary["passRate"] = fmt.Sprintf("%.1f", result.PassRate)
	summary["totalItems"] = fmt.Sprintf("%d", result.TotalItems)
	summary["passedItems"] = fmt.Sprintf("%d", result.PassedItems)
	summary["failedItems"] = fmt.Sprintf("%d", result.FailedItems)
	summary["avgDurationMs"] = fmt.Sprintf("%d", result.AvgDuration.Milliseconds())

	// Add optional metrics if present
	if result.TotalTokens > 0 {
		summary["totalTokens"] = fmt.Sprintf("%d", result.TotalTokens)
	}
	if result.TotalCost > 0 {
		summary["totalCost"] = fmt.Sprintf("%.4f", result.TotalCost)
	}

	return &omniav1alpha1.JobResult{
		Summary: summary,
	}
}
