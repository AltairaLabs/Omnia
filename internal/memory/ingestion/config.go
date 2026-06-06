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

package ingestion

// Strategy + summarizer-backend names. These exact strings are the CRD enum
// values (MemoryPolicy.spec.ingestion) AND the --ingest-* flag values — one
// vocabulary, no casing translation.
const (
	StrategyChunk            = "chunk"
	StrategySummary          = "summary"
	StrategySummaryThenChunk = "summaryThenChunk"

	SummarizerExtractive = "extractive"
	SummarizerAgent      = "agent"
)

// Item.Metadata kinds and keys.
const (
	KindChunk        = "chunk"
	KindSummary      = "summary"
	KindSummaryChunk = "summary_chunk"

	MetaKeyKind  = "kind"
	MetaKeyTitle = "title"
	MetaKeyURL   = "url"
	MetaKeySite  = "site"
	MetaKeyIndex = "index"
)

// Config is the resolved per-request ingestion configuration: which strategy
// shapes a document into items, which summarizer backend produces summary
// text, and the RAG chunk geometry. A zero Config means "chunk with splitter
// defaults" (Strategy=="" normalises to chunk; ChunkSize<=0 defaults to 200).
type Config struct {
	Strategy     string // StrategyChunk | StrategySummary | StrategySummaryThenChunk
	Summarizer   string // SummarizerExtractive | SummarizerAgent (ignored when Strategy==chunk)
	ChunkSize    int
	ChunkOverlap int
}

// EffectiveStrategy normalises an empty/unknown Strategy to StrategyChunk so
// an unconfigured service still ingests rather than failing.
func (c Config) EffectiveStrategy() string {
	switch c.Strategy {
	case StrategySummary, StrategySummaryThenChunk:
		return c.Strategy
	default:
		return StrategyChunk
	}
}

// UsesSummarizer reports whether the effective strategy needs a summary.
func (c Config) UsesSummarizer() bool {
	s := c.EffectiveStrategy()
	return s == StrategySummary || s == StrategySummaryThenChunk
}
