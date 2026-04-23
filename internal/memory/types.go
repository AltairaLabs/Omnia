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

// Package memory provides the PostgreSQL-backed memory store for entity-relation-observation
// memory graphs. Core types (Memory, Store, RetrieveOptions, ListOptions, Extractor, Retriever)
// are re-exported from github.com/AltairaLabs/PromptKit/runtime/memory.
package memory

import (
	"context"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
)

// Re-export PromptKit memory types so existing callers can continue to use memory.Memory, etc.
type (
	Memory          = pkmemory.Memory
	RetrieveOptions = pkmemory.RetrieveOptions
	ListOptions     = pkmemory.ListOptions
)

// Store extends the PromptKit memory.Store interface with Omnia-specific methods.
// ExportAll is needed for DSAR (data subject access request) data export and is
// not part of the PromptKit SDK contract.
// BatchDelete is needed for paginated DSAR deletion (Task 4 cascade).
// RetrieveMultiTier runs a single query across institutional, agent, user and
// user-for-agent tiers and returns ranked results for RAG context injection.
// The three Institutional methods are the admin path for workspace-scoped
// memories (no user_id, no agent_id) — see institutional.go.
// The three AgentScoped methods mirror the institutional admin path but for
// (workspace, agent) rows (user_id IS NULL, agent_id = X) — see
// agent_scoped.go. They power operator-curated agent policies and training
// that should be visible to every user of a given agent.
type Store interface {
	pkmemory.Store
	ExportAll(ctx context.Context, scope map[string]string) ([]*Memory, error)
	BatchDelete(ctx context.Context, scope map[string]string, limit int) (int, error)
	RetrieveMultiTier(ctx context.Context, req MultiTierRequest) (*MultiTierResult, error)
	SaveInstitutional(ctx context.Context, mem *Memory) error
	ListInstitutional(ctx context.Context, workspaceID string, opts ListOptions) ([]*Memory, error)
	DeleteInstitutional(ctx context.Context, workspaceID, memoryID string) error
	SaveAgentScoped(ctx context.Context, mem *Memory) error
	ListAgentScoped(ctx context.Context, workspaceID, agentID string, opts ListOptions) ([]*Memory, error)
	DeleteAgentScoped(ctx context.Context, workspaceID, agentID, memoryID string) error

	// Compaction surface — exposed on the Store interface so a summarizer
	// agent can discover buckets and persist summaries via HTTP tools.
	// See docs/local-backlog/2026-04-23-memory-summarization-via-agent.md.
	FindCompactionCandidates(ctx context.Context, opts FindCompactionCandidatesOptions) ([]CompactionCandidate, error)
	SaveCompactionSummary(ctx context.Context, summary CompactionSummary) (string, error)
}
