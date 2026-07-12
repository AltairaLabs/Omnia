/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	arenastatestore "github.com/AltairaLabs/promptarena/arena/statestore"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

// getContentPath validates and returns the mounted content path.
func getContentPath(cfg *Config) (string, error) {
	// Validate that the content path exists and is accessible
	info, err := os.Stat(cfg.ContentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("content path does not exist: %s", cfg.ContentPath)
		}
		return "", fmt.Errorf("failed to access content path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("content path is not a directory: %s", cfg.ContentPath)
	}
	return cfg.ContentPath, nil
}

// findArenaConfigFile looks for the arena config file in the bundle directory.
// If configFile is provided, it uses that specific file.
// Otherwise, it checks for common naming conventions: config.arena.yaml, arena.yaml, config.yaml.
func findArenaConfigFile(bundlePath, configFile string) string {
	// If a specific config file is provided, use it
	if configFile != "" {
		path := filepath.Join(bundlePath, configFile)
		if _, err := os.Stat(path); err == nil {
			return path
		}
		// Config file was specified but not found - fall through to search
	}

	// Search for common arena config file names
	candidates := []string{
		"config.arena.yaml",
		"config.arena.yml",
		"arena.yaml",
		"arena.yml",
		"config.yaml",
		"config.yml",
	}

	for _, name := range candidates {
		path := filepath.Join(bundlePath, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// processRun processes a single run's state and updates aggregated counts.
func (a *runAggregator) processRun(runID string, state *arenastatestore.ArenaConversationState) {
	if state.RunMetadata == nil {
		a.log.V(1).Info("run has no metadata", "runID", runID)
		return
	}
	meta := state.RunMetadata
	a.totalDuration += meta.Duration

	// Accumulate token counts from message CostInfo.
	for _, msg := range state.Messages {
		if msg.CostInfo != nil {
			a.inputTokens += msg.CostInfo.InputTokens
			a.outputTokens += msg.CostInfo.OutputTokens
		}
	}

	if meta.Error != "" {
		a.errors = append(a.errors, fmt.Sprintf("run %s: %s", runID, meta.Error))
		a.failCount++
		a.log.V(1).Info("run failed", "runID", runID, "error", meta.Error)
	} else {
		a.passCount++
		a.log.V(1).Info("run passed", "runID", runID, "duration", meta.Duration)
	}

	a.processAssertions(runID, meta.ConversationAssertionResults)
}

// processAssertions extracts assertion results and adjusts pass/fail counts.
func (a *runAggregator) processAssertions(runID string, assertions []arenastatestore.ConversationValidationResult) {
	for _, assertion := range assertions {
		a.assertions = append(a.assertions, AssertionResult{
			Name:    assertion.Type,
			Passed:  assertion.Passed,
			Message: assertion.Message,
		})
		a.log.V(1).Info("assertion result",
			"runID", runID,
			"type", assertion.Type,
			"passed", assertion.Passed,
			"message", assertion.Message,
		)
		if !assertion.Passed {
			errMsg := fmt.Sprintf("run %s: assertion [%s] failed: %s",
				runID, assertion.Type, assertion.Message)
			a.errors = append(a.errors, errMsg)
			if a.passCount > 0 {
				a.passCount--
				a.failCount++
			}
		}
	}
}

// buildExecutionResult constructs an ExecutionResult from the engine's state store.
// If pricing is non-nil, cost is computed from token counts and written to metrics.
func buildExecutionResult(
	log logr.Logger, store statestore.Store, runIDs []string, startTime time.Time,
	pricing *providerPricing,
) *ExecutionResult {
	result := &ExecutionResult{
		DurationMs: float64(time.Since(startTime).Milliseconds()),
		Metrics:    make(map[string]float64),
		Assertions: []AssertionResult{},
	}

	arenaStore, ok := store.(*arenastatestore.ArenaStateStore)
	if !ok {
		return buildFallbackResult(result, runIDs)
	}

	agg := &runAggregator{log: log}
	for _, runID := range runIDs {
		state, err := arenaStore.GetArenaState(context.Background(), runID)
		if err != nil {
			log.Error(err, "failed to get run state", "runID", runID)
			agg.failCount++
			continue
		}
		agg.processRun(runID, state)
	}

	result.Assertions = agg.assertions
	populateMetrics(result, agg, len(runIDs), pricing)
	setResultStatus(result, agg)

	return result
}

// buildFallbackResult creates a simple result when arena store is unavailable.
func buildFallbackResult(result *ExecutionResult, runIDs []string) *ExecutionResult {
	result.Status = statusFail
	if len(runIDs) == 0 {
		result.Error = "no runs executed"
	} else {
		result.Error = fmt.Sprintf("unable to read run state for %d run(s) — results unknown", len(runIDs))
	}
	return result
}

// populateMetrics sets the metrics on the result from aggregated data.
// If pricing is non-nil, it also computes and writes the total cost.
func populateMetrics(result *ExecutionResult, agg *runAggregator, totalRuns int, pricing *providerPricing) {
	result.Metrics["totalDurationMs"] = float64(agg.totalDuration.Milliseconds())
	result.Metrics["runsExecuted"] = float64(totalRuns)
	result.Metrics["runsPassed"] = float64(agg.passCount)
	result.Metrics["runsFailed"] = float64(agg.failCount)

	if agg.inputTokens > 0 {
		result.Metrics[metricKeyInputTokens] = float64(agg.inputTokens)
	}
	if agg.outputTokens > 0 {
		result.Metrics[metricKeyOutputTokens] = float64(agg.outputTokens)
	}

	if pricing != nil && (agg.inputTokens > 0 || agg.outputTokens > 0) {
		cost := pricing.computeCost(agg.inputTokens, agg.outputTokens)
		if cost > 0 {
			result.Metrics[metricKeyCost] = cost
		}
	}
}

// setResultStatus determines the overall status based on aggregated counts.
func setResultStatus(result *ExecutionResult, agg *runAggregator) {
	if agg.failCount > 0 {
		result.Status = statusFail
		if len(agg.errors) > 0 {
			result.Error = strings.Join(agg.errors, "; ")
		}
	} else if agg.passCount > 0 {
		result.Status = statusPass
	} else {
		result.Status = statusFail
		result.Error = "no runs completed successfully"
	}
}

// computeCost calculates the total cost from token counts and pricing.
func (p *providerPricing) computeCost(inputTokens, outputTokens int) float64 {
	return float64(inputTokens)*p.inputCostPer1K/1000 + float64(outputTokens)*p.outputCostPer1K/1000
}

// parsePricing extracts pricing from a Provider CRD's spec.pricing field.
// Returns nil if pricing is not configured or has no valid values.
func parsePricing(pricing *v1alpha1.ProviderPricing) *providerPricing {
	if pricing == nil {
		return nil
	}

	p := &providerPricing{}
	if pricing.InputCostPer1K != nil {
		if v, err := strconv.ParseFloat(*pricing.InputCostPer1K, 64); err == nil {
			p.inputCostPer1K = v
		}
	}
	if pricing.OutputCostPer1K != nil {
		if v, err := strconv.ParseFloat(*pricing.OutputCostPer1K, 64); err == nil {
			p.outputCostPer1K = v
		}
	}

	if p.inputCostPer1K == 0 && p.outputCostPer1K == 0 {
		return nil
	}
	return p
}

// recordDetailedMetrics emits per-trial Prometheus metrics from an execution result.
func recordDetailedMetrics(
	wm *WorkerMetrics, jobID string, item *queue.WorkItem,
	result *ExecutionResult, execErr error, durationSec float64,
) {
	scenario := item.ScenarioID
	provider := item.ProviderID

	// Record turn latency (always available).
	wm.RecordTurnLatency(jobID, scenario, provider, durationSec)

	// Record trial outcome.
	status := statusPass
	if execErr != nil || (result != nil && result.Status == statusFail) {
		status = statusFail
	}
	wm.RecordTrial(jobID, scenario, provider, status)

	// Record error if applicable.
	if execErr != nil {
		wm.RecordError(jobID, provider, "execution")
	} else if result != nil && result.Status == statusFail {
		wm.RecordError(jobID, provider, "assertion")
	}

	if result == nil || result.Metrics == nil {
		return
	}

	// Record TTFT if present.
	if ttft, ok := result.Metrics[metricKeyTTFT]; ok && ttft > 0 {
		wm.RecordTTFT(jobID, scenario, provider, ttft)
	}

	// Record tokens.
	if inputTokens, ok := result.Metrics[metricKeyInputTokens]; ok {
		wm.RecordTokens(jobID, provider, "input", inputTokens)
	}
	if outputTokens, ok := result.Metrics[metricKeyOutputTokens]; ok {
		wm.RecordTokens(jobID, provider, "output", outputTokens)
	}
}

// applyToolOverrides modifies LoadedTools in the config to apply overrides from ToolRegistry CRDs.
// For each tool that has an override, it changes the mode to "http" and sets the endpoint URL.
func applyToolOverrides(log logr.Logger, cfg *arenaconfig.Config, toolOverrides map[string]ToolOverrideConfig) error {
	if len(toolOverrides) == 0 {
		return nil
	}

	appliedCount := 0
	for i := range cfg.LoadedTools {
		applied, err := applyToolOverride(log, cfg, i, toolOverrides)
		if err != nil {
			return err
		}
		if applied {
			appliedCount++
		}
	}

	if appliedCount > 0 {
		log.Info("tool overrides applied", "count", appliedCount)
	}

	return nil
}

// applyToolOverride applies a matching ToolRegistry override to the tool at index i
// in cfg.LoadedTools, rewriting it to an HTTP executor. Returns whether an override
// was applied. Tools that don't parse or have no override are left untouched.
func applyToolOverride(
	log logr.Logger, cfg *arenaconfig.Config, i int, toolOverrides map[string]ToolOverrideConfig,
) (bool, error) {
	// Parse the tool YAML
	var wrapper toolConfigWrapper
	if err := yaml.Unmarshal(cfg.LoadedTools[i].Data, &wrapper); err != nil {
		// Skip tools that can't be parsed - they'll fail later in validation
		return false, nil
	}

	// Get tool name (prefer spec.name, fall back to metadata.name)
	toolName := wrapper.Spec.Name
	if toolName == "" {
		toolName = wrapper.Metadata.Name
	}

	// Check if there's an override for this tool
	override, hasOverride := toolOverrides[toolName]
	if !hasOverride {
		return false, nil
	}

	// Apply the override - change mode to http and set endpoint
	wrapper.Spec.Mode = "http"
	if wrapper.Spec.HTTP == nil {
		wrapper.Spec.HTTP = &toolHTTPConfig{}
	}
	wrapper.Spec.HTTP.URL = override.Endpoint
	if wrapper.Spec.HTTP.Method == "" {
		wrapper.Spec.HTTP.Method = "POST"
	}

	// Update description if provided in override
	if override.Description != "" {
		wrapper.Spec.Description = override.Description
	}

	// Serialize back to YAML
	newData, err := yaml.Marshal(&wrapper)
	if err != nil {
		return false, fmt.Errorf("failed to serialize tool %s after override: %w", toolName, err)
	}

	// Update the loaded tool data
	cfg.LoadedTools[i].Data = newData

	log.V(1).Info("tool override applied",
		"tool", toolName,
		"endpoint", override.Endpoint,
		"registry", override.RegistryName,
		"handler", override.HandlerName,
	)
	return true, nil
}
