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

// Package aggregator provides result aggregation for Arena jobs.
// It collects results from completed work items, parses various output formats,
// and produces aggregated metrics and summaries.
package aggregator

import "time"

// Status constants for execution results.
const (
	// StatusPass indicates successful execution.
	StatusPass = "pass"

	// StatusFail indicates failed execution.
	StatusFail = "fail"

	// StatusUnknown indicates unknown execution status.
	StatusUnknown = "unknown"
)

// ExecutionResult represents the result of executing a single work item.
type ExecutionResult struct {
	// WorkItemID is the unique identifier for the work item.
	WorkItemID string `json:"workItemId"`

	// ScenarioID identifies which scenario was executed.
	ScenarioID string `json:"scenarioId"`

	// ProviderID identifies which provider was used.
	ProviderID string `json:"providerId"`

	// Status indicates the execution outcome: "pass" or "fail".
	Status string `json:"status"`

	// Error contains the error message if execution failed.
	Error string `json:"error,omitempty"`

	// Duration is the execution time.
	Duration time.Duration `json:"duration"`

	// Metrics contains additional numeric metrics like latency_ms, tokens, cost.
	Metrics map[string]float64 `json:"metrics,omitempty"`

	// Assertions contains individual assertion results if applicable.
	Assertions []AssertionResult `json:"assertions,omitempty"`
}

// AssertionResult represents the result of a single assertion.
type AssertionResult struct {
	// Name is the assertion identifier or description.
	Name string `json:"name"`

	// Passed indicates whether the assertion passed.
	Passed bool `json:"passed"`

	// Message contains additional details about the assertion result.
	Message string `json:"message,omitempty"`
}

// ScenarioStats contains aggregated statistics for a single scenario.
type ScenarioStats struct {
	// Total is the total number of executions for this scenario.
	Total int `json:"total"`

	// Passed is the number of successful executions.
	Passed int `json:"passed"`

	// Failed is the number of failed executions.
	Failed int `json:"failed"`

	// PassRate is the success rate as a percentage (0-100).
	PassRate float64 `json:"passRate"`

	// TotalDuration is the sum of all execution durations.
	TotalDuration time.Duration `json:"totalDuration"`

	// AvgDuration is the average execution duration.
	AvgDuration time.Duration `json:"avgDuration"`

	// TotalTokens is the total token count if available.
	TotalTokens int64 `json:"totalTokens,omitempty"`

	// TotalCost is the total cost if available.
	TotalCost float64 `json:"totalCost,omitempty"`
}

// ProviderStats contains aggregated statistics for a single provider.
type ProviderStats struct {
	// Total is the total number of executions for this provider.
	Total int `json:"total"`

	// Passed is the number of successful executions.
	Passed int `json:"passed"`

	// Failed is the number of failed executions.
	Failed int `json:"failed"`

	// PassRate is the success rate as a percentage (0-100).
	PassRate float64 `json:"passRate"`

	// TotalDuration is the sum of all execution durations.
	TotalDuration time.Duration `json:"totalDuration"`

	// AvgDuration is the average execution duration.
	AvgDuration time.Duration `json:"avgDuration"`

	// TotalTokens is the total token count if available.
	TotalTokens int64 `json:"totalTokens,omitempty"`

	// TotalCost is the total cost if available.
	TotalCost float64 `json:"totalCost,omitempty"`
}

// ErrorSummary groups errors by message for reporting.
type ErrorSummary struct {
	// Message is the error message.
	Message string `json:"message"`

	// Count is the number of times this error occurred.
	Count int `json:"count"`

	// WorkItemIDs contains the IDs of work items that had this error.
	WorkItemIDs []string `json:"workItemIds,omitempty"`
}

// AggregatedResult contains summary metrics for a completed job.
type AggregatedResult struct {
	// TotalItems is the total number of work items processed.
	TotalItems int `json:"totalItems"`

	// PassedItems is the number of items that passed.
	PassedItems int `json:"passedItems"`

	// FailedItems is the number of items that failed.
	FailedItems int `json:"failedItems"`

	// PassRate is the success rate as a percentage (0-100).
	PassRate float64 `json:"passRate"`

	// TotalDuration is the sum of all execution durations.
	TotalDuration time.Duration `json:"totalDuration"`

	// AvgDuration is the average execution duration.
	AvgDuration time.Duration `json:"avgDuration"`

	// TotalTokens is the total token count across all executions.
	TotalTokens int64 `json:"totalTokens,omitempty"`

	// TotalCost is the total cost across all executions.
	TotalCost float64 `json:"totalCost,omitempty"`

	// ByScenario contains per-scenario statistics.
	ByScenario map[string]*ScenarioStats `json:"byScenario,omitempty"`

	// ByProvider contains per-provider statistics.
	ByProvider map[string]*ProviderStats `json:"byProvider,omitempty"`

	// Errors contains grouped error summaries.
	Errors []ErrorSummary `json:"errors,omitempty"`
}
