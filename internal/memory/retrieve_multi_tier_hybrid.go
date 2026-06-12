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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"

	"github.com/altairalabs/omnia/internal/pgutil"
)

// hybridFanout is the per-ranker candidate cap fed to the FTS and cosine
// rank lists before RRF fusion. Multi-tier truncates Go-side to req.Limit
// after ranking, so this only needs to be wide enough to feed the fusion.
const hybridFanout = 100

// rrfK is the Reciprocal Rank Fusion constant (Cormack 2009; k=60),
// matching hybridRetrieveSQL's single-tier path.
const rrfK = 60.0

// hybridMultiTierTemplate is the RRF-over-tiers SQL. A single `candidates`
// CTE applies the shared filters (workspace, not-forgotten, tier
// NULL-anchoring, type, purpose, confidence) and the active-observation join
// ONCE; the FTS and cosine rank lists then both select from it, so a filter
// added to one ranker can't drift from the other. The two ranked lists are
// fused via RRF (FULL OUTER JOIN) and multiplied by the same
// source_type × confidence × recency quality weights used by the FTS path.
//
// Sprintf slots: %[1]s candidates WHERE, %[2]s RRF expression,
// $%[3]d query text, $%[4]d embedding, $%[5]d fanout.
const hybridMultiTierTemplate = `
WITH candidates AS (
    SELECT e.id AS entity_id, e.kind, e.metadata, e.created_at, e.expires_at,
           e.title, e.virtual_user_id, e.agent_id, e.source_type,
           o.content, o.confidence, o.session_id, o.turn_range, o.observed_at,
           o.accessed_at, o.access_count, o.summary, o.body_size_bytes,
           o.search_vector, o.embedding
    FROM memory_entities e
    JOIN memory_observations o ON o.entity_id = e.id
        AND o.superseded_by IS NULL
        AND (o.valid_until IS NULL OR o.valid_until > now())
    WHERE %[1]s
), fts AS (
    SELECT DISTINCT ON (entity_id) entity_id,
           ts_rank_cd(search_vector, websearch_to_tsquery('english', $%[3]d)) AS fts_rank
    FROM candidates
    WHERE search_vector @@ websearch_to_tsquery('english', $%[3]d)
    ORDER BY entity_id, fts_rank DESC
    LIMIT $%[5]d
), fts_ranked AS (
    SELECT entity_id, row_number() OVER (ORDER BY fts_rank DESC) AS fts_rn FROM fts
), cosine AS (
    -- The inner ORDER BY embedding <=> $ LIMIT N unlocks the HNSW index;
    -- over-fetch (fanout × 4) so per-entity dedup still lands enough distinct
    -- entities when one entity has several close-by observations.
    SELECT entity_id, cos_dist FROM (
        SELECT entity_id, embedding <=> $%[4]d AS cos_dist,
               row_number() OVER (PARTITION BY entity_id ORDER BY embedding <=> $%[4]d) AS rn
        FROM candidates
        WHERE embedding IS NOT NULL
        ORDER BY embedding <=> $%[4]d
        LIMIT $%[5]d * 4
    ) ann WHERE rn = 1 LIMIT $%[5]d
), cosine_ranked AS (
    SELECT entity_id, row_number() OVER (ORDER BY cos_dist) AS cos_rn FROM cosine
), fused AS (
    SELECT coalesce(f.entity_id, c.entity_id) AS entity_id, %[2]s AS rrf
    FROM fts_ranked f FULL OUTER JOIN cosine_ranked c USING (entity_id)
)
SELECT DISTINCT ON (c.entity_id)
    c.entity_id, c.kind, c.metadata, c.created_at, c.expires_at, c.title,
    c.virtual_user_id, c.agent_id,
    c.content, c.confidence, c.session_id, c.turn_range, c.observed_at,
    c.accessed_at, c.access_count, c.summary, c.body_size_bytes,
    fused.rrf
        * (CASE c.source_type
              WHEN 'user_requested'          THEN 1.0
              WHEN 'operator_curated'        THEN 1.0
              WHEN 'reflection'              THEN 0.85
              WHEN 'conversation_extraction' THEN 0.7
              WHEN 'system_generated'        THEN 0.5
              ELSE 0.7 END)
        * coalesce(c.confidence, 0.7)
        * exp(-EXTRACT(EPOCH FROM (now() - c.observed_at)) / 2592000.0) AS final_score
FROM candidates c JOIN fused ON fused.entity_id = c.entity_id
ORDER BY c.entity_id, c.observed_at DESC`

