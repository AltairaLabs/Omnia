/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package aggregator

import (
	"testing"
	"time"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

func TestParseExecutionResult_NilItem(t *testing.T) {
	_, err := ParseExecutionResult(nil)
	if err != ErrNilWorkItem {
		t.Errorf("ParseExecutionResult(nil) error = %v, want %v", err, ErrNilWorkItem)
	}
}

func TestParseExecutionResult_NoResult(t *testing.T) {
	now := time.Now()
	completed := now.Add(time.Second)

	item := &queue.WorkItem{
		ID:          "item-1",
		ScenarioID:  "scenario-1",
		ProviderID:  "provider-1",
		Status:      queue.ItemStatusCompleted,
		StartedAt:   &now,
		CompletedAt: &completed,
	}

	result, err := ParseExecutionResult(item)
	if err != nil {
		t.Fatalf("ParseExecutionResult() error = %v", err)
	}

	if result.Status != StatusPass {
		t.Errorf("Status = %s, want %s", result.Status, StatusPass)
	}
	if result.WorkItemID != "item-1" {
		t.Errorf("WorkItemID = %s, want item-1", result.WorkItemID)
	}
	if result.ScenarioID != "scenario-1" {
		t.Errorf("ScenarioID = %s, want scenario-1", result.ScenarioID)
	}
	if result.Duration != time.Second {
		t.Errorf("Duration = %v, want 1s", result.Duration)
	}
}

func TestParseExecutionResult_FailedItem(t *testing.T) {
	item := &queue.WorkItem{
		ID:     "item-1",
		Status: queue.ItemStatusFailed,
		Error:  "connection timeout",
	}

	result, err := ParseExecutionResult(item)
	if err != nil {
		t.Fatalf("ParseExecutionResult() error = %v", err)
	}

	if result.Status != StatusFail {
		t.Errorf("Status = %s, want %s", result.Status, StatusFail)
	}
	if result.Error != "connection timeout" {
		t.Errorf("Error = %s, want 'connection timeout'", result.Error)
	}
}

func TestParseExecutionResult_JSONResult(t *testing.T) {
	jsonData := []byte(`{
		"status": "pass",
		"durationMs": 1500,
		"metrics": {
			"tokens": 100,
			"cost": 0.05,
			"latency_ms": 250
		},
		"assertions": [
			{"name": "check_output", "passed": true},
			{"name": "check_format", "passed": false, "message": "invalid format"}
		]
	}`)

	item := &queue.WorkItem{
		ID:     "item-1",
		Status: queue.ItemStatusCompleted,
		Result: jsonData,
	}

	result, err := ParseExecutionResult(item)
	if err != nil {
		t.Fatalf("ParseExecutionResult() error = %v", err)
	}

	if result.Status != StatusPass {
		t.Errorf("Status = %s, want %s", result.Status, StatusPass)
	}
	if result.Duration != 1500*time.Millisecond {
		t.Errorf("Duration = %v, want 1.5s", result.Duration)
	}
	if result.Metrics["tokens"] != 100 {
		t.Errorf("Metrics[tokens] = %v, want 100", result.Metrics["tokens"])
	}
	if result.Metrics["cost"] != 0.05 {
		t.Errorf("Metrics[cost] = %v, want 0.05", result.Metrics["cost"])
	}
	if len(result.Assertions) != 2 {
		t.Errorf("Assertions count = %d, want 2", len(result.Assertions))
	}
	if !result.Assertions[0].Passed {
		t.Error("First assertion should have passed")
	}
	if result.Assertions[1].Passed {
		t.Error("Second assertion should have failed")
	}
}

func TestParseExecutionResult_JSONWithDurationString(t *testing.T) {
	jsonData := []byte(`{
		"status": "pass",
		"duration": "2s"
	}`)

	item := &queue.WorkItem{
		ID:     "item-1",
		Status: queue.ItemStatusCompleted,
		Result: jsonData,
	}

	result, err := ParseExecutionResult(item)
	if err != nil {
		t.Fatalf("ParseExecutionResult() error = %v", err)
	}

	if result.Duration != 2*time.Second {
		t.Errorf("Duration = %v, want 2s", result.Duration)
	}
}

func TestParseExecutionResult_InvalidJSON(t *testing.T) {
	item := &queue.WorkItem{
		ID:     "item-1",
		Status: queue.ItemStatusCompleted,
		Result: []byte("not valid json"),
	}

	result, err := ParseExecutionResult(item)
	if err != nil {
		t.Fatalf("ParseExecutionResult() error = %v", err)
	}

	// Should still return a result based on item status
	if result.Status != StatusPass {
		t.Errorf("Status = %s, want %s", result.Status, StatusPass)
	}
}
