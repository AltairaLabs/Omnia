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

	// Verify trial metadata is present
	if config["trialIndex"] != float64(0) {
		t.Errorf("config[trialIndex] = %v, want 0", config["trialIndex"])
	}
	if config["totalTrials"] != float64(1) {
		t.Errorf("config[totalTrials] = %v, want 1", config["totalTrials"])
	}
}

func TestPartitionWithTrials(t *testing.T) {
	input := PartitionInput{
		JobID:     "job-1",
		BundleURL: "http://example.com/bundle.tar.gz",
		Scenarios: []Scenario{
			{ID: "s1", Name: "S1", Path: "s1.yaml", Trials: 3},
			{ID: "s2", Name: "S2", Path: "s2.yaml"},
		},
		Providers: []Provider{
			{ID: "p1", Name: "p1", Namespace: "ns"},
		},
	}

	result, err := Partition(input)
	if err != nil {
		t.Fatalf("Partition() error = %v", err)
	}

	// s1 has 3 trials, s2 has 0 (defaults to 1) → (3+1) × 1 provider = 4 items
	if len(result.Items) != 4 {
		t.Errorf("len(Items) = %d, want 4", len(result.Items))
	}
	if result.TotalCombinations != 4 {
		t.Errorf("TotalCombinations = %d, want 4", result.TotalCombinations)
	}
	if result.TrialCount != 4 {
		t.Errorf("TrialCount = %d, want 4", result.TrialCount)
	}

	// Verify trial indices in config for s1 (first 3 items)
	for i := 0; i < 3; i++ {
		var cfg map[string]any
		if err := json.Unmarshal(result.Items[i].Config, &cfg); err != nil {
			t.Fatalf("unmarshal config[%d]: %v", i, err)
		}
		if cfg["trialIndex"] != float64(i) {
			t.Errorf("item[%d] trialIndex = %v, want %d", i, cfg["trialIndex"], i)
		}
		if cfg["totalTrials"] != float64(3) {
			t.Errorf("item[%d] totalTrials = %v, want 3", i, cfg["totalTrials"])
		}
	}

	// s2's single trial should have trialIndex=0, totalTrials=1
	var cfg map[string]any
	if err := json.Unmarshal(result.Items[3].Config, &cfg); err != nil {
		t.Fatalf("unmarshal config[3]: %v", err)
	}
	if cfg["trialIndex"] != float64(0) {
		t.Errorf("item[3] trialIndex = %v, want 0", cfg["trialIndex"])
	}
	if cfg["totalTrials"] != float64(1) {
		t.Errorf("item[3] totalTrials = %v, want 1", cfg["totalTrials"])
	}
}

func TestPartitionJobTrialsOverride(t *testing.T) {
	input := PartitionInput{
		JobID:     "job-1",
		BundleURL: "http://example.com/bundle.tar.gz",
		Scenarios: []Scenario{
			{ID: "s1", Name: "S1", Path: "s1.yaml", Trials: 3},
			{ID: "s2", Name: "S2", Path: "s2.yaml", Trials: 5},
		},
		Providers: []Provider{
			{ID: "p1", Name: "p1", Namespace: "ns"},
		},
		JobTrials: 10,
	}

	result, err := Partition(input)
	if err != nil {
		t.Fatalf("Partition() error = %v", err)
	}

	// JobTrials=10 overrides per-scenario → 2 scenarios × 1 provider × 10 trials = 20
	if len(result.Items) != 20 {
		t.Errorf("len(Items) = %d, want 20", len(result.Items))
	}
	if result.TrialCount != 20 {
		t.Errorf("TrialCount = %d, want 20", result.TrialCount)
	}

	// All items should have totalTrials=10
	var cfg map[string]any
	if err := json.Unmarshal(result.Items[0].Config, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg["totalTrials"] != float64(10) {
		t.Errorf("totalTrials = %v, want 10", cfg["totalTrials"])
	}
}

func TestResolveTrialCount(t *testing.T) {
	tests := []struct {
		name           string
		jobTrials      int
		scenarioTrials int
		want           int
	}{
		{"job overrides scenario", 5, 3, 5},
		{"scenario used when no job override", 0, 3, 3},
		{"defaults to 1 when both zero", 0, 0, 1},
		{"job overrides even when scenario is zero", 5, 0, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTrialCount(tt.jobTrials, tt.scenarioTrials)
			if got != tt.want {
				t.Errorf("resolveTrialCount(%d, %d) = %d, want %d",
					tt.jobTrials, tt.scenarioTrials, got, tt.want)
			}
		})
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
		{
			name:    "include by scenario ID",
			include: []string{"s2"},
			exclude: nil,
			wantIDs: []string{"s2"},
		},
		{
			name:    "include multiple by ID",
			include: []string{"s1", "s4"},
			exclude: nil,
			wantIDs: []string{"s1", "s4"},
		},
		{
			name:    "exclude by ID",
			include: nil,
			exclude: []string{"s3"},
			wantIDs: []string{"s1", "s2", "s4", "s5"},
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

func TestScenarioMatches(t *testing.T) {
	tests := []struct {
		name     string
		scenario Scenario
		patterns []string
		want     bool
	}{
		{"glob against path", Scenario{ID: "test", Path: "scenarios/test.yaml"}, []string{"scenarios/*.yaml"}, true},
		{"glob against wrong dir", Scenario{ID: "test", Path: "scenarios/test.yaml"}, []string{"tests/*.yaml"}, false},
		{"glob against filename", Scenario{ID: "test", Path: "scenarios/test.yaml"}, []string{"*.yaml"}, true},
		{"exact filename match", Scenario{ID: "test", Path: "scenarios/test.yaml"}, []string{"test.yaml"}, true},
		{"wrong filename", Scenario{ID: "test", Path: "scenarios/test.yaml"}, []string{"other.yaml"}, false},
		{"bare filename glob", Scenario{ID: "test", Path: "test.yaml"}, []string{"*.yaml"}, true},
		{"wrong extension", Scenario{ID: "test", Path: "test.yaml"}, []string{"*.json"}, false},
		// ID matching — the primary use case for scenario filtering
		{"exact ID match", Scenario{ID: "simple-qa", Path: "scenarios/simple-qa.scenario.yaml"}, []string{"simple-qa"}, true},
		{"ID glob match", Scenario{ID: "simple-qa", Path: "scenarios/simple-qa.scenario.yaml"}, []string{"simple-*"}, true},
		{"ID no match", Scenario{ID: "simple-qa", Path: "scenarios/simple-qa.scenario.yaml"}, []string{"deep-*"}, false},
		{
			"ID match among multiple patterns",
			Scenario{ID: "tool-usage", Path: "scenarios/tool-usage.scenario.yaml"},
			[]string{"simple-qa", "tool-usage"}, true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scenarioMatches(tt.scenario, tt.patterns)
			if got != tt.want {
				t.Errorf("scenarioMatches(%q, %v) = %v, want %v",
					tt.scenario.ID, tt.patterns, got, tt.want)
			}
		})
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
