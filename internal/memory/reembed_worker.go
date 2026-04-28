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
	"fmt"
	"time"

	"github.com/go-logr/logr"
)

// ReembedWorkerOptions configures the background re-embed worker.
//
// The worker exists because hybrid recall depends on
// memory_observations.embedding being populated for every active
// row. Two scenarios leave that column NULL or stale:
//
//  1. Observations written before the embedding service was wired up
//     (or while the embed call failed transiently and was swallowed).
//  2. The configured embedding model changes — the saved vector is
//     no longer compatible with new query embeddings, so cosine
//     similarity becomes meaningless until the row is re-embedded.
//
// The worker scans for rows in either state, embeds them via the
// provider, and stamps the model name on the row so subsequent
// model swaps can identify the new generation of stale rows.
type ReembedWorkerOptions struct {
	// Interval between backfill passes. A few hours is sensible —
	// the worker is meant to catch up over time, not block writes.
	Interval time.Duration
	// BatchSize caps how many rows a single pass embeds. Defaults to
	// 50 — enough to make steady progress without throttling the
	// provider or holding any one row's lock too long.
	BatchSize int
	// CurrentModel is the embedding-model identifier the provider
	// produces. Stamped onto each re-embedded row; rows already
	// stamped with this value are skipped. May be empty when the
	// provider doesn't expose a stable name — in that case the
	// worker only fills NULL embeddings.
	CurrentModel string
}

// ReembedProvider is the minimal embedding interface the worker
// needs. EmbeddingProvider satisfies it; the worker takes only what
// it needs so tests can swap in a fixed-vector mock without
// implementing the full provider surface.
type ReembedProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// ReembedWorker periodically backfills missing / stale embeddings
// on active observations. Modelled after CompactionWorker so the
// operational surface (Run / RunOnce / log shape) is familiar.
type ReembedWorker struct {
	store    *PostgresMemoryStore
	provider ReembedProvider
	opts     ReembedWorkerOptions
	log      logr.Logger
}

// NewReembedWorker constructs a worker. provider must be non-nil —
// without one the worker has nothing to call. Interval and BatchSize
// fall through to sensible defaults when zero.
func NewReembedWorker(store *PostgresMemoryStore, provider ReembedProvider, opts ReembedWorkerOptions, log logr.Logger) *ReembedWorker {
	if opts.BatchSize <= 0 {
		opts.BatchSize = 50
	}
	return &ReembedWorker{
		store:    store,
		provider: provider,
		opts:     opts,
		log:      log.WithName("reembed-worker"),
	}
}

// Run blocks until ctx is cancelled, running RunOnce at each tick.
// A nil provider or non-positive interval disables the worker — it
// logs the reason and returns immediately so binaries can wire it
// unconditionally and let configuration decide whether it runs.
func (w *ReembedWorker) Run(ctx context.Context) {
	if w.provider == nil {
		w.log.Info("reembed worker disabled", "reason", "no provider")
		return
	}
	if w.opts.Interval <= 0 {
		w.log.Info("reembed worker disabled", "reason", "interval not set")
		return
	}
	MarkWorkerRunning(WorkerNameReembed)
	defer MarkWorkerStopped(WorkerNameReembed)

	ticker := time.NewTicker(w.opts.Interval)
	defer ticker.Stop()

	w.log.Info("reembed worker started",
		"interval", w.opts.Interval,
		"batchSize", w.opts.BatchSize,
		"currentModel", w.opts.CurrentModel,
	)

	// First pass on startup so a freshly-deployed service catches
	// any pre-existing NULL-embedding rows immediately rather than
	// waiting one Interval.
	if err := w.RunOnce(ctx); err != nil {
		w.log.Error(err, "initial reembed pass failed")
	}

	for {
		select {
		case <-ctx.Done():
			w.log.Info("reembed worker stopped")
			return
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				w.log.Error(err, "reembed pass failed")
			}
		}
	}
}

// RunOnce performs a single backfill pass. Returns the number of
// observations re-embedded so callers (and tests) can chain passes
// until 0 to drain the queue. Per-row failures are logged but do
// not abort the pass — one bad row shouldn't stop the rest.
func (w *ReembedWorker) RunOnce(ctx context.Context) error {
	if w.provider == nil {
		return nil
	}
	rows, err := w.store.FindObservationsMissingEmbedding(ctx, w.opts.CurrentModel, w.opts.BatchSize)
	if err != nil {
		return fmt.Errorf("find candidates: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	texts := make([]string, len(rows))
	for i, r := range rows {
		texts[i] = r.Content
	}

	embeddings, err := w.provider.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed batch: %w", err)
	}
	if len(embeddings) != len(rows) {
		return fmt.Errorf("embed returned %d vectors for %d rows", len(embeddings), len(rows))
	}

	var firstErr error
	for i, r := range rows {
		if len(embeddings[i]) == 0 {
			w.log.V(1).Info("skip empty embedding",
				"observationID", r.ObservationID, "entityID", r.EntityID)
			continue
		}
		if err := w.store.UpdateObservationEmbedding(ctx, r.ObservationID, embeddings[i], w.opts.CurrentModel); err != nil {
			w.log.Error(err, "reembed update failed",
				"observationID", r.ObservationID, "entityID", r.EntityID)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
	}

	w.log.V(1).Info("reembed pass complete",
		"candidates", len(rows),
		"model", w.opts.CurrentModel,
	)
	return firstErr
}
