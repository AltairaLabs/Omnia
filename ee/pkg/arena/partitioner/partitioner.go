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
}

// PartitionResult contains the result of partitioning.
type PartitionResult struct {
	// Items is the list of generated work items.
	Items []queue.WorkItem

	// TotalCombinations is the total number of scenario × provider combinations.
	TotalCombinations int

	// ScenarioCount is the number of scenarios.
	ScenarioCount int

	// ProviderCount is the number of providers.
	ProviderCount int
}

// Partition creates work items for each scenario × provider combination.
// Each work item represents a single evaluation that can be independently executed.
func Partition(input PartitionInput) (*PartitionResult, error) {
	if len(input.Scenarios) == 0 {
		return nil, fmt.Errorf("no scenarios provided")
	}
	if len(input.Providers) == 0 {
		return nil, fmt.Errorf("no providers provided")
	}

	totalCombinations := len(input.Scenarios) * len(input.Providers)
	items := make([]queue.WorkItem, 0, totalCombinations)

	// Create work item for each scenario × provider combination
	for _, scenario := range input.Scenarios {
		for _, provider := range input.Providers {
			config, err := buildConfig(input.Config, scenario, provider)
			if err != nil {
				return nil, fmt.Errorf("failed to build config for %s/%s: %w",
					scenario.ID, provider.ID, err)
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

	return &PartitionResult{
		Items:             items,
		TotalCombinations: totalCombinations,
		ScenarioCount:     len(input.Scenarios),
		ProviderCount:     len(input.Providers),
	}, nil
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
		// Apply include patterns
		for _, scenario := range scenarios {
			if matchesAny(scenario.Path, include) {
				result = append(result, scenario)
			}
		}
	}

	// Apply exclude patterns
	if len(exclude) > 0 {
		filtered := make([]Scenario, 0, len(result))
		for _, scenario := range result {
			if !matchesAny(scenario.Path, exclude) {
				filtered = append(filtered, scenario)
			}
		}
		result = filtered
	}

	return result, nil
}

// matchesAny returns true if the path matches any of the glob patterns.
func matchesAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, path)
		if err == nil && matched {
			return true
		}
		// Also try matching against just the filename
		matched, err = filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
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

// buildConfig creates the config JSON for a work item.
func buildConfig(base map[string]any, scenario Scenario, provider Provider) ([]byte, error) {
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

	return json.Marshal(config)
}

// EstimateWorkItems returns the estimated number of work items without creating them.
// Useful for progress tracking and resource planning.
func EstimateWorkItems(scenarioCount, providerCount int) int {
	return scenarioCount * providerCount
}

// Batch splits work items into batches of the specified size.
// Useful for rate limiting or chunked processing.
func Batch(items []queue.WorkItem, batchSize int) [][]queue.WorkItem {
	if batchSize <= 0 {
		return [][]queue.WorkItem{items}
	}

	var batches [][]queue.WorkItem
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}
		batches = append(batches, items[i:end])
	}
	return batches
}
