/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/events"
	pkproviders "github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/tools/arena/engine"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/pkg/k8s"
	"github.com/altairalabs/omnia/pkg/session/httpclient"
)

func processWorkItems(
	ctx context.Context, log logr.Logger, cfg *Config,
	q queue.WorkQueue, bundlePath string, wm *WorkerMetrics,
) error {
	jobID := cfg.JobName

	// No job-level root span — each work item gets its own trace.
	// This prevents massive traces for load tests with 100+ items.
	// Traces are correlated back to the job via the arena.job attribute.

	// Use VU pool for concurrent processing when configured.
	if cfg.VUsPerWorker > 1 || cfg.Concurrency > 1 {
		var profile *LoadProfile
		if cfg.RampUp > 0 || cfg.RampDown > 0 {
			profile = NewLoadProfile(cfg.Concurrency, cfg.RampUp, cfg.RampDown)
		}
		pool := NewVUPool(VUPoolConfig{
			Size:         cfg.VUsPerWorker,
			Concurrency:  cfg.Concurrency,
			Queue:        q,
			JobID:        jobID,
			Log:          log,
			Metrics:      wm,
			PollInterval: cfg.PollInterval,
			Profile:      profile,
			Execute: func(ctx context.Context, item *queue.WorkItem) (*ExecutionResult, error) {
				return executeWorkItem(ctx, log, cfg, item, bundlePath)
			},
		})
		return pool.Run(ctx)
	}

	// Single-VU mode: existing sequential loop (backward compatible).
	return processSingleVU(ctx, log, cfg, q, jobID, bundlePath, wm)
}

// processSingleVU is the original single-threaded work item processing loop.
func processSingleVU(
	ctx context.Context, log logr.Logger, cfg *Config,
	q queue.WorkQueue, jobID, bundlePath string, wm *WorkerMetrics,
) error {
	emptyCount := 0
	maxEmptyPolls := 10

	log.Info("processing work items", "jobID", jobID)

	wlc := &workerLoopContext{ctx: ctx, log: log, cfg: cfg, queue: q, jobID: jobID}
	for {
		if checkContextDone(ctx, log) {
			return nil
		}

		item, err := q.Pop(ctx, jobID)
		if err != nil {
			done, resetCount, retErr := handlePopError(wlc, err, emptyCount, maxEmptyPolls)
			if retErr != nil {
				return retErr
			}
			if done {
				return nil
			}
			emptyCount = resetCount
			continue
		}

		emptyCount = 0
		log.Info("work item popped",
			"itemID", item.ID,
			"scenarioID", item.ScenarioID,
			"providerID", item.ProviderID,
		)

		executeAndReport(wlc, item, bundlePath, wm)
	}
}

// executeAndReport runs a work item and reports the result via Ack/Nack.
func executeAndReport(
	wlc *workerLoopContext,
	item *queue.WorkItem,
	bundlePath string,
	wm *WorkerMetrics,
) {
	ctx, log, cfg, q, jobID := wlc.ctx, wlc.log, wlc.cfg, wlc.queue, wlc.jobID
	runWorkItemWithTracing(
		ctx,
		jobID,
		item,
		wm,
		func(itemCtx context.Context, workItem *queue.WorkItem) (*ExecutionResult, error) {
			return executeWorkItem(itemCtx, log, cfg, workItem, bundlePath)
		},
		func(reportCtx context.Context, workItem *queue.WorkItem, result *ExecutionResult, execErr error) {
			reportWorkItemResult(reportCtx, log, q, jobID, workItem, result, execErr)
		},
	)
}

