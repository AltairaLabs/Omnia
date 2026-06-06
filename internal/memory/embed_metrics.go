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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Embedding-provider metrics.
//
// Every RAG read path that resolves text to vectors — the dedup
// similarity check, the EmbeddingService write path, and the re-embed
// worker — funnels through the same EmbeddingProvider.Embed call. That
// call is a network round trip to a third-party model API: it's the
// most common stall point and the most common failure point in the
// memory subsystem, but until now it emitted no signal at all.
//
// requests_total splits success/error so operators can alert on a
// provider that's up-but-erroring (rate of outcome=error > 0) without
// waiting for ingestion or retrieval latency to visibly degrade.
// duration_seconds is a single histogram (no labels) because the
// provider call latency is the quantity operators page on; per-caller
// breakdown belongs on the caller's own spans, not here.
const (
	embedOutcomeSuccess = "success"
	embedOutcomeError   = "error"
)

var (
	embedRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_memory_embed_requests_total",
		Help: "Embedding-provider Embed calls by outcome (success|error); a rising error rate means the embedding API is reachable but failing, which stalls every RAG read/write path.",
	}, []string{"outcome"})

	embedDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "omnia_memory_embed_duration_seconds",
		Help:    "Wall-clock latency of embedding-provider Embed calls; the dominant stall point for ingestion, dedup, and re-embed paths.",
		Buckets: prometheus.DefBuckets,
	})
)

// meteredEmbeddingProvider wraps an EmbeddingProvider and records a
// counter + histogram sample for every Embed call. It is transparent:
// it implements EmbeddingProvider itself, so wrapping the real provider
// once at construction instruments every caller (dedup, EmbeddingService,
// re-embed worker) without touching individual call sites.
type meteredEmbeddingProvider struct {
	inner EmbeddingProvider
}

// NewMeteredEmbeddingProvider wraps inner so every Embed call is timed
// and counted by outcome. Dimensions() passes straight through.
func NewMeteredEmbeddingProvider(inner EmbeddingProvider) EmbeddingProvider {
	return &meteredEmbeddingProvider{inner: inner}
}

// Embed times the inner call and records its outcome before returning.
func (m *meteredEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	start := time.Now()
	vecs, err := m.inner.Embed(ctx, texts)
	embedDurationSeconds.Observe(time.Since(start).Seconds())
	outcome := embedOutcomeSuccess
	if err != nil {
		outcome = embedOutcomeError
	}
	embedRequestsTotal.WithLabelValues(outcome).Inc()
	return vecs, err
}

// Dimensions passes through to the wrapped provider.
func (m *meteredEmbeddingProvider) Dimensions() int {
	return m.inner.Dimensions()
}
