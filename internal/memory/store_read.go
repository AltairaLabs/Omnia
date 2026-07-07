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
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// Retrieve returns memories matching scope, a free-text query, and options.
//
// Query handling:
//   - Empty query: returns the most recent observation per entity, ordered
//     by observed_at DESC.
//   - Non-empty query: runs Postgres full-text search (websearch_to_tsquery
//     against the GENERATED search_vector column) and orders results by
//     ts_rank_cd. Stopwords ("my", "the", "is") are dropped, so a query
//     like "my name" matches a memory whose content is "User's name is X".
//
// A successful retrieval also fires a detached UPDATE that bumps
// accessed_at / access_count on the returned entities — the signal LRU
// pruning and recency-weighted ranking depend on.
func (s *PostgresMemoryStore) Retrieve(ctx context.Context, scope map[string]string, query string, opts RetrieveOptions) ([]*Memory, error) {
	if scope[ScopeWorkspaceID] == "" {
		return nil, errors.New(errWorkspaceRequired)
	}

	sql, qb := buildRetrieveQuery(scope, query, opts)

	rows, err := s.pool.Query(ctx, sql, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("memory: retrieve query: %w", err)
	}
	defer rows.Close()

	mems, err := scanMemories(rows, scope)
	if err != nil {
		return nil, err
	}
	s.touchAccessedOnRead(entityIDsFromMemories(mems))
	return mems, nil
}

// List returns memories filtered by scope and options with pagination.
func (s *PostgresMemoryStore) List(ctx context.Context, scope map[string]string, opts ListOptions) ([]*Memory, error) {
	if scope[ScopeWorkspaceID] == "" {
		return nil, errors.New(errWorkspaceRequired)
	}

	sql, qb := buildListQuery(scope, opts)

	rows, err := s.pool.Query(ctx, sql, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("memory: list query: %w", err)
	}
	defer rows.Close()

	// Visible-to-me returns mixed tiers, so it selects per-row scope columns
	// and must be scanned accordingly to derive each row's real tier (#1254).
	if scope[ScopeIncludeShared] == scopeFlagTrue {
		return scanVisibleToMeMemories(rows, scope[ScopeWorkspaceID])
	}
	return scanMemories(rows, scope)
}

// ExportAll returns every memory for a scope (DSAR export), paginating
// internally until exhausted. formatMemorySQL orders by the unique entity id,
// so offset paging is stable across pages.
func (s *PostgresMemoryStore) ExportAll(ctx context.Context, scope map[string]string) ([]*Memory, error) {
	return s.exportAllPaged(ctx, scope, exportPageSize)
}

// exportAllPaged is ExportAll with an explicit page size so tests can exercise
// the multi-page loop without seeding exportPageSize rows.
func (s *PostgresMemoryStore) exportAllPaged(ctx context.Context, scope map[string]string, pageSize int) ([]*Memory, error) {
	if scope[ScopeWorkspaceID] == "" {
		return nil, errors.New(errWorkspaceRequired)
	}

	var all []*Memory
	for offset := 0; ; offset += pageSize {
		page, err := s.exportPage(ctx, scope, pageSize, offset)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			break
		}
	}
	return all, nil
}

