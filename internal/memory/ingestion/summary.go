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
	"fmt"
	"strings"
)

// DocumentSummarizer produces a condensed summary of a document body. The
// default implementation wraps a completion provider (wired in cmd/memory-api);
// tests use a fake. Mirrors the pluggable-summarizer precedent in
// internal/memory/compaction_worker.go (Summarizer), but shaped for documents.
type DocumentSummarizer interface {
	Summarize(ctx context.Context, text string) (string, error)
}

// SummaryStrategy emits a single condensed item per document — the summary-index
// point on the spectrum. Scales to large corpora; grounding comes from
// on-demand fetch at query time.
type SummaryStrategy struct {
	summarizer DocumentSummarizer
}

// NewSummaryStrategy builds a SummaryStrategy backed by the given summarizer.
func NewSummaryStrategy(summarizer DocumentSummarizer) *SummaryStrategy {
	return &SummaryStrategy{summarizer: summarizer}
}

// Ingest summarizes the body into one item. Empty text yields no items; a
// summarizer error fails the doc (caller surfaces it — no silent blob).
func (s *SummaryStrategy) Ingest(ctx context.Context, doc SourceDoc) ([]Item, error) {
	if strings.TrimSpace(doc.Text) == "" {
		return nil, nil
	}
	summary, err := s.summarizer.Summarize(ctx, doc.Text)
	if err != nil {
		return nil, fmt.Errorf("summarize %q: %w", doc.URL, err)
	}
	return []Item{{
		Content:  summary,
		Metadata: baseMetadata(doc, "summary", 0),
	}}, nil
}
