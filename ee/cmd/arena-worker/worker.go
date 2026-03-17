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
	"encoding/hex"
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

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/pkg/k8s"
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

// sessionIDToTraceID converts a UUID session ID to an OpenTelemetry trace ID.
// This mirrors the facade's logic so the arena worker can create span links
// that point to the session-derived trace.
func sessionIDToTraceID(sessionID string) trace.TraceID {
	cleaned := strings.ReplaceAll(sessionID, "-", "")
	var tid trace.TraceID
	_, _ = hex.Decode(tid[:], []byte(cleaned))
	return tid
}

// maxItemTimeout is the maximum time allowed for a single work item execution.
const maxItemTimeout = 10 * time.Minute

const defaultScenarioID = "default"

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

	// Redis configuration
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Worker configuration
	WorkDir       string
	PollInterval  time.Duration
	ShutdownDelay time.Duration
	Verbose       bool // Enable verbose/debug output from promptarena

	// Override configurations (resolved from CRDs)
	ToolOverrides map[string]ToolOverrideConfig // Tool name -> override config

	// K8sClient is an optional pre-configured k8s client for testing.
	// When nil, the worker creates one via k8s.NewClient() (in-cluster config).
	K8sClient client.Client
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

	// Resolve providers and tools from CRDs
	k8sClient := cfg.K8sClient
	if k8sClient == nil {
		var k8sErr error
		k8sClient, k8sErr = k8s.NewClient()
		if k8sErr != nil {
			return nil, fmt.Errorf("failed to create k8s client for CRD resolution: %w", k8sErr)
		}
	}

	var crdFleetProviders []*resolvedFleetProvider
	crdFleetProviders, err = resolveProvidersFromCRD(ctx, log, k8sClient, cfg, arenaCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve providers from CRDs: %w", err)
	}

	if err := resolveToolsFromCRD(ctx, log, k8sClient, cfg); err != nil {
		return nil, fmt.Errorf("failed to resolve tools from CRDs: %w", err)
	}

	// Remap provider IDs so that self-play/judge references in the arena config
	// match the CRD-resolved provider keys in LoadedProviders.
	if err := remapProviderIDs(log, arenaCfg, configPath); err != nil {
		return nil, fmt.Errorf("failed to remap provider IDs: %w", err)
	}

	// Apply tool overrides (from CRD resolution) to the config
	if len(cfg.ToolOverrides) > 0 {
		if err := applyToolOverrides(log, arenaCfg, cfg.ToolOverrides); err != nil {
			return nil, fmt.Errorf("failed to apply tool overrides: %w", err)
		}
	}

	// Build registries and executors from the config.
	// Fleet providers in LoadedProviders are handled by the registered fleet factory
	// (ee/pkg/arena/fleet/factory.go) — no special ordering needed.
	log.Info("building engine components")
	providerRegistry, promptRegistry, mcpRegistry, convExecutor, adapterRegistry, a2aCleanup, _, err :=
		engine.BuildEngineComponents(arenaCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build engine components: %w", err)
	}
	if a2aCleanup != nil {
		defer a2aCleanup()
	}

	// Connect fleet providers to their agent WebSocket endpoints.
	// The factory created them but didn't connect (no context available at factory time).
	if len(crdFleetProviders) > 0 {
		if err := connectFleetProviders(ctx, log, providerRegistry, crdFleetProviders); err != nil {
			return nil, fmt.Errorf("failed to connect fleet providers: %w", err)
		}
		defer closeFleetProviders(providerRegistry, crdFleetProviders)
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

	// Determine scenario filter
	scenarioFilter := []string{}
	if item.ScenarioID != "" && item.ScenarioID != defaultScenarioID {
		scenarioFilter = []string{item.ScenarioID}
	}

	// Determine provider filter — work item's ProviderID is the resolved provider/agent ID
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
		result.Status = statusFail
		result.Error = "no scenario/provider combinations generated — check scenario filter and provider config"
		result.DurationMs = float64(time.Since(start).Milliseconds())
		return result, nil
	}

	providerDesc := "all providers"
	if item.ProviderID != "" {
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
	result.Status = statusFail
	if len(runIDs) == 0 {
		result.Error = "no runs executed"
	} else {
		result.Error = fmt.Sprintf("unable to read run state for %d run(s) — results unknown", len(runIDs))
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
