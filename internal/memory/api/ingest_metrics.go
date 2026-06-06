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

package api

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Ingestion + retrieval-governance metrics.
//
// The ingest path forks by strategy (chunk stores inline, extractive
// summarizes inline, agent enqueues for async summarization) and each
// fork has a different failure surface: a chunk failure is a store
// error, an agent failure is a queue backlog. Without a per-strategy
// outcome counter, "ingestion is broken" can't be narrowed to which
// fork — and the enqueued path persists nothing synchronously, so a
// silent queue stall looks identical to success on the documents_total
// counter unless outcome=enqueued is tracked separately from success.
//
// duration_seconds covers the whole IngestDocument call (including the
// inline chunk/summarize work) so latency regressions in the source
// adapter or summarizer surface here. items_total divided by
// documents_total{outcome=success} gives the average fan-out (chunks or
// summaries per document) — a sudden drop means the chunker or
// summarizer is emitting fewer items than expected.
const (
	ingestLabelStrategy   = "strategy"
	ingestOutcomeSuccess  = "success"
	ingestOutcomeError    = "error"
	ingestOutcomeEnqueued = "enqueued"
)

var (
	ingestDocumentsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_memory_ingest_documents_total",
		Help: "Documents ingested by strategy and outcome (success=stored inline, enqueued=handed to async summarizer, error=failed); outcome=enqueued never reaches the store synchronously, so track it apart from success.",
	}, []string{ingestLabelStrategy, "outcome"})

	ingestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "omnia_memory_ingest_duration_seconds",
		Help:    "Wall-clock latency of IngestDocument by strategy, including inline chunking/summarizing; a regression points at the source adapter or summarizer.",
		Buckets: prometheus.DefBuckets,
	}, []string{ingestLabelStrategy})

	ingestItemsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_memory_ingest_items_total",
		Help: "Items (chunks or summaries) persisted per ingest by strategy; divide by ingest_documents_total{outcome=success} for average fan-out. Zero for the async-enqueue path, which persists later in the summarizer worker.",
	}, []string{ingestLabelStrategy})

	retrievalDeniedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "omnia_memory_retrieval_denied_total",
		Help: "Items dropped at retrieval by the governance deny-filter (denyCEL); a non-zero rate confirms restricted content is being blocked before it reaches the caller.",
	})
)
