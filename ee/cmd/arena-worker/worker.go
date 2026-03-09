/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/AltairaLabs/PromptKit/tools/arena/engine"
	arenastatestore "github.com/AltairaLabs/PromptKit/tools/arena/statestore"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"

	"github.com/altairalabs/omnia/ee/pkg/arena/binding"
	"github.com/altairalabs/omnia/ee/pkg/arena/fleet"
	"github.com/altairalabs/omnia/ee/pkg/arena/overrides"
	"github.com/altairalabs/omnia/ee/pkg/arena/providers"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

// Status constants for execution results.
const (
	statusPass = "pass"
	statusFail = "fail"
)

// jobNameToTraceID derives a deterministic trace ID from a job name by hashing it.
// This makes traces easy to look up in Tempo by job name without searching.
func jobNameToTraceID(jobName string) trace.TraceID {
	h := sha256.Sum256([]byte(jobName))
	var tid trace.TraceID
	copy(tid[:], h[:16]) // trace ID is 128 bits = 16 bytes
	return tid
}

// jobNameToSpanID derives a deterministic span ID from a job name.
// Uses bytes 16–24 of the SHA-256 hash (the half not used for the trace ID).
// A non-zero SpanID is required for SpanContext.IsValid() to return true.
func jobNameToSpanID(jobName string) trace.SpanID {
	h := sha256.Sum256([]byte(jobName))
	var sid trace.SpanID
	copy(sid[:], h[16:24]) // span ID is 64 bits = 8 bytes
	return sid
}

// maxItemTimeout is the maximum time allowed for a single work item execution.
const maxItemTimeout = 10 * time.Minute

