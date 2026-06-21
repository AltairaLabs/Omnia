/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

// embeddingMetricsLabelWorkspace is the per-workspace label (Workspace CR UID,
// the same value the projection metrics carry, so the two correlate).
const embeddingMetricsLabelWorkspace = "workspace"

// EmbeddingMetricsStore is the store surface the collector polls. Satisfied by
// *PostgresMemoryStore; declared here (consumer side) so the collector is
// unit-testable with a fake.
type EmbeddingMetricsStore interface {
	ListWorkspaceIDs(ctx context.Context) ([]string, error)
	EmbeddingCoverage(ctx context.Context, workspaceID string) (total, embedded int, err error)
	CountObservationsMissingEmbedding(ctx context.Context, workspaceID, currentModel string) (int, error)
}

// EmbeddingMetrics holds the embedding-pipeline health gauges. Operational
// signals — Prometheus is the source of truth (see CLAUDE.md observability
// boundary).
type EmbeddingMetrics struct {
	// Coverage is the fraction (0..1) of a workspace's live entities whose
	// latest active observation carries an embedding. Below the projector's
	// dense threshold the Memory Galaxy renders on the lexical basis.
	Coverage *prometheus.GaugeVec
	// Backlog is the count of active observations awaiting (re-)embedding for
	// the current model — the re-embed worker's per-workspace queue depth.
	Backlog *prometheus.GaugeVec
}

// NewEmbeddingMetrics constructs the gauges (not yet registered).
func NewEmbeddingMetrics() *EmbeddingMetrics {
	return &EmbeddingMetrics{
		Coverage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "omnia_memory_embedding_coverage",
			Help: "Fraction (0..1) of a workspace's live entities with an embedding on " +
				"their latest active observation. Below the dense threshold the Memory " +
				"Galaxy degrades to the lexical basis.",
		}, []string{embeddingMetricsLabelWorkspace}),
		Backlog: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "omnia_memory_reembed_backlog",
			Help: "Count of active observations awaiting (re-)embedding for the current " +
				"model — the re-embed worker's per-workspace queue depth.",
		}, []string{embeddingMetricsLabelWorkspace}),
	}
}

// MustRegister registers the gauges with reg.
func (m *EmbeddingMetrics) MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(m.Coverage, m.Backlog)
}

// EmbeddingMetricsCollector periodically refreshes the per-workspace embedding
// coverage and re-embed backlog gauges. It is a read-only poller: a cheap
// aggregate query per workspace on a slow tick, surfacing stateful coverage
// that the activity counters (embed_requests_total) cannot show (#1442).
type EmbeddingMetricsCollector struct {
	store        EmbeddingMetricsStore
	metrics      *EmbeddingMetrics
	currentModel string
	interval     time.Duration
	log          logr.Logger
}

// NewEmbeddingMetricsCollector builds a collector. currentModel is the active
// embedding model name (matches the re-embed worker's notion of "current"), so
// the backlog counts rows that are unembedded OR stamped by an older model.
func NewEmbeddingMetricsCollector(
	store EmbeddingMetricsStore, metrics *EmbeddingMetrics,
	currentModel string, interval time.Duration, log logr.Logger,
) *EmbeddingMetricsCollector {
	return &EmbeddingMetricsCollector{
		store:        store,
		metrics:      metrics,
		currentModel: currentModel,
		interval:     interval,
		log:          log,
	}
}

// Run does an initial collection then refreshes every interval until ctx is
// cancelled. Intended to be started in its own goroutine.
func (c *EmbeddingMetricsCollector) Run(ctx context.Context) {
	c.collectOnce(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collectOnce(ctx)
		}
	}
}

// collectOnce refreshes every workspace's gauges. It Resets the vectors first
// so a workspace that was deleted (or drained to zero live entities) doesn't
// leave a stale series lingering at its last value.
func (c *EmbeddingMetricsCollector) collectOnce(ctx context.Context) {
	wss, err := c.store.ListWorkspaceIDs(ctx)
	if err != nil {
		c.log.Error(err, "embedding metrics: list workspaces")
		return
	}
	c.metrics.Coverage.Reset()
	c.metrics.Backlog.Reset()
	for _, ws := range wss {
		c.collectWorkspace(ctx, ws)
	}
}

// collectWorkspace sets the coverage and backlog gauges for one workspace. A
// query error is logged and skips only that workspace; the next tick retries.
func (c *EmbeddingMetricsCollector) collectWorkspace(ctx context.Context, ws string) {
	total, embedded, err := c.store.EmbeddingCoverage(ctx, ws)
	if err != nil {
		c.log.Error(err, "embedding metrics: coverage", "workspace", ws)
		return
	}
	if total > 0 {
		c.metrics.Coverage.WithLabelValues(ws).Set(float64(embedded) / float64(total))
	}
	backlog, err := c.store.CountObservationsMissingEmbedding(ctx, ws, c.currentModel)
	if err != nil {
		c.log.Error(err, "embedding metrics: backlog", "workspace", ws)
		return
	}
	c.metrics.Backlog.WithLabelValues(ws).Set(float64(backlog))
}
