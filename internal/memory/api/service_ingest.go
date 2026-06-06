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
	"context"
	"fmt"
	"time"

	"github.com/altairalabs/omnia/internal/memory"
	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

// aboutKindSharePointDoc is the about_kind stamped on every ingested item so
// re-ingest supersedes by (url#index) instead of duplicating.
const aboutKindSharePointDoc = "sharepoint_doc"

// SetIngestion wires the fallback ingestion config (from the --ingest-* flags)
// and the optional async summary queue. Called once at startup. A nil queue
// disables the agent path — agent-configured ingests fall back to extractive.
func (s *MemoryService) SetIngestion(fallback ingestion.Config, queue ingestion.SummaryQueue) {
	s.ingestFallback = fallback
	s.summaryQueue = queue
}

// IngestDocument resolves the effective ingestion config for the workspace and
// either stores items synchronously (chunk, or extractive summary) or enqueues
// the document for the async summarizer agent. Returns 202-worthy success;
// embedding is left to the ReembedWorker.
func (s *MemoryService) IngestDocument(ctx context.Context, workspaceID string, doc ingestion.SourceDoc) (err error) {
	cfg := s.resolveIngestionConfig(ctx)
	strategy := cfg.EffectiveStrategy()
	start := time.Now()
	enqueued := false
	items := 0
	defer func() {
		ingestDurationSeconds.WithLabelValues(strategy).Observe(time.Since(start).Seconds())
		ingestDocumentsTotal.WithLabelValues(strategy, ingestOutcome(err, enqueued)).Inc()
		if items > 0 {
			ingestItemsTotal.WithLabelValues(strategy).Add(float64(items))
		}
	}()

	if cfg.UsesSummarizer() {
		items, enqueued, err = s.ingestWithSummary(ctx, workspaceID, doc, cfg)
		return err
	}
	chunks, ingestErr := ingestion.NewChunkStrategy(cfg.ChunkSize, cfg.ChunkOverlap).Ingest(ctx, doc)
	if ingestErr != nil {
		err = fmt.Errorf("ingest %q: %w", doc.URL, ingestErr)
		return err
	}
	items = len(chunks)
	err = s.saveItems(ctx, workspaceID, doc, chunks)
	return err
}

// ingestOutcome maps the IngestDocument result to a documents_total
// outcome label: error wins, then the async-enqueue path, else success.
func ingestOutcome(err error, enqueued bool) string {
	switch {
	case err != nil:
		return ingestOutcomeError
	case enqueued:
		return ingestOutcomeEnqueued
	default:
		return ingestOutcomeSuccess
	}
}

// ingestWithSummary routes summary / summaryThenChunk strategies. The agent
// backend enqueues raw text (async); extractive summarizes inline. When the
// agent backend is selected but no queue is wired, it falls back to extractive
// so ingestion still completes.
func (s *MemoryService) ingestWithSummary(ctx context.Context, workspaceID string, doc ingestion.SourceDoc, cfg ingestion.Config) (items int, enqueued bool, err error) {
	if cfg.Summarizer == ingestion.SummarizerAgent && s.summaryQueue != nil {
		err = s.summaryQueue.Enqueue(ctx, ingestion.WorkItem{
			WorkspaceID:  workspaceID,
			Doc:          doc,
			Strategy:     cfg.EffectiveStrategy(),
			ChunkSize:    cfg.ChunkSize,
			ChunkOverlap: cfg.ChunkOverlap,
			AboutKey:     doc.URL,
		})
		return 0, err == nil, err
	}
	if cfg.Summarizer == ingestion.SummarizerAgent {
		s.log.Info("agent summarizer unavailable; falling back to extractive",
			"reason", "no_queue", "url", doc.URL)
	}
	summary, err := ingestion.NewExtractiveSummarizer(0, 0).Summarize(ctx, doc.Text)
	if err != nil {
		return 0, false, fmt.Errorf("summarize %q: %w", doc.URL, err)
	}
	processed := ingestion.PostProcess(summary, cfg, doc)
	return len(processed), false, s.saveItems(ctx, workspaceID, doc, processed)
}

// saveItems persists each emitted item keyed by
// about={sharepoint_doc, "<url>#<index>"} so re-ingest supersedes per index.
// Shared by the sync ingest path and the async SaveDocumentSummary path.
func (s *MemoryService) saveItems(ctx context.Context, workspaceID string, doc ingestion.SourceDoc, items []ingestion.Item) error {
	for _, it := range items {
		meta := it.Metadata
		meta[memory.MetaKeyAboutKind] = aboutKindSharePointDoc
		meta[memory.MetaKeyAboutKey] = fmt.Sprintf("%s#%v", doc.URL, meta[ingestion.MetaKeyIndex])
		mem := &memory.Memory{
			Type:       "knowledge_reference",
			Content:    it.Content,
			Metadata:   meta,
			Confidence: 1.0,
			Scope:      map[string]string{memory.ScopeWorkspaceID: workspaceID},
		}
		if err := s.SaveInstitutionalMemory(ctx, mem); err != nil {
			return fmt.Errorf("save item %v for %q: %w", meta[ingestion.MetaKeyIndex], doc.URL, err)
		}
	}
	return nil
}

// resolveIngestionConfig returns the effective config: the flag fallback,
// overlaid field-by-field with the workspace's MemoryPolicy.spec.ingestion
// when a PolicyLoader is wired. A loader error or absent policy is fail-safe —
// ingestion proceeds on the fallback rather than erroring.
func (s *MemoryService) resolveIngestionConfig(ctx context.Context) ingestion.Config {
	cfg := s.ingestFallback
	if s.policyLoader == nil {
		return cfg
	}
	policy, err := s.policyLoader.Load(ctx)
	if err != nil || policy == nil {
		return cfg
	}
	if v := policy.IngestionStrategy(); v != "" {
		cfg.Strategy = v
	}
	if v := policy.IngestionSummarizer(); v != "" {
		cfg.Summarizer = v
	}
	if size, overlap, ok := policy.IngestionChunk(); ok {
		cfg.ChunkSize, cfg.ChunkOverlap = size, overlap
	}
	return cfg
}
