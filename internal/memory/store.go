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

package memory

import (
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/altairalabs/omnia/internal/memory/metakeys"
	coreproj "github.com/altairalabs/omnia/internal/memory/projection"
)

// Compile-time interface check.
var _ Store = (*PostgresMemoryStore)(nil)

// Scope key constants used in memory scope maps.
const (
	ScopeWorkspaceID = "workspace_id"
	// ScopeVirtualUserID is the per-subject scope key. The value is always a
	// pseudonym the caller supplies (never a real identity) — it is written
	// verbatim into the memory_entities.virtual_user_id column. The key name
	// intentionally matches that column so the contract is self-documenting.
	// See #1280.
	ScopeVirtualUserID = "virtual_user_id"
	// ScopeLegacyUserID is the pre-#1280 wire name for ScopeVirtualUserID.
	// memory-api still accepts it (query param and request-body scope key) for
	// one transition release, logging a deprecation warning; it is dropped the
	// release after.
	ScopeLegacyUserID = "user_id"
	ScopeAgentID      = "agent_id"
	// ScopeIncludeShared is a list-only control key. When set to scopeFlagTrue,
	// List returns everything visible to the user — institutional + agent
	// tiers plus the user's own — instead of strictly the user's own rows.
	// Other read paths (Retrieve, ExportAll/DSAR) ignore it, so user-private
	// scoping stays strict by construction. See #1254.
	ScopeIncludeShared = "include_shared"
	// scopeFlagTrue is the enabled value for boolean scope control keys.
	scopeFlagTrue = "true"
)

// Error message constants (SonarCloud S1192).
const (
	errWorkspaceRequired = "memory: workspace_id scope is required"
	errUserIDRequired    = "memory: user_id scope is required"
	// errBeginTxFormat is the fmt.Errorf format used when pgxpool.Begin
	// fails. Duplicated 3+ times across the store; extracted for S1192.
	errBeginTxFormat = "memory: begin tx: %w"
)

// ErrNotFound is returned by GetMemory when the requested entity
// doesn't exist in the scope. The HTTP handler maps it to 404.
var ErrNotFound = errors.New("memory: not found")

// SQL column/filter constants to avoid duplication (SonarCloud S1192).
const (
	colWorkspaceID   = "workspace_id=$?"
	colVirtualUserID = "virtual_user_id=$?"
	colEntityForgot  = "e.forgotten = false"
	entityKindFilter = "e.kind=$?"
	confidenceFilter = "o.confidence >= $?"
	// Active-observation filter: superseded rows AND valid_until-expired
	// rows both disappear from recall. The structured-key supersede path
	// (Save) sets valid_until = now() on the prior observation; the
	// resolution-event path (separate update/supersede tools) sets
	// superseded_by to the new observation's id. Either is sufficient
	// to hide the row.
	observationJoin = " JOIN memory_observations o ON o.entity_id = e.id" +
		" AND o.superseded_by IS NULL" +
		" AND (o.valid_until IS NULL OR o.valid_until > now())"
	entityTableAlias  = "e"
	selectEntityCols  = "e.id, e.kind, e.metadata, e.created_at, e.expires_at, e.title"
	selectObserveCols = "o.content, o.confidence, o.session_id, o.turn_range, o.observed_at, o.accessed_at, o.summary, o.body_size_bytes"
	// Multi-tier SELECT extras — extracted so the column list is named
	// once. Multi-tier needs the per-row scope columns to classify the
	// result into a Tier and the access count for the Go-side ranker;
	// single-tier doesn't surface either, which is why these aren't
	// folded into selectEntityCols / selectObserveCols.
	selectEntityScopeCols = "e.virtual_user_id, e.agent_id"
	// selectObserveColsMulti is selectObserveCols with access_count
	// inserted before the large-payload columns, matching the order
	// scanMultiTierRow expects. Kept adjacent to selectObserveCols
	// so a future change to either list is hard to make in only one.
	selectObserveColsMulti = "o.content, o.confidence, o.session_id, o.turn_range, o.observed_at, o.accessed_at, o.access_count, o.summary, o.body_size_bytes"
)

// PostgresMemoryStore implements Store against the memory_entities / memory_observations
// PostgreSQL tables created by the memory database initial schema migration.
type PostgresMemoryStore struct {
	pool        *pgxpool.Pool
	accessTouch *accessTouchBatcher
	// projector is the injected Memory Galaxy projection algorithm. nil in OSS
	// (the t-SNE projector is an enterprise feature); the enterprise binary
	// wires it via SetProjector so internal/memory never imports ee (#1669).
	projector coreproj.Projector
}

// NewPostgresMemoryStore creates a new store backed by the given connection pool.
// The store starts a debounced batcher goroutine that coalesces accessed_at
// updates from the recall hot path; call Close to stop it.
func NewPostgresMemoryStore(pool *pgxpool.Pool) *PostgresMemoryStore {
	s := &PostgresMemoryStore{pool: pool}
	s.accessTouch = newAccessTouchBatcher(s.runBatchedAccessUpdate)
	return s
}

