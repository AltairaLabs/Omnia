/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/logger"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/AltairaLabs/PromptKit/tools/arena/engine"
	arenastatestore "github.com/AltairaLabs/PromptKit/tools/arena/statestore"
	"github.com/altairalabs/omnia/ee/pkg/arena/binding"
	"github.com/altairalabs/omnia/ee/pkg/arena/overrides"
	"github.com/altairalabs/omnia/ee/pkg/arena/providers"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"gopkg.in/yaml.v3"
)

// Status constants for execution results.
const (
	statusPass = "pass"
	statusFail = "fail"
)

// Config holds the worker configuration from environment variables.
type Config struct {
	// Job identification
	JobName      string
	JobNamespace string
	ConfigName   string
	JobType      string

	// Filesystem content configuration
	// ContentPath is the mount point for the job's content (e.g., /workspace-content)
	// The content is isolated via subPath to only show the job's root folder
	ContentPath    string
	ContentVersion string // Content-addressable version hash
	ConfigFile     string // Arena config filename within the content path

	// Override configuration from mounted ConfigMap
	// OverridesPath is the path to the mounted overrides.json file
	OverridesPath string

	// Redis configuration
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Worker configuration
	WorkDir       string
	PollInterval  time.Duration
	ShutdownDelay time.Duration
	Verbose       bool // Enable verbose/debug output from promptarena

	// Override configurations (resolved from CRDs by controller)
	// Deprecated: Use OverridesPath instead
	ToolOverrides map[string]ToolOverrideConfig // Tool name -> override config
}

// ToolOverrideConfig contains the configuration for a tool override from ToolRegistry CRD.
type ToolOverrideConfig struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	HandlerName  string `json:"handlerName"`
	RegistryName string `json:"registryName"`
	HandlerType  string `json:"handlerType,omitempty"`
}

