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

import (
	"context"
	"strings"
)

// PostProcess turns produced summary text into the items stored for a
// document. It is the single convergence point for both summarizer backends
// (extractive, agent) and both summary strategies:
//
//   - StrategySummary           -> one item (kind=summary, index 0)
//   - StrategySummaryThenChunk  -> RAG-chunk the summary (kind=summary_chunk)
//
// Empty/whitespace summary yields no items. cfg.Strategy is assumed already
// effective (a summary strategy); any other value is treated as
// StrategySummary (one item).
func PostProcess(summary string, cfg Config, doc SourceDoc) []Item {
	if strings.TrimSpace(summary) == "" {
		return nil
	}
	if cfg.EffectiveStrategy() == StrategySummaryThenChunk {
		chunked, _ := NewChunkStrategy(cfg.ChunkSize, cfg.ChunkOverlap).
			Ingest(context.Background(), SourceDoc{
				Title: doc.Title, URL: doc.URL, Site: doc.Site, Text: summary,
			})
		for i := range chunked {
			chunked[i].Metadata[MetaKeyKind] = KindSummaryChunk
		}
		return chunked
	}
	return []Item{{
		Content:  summary,
		Metadata: baseMetadata(doc, KindSummary, 0),
	}}
}