// Close stops the background access-touch batcher. Pool ownership
// stays with the caller (memory-api) — Close only releases the
// store's own goroutines.
func (s *PostgresMemoryStore) Close() {
	if s.accessTouch != nil {
		s.accessTouch.Stop()
	}
}

// Pool returns the underlying connection pool. Test helpers reach in for
// direct DB inspection; production code goes through the typed methods.
func (s *PostgresMemoryStore) Pool() *pgxpool.Pool {
	return s.pool
}

// Metadata keys are defined in the leaf metakeys package (which imports
// nothing) so writer packages like internal/runtime can share the same wire
// contract without an import cycle. They are re-exported here as MetaKey* so
// existing internal/memory callers are unchanged.
//
//   - MetaKeyPurpose: read at insert time, written to memory_entities.purpose
//     so retrieval can filter without re-parsing JSON metadata.
//   - MetaKeyConsentCategory: written to memory_entities.consent_category so
//     the retention worker's consent revocation cascade can match grants.
//   - MetaKeyAboutKind / MetaKeyAboutKey: structured-dedup hint from
//     PromptKit's `about` parameter. When both are set, Save treats
//     (workspace, user, agent, about_kind, about_key) as a soft-unique key
//     and supersedes in place. Free-form writes leave both empty and fall
//     through to similarity-based dedup.
//   - MetaKeyTitle / MetaKeySummary: display fields for large memories so the
//     recall path can return a synopsis instead of the full body.
//   - MetaKeyBodySize: server-stamped octet length of the active observation.
const (
	MetaKeyPurpose         = metakeys.Purpose
	MetaKeyConsentCategory = metakeys.ConsentCategory
	MetaKeyAboutKind       = metakeys.AboutKind
	MetaKeyAboutKey        = metakeys.AboutKey
	MetaKeyTitle           = metakeys.Title
	MetaKeySummary         = metakeys.Summary
	MetaKeyBodySize        = metakeys.BodySize
)

// sourceTypeWeightSQL maps the schema's source_type strings to a
// confidence-style multiplier. The agent's explicit user_requested
// writes get full weight; passive conversation_extraction signals
// are discounted; system-generated rows are discounted further.
// Inlined as a CASE so it's free per row (no JOIN, no function call).
const sourceTypeWeightSQL = `
		CASE e.source_type
			WHEN 'user_requested'           THEN 1.0
			WHEN 'operator_curated'         THEN 1.0
			WHEN 'reflection'               THEN 0.85
			WHEN 'conversation_extraction'  THEN 0.7
			WHEN 'system_generated'         THEN 0.5
			ELSE 0.7
		END`

// recencyDecaySQL applies an exponential decay to observation age.
// Half-life is currently a single value (30 days = 2,592,000 s);
// per-tier half-lives (institutional / agent / user) live in
// MemoryPolicy.recall.halfLife and will replace this constant in a
// follow-up. exp(-age/half_life) gives 1.0 for fresh rows, 0.5 at
// the half-life, ~0 at 5×half-life.
const recencyDecaySQL = `
		exp(-EXTRACT(EPOCH FROM (now() - o.observed_at)) / 2592000.0)`

// exportPageSize bounds each DSAR export page. ExportAll loops over pages
// until a short page signals the end, so a data subject with more than one
// page still receives a *complete* export (SEC-7) — a truncated GDPR Art. 15
// export that looks complete is worse than a slow one.
const exportPageSize = 10000

// HNSW ef_search bounds. pgvector's default hnsw.ef_search is 40 — an HNSW
// scan returns at most ef_search candidates, then scope/active/confidence
// post-filtering thins those further, so the cosine arm silently under-returns
// (and the FULL OUTER JOIN with FTS masks it) as tenant count grows (PERF-3).
// We raise it per query to at least the cosine over-fetch, bounded to the
// range pgvector accepts.
const (
	minHNSWEFSearch = 100
	maxHNSWEFSearch = 1000
)