// ExecutionResult represents the result of running a scenario.
type ExecutionResult struct {
	Status     string             `json:"status"`
	DurationMs float64            `json:"durationMs"`
	Error      string             `json:"error,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Assertions []AssertionResult  `json:"assertions,omitempty"`
}

// AssertionResult represents a single assertion result.
type AssertionResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

func loadConfig() (*Config, error) {
	cfg := &Config{
		JobName:        os.Getenv("ARENA_JOB_NAME"),
		JobNamespace:   os.Getenv("ARENA_JOB_NAMESPACE"),
		ConfigName:     os.Getenv("ARENA_CONFIG_NAME"),
		JobType:        os.Getenv("ARENA_JOB_TYPE"),
		ContentPath:    os.Getenv("ARENA_CONTENT_PATH"),
		ContentVersion: os.Getenv("ARENA_CONTENT_VERSION"),
		ConfigFile:     os.Getenv("ARENA_CONFIG_FILE"), // Config file name in content path
		OverridesPath:  os.Getenv("ARENA_OVERRIDES_PATH"),
		RedisAddr:      getEnvOrDefault("REDIS_ADDR", "redis:6379"),
		RedisPassword:  os.Getenv("REDIS_PASSWORD"),
		RedisDB:        0,
		WorkDir:        getEnvOrDefault("ARENA_WORK_DIR", "/tmp/arena"),
		PollInterval:   getDurationEnv("ARENA_POLL_INTERVAL", 100*time.Millisecond),
		ShutdownDelay:  getDurationEnv("ARENA_SHUTDOWN_DELAY", 5*time.Second),
		Verbose:        os.Getenv("ARENA_VERBOSE") == "true",
	}

	if cfg.JobName == "" {
		return nil, errors.New("ARENA_JOB_NAME is required")
	}
	if cfg.ContentPath == "" {
		return nil, errors.New("ARENA_CONTENT_PATH is required")
	}

	// Parse tool overrides if provided (legacy env var, prefer OverridesPath)
	if toolOverridesJSON := os.Getenv("ARENA_TOOL_OVERRIDES"); toolOverridesJSON != "" {
		var toolOverrides map[string]ToolOverrideConfig
		if err := json.Unmarshal([]byte(toolOverridesJSON), &toolOverrides); err != nil {
			return nil, fmt.Errorf("failed to parse ARENA_TOOL_OVERRIDES: %w", err)
		}
		cfg.ToolOverrides = toolOverrides
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}

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

func processWorkItems(ctx context.Context, cfg *Config, q queue.WorkQueue, bundlePath string) error {
	jobID := cfg.JobName
	emptyCount := 0
	maxEmptyPolls := 10 // Exit after this many consecutive empty polls

	fmt.Printf("Processing work items for job: %s\n", jobID)

	for {
		if checkContextDone(ctx) {
			return nil
		}

		// Pop next work item
		item, err := q.Pop(ctx, jobID)
		if err != nil {
			done, resetCount, retErr := handlePopError(ctx, err, emptyCount, maxEmptyPolls, cfg, q, jobID)
			if retErr != nil {
				return retErr
			}
			if done {
				return nil
			}
			emptyCount = resetCount
			continue
		}

		emptyCount = 0 // Reset on successful pop

		// Execute and report result
		result, execErr := executeWorkItem(ctx, cfg, item, bundlePath)
		reportWorkItemResult(ctx, q, jobID, item, result, execErr)
	}
}

// checkContextDone returns true if the context is cancelled.
func checkContextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		fmt.Println("Shutdown signal received, exiting...")
		return true
	default:
		return false
	}
}

// handlePopError handles errors from queue.Pop and returns (done, newEmptyCount, error).
//
//nolint:unparam // maxEmptyPolls kept as parameter for testability
func handlePopError(
	ctx context.Context, err error, emptyCount, maxEmptyPolls int, cfg *Config, q queue.WorkQueue, jobID string,
) (bool, int, error) {
	if !errors.Is(err, queue.ErrQueueEmpty) {
		return false, emptyCount, fmt.Errorf("failed to pop work item: %w", err)
	}

	emptyCount++
	if emptyCount >= maxEmptyPolls {
		done, err := checkJobCompletion(ctx, q, jobID, emptyCount)
		if err != nil {
			return false, 0, err
		}
		if done {
			return true, 0, nil
		}
		emptyCount = 0 // Reset for retry
	}

	time.Sleep(cfg.PollInterval)
	return false, emptyCount, nil
}

// checkJobCompletion checks if the job is complete and returns (done, error).
func checkJobCompletion(ctx context.Context, q queue.WorkQueue, jobID string, emptyCount int) (bool, error) {
	fmt.Printf("Queue empty after %d polls, checking if job is complete...\n", emptyCount)

	progress, err := q.Progress(ctx, jobID)
	if err != nil {
		return false, fmt.Errorf("failed to get job progress: %w", err)
	}

	if progress.IsComplete() {
		fmt.Printf("Job complete: %d/%d items processed\n",
			progress.Completed+progress.Failed, progress.Total)
		return true, nil
	}

	return false, nil
}

// reportWorkItemResult reports the result of a work item execution.
func reportWorkItemResult(
	ctx context.Context, q queue.WorkQueue, jobID string, item *queue.WorkItem, result *ExecutionResult, execErr error,
) {
	if execErr != nil {
		fmt.Printf("  [FAIL] %s: %v\n", item.ID, execErr)
		if err := q.Nack(ctx, jobID, item.ID, execErr); err != nil {
			fmt.Printf("  Warning: failed to nack item %s: %v\n", item.ID, err)
		}
		return
	}

	resultJSON, _ := json.Marshal(result)
	fmt.Printf("  [%s] %s (%.0fms)\n", result.Status, item.ID, result.DurationMs)
	if err := q.Ack(ctx, jobID, item.ID, resultJSON); err != nil {
		fmt.Printf("  Warning: failed to ack item %s: %v\n", item.ID, err)
	}
}

func executeWorkItem(
	ctx context.Context,
	cfg *Config,
	item *queue.WorkItem,
	bundlePath string,
) (*ExecutionResult, error) {
	start := time.Now()

	result := &ExecutionResult{
		Metrics: make(map[string]float64),
	}

	// Find the arena config file
	configPath := findArenaConfigFile(bundlePath, cfg.ConfigFile)
	if configPath == "" {
		return nil, fmt.Errorf("arena config file not found in bundle: %s", bundlePath)
	}

	// Configure verbose logging BEFORE creating the engine
	if cfg.Verbose {
		fmt.Printf("  Loading arena config from: %s\n", configPath)
		logger.SetVerbose(true)
		logger.SetOutput(os.Stderr) // Ensure logs go to stderr for kubectl logs
	}

	// Load configuration from file BEFORE creating engine so we can modify it
	arenaCfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Configure settings BEFORE creating engine (media storage is created during engine init)
	if cfg.Verbose {
		arenaCfg.Defaults.Verbose = true
	}

	// Set output directory to a writable location
	// The workspace content is mounted read-only, so we need a writable path for media files
	arenaCfg.Defaults.Output.Dir = "/tmp/arena-output"

	// Load and apply overrides from ConfigMap (new method - takes precedence)
	overrideCfg, err := loadOverrides(cfg.OverridesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load overrides: %w", err)
	}
	if overrideCfg != nil {
		if err := applyOverridesFromConfig(arenaCfg, overrideCfg, cfg.Verbose); err != nil {
			return nil, fmt.Errorf("failed to apply overrides from ConfigMap: %w", err)
		}
	} else if len(cfg.ToolOverrides) > 0 {
		// Fall back to legacy tool overrides from env var (for backwards compatibility)
		if err := applyToolOverrides(arenaCfg, cfg.ToolOverrides, cfg.Verbose); err != nil {
			return nil, fmt.Errorf("failed to apply tool overrides: %w", err)
		}
	}

	// Apply provider bindings (annotation-based credential resolution)
	// This runs after explicit overrides so they take precedence
	if overrideCfg != nil && len(overrideCfg.Bindings) > 0 {
		if err := applyProviderBindings(arenaCfg, overrideCfg.Bindings, configPath, cfg.Verbose); err != nil {
			// Non-fatal: log warning and continue
			fmt.Fprintf(os.Stderr, "Warning: failed to apply provider bindings: %v\n", err)
		}
	}

	// Build registries and executors from the config
	providerRegistry, promptRegistry, mcpRegistry, convExecutor, adapterRegistry, err :=
		engine.BuildEngineComponents(arenaCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build engine components: %w", err)
	}

	// Create engine with all components
	eng, err := engine.NewEngine(arenaCfg, providerRegistry, promptRegistry, mcpRegistry, convExecutor, adapterRegistry)
	if err != nil {
		return nil, fmt.Errorf("failed to create engine: %w", err)
	}
	defer func() {
		if err := eng.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close engine: %v\n", err)
		}
	}()

	// Validate provider credentials before execution
	if err := providers.ValidateProviderCredentials(arenaCfg, item.ProviderID); err != nil {
		result.Status = statusFail
		result.Error = err.Error()
		result.DurationMs = float64(time.Since(start).Milliseconds())
		return result, nil
	}

	// Determine scenario filter
	scenarioFilter := []string{}
	if item.ScenarioID != "" && item.ScenarioID != "default" {
		scenarioFilter = []string{item.ScenarioID}
	}

	// Determine provider filter - empty means use all providers from arena config
	providerFilter := []string{}
	if item.ProviderID != "" {
		providerFilter = []string{item.ProviderID}
	}

	// Generate run plan for this specific provider (or all if no override)
	plan, err := eng.GenerateRunPlan(
		[]string{},     // no region filter
		providerFilter, // filter to this provider (empty = all from config)
		scenarioFilter, // scenario filter (empty = all)
		[]string{},     // no eval filter
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate run plan: %w", err)
	}

	if len(plan.Combinations) == 0 {
		result.Status = statusPass
		result.DurationMs = float64(time.Since(start).Milliseconds())
		return result, nil
	}

	if cfg.Verbose {
		providerDesc := "all providers"
		if item.ProviderID != "" {
			providerDesc = "provider " + item.ProviderID
		}
		fmt.Printf("  Executing %d scenario(s) with %s\n",
			len(plan.Combinations), providerDesc)
	}

	// Execute runs with concurrency of 1 (single work item at a time)
	runIDs, err := eng.ExecuteRuns(ctx, plan, 1)
	if err != nil {
		return nil, fmt.Errorf("execution failed: %w", err)
	}

	// Build result from state store
	result = buildExecutionResult(eng.GetStateStore(), runIDs, start, cfg.Verbose)
	return result, nil
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

// runAggregator collects and aggregates run results.
type runAggregator struct {
	passCount     int
	failCount     int
	errors        []string
	totalDuration time.Duration
	assertions    []AssertionResult
	verbose       bool
}

// processRun processes a single run's state and updates aggregated counts.
func (a *runAggregator) processRun(runID string, state *arenastatestore.ArenaConversationState) {
	if state.RunMetadata == nil {
		if a.verbose {
			fmt.Printf("    Run %s: no metadata available\n", runID)
		}
		return
	}
	meta := state.RunMetadata
	a.totalDuration += meta.Duration

	if meta.Error != "" {
		a.errors = append(a.errors, fmt.Sprintf("run %s: %s", runID, meta.Error))
		a.failCount++
		if a.verbose {
			fmt.Printf("    Run %s FAILED: %s\n", runID, meta.Error)
		}
	} else {
		a.passCount++
		if a.verbose {
			fmt.Printf("    Run %s PASSED (duration: %v)\n", runID, meta.Duration)
		}
	}

	a.processAssertions(meta.ConversationAssertionResults)
}

// processAssertions extracts assertion results and adjusts pass/fail counts.
func (a *runAggregator) processAssertions(assertions []arenastatestore.ConversationValidationResult) {
	for _, assertion := range assertions {
		a.assertions = append(a.assertions, AssertionResult{
			Name:    assertion.Type,
			Passed:  assertion.Passed,
			Message: assertion.Message,
		})
		if a.verbose {
			status := "PASS"
			if !assertion.Passed {
				status = "FAIL"
			}
			fmt.Printf("      Assertion [%s] %s: %s\n", assertion.Type, status, assertion.Message)
		}
		if !assertion.Passed && a.passCount > 0 {
			a.passCount--
			a.failCount++
		}
	}
}

// buildExecutionResult constructs an ExecutionResult from the engine's state store.
func buildExecutionResult(store statestore.Store, runIDs []string, startTime time.Time, verbose bool) *ExecutionResult {
	result := &ExecutionResult{
		DurationMs: float64(time.Since(startTime).Milliseconds()),
		Metrics:    make(map[string]float64),
		Assertions: []AssertionResult{},
	}

	arenaStore, ok := store.(*arenastatestore.ArenaStateStore)
	if !ok {
		return buildFallbackResult(result, runIDs)
	}

	agg := &runAggregator{verbose: verbose}
	for _, runID := range runIDs {
		state, err := arenaStore.GetArenaState(context.Background(), runID)
		if err != nil {
			if verbose {
				fmt.Printf("  Warning: failed to get state for run %s: %v\n", runID, err)
			}
			agg.failCount++
			continue
		}
		agg.processRun(runID, state)
	}

	result.Assertions = agg.assertions
	populateMetrics(result, agg, len(runIDs))
	setResultStatus(result, agg)

	return result
}

// buildFallbackResult creates a simple result when arena store is unavailable.
func buildFallbackResult(result *ExecutionResult, runIDs []string) *ExecutionResult {
	if len(runIDs) > 0 {
		result.Status = statusPass
	} else {
		result.Status = statusFail
		result.Error = "no runs executed"
	}
	return result
}

// populateMetrics sets the metrics on the result from aggregated data.
func populateMetrics(result *ExecutionResult, agg *runAggregator, totalRuns int) {
	result.Metrics["totalDurationMs"] = float64(agg.totalDuration.Milliseconds())
	result.Metrics["runsExecuted"] = float64(totalRuns)
	result.Metrics["runsPassed"] = float64(agg.passCount)
	result.Metrics["runsFailed"] = float64(agg.failCount)
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

// toolConfigWrapper wraps tool configuration for YAML parsing/serialization.
type toolConfigWrapper struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   toolMetadata   `yaml:"metadata"`
	Spec       toolSpecConfig `yaml:"spec"`
}

type toolMetadata struct {
	Name string `yaml:"name"`
}

type toolSpecConfig struct {
	Name         string                 `yaml:"name,omitempty"`
	Description  string                 `yaml:"description,omitempty"`
	InputSchema  map[string]interface{} `yaml:"input_schema,omitempty"`
	OutputSchema map[string]interface{} `yaml:"output_schema,omitempty"`
	Mode         string                 `yaml:"mode,omitempty"`
	TimeoutMs    int                    `yaml:"timeout_ms,omitempty"`
	MockResult   interface{}            `yaml:"mock_result,omitempty"`
	MockTemplate string                 `yaml:"mock_template,omitempty"`
	HTTP         *toolHTTPConfig        `yaml:"http,omitempty"`
}

type toolHTTPConfig struct {
	URL            string            `yaml:"url"`
	Method         string            `yaml:"method,omitempty"`
	Headers        map[string]string `yaml:"headers,omitempty"`
	HeadersFromEnv []string          `yaml:"headers_from_env,omitempty"`
	TimeoutMs      int               `yaml:"timeout_ms,omitempty"`
}

// applyToolOverrides modifies LoadedTools in the config to apply overrides from ToolRegistry CRDs.
// For each tool that has an override, it changes the mode to "http" and sets the endpoint URL.
func applyToolOverrides(cfg *config.Config, toolOverrides map[string]ToolOverrideConfig, verbose bool) error {
	if len(toolOverrides) == 0 {
		return nil
	}

	appliedCount := 0
	for i, toolData := range cfg.LoadedTools {
		// Parse the tool YAML
		var wrapper toolConfigWrapper
		if err := yaml.Unmarshal(toolData.Data, &wrapper); err != nil {
			// Skip tools that can't be parsed - they'll fail later in validation
			continue
		}

		// Get tool name (prefer spec.name, fall back to metadata.name)
		toolName := wrapper.Spec.Name
		if toolName == "" {
			toolName = wrapper.Metadata.Name
		}

		// Check if there's an override for this tool
		override, hasOverride := toolOverrides[toolName]
		if !hasOverride {
			continue
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
			return fmt.Errorf("failed to serialize tool %s after override: %w", toolName, err)
		}

		// Update the loaded tool data
		cfg.LoadedTools[i].Data = newData
		appliedCount++

		if verbose {
			fmt.Printf("  Applied tool override: %s -> %s (registry: %s, handler: %s)\n",
				toolName, override.Endpoint, override.RegistryName, override.HandlerName)
		}
	}

	if verbose && appliedCount > 0 {
		fmt.Printf("  Applied %d tool override(s)\n", appliedCount)
	}

	return nil
}

// loadOverrides reads the override config from the mounted ConfigMap file.
// Returns nil if the file doesn't exist (no overrides configured).
func loadOverrides(path string) (*overrides.OverrideConfig, error) {
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read overrides file: %w", err)
	}

	var cfg overrides.OverrideConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse overrides: %w", err)
	}

	return &cfg, nil
}

// applyProviderOverrides injects provider configs from CRD overrides into the arena config.
// Providers are added to LoadedProviders and can be used by the engine.
func applyProviderOverrides(
	arenaCfg *config.Config,
	providersByGroup map[string][]overrides.ProviderOverride,
	verbose bool,
) {
	if len(providersByGroup) == 0 {
		return
	}

	if arenaCfg.LoadedProviders == nil {
		arenaCfg.LoadedProviders = make(map[string]*config.Provider)
	}

	appliedCount := 0
	for groupName, groupProviders := range providersByGroup {
		for _, p := range groupProviders {
			// Create a PromptKit-compatible provider config
			provider := &config.Provider{
				ID:      p.ID,
				Type:    p.Type,
				Model:   p.Model,
				BaseURL: p.BaseURL,
				Defaults: config.ProviderDefaults{
					Temperature: float32(p.Temperature),
					TopP:        float32(p.TopP),
					MaxTokens:   p.MaxTokens,
				},
			}

			// Set credential from override config
			if p.SecretEnvVar != "" {
				provider.Credential = &config.CredentialConfig{
					CredentialEnv: p.SecretEnvVar,
				}
			} else if p.CredentialFile != "" {
				provider.Credential = &config.CredentialConfig{
					CredentialFile: p.CredentialFile,
				}
			}

			// Add to LoadedProviders (overwriting any existing provider with same ID)
			arenaCfg.LoadedProviders[p.ID] = provider

			// Track provider group for filtering
			if arenaCfg.ProviderGroups == nil {
				arenaCfg.ProviderGroups = make(map[string]string)
			}
			arenaCfg.ProviderGroups[p.ID] = groupName

			appliedCount++
			if verbose {
				credStatus := "no credentials required"
				if p.SecretEnvVar != "" {
					if os.Getenv(p.SecretEnvVar) != "" {
						credStatus = fmt.Sprintf("✓ %s set", p.SecretEnvVar)
					} else {
						credStatus = fmt.Sprintf("✗ %s MISSING", p.SecretEnvVar)
					}
				} else if p.CredentialFile != "" {
					if _, err := os.Stat(p.CredentialFile); err == nil {
						credStatus = fmt.Sprintf("✓ file %s exists", p.CredentialFile)
					} else {
						credStatus = fmt.Sprintf("✗ file %s MISSING", p.CredentialFile)
					}
				}
				fmt.Printf("  Provider override: %s (%s/%s) group=%s [%s]\n",
					p.ID, p.Type, p.Model, groupName, credStatus)
			}
		}
	}

	if verbose && appliedCount > 0 {
		fmt.Printf("  Applied %d provider override(s)\n", appliedCount)
	}
}

// applyProviderBindings resolves provider binding annotations against the registry
// and injects credentials into providers that don't already have them.
func applyProviderBindings(
	cfg *config.Config,
	registry map[string]overrides.ProviderOverride,
	configPath string,
	verbose bool,
) error {
	bindings, err := binding.ParseProviderAnnotations(cfg, configPath)
	if err != nil {
		return fmt.Errorf("failed to parse provider annotations: %w", err)
	}

	boundCount := binding.ApplyBindings(cfg, bindings, registry, verbose)
	matchedCount := binding.ApplyNameMatching(cfg, registry, verbose)

	if verbose && (boundCount > 0 || matchedCount > 0) {
		fmt.Printf("  Provider bindings: %d annotation-based, %d name-matched\n", boundCount, matchedCount)
	}

	return nil
}

// applyOverridesFromConfig applies all overrides from the loaded override config.
// This handles both provider and tool overrides from the ConfigMap.
func applyOverridesFromConfig(
	arenaCfg *config.Config,
	overrideCfg *overrides.OverrideConfig,
	verbose bool,
) error {
	if overrideCfg == nil {
		return nil
	}

	// Apply provider overrides first
	applyProviderOverrides(arenaCfg, overrideCfg.Providers, verbose)

	// Apply tool overrides
	if len(overrideCfg.Tools) > 0 {
		// Convert to the legacy ToolOverrideConfig format for compatibility
		toolOverrides := make(map[string]ToolOverrideConfig)
		for _, t := range overrideCfg.Tools {
			toolOverrides[t.Name] = ToolOverrideConfig{
				Name:         t.Name,
				Description:  t.Description,
				Endpoint:     t.Endpoint,
				HandlerType:  t.HandlerType,
				RegistryName: t.RegistryName,
				HandlerName:  t.HandlerName,
			}
		}
		if err := applyToolOverrides(arenaCfg, toolOverrides, verbose); err != nil {
			return fmt.Errorf("failed to apply tool overrides: %w", err)
		}
	}

	return nil
}
