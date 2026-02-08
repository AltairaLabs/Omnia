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

package compaction

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/altairalabs/omnia/pkg/metrics"
)

// Config tunes the compaction engine behaviour.
type Config struct {
	BatchSize   int
	MaxRetries  int
	RetryDelay  time.Duration
	Compression string
	DryRun      bool
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BatchSize:   1000,
		MaxRetries:  3,
		RetryDelay:  5 * time.Second,
		Compression: "snappy",
	}
}

// Result summarises a compaction run.
type Result struct {
	SessionsCompacted int64
	BatchesProcessed  int
	ColdPurged        bool
	Errors            []error
}

// Engine performs batched warm→cold compaction.
type Engine struct {
	warmStore   providers.WarmStoreProvider
	coldArchive providers.ColdArchiveProvider
	hotCache    providers.HotCacheProvider // may be nil
	retention   *RetentionConfig
	cfg         Config
	metrics     *metrics.CompactionMetrics
	log         *zap.SugaredLogger
}

// NewEngine creates a compaction engine.
func NewEngine(
	warm providers.WarmStoreProvider,
	cold providers.ColdArchiveProvider,
	hot providers.HotCacheProvider,
	retention *RetentionConfig,
	cfg Config,
	m *metrics.CompactionMetrics,
	log *zap.SugaredLogger,
) *Engine {
	return &Engine{
		warmStore:   warm,
		coldArchive: cold,
		hotCache:    hot,
		retention:   retention,
		cfg:         cfg,
		metrics:     m,
		log:         log,
	}
}

// Run executes the full compaction cycle: warm→cold, then cold purge.
func (e *Engine) Run(ctx context.Context) (*Result, error) {
	start := time.Now()
	result := &Result{}

	if err := e.compactWarmToCold(ctx, result); err != nil {
		e.recordMetrics(start)
		return result, fmt.Errorf("warm-to-cold compaction: %w", err)
	}

	e.purgeExpiredCold(ctx, result)
	e.recordMetrics(start)
	return result, nil
}

func (e *Engine) compactWarmToCold(ctx context.Context, result *Result) error {
	now := time.Now()
	minCutoff := e.retention.MinWarmCutoff(now)
	e.log.Infow("starting warm-to-cold compaction", "minCutoff", minCutoff, "batchSize", e.cfg.BatchSize)

	for {
		if ctx.Err() != nil {
			return nil
		}

		sessions, err := e.warmStore.GetSessionsOlderThan(ctx, minCutoff, e.cfg.BatchSize)
		if err != nil {
			return fmt.Errorf("querying warm store: %w", err)
		}
		if len(sessions) == 0 {
			break
		}

		eligible := e.filterByWorkspaceCutoff(sessions, now)
		if len(eligible) == 0 {
			// All returned sessions are within per-workspace retention; stop.
			break
		}

		if err := e.processBatch(ctx, eligible); err != nil {
			return err
		}

		result.SessionsCompacted += int64(len(eligible))
		result.BatchesProcessed++

		if e.metrics != nil {
			e.metrics.RecordSessionsCompacted(int64(len(eligible)))
			e.metrics.RecordBatchProcessed()
		}

		// In dry-run mode, stop after the first batch since sessions are
		// not actually deleted and would be returned again.
		if e.cfg.DryRun {
			break
		}
	}

	e.log.Infow("warm-to-cold compaction complete",
		"sessionsCompacted", result.SessionsCompacted,
		"batchesProcessed", result.BatchesProcessed)
	return nil
}

func (e *Engine) processBatch(ctx context.Context, sessions []*session.Session) error {
	if e.cfg.DryRun {
		ids := make([]string, len(sessions))
		for i, s := range sessions {
			ids[i] = s.ID
		}
		e.log.Infow("dry-run: would compact sessions", "count", len(sessions), "ids", ids)
		return nil
	}

	// Write to cold archive with retry.
	writeOpts := providers.WriteOpts{Compression: e.cfg.Compression}
	if err := e.withRetry(ctx, "write_parquet", func() error {
		return e.coldArchive.WriteParquet(ctx, sessions, writeOpts)
	}); err != nil {
		return fmt.Errorf("writing parquet: %w", err)
	}

	// Delete from warm store with retry.
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}
	if err := e.withRetry(ctx, "delete_warm", func() error {
		return e.warmStore.DeleteSessionsBatch(ctx, ids)
	}); err != nil {
		return fmt.Errorf("deleting from warm store: %w", err)
	}

	// Best-effort hot cache invalidation.
	e.invalidateHotCache(ctx, ids)

	return nil
}

func (e *Engine) filterByWorkspaceCutoff(sessions []*session.Session, now time.Time) []*session.Session {
	var eligible []*session.Session
	for _, s := range sessions {
		cutoff := e.retention.WarmCutoff(s.WorkspaceName, now)
		if s.UpdatedAt.Before(cutoff) {
			eligible = append(eligible, s)
		}
	}
	return eligible
}

func (e *Engine) invalidateHotCache(ctx context.Context, ids []string) {
	if e.hotCache == nil {
		return
	}
	for _, id := range ids {
		if err := e.hotCache.Invalidate(ctx, id); err != nil {
			e.log.Warnw("hot cache invalidation failed (best-effort)", "sessionID", id, "error", err)
		}
	}
}

func (e *Engine) purgeExpiredCold(ctx context.Context, result *Result) {
	cutoff := e.retention.ColdCutoff(time.Now())
	if cutoff.IsZero() {
		e.log.Info("cold purge skipped: no retention cutoff configured")
		return
	}

	e.log.Infow("purging expired cold archive data", "cutoff", cutoff)
	err := e.withRetry(ctx, "purge_cold", func() error {
		return e.coldArchive.DeleteOlderThan(ctx, cutoff)
	})
	if err != nil {
		e.log.Errorw("cold purge failed (non-fatal)", "error", err)
		result.Errors = append(result.Errors, fmt.Errorf("cold purge: %w", err))
		return
	}
	result.ColdPurged = true
	e.log.Info("cold purge complete")
}

func (e *Engine) withRetry(ctx context.Context, operation string, fn func() error) error {
	delay := e.cfg.RetryDelay
	var lastErr error
	for attempt := 0; attempt <= e.cfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt > 0 {
			e.log.Warnw("retrying operation", "operation", operation, "attempt", attempt, "error", lastErr)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
			delay *= 2
		}
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if e.metrics != nil {
			e.metrics.RecordError(operation)
		}
	}
	return fmt.Errorf("%s failed after %d retries: %w", operation, e.cfg.MaxRetries, lastErr)
}

func (e *Engine) recordMetrics(start time.Time) {
	if e.metrics == nil {
		return
	}
	e.metrics.RecordDuration(time.Since(start))
	e.metrics.RecordLastRun()
}
