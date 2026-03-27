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
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

// VUPoolConfig holds the configuration for creating a VUPool.
type VUPoolConfig struct {
	Size         int // Number of VU goroutines
	Concurrency  int // Global concurrency limit (0 = unlimited)
	Queue        queue.WorkQueue
	JobID        string
	Log          logr.Logger
	Metrics      *WorkerMetrics
	PollInterval time.Duration
	Profile      *LoadProfile // Optional load profile for ramp-up/down
	Execute      func(ctx context.Context, item *queue.WorkItem) (*ExecutionResult, error)
}

// VUPool manages a pool of virtual users that concurrently process work items.
type VUPool struct {
	size         int
	concurrency  int
	queue        queue.WorkQueue
	jobID        string
	log          logr.Logger
	metrics      *WorkerMetrics
	pollInterval time.Duration
	profile      *LoadProfile
	execute      func(ctx context.Context, item *queue.WorkItem) (*ExecutionResult, error)
}

// NewVUPool creates a new VU pool from the given configuration.
func NewVUPool(cfg VUPoolConfig) *VUPool {
	size := cfg.Size
	if size < 1 {
		size = 1
	}
	pollInterval := cfg.PollInterval
	if pollInterval == 0 {
		pollInterval = 100 * time.Millisecond
	}
	return &VUPool{
		size:         size,
		concurrency:  cfg.Concurrency,
		queue:        cfg.Queue,
		jobID:        cfg.JobID,
		log:          cfg.Log,
		metrics:      cfg.Metrics,
		pollInterval: pollInterval,
		profile:      cfg.Profile,
		execute:      cfg.Execute,
	}
}

// Run starts all VU goroutines and blocks until all have finished.
// Each VU independently pops, executes, and reports work items.
// Returns the first fatal error encountered, or nil on clean shutdown.
func (p *VUPool) Run(ctx context.Context) error {
	if p.profile != nil {
		p.profile.Start()
	}

	p.log.Info("VU pool starting",
		"vus", p.size,
		"concurrency", p.concurrency,
		"hasProfile", p.profile != nil,
		"jobID", p.jobID,
	)

	if p.metrics != nil {
		p.metrics.SetActiveVUs(float64(p.size))
	}

	var wg sync.WaitGroup
	errCh := make(chan error, p.size)

	for i := range p.size {
		wg.Add(1)
		go func(vuID int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					p.log.Error(fmt.Errorf("panic: %v", r), "VU panicked", "vu", fmt.Sprintf("vu-%d", vuID))
				}
			}()
			vuLog := p.log.WithValues("vu", fmt.Sprintf("vu-%d", vuID))
			if err := p.vuLoop(ctx, vuLog); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	if p.metrics != nil {
		p.metrics.SetActiveVUs(0)
	}

	// Return the first error, if any.
	for err := range errCh {
		if err != nil {
			return err
		}
	}

	p.log.Info("VU pool finished", "jobID", p.jobID)
	return nil
}

