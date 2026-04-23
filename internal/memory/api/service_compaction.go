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

	"github.com/altairalabs/omnia/internal/memory"
)

// compactionScopeTag tags audit events on the compaction admin path so
// dashboards can distinguish summarizer writes from user / operator writes.
const compactionScopeTag = "compaction"

const (
	findCompactionCandidatesOp = "find_compaction_candidates"
	saveCompactionSummaryOp    = "save_compaction_summary"
)

// FindCompactionCandidates delegates to the store and emits an audit event
// tagged scope=compaction so dashboards can separate summarizer scans from
// user-facing retrieval activity.
func (s *MemoryService) FindCompactionCandidates(
	ctx context.Context, opts memory.FindCompactionCandidatesOptions,
) ([]memory.CompactionCandidate, error) {
	candidates, err := s.store.FindCompactionCandidates(ctx, opts)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: opts.WorkspaceID,
		Metadata: map[string]string{
			"scope":     compactionScopeTag,
			"operation": findCompactionCandidatesOp,
		},
	})
	return candidates, nil
}

// SaveCompactionSummary delegates to the store and emits a memory_created
// audit tagged scope=compaction. The sentinel memory.ErrCompactionRaced
// propagates unchanged so the HTTP handler can map it to 409 Conflict.
func (s *MemoryService) SaveCompactionSummary(
	ctx context.Context, summary memory.CompactionSummary,
) (string, error) {
	id, err := s.store.SaveCompactionSummary(ctx, summary)
	if err != nil {
		return "", err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    id,
		WorkspaceID: summary.WorkspaceID,
		Kind:        summary.Kind,
		Metadata: map[string]string{
			"scope":     compactionScopeTag,
			"operation": saveCompactionSummaryOp,
		},
	})
	return id, nil
}
