/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
)

// Summarizer is the minimal abstraction the compaction worker needs from the
// LLM integration. It takes a bucket of old observations and returns a single
// synthesised summary string. Real implementations live outside this package
// (alongside the runtime's PromptKit wiring); the default implementation used
// for tests is NoopSummarizer.
type Summarizer interface {
	Summarize(ctx context.Context, entries []CompactionEntry) (string, error)
}

// NoopSummarizer returns a canned "summarized N entries" line without calling
// any LLM. Used in tests and as a safe default when no real summarizer is
// configured — the worker still supersedes the originals, shrinking the
// retrieval surface even without an LLM.
type NoopSummarizer struct{}

// Summarize concatenates the count and a preview of the first entry's
// content. Deterministic so tests can assert on the output.
func (NoopSummarizer) Summarize(_ context.Context, entries []CompactionEntry) (string, error) {
	if len(entries) == 0 {
		return "", errors.New("memory: noop summarizer called with no entries")
	}
	preview := entries[0].Content
	if len(preview) > 80 {
		preview = preview[:80]
	}
	return fmt.Sprintf("Summary of %d observations. First: %s", len(entries), preview), nil
}

// CompactionWorkerOptions configures the background worker.
type CompactionWorkerOptions struct {
	// Interval between compaction passes. A few hours is a sensible default.
	Interval time.Duration
	// WorkspaceIDs the worker scans each tick. Empty slice = no-op.
	WorkspaceIDs []string
	// Age of observations before they become compaction candidates. 30d is
	// a sensible default for conversational memory.
	Age time.Duration
	// MinGroupSize / MaxCandidates / MaxPerCandidate passed to
	// FindCompactionCandidates. Zero = store-level defaults.
	MinGroupSize    int
	MaxCandidates   int
	MaxPerCandidate int
}

// CompactionWorker periodically summarizes old memories and supersedes the
// originals. It's modeled after RetentionWorker so the operational surface is
// familiar to anyone who's already wired that into a binary.
type CompactionWorker struct {
	store      *PostgresMemoryStore
	summarizer Summarizer
	opts       CompactionWorkerOptions
	log        logr.Logger
}

// NewCompactionWorker constructs a worker. If summarizer is nil a
// NoopSummarizer is installed.
func NewCompactionWorker(store *PostgresMemoryStore, summarizer Summarizer, opts CompactionWorkerOptions, log logr.Logger) *CompactionWorker {
	if summarizer == nil {
		summarizer = NoopSummarizer{}
	}
	return &CompactionWorker{
		store:      store,
		summarizer: summarizer,
		opts:       opts,
		log:        log,
	}
}

// Run blocks until ctx is cancelled, running RunOnce at each interval tick.
func (w *CompactionWorker) Run(ctx context.Context) {
	if w.opts.Interval <= 0 {
		w.log.Info("compaction worker disabled", "reason", "interval not set")
		return
	}
	ticker := time.NewTicker(w.opts.Interval)
	defer ticker.Stop()

	w.log.Info("compaction worker started",
		"interval", w.opts.Interval,
		"age", w.opts.Age,
		"workspaces", len(w.opts.WorkspaceIDs),
	)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("compaction worker stopped")
			return
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				w.log.Error(err, "compaction pass failed")
			}
		}
	}
}

// RunOnce performs a single compaction pass across all configured workspaces.
// Exposed for tests and on-demand triggers. Returns the first error seen, but
// doesn't stop on per-workspace failures — one bad workspace shouldn't block
// the others.
func (w *CompactionWorker) RunOnce(ctx context.Context) error {
	if len(w.opts.WorkspaceIDs) == 0 {
		return nil
	}
	age := w.opts.Age
	if age <= 0 {
		age = 30 * 24 * time.Hour
	}
	olderThan := time.Now().Add(-age)

	var firstErr error
	for _, ws := range w.opts.WorkspaceIDs {
		if err := w.runWorkspace(ctx, ws, olderThan); err != nil {
			w.log.Error(err, "compaction workspace failed", "workspace", ws)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (w *CompactionWorker) runWorkspace(ctx context.Context, workspaceID string, olderThan time.Time) error {
	candidates, err := w.store.FindCompactionCandidates(ctx, FindCompactionCandidatesOptions{
		WorkspaceID:     workspaceID,
		OlderThan:       olderThan,
		MinGroupSize:    w.opts.MinGroupSize,
		MaxCandidates:   w.opts.MaxCandidates,
		MaxPerCandidate: w.opts.MaxPerCandidate,
	})
	if err != nil {
		return fmt.Errorf("find candidates: %w", err)
	}
	if len(candidates) == 0 {
		return nil
	}

	w.log.V(1).Info("compaction candidates",
		"workspace", workspaceID,
		"buckets", len(candidates),
	)

	for _, c := range candidates {
		if err := w.compactBucket(ctx, c); err != nil {
			// Race isn't a real failure — another pass already summarized.
			if errors.Is(err, ErrCompactionRaced) {
				w.log.V(1).Info("compaction race ignored", "workspace", workspaceID)
				continue
			}
			return fmt.Errorf("compact bucket: %w", err)
		}
	}
	return nil
}

func (w *CompactionWorker) compactBucket(ctx context.Context, c CompactionCandidate) error {
	summary, err := w.summarizer.Summarize(ctx, c.Entries)
	if err != nil {
		return fmt.Errorf("summarize: %w", err)
	}
	id, err := w.store.SaveCompactionSummary(ctx, CompactionSummary{
		WorkspaceID:            c.WorkspaceID,
		UserID:                 c.UserID,
		AgentID:                c.AgentID,
		Content:                summary,
		SupersededObservations: c.ObservationIDs,
	})
	if err != nil {
		return err
	}
	w.log.V(1).Info("compaction summary written",
		"workspace", c.WorkspaceID,
		"summary_id", id,
		"superseded", len(c.ObservationIDs),
	)
	return nil
}