// buildMultiTierHybridQuery constructs the RRF-over-tiers SQL and its
// positional args. The shared filter predicates are built once via the same
// QueryBuilder helpers the FTS-only multi-tier path uses; the FTS @@ predicate
// is intentionally NOT added here (it lives inside the fts CTE so cosine-only
// matches still surface). The trailing args after the builder's are: query
// text, embedding (placeholder — RetrieveMultiTierHybrid overwrites it), and
// the fanout cap.
func buildMultiTierHybridQuery(req MultiTierRequest) (string, []any, error) {
	if req.WorkspaceID == "" {
		return "", nil, errors.New(errWorkspaceRequired)
	}

	var qb pgutil.QueryBuilder
	qb.Add("e.workspace_id=$?", req.WorkspaceID)
	addUserTierClause(&qb, req.UserID)
	addAgentTierClause(&qb, req.AgentID)
	addTypeFilters(&qb, req.Types)
	addConfidenceFilter(&qb, req.MinConfidence)
	addPurposeFilters(&qb, req.Purposes)

	args := qb.Args()
	qIdx := len(args) + 1
	eIdx := len(args) + 2
	fIdx := len(args) + 3
	args = append(args, req.Query, pgvector.NewVector(nil), hybridFanout)

	rrfExpr := fmt.Sprintf(
		"coalesce(1.0/(%g + f.fts_rn), 0) + coalesce(1.0/(%g + c.cos_rn), 0)",
		rrfK, rrfK,
	)

	// QueryBuilder.Where() returns a leading " AND ", so prefix the literal
	// forgotten predicate as the first WHERE term (mirrors buildMultiTierQuery).
	where := colEntityForgot + qb.Where()
	sql := fmt.Sprintf(hybridMultiTierTemplate, where, rrfExpr, qIdx, eIdx, fIdx)
	return sql, args, nil
}

// RetrieveMultiTierHybrid — see the Store interface. When the query or the
// embedding is empty there is nothing to fuse, so it falls through to the
// FTS-only RetrieveMultiTier (which the callers also do on embed failure).
func (s *PostgresMemoryStore) RetrieveMultiTierHybrid(ctx context.Context, req MultiTierRequest, queryEmbedding []float32) (*MultiTierResult, error) {
	if req.Query == "" || len(queryEmbedding) == 0 {
		return s.RetrieveMultiTier(ctx, req)
	}

	sql, args, err := buildMultiTierHybridQuery(req)
	if err != nil {
		return nil, err
	}
	// args[eIdx-1] is the embedding placeholder appended by the builder.
	args[len(args)-2] = pgvector.NewVector(queryEmbedding)

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: multi-tier hybrid query: %w", err)
	}
	defer rows.Close()

	memories, err := scanMultiTierHybridRows(rows, req.WorkspaceID)
	if err != nil {
		return nil, err
	}

	memories, err = s.mergeMultiMode(ctx, req, memories)
	if err != nil {
		return nil, err
	}

	rankHybridResults(memories, req.Ranker)

	limit := req.Limit
	if limit <= 0 {
		limit = defaultMultiTierLimit
	}
	if len(memories) > limit {
		memories = memories[:limit]
	}
	s.touchAccessedOnRead(entityIDsFromMultiTier(memories))

	return &MultiTierResult{Memories: memories, Total: len(memories)}, nil
}

// scanMultiTierHybridRows reads the hybrid multi-tier row set — identical to
// the FTS-only multi-tier columns plus a trailing final_score the SQL has
// already folded RRF × source × confidence × recency into.
func scanMultiTierHybridRows(rows pgx.Rows, workspaceID string) ([]*MultiTierMemory, error) {
	var results []*MultiTierMemory
	for rows.Next() {
		mem, err := scanMultiTierHybridRow(rows, workspaceID)
		if err != nil {
			return nil, err
		}
		results = append(results, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: multi-tier hybrid rows iteration: %w", err)
	}
	if results == nil {
		results = []*MultiTierMemory{}
	}
	return results, nil
}

// scanMultiTierHybridRow mirrors scanMultiTierRow with one extra trailing
// float64 (final_score) which seeds the MultiTierMemory.Score.
func scanMultiTierHybridRow(row pgx.Rows, workspaceID string) (*MultiTierMemory, error) {
	var (
		mem            Memory
		metadataJSON   []byte
		expiresAt      *time.Time
		userID         *string
		agentID        *string
		sessionID      *string
		turnRange      []int
		observedAt     *time.Time
		accessedAt     *time.Time
		accessCount    int
		title, summary *string
		bodySizeBytes  *int32
		finalScore     float64
	)

	err := row.Scan(
		&mem.ID, &mem.Type, &metadataJSON, &mem.CreatedAt, &expiresAt, &title,
		&userID, &agentID,
		&mem.Content, &mem.Confidence, &sessionID, &turnRange, &observedAt, &accessedAt, &accessCount,
		&summary, &bodySizeBytes,
		&finalScore,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: scan multi-tier hybrid row: %w", err)
	}

	mem.Scope = buildScope(workspaceID, userID, agentID)
	mem.ExpiresAt = expiresAt
	if sessionID != nil {
		mem.SessionID = *sessionID
	}
	if len(turnRange) == 2 {
		mem.TurnRange = [2]int{turnRange[0], turnRange[1]}
	}
	if accessedAt != nil {
		mem.AccessedAt = *accessedAt
	}
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &mem.Metadata)
	}
	stampLargeMemoryFields(&mem, title, summary, bodySizeBytes)

	return &MultiTierMemory{
		Memory:      &mem,
		Tier:        classifyTier(userID, agentID),
		AccessCount: accessCount,
		Score:       finalScore,
	}, nil
}

// rankHybridResults applies the TierRanker to the SQL-provided fused base
// score (already recency/confidence/source weighted) and sorts descending.
// Unlike rankResults it does NOT recompute the base via computeScore — the
// hybrid SQL owns the base; here we only apply per-tier bias.
func rankHybridResults(results []*MultiTierMemory, ranker TierRanker) {
	if ranker == nil {
		ranker = IdentityTierRanker{}
	}
	for _, r := range results {
		r.Score = ranker.Adjust(r.Score, r.Tier)
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
}