// exportPage reads one page of the export. A fresh QueryBuilder per page is
// required because AppendPagination mutates the builder's args.
func (s *PostgresMemoryStore) exportPage(ctx context.Context, scope map[string]string, pageSize, offset int) ([]*Memory, error) {
	qb := buildBaseMemoryQuery(scope, nil, "")
	sql := formatMemorySQL(qb, pageSize, offset)
	rows, err := s.pool.Query(ctx, sql, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("memory: export all query: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows, scope)
}

// GetMemory returns the entity by ID with its current active
// observation. The active filter mirrors recall — superseded /
// expired observations are excluded. Returns ErrNotFound when
// nothing matches in scope. Used by memory__open to fetch the full
// content of large memories that recall returned only summarised.
func (s *PostgresMemoryStore) GetMemory(ctx context.Context, scope map[string]string, entityID string) (*Memory, error) {
	if scope[ScopeWorkspaceID] == "" {
		return nil, errors.New(errWorkspaceRequired)
	}

	row := s.pool.QueryRow(ctx, `
		SELECT e.id, e.kind, e.metadata, e.created_at, e.expires_at, e.title,
		       o.content, o.confidence, o.session_id, o.turn_range,
		       o.observed_at, o.accessed_at,
		       o.summary, o.body_size_bytes
		FROM memory_entities e
		JOIN memory_observations o ON o.entity_id = e.id
		  AND o.superseded_by IS NULL
		  AND (o.valid_until IS NULL OR o.valid_until > now())
		WHERE e.id = $1
		  AND e.workspace_id = $2
		  AND e.virtual_user_id IS NOT DISTINCT FROM $3::text
		  AND ($4::uuid IS NULL OR e.agent_id = $4)
		  AND NOT e.forgotten
		ORDER BY o.observed_at DESC
		LIMIT 1`,
		entityID,
		scope[ScopeWorkspaceID],
		scopeOrNil(scope, ScopeVirtualUserID),
		scopeOrNil(scope, ScopeAgentID),
	)

	mem, err := scanSingleMemory(row, scope)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("memory: get memory: %w", err)
	}
	return mem, nil
}

// FindRelatedEntities returns outgoing memory_relations rows from
// the given source entity IDs, capped at maxPerEntity per source.
// Used by recall enrichment so each returned memory carries a
// `related[]` list — the agent uses these refs to navigate the
// memory graph (preferences → user identity, derived facts →
// anchors).
func (s *PostgresMemoryStore) FindRelatedEntities(
	ctx context.Context,
	scope map[string]string,
	entityIDs []string,
	maxPerEntity int,
) ([]EntityRelation, error) {
	if scope[ScopeWorkspaceID] == "" {
		return nil, errors.New(errWorkspaceRequired)
	}
	if len(entityIDs) == 0 {
		return nil, nil
	}
	if maxPerEntity <= 0 {
		maxPerEntity = 3
	}

	// Per-source LIMIT via window function. Returns rows ordered by
	// weight DESC then created_at DESC so the most-relevant relations
	// surface when the cap clips.
	rows, err := s.pool.Query(ctx, `
		WITH ranked AS (
			SELECT source_entity_id, target_entity_id, relation_type,
			       coalesce(weight, 1.0) AS w,
			       row_number() OVER (
			           PARTITION BY source_entity_id
			           ORDER BY coalesce(weight, 1.0) DESC, created_at DESC
			       ) AS rn
			FROM memory_relations
			WHERE workspace_id = $1
			  AND source_entity_id = ANY($2)
			  AND (expires_at IS NULL OR expires_at > now())
		)
		SELECT source_entity_id, target_entity_id, relation_type, w
		FROM ranked
		WHERE rn <= $3`,
		scope[ScopeWorkspaceID], entityIDs, maxPerEntity,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: find related: %w", err)
	}
	defer rows.Close()

	var out []EntityRelation
	for rows.Next() {
		var rel EntityRelation
		if err := rows.Scan(&rel.SourceEntityID, &rel.TargetEntityID,
			&rel.RelationType, &rel.Weight); err != nil {
			return nil, fmt.Errorf("memory: find related scan: %w", err)
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}

// FindSimilarObservations returns active observations under the
// scope whose embedding's cosine similarity to queryEmbedding is at
// least minSimilarity, ordered most-similar first. Limited to k
// results. Used by the dedup-on-write path: SaveMemoryWithResult
// embeds the new content and either auto-supersedes a high-
// similarity match (≥0.95) or surfaces mid-similarity matches
// (≥0.85) as PotentialDuplicates.
//
// Scope filtering matches the recall path: workspace + virtual_user
// (when set) + agent (when set). Observations without an embedding
// are skipped — the caller has no signal to dedup against, and the
// embedding service will fill them in async (next cosine run will
// catch them).
func (s *PostgresMemoryStore) FindSimilarObservations(
	ctx context.Context,
	scope map[string]string,
	queryEmbedding []float32,
	k int,
	minSimilarity float64,
) ([]SimilarObservation, error) {
	if scope[ScopeWorkspaceID] == "" {
		return nil, errors.New(errWorkspaceRequired)
	}
	if k <= 0 {
		k = DefaultDuplicateCandidateLimit
	}

	// pgvector cosine distance is in [0, 2]; cosine similarity is
	// (1 - distance) clamped to [-1, 1]. minSimilarity converts to a
	// max-distance bound for indexed range filtering.
	maxDistance := 1.0 - minSimilarity

	var out []SimilarObservation
	// PERF-3: raise hnsw.ef_search to at least k so the HNSW scan can return
	// the k candidates the query asks for.
	err := s.withHNSWEFSearch(ctx, clampEFSearch(k), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT o.id, e.id, o.content,
			       1 - (o.embedding <=> $1) AS similarity
			FROM memory_entities e
			JOIN memory_observations o ON o.entity_id = e.id
			WHERE e.workspace_id = $2
			  AND ($3::text IS NULL OR e.virtual_user_id = $3)
			  AND ($4::uuid IS NULL OR e.agent_id = $4)
			  AND NOT e.forgotten
			  AND o.superseded_by IS NULL
			  AND (o.valid_until IS NULL OR o.valid_until > now())
			  AND o.embedding IS NOT NULL
			  AND o.embedding <=> $1 <= $5
			ORDER BY o.embedding <=> $1
			LIMIT $6`,
			pgvector.NewVector(queryEmbedding),
			scope[ScopeWorkspaceID],
			scopeOrNil(scope, ScopeVirtualUserID),
			scopeOrNil(scope, ScopeAgentID),
			maxDistance,
			k,
		)
		if err != nil {
			return fmt.Errorf("memory: similar observations: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var m SimilarObservation
			if err := rows.Scan(&m.ObservationID, &m.EntityID, &m.Content, &m.Similarity); err != nil {
				return fmt.Errorf("memory: similar scan: %w", err)
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// clampEFSearch returns an hnsw.ef_search large enough to surface `want`
// candidates from the HNSW scan, bounded to pgvector's accepted range.
func clampEFSearch(want int) int {
	switch {
	case want < minHNSWEFSearch:
		return minHNSWEFSearch
	case want > maxHNSWEFSearch:
		return maxHNSWEFSearch
	default:
		return want
	}
}

// withHNSWEFSearch runs fn inside a transaction that first raises
// hnsw.ef_search to efSearch via set_config(..., is_local => true), so the
// HNSW scan returns enough candidates for the cosine over-fetch to survive
// post-filtering (PERF-3). The setting is scoped to this transaction only;
// fn must run its query and scan against the supplied tx before it returns,
// since the rows are tied to the transaction's lifetime.
func (s *PostgresMemoryStore) withHNSWEFSearch(ctx context.Context, efSearch int, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("memory: begin ef_search tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT set_config('hnsw.ef_search', $1, true)", strconv.Itoa(efSearch)); err != nil {
		return fmt.Errorf("memory: set hnsw.ef_search: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RetrieveHybrid runs the hybrid (lexical + semantic) recall path
// described on the Store interface. See hybridRetrieveSQL for the
// query shape and rrfK for the fusion constant.
//
// When the query text is empty there is nothing for FTS to match on,
// and when queryEmbedding is empty there is nothing for cosine to
// score; both cases fall through to plain Retrieve so callers don't
// need to special-case them.
func (s *PostgresMemoryStore) RetrieveHybrid(
	ctx context.Context,
	scope map[string]string,
	query string,
	queryEmbedding []float32,
	opts RetrieveOptions,
) ([]*Memory, error) {
	if scope[ScopeWorkspaceID] == "" {
		return nil, errors.New(errWorkspaceRequired)
	}
	if query == "" || len(queryEmbedding) == 0 {
		return s.Retrieve(ctx, scope, query, opts)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultMemoryLimit
	}
	fanout := limit * 5
	if fanout < 50 {
		fanout = 50
	}
	minConfidence := opts.MinConfidence
	if minConfidence < 0 {
		minConfidence = 0
	}

	// PERF-3: the cosine CTE over-fetches fanout×4 from the HNSW index, so
	// raise hnsw.ef_search to match or the index caps candidates at its
	// default (40) before post-filtering.
	var mems []*Memory
	err := s.withHNSWEFSearch(ctx, clampEFSearch(fanout*4), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, hybridRetrieveSQL,
			scope[ScopeWorkspaceID],
			scopeOrNil(scope, ScopeVirtualUserID),
			scopeOrNil(scope, ScopeAgentID),
			query,
			pgvector.NewVector(queryEmbedding),
			fanout,
			limit,
			minConfidence,
		)
		if err != nil {
			return fmt.Errorf("memory: hybrid retrieve: %w", err)
		}
		defer rows.Close()

		mems, err = scanHybridMemories(rows, scope)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.touchAccessedOnRead(entityIDsFromMemories(mems))
	return mems, nil
}

// FindObservationsMissingEmbedding returns active observations that
// either have no embedding at all or were embedded with a different
// model name. Used by the re-embed worker on startup and on a slow
// ticker to backfill rows that pre-date the embedding wiring or
// were stamped by a now-superseded model. Bounded by limit so the
// worker can stream the catalogue without loading it all at once.
func (s *PostgresMemoryStore) FindObservationsMissingEmbedding(
	ctx context.Context, currentModel string, limit int,
) ([]MissingEmbedding, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, o.entity_id, o.content
		FROM memory_observations o
		JOIN memory_entities e ON e.id = o.entity_id AND e.forgotten = false
		WHERE o.superseded_by IS NULL
		  AND (o.valid_until IS NULL OR o.valid_until > now())
		  AND (o.embedding IS NULL
		       OR ($1 <> '' AND coalesce(o.embedding_model, '') <> $1))
		ORDER BY o.observed_at
		LIMIT $2`, currentModel, limit)
	if err != nil {
		return nil, fmt.Errorf("memory: find missing embeddings: %w", err)
	}
	defer rows.Close()

	var out []MissingEmbedding
	for rows.Next() {
		var m MissingEmbedding
		if err := rows.Scan(&m.ObservationID, &m.EntityID, &m.Content); err != nil {
			return nil, fmt.Errorf("memory: scan missing embedding: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountObservationsMissingEmbedding returns how many active observations in a
// workspace either have no embedding or were embedded with a different model
// name — the re-embed worker's per-workspace backlog depth. Same predicate as
// FindObservationsMissingEmbedding, scoped to one workspace and aggregated so
// the metrics collector can poll it cheaply (#1442).
func (s *PostgresMemoryStore) CountObservationsMissingEmbedding(
	ctx context.Context, workspaceID, currentModel string,
) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM memory_observations o
		JOIN memory_entities e ON e.id = o.entity_id AND e.forgotten = false
		WHERE e.workspace_id = $1
		  AND o.superseded_by IS NULL
		  AND (o.valid_until IS NULL OR o.valid_until > now())
		  AND (o.embedding IS NULL
		       OR ($2 <> '' AND coalesce(o.embedding_model, '') <> $2))`,
		workspaceID, currentModel).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("memory: count missing embeddings: %w", err)
	}
	return n, nil
}

// ListWorkspaceIDs returns the workspace IDs the workers (compaction,
// tombstone GC, retention, re-embed) iterate per tick. Reads from the
// memory_workspaces registry table maintained by the
// memory_entities_track_workspace trigger added in migration 000008,
// then filters to ones still holding at least one non-forgotten
// entity via EXISTS — the contract is "workspaces with live data."
//
// Bounded by the registry size, not the 1M-entity scale of
// memory_entities — `SELECT DISTINCT workspace_id FROM
// memory_entities` was a seq-scan + hash-aggregate every tick.
// Each EXISTS probe hits the (workspace_id, ...) entity index and
// returns immediately once one unforgotten row is found; only
// fully-dead workspaces pay the worst case.
func (s *PostgresMemoryStore) ListWorkspaceIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT w.workspace_id
		FROM memory_workspaces w
		WHERE EXISTS (
			SELECT 1 FROM memory_entities e
			WHERE e.workspace_id = w.workspace_id
			  AND NOT e.forgotten
			LIMIT 1
		)
		ORDER BY w.workspace_id`)
	if err != nil {
		return nil, fmt.Errorf("memory: list workspace ids: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var ws string
		if err := rows.Scan(&ws); err != nil {
			return nil, fmt.Errorf("memory: scan workspace id: %w", err)
		}
		out = append(out, ws)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: iterate workspace ids: %w", err)
	}
	return out, nil
}
