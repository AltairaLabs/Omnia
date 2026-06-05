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
	"errors"
	"fmt"

	"github.com/altairalabs/omnia/internal/memory"
	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

// ErrNoIngestionStrategy is returned when IngestDocument is called before a
// strategy is configured.
var ErrNoIngestionStrategy = errors.New("no ingestion strategy configured")

// SetIngestionStrategy wires the document-ingestion strategy. Called once at
// startup; one strategy per service (the spectrum knob).
func (s *MemoryService) SetIngestionStrategy(strategy ingestion.IngestionStrategy) {
	s.ingestionStrategy = strategy
}

// IngestDocument runs the configured strategy over a source document and saves
// each emitted item as an institutional memory. Items are keyed by
// about={kind:"sharepoint_doc", key:"<url>#<index>"} so re-ingesting an
// unchanged doc supersedes rather than duplicates (idempotent re-seed).
// Embedding is left to the ReembedWorker — no inline embed.
func (s *MemoryService) IngestDocument(ctx context.Context, workspaceID string, doc ingestion.SourceDoc) error {
	if s.ingestionStrategy == nil {
		return ErrNoIngestionStrategy
	}
	items, err := s.ingestionStrategy.Ingest(ctx, doc)
	if err != nil {
		return fmt.Errorf("ingest %q: %w", doc.URL, err)
	}
	for _, it := range items {
		meta := it.Metadata
		meta[memory.MetaKeyAboutKind] = "sharepoint_doc"
		meta[memory.MetaKeyAboutKey] = fmt.Sprintf("%s#%v", doc.URL, meta["index"])
		mem := &memory.Memory{
			Type:       "knowledge_reference",
			Content:    it.Content,
			Metadata:   meta,
			Confidence: 1.0,
			Scope:      map[string]string{memory.ScopeWorkspaceID: workspaceID},
		}
		if err := s.SaveInstitutionalMemory(ctx, mem); err != nil {
			return fmt.Errorf("save item %v for %q: %w", meta["index"], doc.URL, err)
		}
	}
	return nil
}
