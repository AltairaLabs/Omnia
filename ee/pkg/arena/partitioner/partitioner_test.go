/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package partitioner

import (
	"encoding/json"
	"testing"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

func TestPartition(t *testing.T) {
	input := PartitionInput{
		JobID:     "job-1",
		BundleURL: "http://example.com/bundle.tar.gz",
		Scenarios: []Scenario{
			{ID: "scenario-1", Name: "Test Scenario 1", Path: "scenarios/test1.yaml"},
			{ID: "scenario-2", Name: "Test Scenario 2", Path: "scenarios/test2.yaml"},
		},
		Providers: []Provider{
			{ID: "ns1/provider-1", Name: "provider-1", Namespace: "ns1"},
			{ID: "ns2/provider-2", Name: "provider-2", Namespace: "ns2"},
		},
		MaxRetries: 3,
	}

	result, err := Partition(input)
	if err != nil {
		t.Fatalf("Partition() error = %v", err)
	}

	// Should have 2 scenarios × 2 providers = 4 work items
	if result.TotalCombinations != 4 {
		t.Errorf("TotalCombinations = %d, want 4", result.TotalCombinations)
	}
	if len(result.Items) != 4 {
		t.Errorf("len(Items) = %d, want 4", len(result.Items))
	}
	if result.ScenarioCount != 2 {
		t.Errorf("ScenarioCount = %d, want 2", result.ScenarioCount)
	}
	if result.ProviderCount != 2 {
		t.Errorf("ProviderCount = %d, want 2", result.ProviderCount)
	}

	// Verify work items have correct fields
	for _, item := range result.Items {
		if item.JobID != "job-1" {
			t.Errorf("Item.JobID = %s, want job-1", item.JobID)
		}
		if item.BundleURL != "http://example.com/bundle.tar.gz" {
			t.Errorf("Item.BundleURL = %s, want http://example.com/bundle.tar.gz", item.BundleURL)
		}
		if item.MaxAttempts != 3 {
			t.Errorf("Item.MaxAttempts = %d, want 3", item.MaxAttempts)
		}
		if item.ID == "" {
			t.Error("Item.ID is empty")
		}
	}
}

func TestPartitionWithConfig(t *testing.T) {
	input := PartitionInput{
		JobID:     "job-1",
		BundleURL: "http://example.com/bundle.tar.gz",
		Scenarios: []Scenario{
			{ID: "scenario-1", Name: "Test", Path: "test.yaml", Tags: []string{"smoke"}},
		},
		Providers: []Provider{
			{ID: "ns/provider", Name: "provider", Namespace: "ns"},
		},
		Config: map[string]any{
			"timeout": "30s",
			"verbose": true,
		},
	}

	result, err := Partition(input)
	if err != nil {
		t.Fatalf("Partition() error = %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(result.Items))
	}

	// Verify config contains base config and scenario/provider info
	var config map[string]any
	if err := json.Unmarshal(result.Items[0].Config, &config); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if config["timeout"] != "30s" {
		t.Errorf("config[timeout] = %v, want 30s", config["timeout"])
	}
	if config["verbose"] != true {
		t.Errorf("config[verbose] = %v, want true", config["verbose"])
	}

	scenario := config["scenario"].(map[string]any)
	if scenario["id"] != "scenario-1" {
		t.Errorf("config[scenario][id] = %v, want scenario-1", scenario["id"])
	}

	provider := config["provider"].(map[string]any)
	if provider["name"] != "provider" {
		t.Errorf("config[provider][name] = %v, want provider", provider["name"])
	}
}

func TestPartitionNoScenarios(t *testing.T) {
	input := PartitionInput{
		JobID:     "job-1",
		Scenarios: []Scenario{},
		Providers: []Provider{{ID: "p1", Name: "p1"}},
	}

	_, err := Partition(input)
	if err == nil {
		t.Error("Partition() expected error for empty scenarios")
	}
}

func TestPartitionNoProviders(t *testing.T) {
	input := PartitionInput{
		JobID:     "job-1",
		Scenarios: []Scenario{{ID: "s1", Name: "s1"}},
		Providers: []Provider{},
	}

	_, err := Partition(input)
	if err == nil {
		t.Error("Partition() expected error for empty providers")
	}
}

func TestFilter(t *testing.T) {
	scenarios := []Scenario{
		{ID: "s1", Path: "scenarios/billing.yaml"},
		{ID: "s2", Path: "scenarios/auth.yaml"},
		{ID: "s3", Path: "scenarios/billing-wip.yaml"},
		{ID: "s4", Path: "tests/integration.yaml"},
		{ID: "s5", Path: "tests/unit.yaml"},
	}

	tests := []struct {
		name    string
		include []string
		exclude []string
		wantIDs []string
	}{
		{
			name:    "no filters includes all",
			include: nil,
			exclude: nil,
			wantIDs: []string{"s1", "s2", "s3", "s4", "s5"},
		},
		{
			name:    "include scenarios only",
			include: []string{"scenarios/*.yaml"},
			exclude: nil,
			wantIDs: []string{"s1", "s2", "s3"},
		},
		{
			name:    "exclude wip files",
			include: nil,
			exclude: []string{"*-wip.yaml"},
			wantIDs: []string{"s1", "s2", "s4", "s5"},
		},
		{
			name:    "include and exclude",
			include: []string{"scenarios/*.yaml"},
			exclude: []string{"*-wip.yaml"},
			wantIDs: []string{"s1", "s2"},
		},
		{
			name:    "include tests only",
			include: []string{"tests/*.yaml"},
			exclude: nil,
			wantIDs: []string{"s4", "s5"},
		},
		{
			name:    "include by filename",
			include: []string{"billing.yaml"},
			exclude: nil,
			wantIDs: []string{"s1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Filter(scenarios, tt.include, tt.exclude)
			if err != nil {
				t.Fatalf("Filter() error = %v", err)
			}

			if len(result) != len(tt.wantIDs) {
				t.Errorf("len(result) = %d, want %d", len(result), len(tt.wantIDs))
				return
			}

			gotIDs := make(map[string]bool)
			for _, s := range result {
				gotIDs[s.ID] = true
			}

			for _, wantID := range tt.wantIDs {
				if !gotIDs[wantID] {
					t.Errorf("Missing expected scenario %s", wantID)
				}
			}
		})
	}
}

func TestEstimateWorkItems(t *testing.T) {
	tests := []struct {
		scenarios int
		providers int
		want      int
	}{
		{10, 5, 50},
		{1, 1, 1},
		{100, 10, 1000},
		{0, 5, 0},
		{5, 0, 0},
	}

	for _, tt := range tests {
		got := EstimateWorkItems(tt.scenarios, tt.providers)
		if got != tt.want {
			t.Errorf("EstimateWorkItems(%d, %d) = %d, want %d",
				tt.scenarios, tt.providers, got, tt.want)
		}
	}
}

func TestBatch(t *testing.T) {
	// Create 10 work items
	items := make([]queue.WorkItem, 10)
	for i := range items {
		items[i].ID = string(rune('A' + i))
	}

	tests := []struct {
		name      string
		batchSize int
		wantCount int
		wantSizes []int
	}{
		{
			name:      "batch of 3",
			batchSize: 3,
			wantCount: 4,
			wantSizes: []int{3, 3, 3, 1},
		},
		{
			name:      "batch of 5",
			batchSize: 5,
			wantCount: 2,
			wantSizes: []int{5, 5},
		},
		{
			name:      "batch larger than items",
			batchSize: 20,
			wantCount: 1,
			wantSizes: []int{10},
		},
		{
			name:      "batch of 1",
			batchSize: 1,
			wantCount: 10,
			wantSizes: []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		},
		{
			name:      "zero batch size returns all",
			batchSize: 0,
			wantCount: 1,
			wantSizes: []int{10},
		},
		{
			name:      "negative batch size returns all",
			batchSize: -1,
			wantCount: 1,
			wantSizes: []int{10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batches := Batch(items, tt.batchSize)

			if len(batches) != tt.wantCount {
				t.Errorf("len(batches) = %d, want %d", len(batches), tt.wantCount)
				return
			}

			for i, batch := range batches {
				if len(batch) != tt.wantSizes[i] {
					t.Errorf("len(batches[%d]) = %d, want %d", i, len(batch), tt.wantSizes[i])
				}
			}
		})
	}
}

func TestBatchEmpty(t *testing.T) {
	var items []queue.WorkItem
	batches := Batch(items, 5)

	if len(batches) != 0 {
		t.Errorf("len(batches) = %d, want 0 for empty input", len(batches))
	}
}

func TestMatchesAny(t *testing.T) {
	tests := []struct {
		path     string
		patterns []string
		want     bool
	}{
		{"scenarios/test.yaml", []string{"scenarios/*.yaml"}, true},
		{"scenarios/test.yaml", []string{"tests/*.yaml"}, false},
		{"scenarios/test.yaml", []string{"*.yaml"}, true},
		{"scenarios/test.yaml", []string{"test.yaml"}, true},
		{"scenarios/test.yaml", []string{"other.yaml"}, false},
		{"test.yaml", []string{"*.yaml"}, true},
		{"test.yaml", []string{"*.json"}, false},
	}

	for _, tt := range tests {
		got := matchesAny(tt.path, tt.patterns)
		if got != tt.want {
			t.Errorf("matchesAny(%q, %v) = %v, want %v",
				tt.path, tt.patterns, got, tt.want)
		}
	}
}

func TestGenerateItemID(t *testing.T) {
	id1 := generateItemID("job-1", "scenario-1", "provider-1")
	id2 := generateItemID("job-1", "scenario-1", "provider-1")

	// IDs should be unique
	if id1 == id2 {
		t.Error("generateItemID should produce unique IDs")
	}

	// IDs should start with scenario prefix
	if len(id1) < 8 {
		t.Errorf("ID too short: %s", id1)
	}
}