// hybridRetrieveSQL is the canonical RRF query template. Two ranked
// CTEs (FTS and cosine) are computed up to a fanout cap, joined via
// FULL OUTER JOIN so a memory present in either list still scores,
// then multiplied by the same source_type × confidence × recency
// quality multipliers used by the FTS-only path. Argument order:
//
//	$1  workspace_id (uuid)
//	$2  user_id      (text or NULL)
//	$3  agent_id     (uuid or NULL)
//	$4  query        (text)
//	$5  embedding    (pgvector)
//	$6  fanout       (int — candidates per ranker)
//	$7  limit        (int — final result cap)
//	$8  min_confidence (float — applied inside both CTEs)
const hybridRetrieveSQL = `
WITH fts AS (
    SELECT DISTINCT ON (e.id)
        e.id AS entity_id,
        ts_rank_cd(o.search_vector, websearch_to_tsquery('english', $4)) AS fts_rank
    FROM memory_entities e
    JOIN memory_observations o ON o.entity_id = e.id
        AND o.superseded_by IS NULL
        AND (o.valid_until IS NULL OR o.valid_until > now())
    WHERE e.workspace_id = $1
      AND ($2::text IS NULL OR e.virtual_user_id = $2)
      AND ($3::uuid IS NULL OR e.agent_id = $3)
      AND e.forgotten = false
      AND o.confidence >= $8
      AND o.search_vector @@ websearch_to_tsquery('english', $4)
    ORDER BY e.id, fts_rank DESC
    LIMIT $6
), fts_ranked AS (
    SELECT entity_id, row_number() OVER (ORDER BY fts_rank DESC) AS fts_rn
    FROM fts
), cosine_ann AS (
    -- Bare ORDER BY o.embedding <=> $5 LIMIT N over the base observations
    -- table so the HNSW index drives the scan. There is deliberately NO
    -- window function here: a row_number() OVER (PARTITION ...) in this
    -- SELECT forces the planner to materialise and sort the whole filtered
    -- set before the LIMIT, defeating the index (#1369). Over-fetch
    -- (fanout × 4) so the per-entity dedup below still lands $6 distinct
    -- entities even when one entity has many close-by observations.
    SELECT o.entity_id, o.embedding <=> $5 AS cos_dist
    FROM memory_observations o
    JOIN memory_entities e ON e.id = o.entity_id
        AND e.workspace_id = $1
        AND ($2::text IS NULL OR e.virtual_user_id = $2)
        AND ($3::uuid IS NULL OR e.agent_id = $3)
        AND e.forgotten = false
    WHERE o.superseded_by IS NULL
      AND (o.valid_until IS NULL OR o.valid_until > now())
      AND o.embedding IS NOT NULL
      AND o.confidence >= $8
    ORDER BY o.embedding <=> $5
    LIMIT $6 * 4
), cosine AS (
    -- Per-entity dedup over the bounded ANN candidate set, keeping each
    -- entity's nearest observation.
    SELECT DISTINCT ON (entity_id) entity_id, cos_dist
    FROM cosine_ann
    ORDER BY entity_id, cos_dist
), cosine_ranked AS (
    SELECT entity_id, row_number() OVER (ORDER BY cos_dist) AS cos_rn
    FROM cosine
    ORDER BY cos_dist
    LIMIT $6
), fused AS (
    -- Reciprocal Rank Fusion. The 60.0 constant is the k from
    -- Cormack 2009 reproduced by Weaviate, Vespa, OpenSearch,
    -- Elastic, et al. — larger flattens per-list contribution,
    -- smaller amplifies the top of each list.
    SELECT
        coalesce(f.entity_id, c.entity_id) AS entity_id,
        coalesce(1.0/(60.0 + f.fts_rn), 0)
          + coalesce(1.0/(60.0 + c.cos_rn), 0) AS rrf
    FROM fts_ranked f FULL OUTER JOIN cosine_ranked c USING (entity_id)
)
SELECT id, kind, metadata, created_at, expires_at, title,
       content, confidence, session_id, turn_range, observed_at, accessed_at,
       summary, body_size_bytes, final_score
FROM (
    -- DISTINCT ON requires ORDER BY to start with the distinct key.
    -- Per entity we pick the newest active observation (the active
    -- filter on the JOIN keeps observation count to ~1 in practice;
    -- this is the deterministic tiebreak in the rare race where
    -- two active observations briefly coexist). The outer query
    -- re-sorts the one-row-per-entity result set by final_score so
    -- LIMIT picks the top-K entities by score, not by entity id.
    SELECT DISTINCT ON (e.id)
        e.id, e.kind, e.metadata, e.created_at, e.expires_at, e.title,
        o.content, o.confidence, o.session_id, o.turn_range, o.observed_at, o.accessed_at,
        o.summary, o.body_size_bytes,
        fused.rrf
            * (CASE e.source_type
                  WHEN 'user_requested'           THEN 1.0
                  WHEN 'operator_curated'         THEN 1.0
                  WHEN 'reflection'               THEN 0.85
                  WHEN 'conversation_extraction'  THEN 0.7
                  WHEN 'system_generated'         THEN 0.5
                  ELSE 0.7 END)
            * o.confidence
            * exp(-EXTRACT(EPOCH FROM (now() - o.observed_at)) / 2592000.0) AS final_score
    FROM fused
    JOIN memory_entities e ON e.id = fused.entity_id
    JOIN memory_observations o ON o.entity_id = e.id
        AND o.superseded_by IS NULL
        AND (o.valid_until IS NULL OR o.valid_until > now())
    WHERE e.forgotten = false
    ORDER BY e.id, o.observed_at DESC
) ranked
ORDER BY final_score DESC
LIMIT $7`

// MissingEmbedding describes one observation that needs re-embedding.
// Returned by FindObservationsMissingEmbedding so the worker has the
// content to feed the provider plus the row identity to write back.
type MissingEmbedding struct {
	ObservationID string
	EntityID      string
	Content       string
}

// defaultMemoryLimit is applied when no explicit limit is provided.
const defaultMemoryLimit = 50
