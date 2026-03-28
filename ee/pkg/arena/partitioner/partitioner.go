/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package partitioner provides functionality for partitioning Arena scenarios
// and providers into discrete work items for distribution to workers.
package partitioner

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

// Scenario represents a single scenario from a PromptKit bundle.
type Scenario struct {
	// ID is the unique identifier for this scenario.
	ID string `json:"id"`

	// Name is the human-readable name of the scenario.
	Name string `json:"name"`

	// Path is the relative path to the scenario file in the bundle.
	Path string `json:"path"`

	// Description is an optional description of the scenario.
	Description string `json:"description,omitempty"`

	// Tags are optional labels for categorization.
	Tags []string `json:"tags,omitempty"`

	// Trials is the number of times to execute this scenario for statistical evaluation.
	// When > 1, each scenario × provider combination produces N work items.
	Trials int `json:"trials,omitempty"`
}

// Provider represents a provider configuration for work items.
type Provider struct {
	// ID is the unique identifier for this provider (namespace/name).
	ID string `json:"id"`

	// Name is the provider resource name.
	Name string `json:"name"`

	// Namespace is the provider resource namespace.
	Namespace string `json:"namespace"`
}

// PartitionInput contains the input for partitioning work items.
type PartitionInput struct {
	// JobID is the unique identifier for the ArenaJob.
	JobID string

	// BundleURL is the URL to fetch the PromptKit bundle.
	BundleURL string

	// Scenarios is the list of scenarios to partition.
	Scenarios []Scenario

	// Providers is the list of providers to use.
	Providers []Provider

	// MaxRetries is the maximum retry attempts per work item.
	MaxRetries int

	// Config is optional configuration to include in work items.
	Config map[string]any

	// JobTrials is a job-level trial count override.
	// If > 0, overrides per-scenario Trials for all scenarios.
	// If 0, per-scenario Trials is used (defaulting to 1 if unset).
	JobTrials int
}

// PartitionResult contains the result of partitioning.
type PartitionResult struct {
	// Items is the list of generated work items.
	Items []queue.WorkItem

	// TotalCombinations is the total number of scenario × provider × trial combinations.
	TotalCombinations int

	// ScenarioCount is the number of scenarios.
	ScenarioCount int

	// ProviderCount is the number of providers.
	ProviderCount int

	// TrialCount is the total number of trials across all scenarios.
	TrialCount int
}

// Partition creates work items for each scenario × provider × trial combination.
// Each work item represents a single evaluation that can be independently executed.
func Partition(input PartitionInput) (*PartitionResult, error) {
	if len(input.Scenarios) == 0 {
		return nil, fmt.Errorf("no scenarios provided")
	}
	if len(input.Providers) == 0 {
		return nil, fmt.Errorf("no providers provided")
	}

	totalTrials := 0
	items := make([]queue.WorkItem, 0, len(input.Scenarios)*len(input.Providers)*max(input.JobTrials, 1))

	for _, scenario := range input.Scenarios {
		trialCount := resolveTrialCount(input.JobTrials, scenario.Trials)
		totalTrials += trialCount

		for _, provider := range input.Providers {
			for trial := 0; trial < trialCount; trial++ {
				config, err := buildTrialConfig(input.Config, scenario, provider, trial, trialCount)
				if err != nil {
					return nil, fmt.Errorf("failed to build config for %s/%s trial %d: %w",
						scenario.ID, provider.ID, trial, err)
				}

				item := queue.WorkItem{
					ID:          generateItemID(input.JobID, scenario.ID, provider.ID),
					JobID:       input.JobID,
					ScenarioID:  scenario.ID,
					ProviderID:  provider.ID,
					BundleURL:   input.BundleURL,
					Config:      config,
					MaxAttempts: input.MaxRetries,
				}
				items = append(items, item)
			}
		}
	}

	return &PartitionResult{
		Items:             items,
		TotalCombinations: len(items),
		ScenarioCount:     len(input.Scenarios),
		ProviderCount:     len(input.Providers),
		TrialCount:        totalTrials,
	}, nil
}

// resolveTrialCount returns the effective trial count using the priority:
// jobTrials (if > 0) > scenarioTrials (if > 0) > 1.
func resolveTrialCount(jobTrials, scenarioTrials int) int {
	if jobTrials > 0 {
		return jobTrials
	}
	if scenarioTrials > 0 {
		return scenarioTrials
	}
	return 1
}

// Filter applies include/exclude patterns to a list of scenarios.
// Include patterns are applied first (if empty, all scenarios are included).
// Exclude patterns are applied second to filter out unwanted scenarios.
func Filter(scenarios []Scenario, include, exclude []string) ([]Scenario, error) {
	var result []Scenario

	// If no include patterns, include all
	if len(include) == 0 {
		result = make([]Scenario, len(scenarios))
		copy(result, scenarios)
	} else {
		// Apply include patterns — match against path, filename, or scenario ID
		for _, scenario := range scenarios {
			if scenarioMatches(scenario, include) {
				result = append(result, scenario)
			}
		}
	}

	// Apply exclude patterns
	if len(exclude) > 0 {
		filtered := make([]Scenario, 0, len(result))
		for _, scenario := range result {
			if !scenarioMatches(scenario, exclude) {
				filtered = append(filtered, scenario)
			}
		}
		result = filtered
	}

	return result, nil
}

// scenarioMatches returns true if the scenario matches any of the patterns.
// Patterns are matched against the scenario ID, full path, and filename.
func scenarioMatches(scenario Scenario, patterns []string) bool {
	for _, pattern := range patterns {
		// Exact ID match (most common: "simple-qa")
		if pattern == scenario.ID {
			return true
		}
		// Glob against ID
		if matched, err := filepath.Match(pattern, scenario.ID); err == nil && matched {
			return true
		}
		// Glob against full path (e.g., "scenarios/simple-qa.scenario.yaml")
		if matched, err := filepath.Match(pattern, scenario.Path); err == nil && matched {
			return true
		}
		// Glob against filename only
		if matched, err := filepath.Match(pattern, filepath.Base(scenario.Path)); err == nil && matched {
			return true
		}
	}
	return false
}

// generateItemID creates a unique ID for a work item.
func generateItemID(_, scenarioID, _ string) string {
	// Use UUID for uniqueness, with scenario prefix for debugging
	return fmt.Sprintf("%s-%s", scenarioID[:min(8, len(scenarioID))], uuid.New().String()[:8])
}

// buildTrialConfig creates the config JSON for a work item including trial metadata.
func buildTrialConfig(
	base map[string]any, scenario Scenario, provider Provider,
	trialIndex, totalTrials int,
) ([]byte, error) {
	config := make(map[string]any)

	// Copy base config
	for k, v := range base {
		config[k] = v
	}

	// Add scenario info
	config["scenario"] = map[string]any{
		"id":          scenario.ID,
		"name":        scenario.Name,
		"path":        scenario.Path,
		"description": scenario.Description,
		"tags":        scenario.Tags,
	}

	// Add provider info
	config["provider"] = map[string]any{
		"id":        provider.ID,
		"name":      provider.Name,
		"namespace": provider.Namespace,
	}

	// Add trial metadata
	config["trialIndex"] = trialIndex
	config["totalTrials"] = totalTrials

	return json.Marshal(config)
}
