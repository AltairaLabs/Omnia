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

package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/logger"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/AltairaLabs/PromptKit/tools/arena/engine"
	arenastatestore "github.com/AltairaLabs/PromptKit/tools/arena/statestore"
	"github.com/altairalabs/omnia/pkg/arena/providers"
	"github.com/altairalabs/omnia/pkg/arena/queue"
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

	// Artifact configuration (legacy tar.gz download)
	ArtifactURL      string
	ArtifactRevision string

	// Filesystem content configuration (new - takes precedence over ArtifactURL)
	// Full path to mounted content (e.g., /workspace-content/ns/default/arena/name/.arena/versions/hash)
	ContentPath    string
	ContentVersion string // Content-addressable version hash

	// Redis configuration
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Worker configuration
	WorkDir        string
	PromptArenaBin string
	PollInterval   time.Duration
	ShutdownDelay  time.Duration
	Verbose        bool // Enable verbose/debug output from promptarena

	// Override configurations (resolved from CRDs by controller)
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
		JobName:          os.Getenv("ARENA_JOB_NAME"),
		JobNamespace:     os.Getenv("ARENA_JOB_NAMESPACE"),
		ConfigName:       os.Getenv("ARENA_CONFIG_NAME"),
		JobType:          os.Getenv("ARENA_JOB_TYPE"),
		ArtifactURL:      os.Getenv("ARENA_ARTIFACT_URL"),
		ArtifactRevision: os.Getenv("ARENA_ARTIFACT_REVISION"),
		ContentPath:      os.Getenv("ARENA_CONTENT_PATH"),    // New: filesystem path
		ContentVersion:   os.Getenv("ARENA_CONTENT_VERSION"), // New: version hash
		RedisAddr:        getEnvOrDefault("REDIS_ADDR", "redis:6379"),
		RedisPassword:    os.Getenv("REDIS_PASSWORD"),
		RedisDB:          0,
		WorkDir:          getEnvOrDefault("ARENA_WORK_DIR", "/tmp/arena"),
		PromptArenaBin:   getEnvOrDefault("PROMPTARENA_BIN", "promptarena"),
		PollInterval:     getDurationEnv("ARENA_POLL_INTERVAL", 100*time.Millisecond),
		ShutdownDelay:    getDurationEnv("ARENA_SHUTDOWN_DELAY", 5*time.Second),
		Verbose:          os.Getenv("ARENA_VERBOSE") == "true",
	}

	if cfg.JobName == "" {
		return nil, errors.New("ARENA_JOB_NAME is required")
	}
	// ContentPath takes precedence; ArtifactURL is only required if ContentPath is not set
	if cfg.ContentPath == "" && cfg.ArtifactURL == "" {
		return nil, errors.New("either ARENA_CONTENT_PATH or ARENA_ARTIFACT_URL is required")
	}

	// Parse tool overrides if provided
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

// getBundlePath returns the path to the content bundle.
// If ContentPath is set (filesystem mode), it uses the mounted path directly.
// Otherwise, it falls back to downloading and extracting the tar.gz artifact.
func getBundlePath(ctx context.Context, cfg *Config) (string, error) {
	// Filesystem mode: use mounted content directly
	if cfg.ContentPath != "" {
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
		// Return the mounted content path directly - no download/extraction needed
		return cfg.ContentPath, nil
	}

	// Legacy mode: download and extract tar.gz artifact
	return downloadAndExtract(ctx, cfg)
}

func downloadAndExtract(ctx context.Context, cfg *Config) (string, error) {
	// Create work directory
	bundleDir := filepath.Join(cfg.WorkDir, "bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create work directory: %w", err)
	}

	// Download artifact
	tarPath := filepath.Join(cfg.WorkDir, "bundle.tar.gz")
	if err := downloadFile(ctx, cfg.ArtifactURL, tarPath); err != nil {
		return "", fmt.Errorf("failed to download artifact: %w", err)
	}

	// Extract tarball
	if err := extractTarGz(tarPath, bundleDir); err != nil {
		return "", fmt.Errorf("failed to extract artifact: %w", err)
	}

	// Clean up tarball (ignore error, non-critical)
	_ = os.Remove(tarPath)

	return bundleDir, nil
}

func downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractTarGz(tarPath, destDir string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Sanitize path to prevent directory traversal
		target := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := extractRegularFile(target, tr, header.Mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Validate and sanitize symlink target to prevent symlink escape attacks
			safeLinkTarget, err := sanitizeSymlinkTarget(destDir, target, header.Linkname)
			if err != nil {
				return fmt.Errorf("invalid symlink in archive: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.Symlink(safeLinkTarget, target); err != nil {
				return err
			}
		}
	}

	return nil
}

func extractRegularFile(target string, tr *tar.Reader, mode int64) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return err
	}
	defer func() { _ = outFile.Close() }()

	_, err = io.Copy(outFile, tr)
	return err
}

// sanitizeSymlinkTarget validates a symlink target from an archive and returns
// a safe relative path. This prevents symlink escape attacks (CWE-22).
func sanitizeSymlinkTarget(destDir, symlinkPath, linkTarget string) (string, error) {
	// Reject absolute symlink targets
	if filepath.IsAbs(linkTarget) {
		return "", fmt.Errorf("absolute symlink target not allowed: %s", linkTarget)
	}

	// Resolve the symlink target relative to the symlink's directory
	linkDir := filepath.Dir(symlinkPath)
	resolvedPath := filepath.Join(linkDir, linkTarget)
	resolvedPath = filepath.Clean(resolvedPath)

	// Verify resolved path stays within destDir
	cleanDestDir := filepath.Clean(destDir)
	if !strings.HasPrefix(resolvedPath, cleanDestDir+string(os.PathSeparator)) &&
		resolvedPath != cleanDestDir {
		return "", fmt.Errorf("symlink target escapes destination: %s", linkTarget)
	}

	// Compute the relative path from the symlink to the target
	// This ensures we're not using the raw archive data
	relPath, err := filepath.Rel(linkDir, resolvedPath)
	if err != nil {
		return "", fmt.Errorf("failed to compute relative path: %w", err)
	}

	return relPath, nil
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
	configPath := findArenaConfigFile(bundlePath)
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

	// Apply tool overrides from ToolRegistry CRDs
	if len(cfg.ToolOverrides) > 0 {
		if err := applyToolOverrides(arenaCfg, cfg.ToolOverrides, cfg.Verbose); err != nil {
			return nil, fmt.Errorf("failed to apply tool overrides: %w", err)
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
	defer func() { _ = eng.Close() }()

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

	// Generate run plan for this specific provider
	plan, err := eng.GenerateRunPlan(
		[]string{},                // no region filter
		[]string{item.ProviderID}, // filter to this provider
		scenarioFilter,            // scenario filter (empty = all)
		[]string{},                // no eval filter
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
		fmt.Printf("  Executing %d scenario(s) with provider %s\n",
			len(plan.Combinations), item.ProviderID)
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
// It checks for common naming conventions: config.arena.yaml, arena.yaml, config.yaml.
func findArenaConfigFile(bundlePath string) string {
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
func applyToolOverrides(cfg *config.Config, overrides map[string]ToolOverrideConfig, verbose bool) error {
	if len(overrides) == 0 {
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
		override, hasOverride := overrides[toolName]
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
