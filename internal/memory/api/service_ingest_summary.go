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

	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

// ErrSummaryQueueDisabled is returned by the agent work-queue endpoints when
// no --ingest-queue-dir is configured (the agent path is off).
var ErrSummaryQueueDisabled = errors.New("summary queue not configured")

// ListSummaryCandidates returns up to limit pending document-summary work
// items for the summarizer agent. Empty (not an error) when the queue is off.
func (s *MemoryService) ListSummaryCandidates(ctx context.Context, limit int) ([]ingestion.WorkItem, error) {
	if s.summaryQueue == nil {
		return nil, nil
	}
	return s.summaryQueue.List(ctx, limit)
}

// SaveDocumentSummary stores the agent-produced summary for a pending work
// item, then completes (deletes) the work item. The work item carries the
// strategy + chunk geometry captured at enqueue, so completion is
// deterministic. Returns the number of stored items.
func (s *MemoryService) SaveDocumentSummary(ctx context.Context, workspaceID, aboutKey, summary string) (int, error) {
	if s.summaryQueue == nil {
		return 0, ErrSummaryQueueDisabled
	}
	item, err := s.summaryQueue.Get(ctx, workspaceID, aboutKey)
	if err != nil {
		return 0, err // ErrWorkItemNotFound surfaces as 404
	}
	cfg := ingestion.Config{
		Strategy:     item.Strategy,
		ChunkSize:    item.ChunkSize,
		ChunkOverlap: item.ChunkOverlap,
	}
	items := ingestion.PostProcess(summary, cfg, item.Doc)
	if err := s.saveItems(ctx, workspaceID, item.Doc, items); err != nil {
		return 0, fmt.Errorf("store summary items for %q: %w", aboutKey, err)
	}
	if err := s.summaryQueue.Complete(ctx, workspaceID, aboutKey); err != nil {
		return 0, fmt.Errorf("complete work item %q: %w", aboutKey, err)
	}
	return len(items), nil
}