// vuLoop is the main loop for a single virtual user.
func (p *VUPool) vuLoop(ctx context.Context, log logr.Logger) error {
	emptyCount := 0
	maxEmptyPolls := 10

	log.V(1).Info("VU started")

	for {
		if ctx.Err() != nil {
			log.V(1).Info("VU shutting down")
			return nil
		}

		// Check concurrency limit before popping.
		if p.concurrency > 0 {
			atLimit, checkErr := p.atConcurrencyLimit(ctx)
			if checkErr != nil {
				return checkErr
			}
			if atLimit {
				time.Sleep(p.pollInterval)
				continue
			}
		}

		item, err := p.queue.Pop(ctx, p.jobID)
		if err != nil {
			done, resetCount, retErr := p.handleVUPopError(ctx, log, err, emptyCount, maxEmptyPolls)
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

		p.executeAndReport(ctx, log, item)
	}
}

// atConcurrencyLimit checks if the global concurrency limit has been reached.
// This is a best-effort check: between checking Progress and calling Pop,
// other VUs may also pass this check and pop items. The actual in-flight count
// may briefly exceed the concurrency limit by up to (VUsPerWorker - 1).
// For load testing, this is acceptable — the limit controls approximate pressure,
// not an exact contract.
func (p *VUPool) atConcurrencyLimit(ctx context.Context) (bool, error) {
	progress, err := p.queue.Progress(ctx, p.jobID)
	if err != nil {
		if errors.Is(err, queue.ErrJobNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check concurrency: %w", err)
	}

	limit := p.concurrency
	if p.profile != nil {
		elapsed := time.Since(p.profile.startTime)
		limit = p.profile.AllowedConcurrency(elapsed, progress.Pending)
	}

	return progress.Processing >= limit, nil
}

// handleVUPopError handles errors from queue.Pop within a VU loop.
func (p *VUPool) handleVUPopError(
	ctx context.Context, log logr.Logger, err error, emptyCount, maxEmptyPolls int,
) (bool, int, error) {
	if !errors.Is(err, queue.ErrQueueEmpty) {
		return false, emptyCount, fmt.Errorf("failed to pop work item: %w", err)
	}

	emptyCount++
	if emptyCount >= maxEmptyPolls {
		done, checkErr := checkJobCompletion(ctx, log, p.queue, p.jobID, emptyCount)
		if checkErr != nil {
			return false, 0, checkErr
		}
		if done {
			return true, 0, nil
		}
		emptyCount = 0
	}

	time.Sleep(p.pollInterval)
	return false, emptyCount, nil
}

// executeAndReport runs a single work item and reports the result.
func (p *VUPool) executeAndReport(ctx context.Context, log logr.Logger, item *queue.WorkItem) {
	// Each work item gets its own trace (not a child of a job-level root).
	// This keeps traces small and queryable via arena.job attribute.
	traceID := workItemToTraceID(p.jobID, item.ID)
	spanID := workItemToSpanID(item.ID)
	remoteCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	itemCtx := trace.ContextWithRemoteSpanContext(context.Background(), remoteCtx)
	itemCtx, itemCancel := context.WithTimeout(itemCtx, maxItemTimeout)
	itemCtx, span := otel.Tracer("omnia-arena-worker").Start(itemCtx, "arena.work-item",
		trace.WithAttributes(
			attribute.String("arena.job", p.jobID),
			attribute.String("arena.item.id", item.ID),
			attribute.String("arena.scenario", item.ScenarioID),
			attribute.String("arena.provider", item.ProviderID),
		),
	)
	itemStart := time.Now()
	result, execErr := p.execute(itemCtx, item)
	if execErr != nil {
		span.RecordError(execErr)
	}
	itemCancel()
	if itemCtx.Err() == context.DeadlineExceeded {
		execErr = fmt.Errorf("work item timed out after %v", maxItemTimeout)
	}

	ackCtx := trace.ContextWithSpan(ctx, span)
	p.reportResult(ackCtx, log, item, result, execErr)
	span.End()

	itemDuration := time.Since(itemStart).Seconds()
	status := statusPass
	if execErr != nil || (result != nil && result.Status == statusFail) {
		status = statusFail
	}
	p.metrics.RecordWorkItem(p.jobID, status, itemDuration)
	recordDetailedMetrics(p.metrics, p.jobID, item, result, execErr, itemDuration)
}

// reportResult reports the work item result via CompleteItem or Nack.
func (p *VUPool) reportResult(
	ctx context.Context, log logr.Logger, item *queue.WorkItem,
	result *ExecutionResult, execErr error,
) {
	// Use a fresh context for ack/nack if the parent is cancelled,
	// so in-flight items are properly reported during shutdown.
	reportCtx := ctx
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		reportCtx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}

	if execErr != nil {
		log.Error(execErr, "work item failed", "itemID", item.ID)
		if err := p.queue.Nack(reportCtx, p.jobID, item.ID, execErr); err != nil {
			log.Error(err, "failed to nack item", "itemID", item.ID)
		}
		return
	}

	log.Info("work item completed",
		"itemID", item.ID,
		"status", result.Status,
		"durationMs", result.DurationMs,
	)
	if err := p.queue.CompleteItem(reportCtx, p.jobID, item.ID, toItemResult(result)); err != nil {
		log.Error(err, "failed to complete item", "itemID", item.ID)
	}
}