// Execution mode constants.
const (
	executionModeFleet = "fleet"
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

	// Execution mode
	ExecutionMode string // "direct" or "fleet"
	FleetWSURL    string // WebSocket URL for fleet mode

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

	cfg.ExecutionMode = getEnvOrDefault("ARENA_EXECUTION_MODE", "direct")
	cfg.FleetWSURL = os.Getenv("ARENA_FLEET_WS_URL")

	if cfg.JobName == "" {
		return nil, errors.New("ARENA_JOB_NAME is required")
	}
	if cfg.ContentPath == "" {
		return nil, errors.New("ARENA_CONTENT_PATH is required")
	}
	if cfg.ExecutionMode == executionModeFleet && cfg.FleetWSURL == "" {
		return nil, errors.New("ARENA_FLEET_WS_URL is required when ARENA_EXECUTION_MODE is fleet")
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

func processWorkItems(ctx context.Context, log logr.Logger, cfg *Config, q queue.WorkQueue, bundlePath string) error {
	jobID := cfg.JobName
	emptyCount := 0
	maxEmptyPolls := 10 // Exit after this many consecutive empty polls

	// Derive a deterministic trace ID from the job name so traces are easy
	// to look up in Tempo without searching by span attribute.
	traceID := jobNameToTraceID(jobID)
	spanID := jobNameToSpanID(jobID)
	remoteCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx = trace.ContextWithRemoteSpanContext(ctx, remoteCtx)

	// Root span for the entire worker lifecycle so all Redis/fleet operations
	// are correlated under a common parent trace.
	ctx, rootSpan := otel.Tracer("omnia-arena-worker").Start(ctx, "arena.worker",
		trace.WithAttributes(
			attribute.String("arena.job", jobID),
			attribute.String("arena.execution_mode", cfg.ExecutionMode),
		),
	)
	defer rootSpan.End()

	log.Info("processing work items", "jobID", jobID)

	for {
		if checkContextDone(ctx, log) {
			return nil
		}

		// Pop next work item
		item, err := q.Pop(ctx, jobID)
		if err != nil {
			done, resetCount, retErr := handlePopError(ctx, log, err, emptyCount, maxEmptyPolls, cfg, q, jobID)
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

		log.Info("work item popped",
			"itemID", item.ID,
			"scenarioID", item.ScenarioID,
			"providerID", item.ProviderID,
		)

		// Execute with per-item timeout, wrapped in a trace span.
		// The span covers the full lifecycle: execute → ack/nack so that
		// Redis operations are correlated under the work-item span.
		itemCtx, itemCancel := context.WithTimeout(ctx, maxItemTimeout)
		itemCtx, span := otel.Tracer("omnia-arena-worker").Start(itemCtx, "arena.work-item",
			trace.WithAttributes(
				attribute.String("arena.job", jobID),
				attribute.String("arena.scenario", item.ScenarioID),
				attribute.String("arena.provider", item.ProviderID),
				attribute.String("arena.execution_mode", cfg.ExecutionMode),
			),
		)
		result, execErr := executeWorkItem(itemCtx, log, cfg, item, bundlePath)
		if execErr != nil {
			span.RecordError(execErr)
		}
		itemCancel()
		if itemCtx.Err() == context.DeadlineExceeded {
			execErr = fmt.Errorf("work item timed out after %v", maxItemTimeout)
		}
		// Ack/Nack uses the root ctx (not the timed-out itemCtx) but carries
		// the work-item span so Redis operations are children of this item.
		ackCtx := trace.ContextWithSpan(ctx, span)
		reportWorkItemResult(ackCtx, log, q, jobID, item, result, execErr)
		span.End()
	}
}

// checkContextDone returns true if the context is cancelled.
func checkContextDone(ctx context.Context, log logr.Logger) bool {
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
		return true
	default:
		return false
	}
}

// handlePopError handles errors from queue.Pop and returns (done, newEmptyCount, error).
//
//nolint:unparam // maxEmptyPolls kept as parameter for testability
func handlePopError(
	ctx context.Context, log logr.Logger, err error, emptyCount, maxEmptyPolls int,
	cfg *Config, q queue.WorkQueue, jobID string,
) (bool, int, error) {
	if !errors.Is(err, queue.ErrQueueEmpty) {
		return false, emptyCount, fmt.Errorf("failed to pop work item: %w", err)
	}

	emptyCount++
	if emptyCount >= maxEmptyPolls {
		done, checkErr := checkJobCompletion(ctx, log, q, jobID, emptyCount)
		if checkErr != nil {
			return false, 0, checkErr
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
func checkJobCompletion(
	ctx context.Context, log logr.Logger, q queue.WorkQueue, jobID string, emptyCount int,
) (bool, error) {
	log.V(1).Info("queue empty, checking job completion", "emptyPolls", emptyCount)

	progress, err := q.Progress(ctx, jobID)
	if err != nil {
		return false, fmt.Errorf("failed to get job progress: %w", err)
	}

	if progress.IsComplete() {
		log.Info("job complete",
			"completed", progress.Completed+progress.Failed,
			"total", progress.Total,
		)
		return true, nil
	}

	log.V(1).Info("job still in progress",
		"pending", progress.Pending,
		"processing", progress.Processing,
		"completed", progress.Completed,
		"total", progress.Total,
	)
	return false, nil
}

// reportWorkItemResult reports the result of a work item execution.
func reportWorkItemResult(
	ctx context.Context, log logr.Logger, q queue.WorkQueue, jobID string,
	item *queue.WorkItem, result *ExecutionResult, execErr error,
) {
	if execErr != nil {
		log.Error(execErr, "work item failed", "itemID", item.ID)
		if err := q.Nack(ctx, jobID, item.ID, execErr); err != nil {
			log.Error(err, "failed to nack item", "itemID", item.ID)
		}
		return
	}

	resultJSON, _ := json.Marshal(result)
	log.Info("work item completed",
		"itemID", item.ID,
		"status", result.Status,
		"durationMs", result.DurationMs,
	)
	if result.Status == statusFail && result.Error != "" {
		log.Info("work item result error",
			"itemID", item.ID,
			"error", result.Error,
		)
	}
	if err := q.Ack(ctx, jobID, item.ID, resultJSON); err != nil {
		log.Error(err, "failed to ack item", "itemID", item.ID)
	}
}

func executeWorkItem(
	ctx context.Context,
	log logr.Logger,
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

	log.Info("loading arena config", "configPath", configPath)

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
		if err := applyOverridesFromConfig(log, arenaCfg, overrideCfg); err != nil {
			return nil, fmt.Errorf("failed to apply overrides from ConfigMap: %w", err)
		}
	} else if len(cfg.ToolOverrides) > 0 {
		// Fall back to legacy tool overrides from env var (for backwards compatibility)
		if err := applyToolOverrides(log, arenaCfg, cfg.ToolOverrides); err != nil {
			return nil, fmt.Errorf("failed to apply tool overrides: %w", err)
		}
	}

	// Apply provider bindings (annotation-based credential resolution)
	// This runs after explicit overrides so they take precedence
	if overrideCfg != nil && len(overrideCfg.Bindings) > 0 {
		if err := applyProviderBindings(log, arenaCfg, overrideCfg.Bindings, configPath); err != nil {
			// Non-fatal: log warning and continue
			log.Error(err, "failed to apply provider bindings")
		}
	}

	// For fleet mode, connect to the agent and prepare provider injection
	var fleetProvider *fleet.Provider
	if cfg.ExecutionMode == executionModeFleet {
		fleetProvider = fleet.NewProvider("fleet-agent", cfg.FleetWSURL, nil)
		if err := fleetProvider.Connect(ctx); err != nil {
			return nil, fmt.Errorf("failed to connect to fleet agent: %w", err)
		}
		defer func() { _ = fleetProvider.Close() }()

		log.Info("fleet agent connected", "wsURL", cfg.FleetWSURL)
	}

	// Build registries and executors from the config
	log.Info("building engine components")
	providerRegistry, promptRegistry, mcpRegistry, convExecutor, adapterRegistry, a2aCleanup, _, err :=
		engine.BuildEngineComponents(arenaCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build engine components: %w", err)
	}
	if a2aCleanup != nil {
		defer a2aCleanup()
	}

	// For fleet mode, register the fleet provider into the engine's registry
	if fleetProvider != nil {
		providerRegistry.Register(fleetProvider)
	}

	// Create engine with all components
	eng, err := engine.NewEngine(arenaCfg, providerRegistry, promptRegistry, mcpRegistry, convExecutor, adapterRegistry)
	if err != nil {
		return nil, fmt.Errorf("failed to create engine: %w", err)
	}
	log.Info("engine created")
	defer func() {
		if closeErr := eng.Close(); closeErr != nil {
			log.Error(closeErr, "failed to close engine")
		}
	}()

	// Validate provider credentials before execution (skip for fleet — no external credentials needed)
	if cfg.ExecutionMode != executionModeFleet {
		if err := providers.ValidateProviderCredentials(arenaCfg, item.ProviderID); err != nil {
			result.Status = statusFail
			result.Error = err.Error()
			result.DurationMs = float64(time.Since(start).Milliseconds())
			return result, nil
		}
	}

	// Determine scenario filter
	scenarioFilter := []string{}
	if item.ScenarioID != "" && item.ScenarioID != "default" {
		scenarioFilter = []string{item.ScenarioID}
	}

	// Determine provider filter
	// For fleet mode, route to the fleet provider only
	providerFilter := []string{}
	if cfg.ExecutionMode == executionModeFleet {
		providerFilter = []string{"fleet-agent"}
	} else if item.ProviderID != "" {
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

	providerDesc := "all providers"
	if cfg.ExecutionMode == executionModeFleet {
		providerDesc = "fleet agent"
	} else if item.ProviderID != "" {
		providerDesc = item.ProviderID
	}
	log.Info("executing scenarios",
		"combinations", len(plan.Combinations),
		"provider", providerDesc,
	)

	// Execute runs with concurrency of 1 (single work item at a time).
	ctx, runSpan := otel.Tracer("omnia-arena-worker").Start(ctx, "arena.engine.execute",
		trace.WithAttributes(
			attribute.Int("arena.combinations", len(plan.Combinations)),
		),
	)
	runIDs, err := eng.ExecuteRuns(ctx, plan, 1)
	runSpan.End()
	if err != nil {
		return nil, fmt.Errorf("execution failed: %w", err)
	}

	// Build result from state store
	result = buildExecutionResult(log, eng.GetStateStore(), runIDs, start)
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
	log           logr.Logger
}

// processRun processes a single run's state and updates aggregated counts.
func (a *runAggregator) processRun(runID string, state *arenastatestore.ArenaConversationState) {
	if state.RunMetadata == nil {
		a.log.V(1).Info("run has no metadata", "runID", runID)
		return
	}
	meta := state.RunMetadata
	a.totalDuration += meta.Duration

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
func buildExecutionResult(
	log logr.Logger, store statestore.Store, runIDs []string, startTime time.Time,
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
func applyToolOverrides(log logr.Logger, cfg *config.Config, toolOverrides map[string]ToolOverrideConfig) error {
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

		log.V(1).Info("tool override applied",
			"tool", toolName,
			"endpoint", override.Endpoint,
			"registry", override.RegistryName,
			"handler", override.HandlerName,
		)
	}

	if appliedCount > 0 {
		log.Info("tool overrides applied", "count", appliedCount)
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
	log logr.Logger,
	arenaCfg *config.Config,
	providersByGroup map[string][]overrides.ProviderOverride,
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

			hasCreds := (p.SecretEnvVar != "" && os.Getenv(p.SecretEnvVar) != "") ||
				(p.CredentialFile != "" && fileExists(p.CredentialFile))
			log.V(1).Info("provider override applied",
				"providerID", p.ID,
				"providerType", p.Type,
				"model", p.Model,
				"group", groupName,
				"hasCreds", hasCreds,
			)
		}
	}

	if appliedCount > 0 {
		log.Info("provider overrides applied", "count", appliedCount)
	}
}

// fileExists returns true if the file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// applyProviderBindings resolves provider binding annotations against the registry
// and injects credentials into providers that don't already have them.
func applyProviderBindings(
	log logr.Logger,
	cfg *config.Config,
	registry map[string]overrides.ProviderOverride,
	configPath string,
) error {
	bindings, err := binding.ParseProviderAnnotations(cfg, configPath)
	if err != nil {
		return fmt.Errorf("failed to parse provider annotations: %w", err)
	}

	verbose := log.V(1).Enabled()
	boundCount := binding.ApplyBindings(cfg, bindings, registry, verbose)
	matchedCount := binding.ApplyNameMatching(cfg, registry, verbose)

	if boundCount > 0 || matchedCount > 0 {
		log.V(1).Info("provider bindings applied",
			"annotationBound", boundCount,
			"nameMatched", matchedCount,
		)
	}

	return nil
}

// applyOverridesFromConfig applies all overrides from the loaded override config.
// This handles both provider and tool overrides from the ConfigMap.
func applyOverridesFromConfig(
	log logr.Logger,
	arenaCfg *config.Config,
	overrideCfg *overrides.OverrideConfig,
) error {
	if overrideCfg == nil {
		return nil
	}

	// Apply provider overrides first
	applyProviderOverrides(log, arenaCfg, overrideCfg.Providers)

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
		if err := applyToolOverrides(log, arenaCfg, toolOverrides); err != nil {
			return fmt.Errorf("failed to apply tool overrides: %w", err)
		}
	}

	return nil
}
