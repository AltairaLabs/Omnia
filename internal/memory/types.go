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

// SaveAction enumerates how the server resolved a Save call. The
// agent uses this to phrase its reply ("Got it" vs "Updated your
// name from X to Y") and to decide whether to call memory__update
// next turn.
type SaveAction string

const (
	// SaveActionAdded — new entity created, no conflict.
	SaveActionAdded SaveAction = "added"
	// SaveActionAutoSuperseded — server detected a conflict (via the
	// structured `about={kind, key}` index or via embedding cosine
	// above the auto threshold) and superseded the prior
	// observation(s) under the existing entity in the same write.
	SaveActionAutoSuperseded SaveAction = "auto_superseded"
)

// SaveSupersedeReason names the dedup mechanism that fired.
type SaveSupersedeReason string

const (
	// ReasonStructuredKey — `about_kind` + `about_key` matched an
	// existing entity; ON CONFLICT path supersedes deterministically.
	ReasonStructuredKey SaveSupersedeReason = "structured_key"
	// ReasonHighSimilarity — embedding cosine ≥ the auto threshold;
	// the server considers it a near-duplicate.
	ReasonHighSimilarity SaveSupersedeReason = "high_similarity"
)

// DuplicateCandidate is one of the mid-similarity observations
// surfaced to the agent in SaveResult.PotentialDuplicates. The
// agent decides whether to call memory__update on a later turn.
type DuplicateCandidate struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Similarity float64 `json:"similarity"`
}

// EntityRelation is the store-level row returned by
// FindRelatedEntities — one outgoing edge from the source entity
// in memory_relations. The recall enrichment path collects these
// per memory and serialises them as `related[]` in the response so
// the agent can navigate the memory graph.
type EntityRelation struct {
	SourceEntityID string  `json:"source_entity_id"`
	TargetEntityID string  `json:"target_entity_id"`
	RelationType   string  `json:"relation_type"`
	Weight         float64 `json:"weight"`
}

// SimilarObservation is the store-level match returned by
// FindSimilarObservations. The service layer turns it into either
// an auto-supersede (≥ AutoSupersedeThreshold) or a
// DuplicateCandidate (≥ SurfaceDuplicateThreshold).
type SimilarObservation struct {
	ObservationID string
	EntityID      string
	Content       string
	Similarity    float64
}

// Default similarity thresholds. Configurable later via
// MemoryPolicy.dedup.embeddingSimilarity. The 0.95 / 0.85 split
// errs on the side of *not* auto-superseding — surprising silent
// merges of distinct facts are worse than one extra observation.
const (
	DefaultAutoSupersedeSimilarity    = 0.95
	DefaultSurfaceDuplicateSimilarity = 0.85
	DefaultDuplicateCandidateLimit    = 5
)

// SaveResult is the rich response returned by SaveWithResult,
// surfacing what the server's dedup pipeline did. Older callers
// using the plain pkmemory.Store.Save signature get just an error
// and never see this — that path is for compatibility.
type SaveResult struct {
	// ID is the entity ID the new observation lives under (may be
	// existing, may be newly created).
	ID string `json:"id"`
	// Action describes the high-level outcome.
	Action SaveAction `json:"action"`
	// SupersededObservationIDs are observation IDs the server marked
	// inactive in this same write. Populated only when
	// Action == SaveActionAutoSuperseded.
	SupersededObservationIDs []string `json:"supersedes,omitempty"`
	// SupersedeReason is set alongside SupersededObservationIDs.
	SupersedeReason SaveSupersedeReason `json:"supersede_reason,omitempty"`
	// PotentialDuplicates are mid-similarity candidates for the agent
	// to consider on a later turn. Empty when no embedding service
	// is configured or nothing crossed the surface threshold.
	PotentialDuplicates []DuplicateCandidate `json:"potential_duplicates,omitempty"`
}

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
	// SaveWithResult is the rich Omnia write API. The agent's
	// memory__remember handler calls this so it can return action /
	// supersedes / potential_duplicates info up to the agent. The
	// PromptKit-compatible Save method on this same store is a
	// backwards-compatible wrapper that discards the result.
	SaveWithResult(ctx context.Context, mem *Memory) (*SaveResult, error)

	// FindSimilarObservations powers the embedding-similarity dedup
	// path: SaveMemoryWithResult uses it to decide whether a free-form
	// remember is a near-duplicate of something already stored.
	FindSimilarObservations(ctx context.Context, scope map[string]string,
		queryEmbedding []float32, k int, minSimilarity float64) ([]SimilarObservation, error)

	// AppendObservationToEntity attaches a new observation to an
	// existing entity and supersedes that entity's prior active
	// observations atomically. Used by the embedding-similarity
	// auto-supersede path; not needed by structured-key dedup which
	// has its own ON CONFLICT route inside SaveWithResult.
	AppendObservationToEntity(ctx context.Context, entityID string, mem *Memory) ([]string, error)

	// GetMemory returns a single memory by entity ID with its current
	// active observation (full content; no inline-vs-summary
	// truncation). Returns ErrNotFound when the entity doesn't exist
	// in the scope. Powers the memory__open tool.
	GetMemory(ctx context.Context, scope map[string]string, entityID string) (*Memory, error)

	// LinkEntities inserts a directed edge in memory_relations so
	// derived facts attached via relationType ("ABOUT", "MENTIONS",
	// etc.) survive renames of the target entity. weight defaults to
	// 1.0 when zero. Returns the relation ID.
	LinkEntities(ctx context.Context, scope map[string]string,
		sourceEntityID, targetEntityID, relationType string, weight float64) (string, error)

	// FindRelatedEntities returns the memory_relations rows whose
	// source_entity_id is in entityIDs, capped at maxPerEntity per
	// source. Used by the recall enrichment path to surface a per-
	// memory related[] list — the agent uses these refs to know
	// which memories share an entity (the user identity, a project,
	// etc.) so it can decide updates and supersessions correctly.
	FindRelatedEntities(ctx context.Context, scope map[string]string,
		entityIDs []string, maxPerEntity int) ([]EntityRelation, error)

	// RetrieveHybrid runs a hybrid lexical + semantic recall and
	// returns memories ordered by Reciprocal Rank Fusion of FTS
	// rank and pgvector cosine rank, then multiplied by the standard
	// source_type / confidence / recency quality weights.
	//
	// Both rankers are computed independently up to a fanout cap
	// (defaulting to max(opts.Limit*5, 50)) and merged via RRF with
	// k=60. A memory that scores in either list contributes to the
	// final ranking — semantic-only matches surface even when the
	// query text isn't a lexical hit, and vice versa.
	//
	// queryEmbedding must match the dimensionality of stored
	// embeddings. Callers without an embedding provider should
	// fall back to plain Retrieve.
	RetrieveHybrid(ctx context.Context, scope map[string]string,
		query string, queryEmbedding []float32, opts RetrieveOptions) ([]*Memory, error)
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