// runWorkItemWithTracing executes one item with standard per-item tracing,
// timeout handling, result reporting, and metrics recording.
func runWorkItemWithTracing(
	parentCtx context.Context,
	jobID string,
	item *queue.WorkItem,
	wm *WorkerMetrics,
	execute workItemExecutor,
	report workItemReporter,
) {
	// Each work item gets its own trace (not a child of a job-level root).
	traceID := workItemToTraceID(jobID, item.ID)
	spanID := workItemToSpanID(item.ID)
	remoteCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	// Per-item trace root — NOT a child of the job context. This keeps traces
	// small and queryable via arena.job attribute. Lifetime is bounded by
	// maxItemTimeout on the next line.
	itemCtx := trace.ContextWithRemoteSpanContext(context.Background(), remoteCtx)
	itemCtx, itemCancel := context.WithTimeout(itemCtx, maxItemTimeout)
	itemCtx, span := otel.Tracer("omnia-arena-worker").Start(itemCtx, "arena.work-item",
		trace.WithAttributes(
			attribute.String("arena.job", jobID),
			attribute.String("arena.item.id", item.ID),
			attribute.String("arena.scenario", item.ScenarioID),
			attribute.String("arena.provider", item.ProviderID),
		),
	)
	itemStart := time.Now()
	result, execErr := execute(itemCtx, item)
	if execErr != nil {
		span.RecordError(execErr)
	}
	itemCancel()
	if itemCtx.Err() == context.DeadlineExceeded {
		execErr = fmt.Errorf("work item timed out after %v", maxItemTimeout)
	}
	reportCtx := trace.ContextWithSpan(parentCtx, span)
	report(reportCtx, item, result, execErr)
	span.End()

	itemDuration := time.Since(itemStart).Seconds()
	status := statusPass
	if execErr != nil || (result != nil && result.Status == statusFail) {
		status = statusFail
	}
	wm.RecordWorkItem(jobID, status, itemDuration)
	recordDetailedMetrics(wm, jobID, item, result, execErr, itemDuration)
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
	wlc *workerLoopContext,
	err error,
	emptyCount, maxEmptyPolls int,
) (bool, int, error) {
	ctx, log, cfg, q, jobID := wlc.ctx, wlc.log, wlc.cfg, wlc.queue, wlc.jobID
	if !isRecoverablePopError(err) {
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

func isRecoverablePopError(err error) bool {
	return errors.Is(err, queue.ErrQueueEmpty) || errors.Is(err, queue.ErrItemNotFound)
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
	reportWorkItemResultWithOptions(ctx, log, q, jobID, item, result, execErr, true)
}

func reportWorkItemResultWithOptions(
	ctx context.Context,
	log logr.Logger,
	q queue.WorkQueue,
	jobID string,
	item *queue.WorkItem,
	result *ExecutionResult,
	execErr error,
	logFailedResultError bool,
) {
	if execErr != nil {
		log.Error(execErr, "work item failed", "itemID", item.ID)
		if err := q.Nack(ctx, jobID, item.ID, execErr); err != nil {
			log.Error(err, "failed to nack item", "itemID", item.ID)
		}
		return
	}

	log.Info("work item completed",
		"itemID", item.ID,
		"status", result.Status,
		"durationMs", result.DurationMs,
	)
	if logFailedResultError && result.Status == statusFail && result.Error != "" {
		log.Info("work item result error",
			"itemID", item.ID,
			"error", result.Error,
		)
	}
	if err := q.CompleteItem(ctx, jobID, item.ID, toItemResult(result)); err != nil {
		log.Error(err, "failed to complete item", "itemID", item.ID)
	}
}

// toItemResult converts an ExecutionResult to a queue.ItemResult for accumulator updates.
func toItemResult(result *ExecutionResult) *queue.ItemResult {
	assertions := make([]queue.AssertionResult, len(result.Assertions))
	for i, a := range result.Assertions {
		assertions[i] = queue.AssertionResult{
			Name:    a.Name,
			Passed:  a.Passed,
			Message: a.Message,
		}
	}
	return &queue.ItemResult{
		Status:     result.Status,
		DurationMs: result.DurationMs,
		Error:      result.Error,
		Metrics:    result.Metrics,
		Assertions: assertions,
		SessionID:  result.SessionID,
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

	arenaCfg, configPath, err := loadArenaConfig(log, cfg, bundlePath)
	if err != nil {
		return nil, err
	}

	crdFleetProviders, pricingMap, err := resolveCRDConfig(ctx, log, cfg, arenaCfg, configPath)
	if err != nil {
		return nil, err
	}

	// Build registries and executors from the config.
	// Fleet providers in LoadedProviders are handled by the registered fleet factory
	// (ee/pkg/arena/fleet/factory.go) — no special ordering needed.
	log.Info("building engine components")
	providerRegistry, promptRegistry, mcpRegistry, convExecutor, adapterRegistry, a2aCleanup, toolRegistry, _, err :=
		engine.BuildEngineComponents(arenaCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build engine components: %w", err)
	}
	if a2aCleanup != nil {
		defer a2aCleanup()
	}

	// Connect fleet providers to their agent WebSocket endpoints.
	// The factory created them but didn't connect (no context available at factory time).
	if len(crdFleetProviders) > 0 {
		if err := connectCRDFleetProviders(ctx, log, cfg, providerRegistry, crdFleetProviders); err != nil {
			return nil, err
		}
		defer closeFleetProviders(providerRegistry, crdFleetProviders)
	}

	// Create engine with all components
	eng, err := engine.NewEngine(
		arenaCfg, providerRegistry, promptRegistry, mcpRegistry,
		convExecutor, adapterRegistry, toolRegistry,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create engine: %w", err)
	}
	log.Info("engine created")
	defer func() {
		if closeErr := eng.Close(); closeErr != nil {
			log.Error(closeErr, "failed to close engine")
		}
	}()

	arenaMeta := newArenaSessionMeta(cfg, item)

	// Load-test fleet runs drive a live agent whose facade already records the
	// conversation + cost into its own session. Creating a separate arena session
	// for these just produces an empty shell. Skip the session manager and instead
	// decorate the facade-recorded session with arena context after the run (see
	// decorateFleetSessions below). Other job types (evaluation/datagen) keep the
	// session manager so their engine-emitted events are recorded as today.
	loadTestFleet := isLoadTestFleet(cfg, crdFleetProviders)

	// Wire session recording if session-api is configured.
	// Events from all engine runs are forwarded to session-api via OmniaEventStore.
	sessionMgr, spCollector := wireSessionRecording(log, cfg, eng, arenaMeta, item.ID, loadTestFleet)
	if sessionMgr != nil {
		defer sessionMgr.CompleteAll(ctx)
	}

	// Override trials to 1 on all loaded scenarios — the partitioner has already
	// expanded trial repetitions into separate work items, so PromptKit must not
	// do its own internal trial expansion.
	for _, scenario := range arenaCfg.LoadedScenarios {
		scenario.Trials = 1
	}

	scenarioFilter, providerFilter := workItemFilters(item)

	plan, emptyResult, err := generateRunPlan(log, eng, item, providerFilter, scenarioFilter, start)
	if err != nil {
		return nil, err
	}
	if emptyResult != nil {
		return emptyResult, nil
	}

	runIDs, err := executeRunPlan(ctx, eng, plan)
	if err != nil {
		return nil, err
	}

	// Build result from state store, with cost calculation from provider pricing.
	result = buildExecutionResult(log, eng.GetStateStore(), runIDs, start, pricingMap[item.ProviderID])

	// Extract TTFT from fleet providers — the engine doesn't propagate this,
	// so we read it directly from the provider after execution completes.
	extractFleetTTFT(providerRegistry, crdFleetProviders, result)

	// Extract session ID for job-to-session correlation. For load-test fleet runs
	// this also attaches the self-play/judge provider calls and persona context to
	// the facade session.
	result.SessionID = resolveResultSessionID(ctx, log, cfg, fleetSessionInputs{
		loadTestFleet: loadTestFleet,
		meta:          arenaMeta,
		personaIDs:    loadedPersonaIDs(arenaCfg),
		selfPlayCalls: collectedSelfPlayCalls(spCollector),
		sessionMgr:    sessionMgr,
		registry:      providerRegistry,
		fleet:         crdFleetProviders,
	})

	return result, nil
}

// loadArenaConfig finds, loads, and configures the arena config from the bundle.
// It applies verbose mode and the resolved output directory before the engine is
// built (media storage is created during engine init). Returns the config and the
// resolved config-file path.
func loadArenaConfig(log logr.Logger, cfg *Config, bundlePath string) (*config.Config, string, error) {
	configPath := findArenaConfigFile(bundlePath, cfg.ConfigFile)
	if configPath == "" {
		return nil, "", fmt.Errorf("arena config file not found in bundle: %s", bundlePath)
	}

	log.Info("loading arena config", "configPath", configPath)

	// Load configuration from file BEFORE creating engine so we can modify it
	arenaCfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load configuration: %w", err)
	}

	// Configure settings BEFORE creating engine (media storage is created during engine init)
	if cfg.Verbose {
		arenaCfg.Defaults.Verbose = true
	}

	// Set output directory to a writable location.
	// The workspace content is mounted read-only, so we need a writable path for media files.
	// resolveOutputDir reads from OutputConfig: PVC uses the mounted path, S3 stages
	// to /tmp/arena-output (uploaded after all items complete), nil uses the fallback.
	arenaCfg.Defaults.Output.Dir = resolveOutputDir(cfg)

	return arenaCfg, configPath, nil
}

// resolveK8sClient returns the pre-configured client for testing, or creates an
// in-cluster client for CRD resolution.
func resolveK8sClient(cfg *Config) (client.Client, error) {
	if cfg.K8sClient != nil {
		return cfg.K8sClient, nil
	}
	k8sClient, err := k8s.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client for CRD resolution: %w", err)
	}
	return k8sClient, nil
}

// wireSessionRecording attaches an event bus to the engine for session recording
// when session-api is configured. Load-test fleet runs only capture self-play /
// judge provider calls (the facade records the agent conversation); other runs get
// a full arena session manager. Returns the session manager (caller must defer
// CompleteAll) and self-play collector, either of which may be nil.
func wireSessionRecording(
	log logr.Logger, cfg *Config, eng *engine.Engine,
	arenaMeta arenaSessionMetadata, itemID string, loadTestFleet bool,
) (*arenaSessionManager, *selfPlayCollector) {
	switch {
	case cfg.SessionAPIURL == "":
		// No recording configured.
		return nil, nil
	case loadTestFleet:
		// Fleet mode: the facade records the agent conversation + cost. Capture the
		// self-play / judge provider calls (which run in this worker's engine, not
		// behind the facade) so they can be attached to the facade session after
		// the run. No session is created here, so no empty shell.
		spCollector := newSelfPlayCollector()
		bus := events.NewEventBus()
		bus.SubscribeAll(spCollector.OnEvent)
		eng.SetEventBus(bus)
		return nil, spCollector
	default:
		sessionMgr := newArenaSessionManager(
			httpclient.NewStore(cfg.SessionAPIURL, log),
			log,
			arenaMeta,
			itemID,
		)
		bus := events.NewEventBus()
		bus.SubscribeAll(sessionMgr.OnEvent)
		eng.SetEventBus(bus, engine.WithMessageEvents())
		return sessionMgr, nil
	}
}

// workItemFilters derives the scenario and provider filters for an item's run plan.
// Empty slices mean "all" (no override).
func workItemFilters(item *queue.WorkItem) (scenarioFilter, providerFilter []string) {
	scenarioFilter = []string{}
	if item.ScenarioID != "" && item.ScenarioID != defaultScenarioID {
		scenarioFilter = []string{item.ScenarioID}
	}

	// Work item's ProviderID is the resolved provider/agent ID.
	providerFilter = []string{}
	if item.ProviderID != "" {
		providerFilter = []string{item.ProviderID}
	}
	return scenarioFilter, providerFilter
}

// resolveCRDConfig resolves providers and tools from CRDs, remaps provider IDs so
// self-play/judge references match resolved keys, and applies tool overrides to the
// arena config. Returns the resolved fleet providers and per-provider pricing map.
func resolveCRDConfig(
	ctx context.Context, log logr.Logger, cfg *Config, arenaCfg *config.Config, configPath string,
) ([]*resolvedFleetProvider, map[string]*providerPricing, error) {
	k8sClient, err := resolveK8sClient(cfg)
	if err != nil {
		return nil, nil, err
	}

	crdFleetProviders, pricingMap, err := resolveProvidersFromCRD(ctx, log, k8sClient, cfg, arenaCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve providers from CRDs: %w", err)
	}

	if err := resolveToolsFromCRD(ctx, log, k8sClient, cfg); err != nil {
		return nil, nil, fmt.Errorf("failed to resolve tools from CRDs: %w", err)
	}

	// Remap provider IDs so that self-play/judge references in the arena config
	// match the CRD-resolved provider keys in LoadedProviders.
	if err := remapProviderIDs(log, arenaCfg, configPath); err != nil {
		return nil, nil, fmt.Errorf("failed to remap provider IDs: %w", err)
	}

	// Apply tool overrides (from CRD resolution) to the config.
	if len(cfg.ToolOverrides) > 0 {
		if err := applyToolOverrides(log, arenaCfg, cfg.ToolOverrides); err != nil {
			return nil, nil, fmt.Errorf("failed to apply tool overrides: %w", err)
		}
	}

	return crdFleetProviders, pricingMap, nil
}

// connectCRDFleetProviders builds the fleet token source and connects the resolved
// fleet providers to their agent WebSocket endpoints. The factory created the
// providers but couldn't connect them (no context at factory time).
func connectCRDFleetProviders(
	ctx context.Context, log logr.Logger, cfg *Config,
	providerRegistry *pkproviders.Registry, crdFleetProviders []*resolvedFleetProvider,
) error {
	fleetTokenSource, tokErr := buildFleetTokenSource(log, cfg.MgmtPlaneTokenURL)
	if tokErr != nil {
		return tokErr
	}
	if err := connectFleetProviders(
		ctx, log, providerRegistry, crdFleetProviders, fleetTokenSource, cfg.WorkspaceName,
	); err != nil {
		return fmt.Errorf("failed to connect fleet providers: %w", err)
	}
	return nil
}

// generateRunPlan builds the run plan for the item's provider/scenario filters and
// logs the planned execution. When the plan has no combinations it returns a
// populated failure ExecutionResult (the second return) signalling an early exit;
// otherwise that result is nil and the plan is returned.
func generateRunPlan(
	log logr.Logger, eng *engine.Engine, item *queue.WorkItem,
	providerFilter, scenarioFilter []string, start time.Time,
) (*engine.RunPlan, *ExecutionResult, error) {
	// Generate run plan for this specific provider (or all if no override).
	plan, err := eng.GenerateRunPlan(
		[]string{},     // no region filter
		providerFilter, // filter to this provider (empty = all from config)
		scenarioFilter, // scenario filter (empty = all)
		[]string{},     // no eval filter
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate run plan: %w", err)
	}

	if len(plan.Combinations) == 0 {
		return nil, &ExecutionResult{
			Status:     statusFail,
			Error:      "no scenario/provider combinations generated — check scenario filter and provider config",
			DurationMs: float64(time.Since(start).Milliseconds()),
		}, nil
	}

	providerDesc := "all providers"
	if item.ProviderID != "" {
		providerDesc = item.ProviderID
	}
	log.Info("executing scenarios",
		"combinations", len(plan.Combinations),
		"provider", providerDesc,
	)

	return plan, nil, nil
}

// executeRunPlan runs the plan with concurrency of 1 (single work item at a time),
// wrapped in an arena.engine.execute span.
func executeRunPlan(ctx context.Context, eng *engine.Engine, plan *engine.RunPlan) ([]string, error) {
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
	return runIDs, nil
}
