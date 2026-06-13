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
// %[3]s per-tier recency-decay expression, $%[4]d query text,
// $%[5]d embedding, $%[6]d fanout, %[7]s source-type weight CASE
// (the shared sourceTypeWeightSQL const, keyed on alias e).
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
           ts_rank_cd(search_vector, websearch_to_tsquery('english', $%[4]d)) AS fts_rank
    FROM candidates
    WHERE search_vector @@ websearch_to_tsquery('english', $%[4]d)
    ORDER BY entity_id, fts_rank DESC
    LIMIT $%[6]d
), fts_ranked AS (
    SELECT entity_id, row_number() OVER (ORDER BY fts_rank DESC) AS fts_rn FROM fts
), cosine AS (
    -- The inner ORDER BY embedding <=> $ LIMIT N unlocks the HNSW index;
    -- over-fetch (fanout × 4) so per-entity dedup still lands enough distinct
    -- entities when one entity has several close-by observations.
    SELECT entity_id, cos_dist FROM (
        SELECT entity_id, embedding <=> $%[5]d AS cos_dist,
               row_number() OVER (PARTITION BY entity_id ORDER BY embedding <=> $%[5]d) AS rn
        FROM candidates
        WHERE embedding IS NOT NULL
        ORDER BY embedding <=> $%[5]d
        LIMIT $%[6]d * 4
    ) ann WHERE rn = 1 LIMIT $%[6]d
), cosine_ranked AS (
    SELECT entity_id, row_number() OVER (ORDER BY cos_dist) AS cos_rn FROM cosine
), fused AS (
    SELECT coalesce(f.entity_id, c.entity_id) AS entity_id, %[2]s AS rrf
    FROM fts_ranked f FULL OUTER JOIN cosine_ranked c USING (entity_id)
)
SELECT DISTINCT ON (e.entity_id)
    e.entity_id, e.kind, e.metadata, e.created_at, e.expires_at, e.title,
    e.virtual_user_id, e.agent_id,
    e.content, e.confidence, e.session_id, e.turn_range, e.observed_at,
    e.accessed_at, e.access_count, e.summary, e.body_size_bytes,
    fused.rrf * (%[7]s) * coalesce(e.confidence, 0.7) * %[3]s AS final_score
FROM candidates e JOIN fused ON fused.entity_id = e.entity_id
ORDER BY e.entity_id, e.observed_at DESC`

// buildMultiTierHybridQuery constructs the RRF-over-tiers SQL and its
// positional args. The shared filter predicates are built once via the same
// QueryBuilder helpers the FTS-only multi-tier path uses; the FTS @@ predicate
// is intentionally NOT added here (it lives inside the fts CTE so cosine-only
// matches still surface). The trailing args after the builder's are: query
// text, embedding, fanout cap, and the three per-tier half-life seconds
// (user, agent, institutional) consumed by the recency CASE.
func buildMultiTierHybridQuery(req MultiTierRequest, queryEmbedding []float32, hl TierHalfLife) (string, []any, error) {
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
	userHLIdx := len(args) + 4
	agentHLIdx := len(args) + 5
	instHLIdx := len(args) + 6
	args = append(args, req.Query, pgvector.NewVector(queryEmbedding), hybridFanout,
		hl.User.Seconds(), hl.Agent.Seconds(), hl.Institutional.Seconds())

	rrfExpr := fmt.Sprintf(
		"coalesce(1.0/(%g + f.fts_rn), 0) + coalesce(1.0/(%g + c.cos_rn), 0)",
		rrfK, rrfK,
	)

	// Per-tier recency decay: exp(-ln2 * age / halfLife), = 0.5 at age==halfLife.
	// The CASE picks the tier's half-life from the row's scope columns, mirroring
	// classifyTier precedence (a user-bearing row — incl. user_for_agent — uses
	// the user half-life; agent-only uses agent; the rest institutional).
	// greatest(-700, ...) floors the exponent so exp() can't underflow (which
	// Postgres raises as an error, not 0) for very old rows / tiny half-lives.
	// exp(-700) ≈ 1e-304, indistinguishable from zero for ranking.
	recencyExpr := fmt.Sprintf(
		"exp(greatest((-700)::float8, (-%v)::float8 * "+
			"EXTRACT(EPOCH FROM (now() - e.observed_at))::float8 / "+
			"(CASE WHEN e.virtual_user_id IS NOT NULL THEN $%d::float8 "+
			"WHEN e.agent_id IS NOT NULL THEN $%d::float8 ELSE $%d::float8 END)))",
		ln2, userHLIdx, agentHLIdx, instHLIdx,
	)

	// QueryBuilder.Where() returns a leading " AND ", so prefix the literal
	// forgotten predicate as the first WHERE term (mirrors buildMultiTierQuery).
	where := colEntityForgot + qb.Where()
	sql := fmt.Sprintf(hybridMultiTierTemplate, where, rrfExpr, recencyExpr, qIdx, eIdx, fIdx, sourceTypeWeightSQL)
	return sql, args, nil
}

// RetrieveMultiTierHybrid — see the Store interface. When the query or the
// embedding is empty there is nothing to fuse, so it falls through to the
// FTS-only RetrieveMultiTier (which the callers also do on embed failure).
func (s *PostgresMemoryStore) RetrieveMultiTierHybrid(ctx context.Context, req MultiTierRequest, queryEmbedding []float32) (*MultiTierResult, error) {
	if req.Query == "" || len(queryEmbedding) == 0 {
		return s.RetrieveMultiTier(ctx, req)
	}

	sql, args, err := buildMultiTierHybridQuery(req, queryEmbedding, req.HalfLife.orDefaults())
	if err != nil {
		return nil, err
	}

	// PERF-3: the cosine CTE over-fetches hybridFanout×4 from the HNSW index,
	// so raise hnsw.ef_search to match or the index caps candidates at its
	// default (40) before tier post-filtering.
	var memories []*MultiTierMemory
	err = s.withHNSWEFSearch(ctx, clampEFSearch(hybridFanout*4), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return fmt.Errorf("memory: multi-tier hybrid query: %w", err)
		}
		defer rows.Close()

		memories, err = scanMultiTierHybridRows(rows, req.WorkspaceID)
		return err
	})
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

// scanMultiTierHybridRows reads the hybrid multi-tier row set — the FTS-only
// multi-tier columns plus a trailing final_score the SQL has already folded
// RRF × source × confidence × recency into.
func scanMultiTierHybridRows(rows pgx.Rows, workspaceID string) ([]*MultiTierMemory, error) {
	return scanMultiTierMemoryRows(rows, workspaceID, scanMultiTierHybridRow)
}

// scanMultiTierHybridRow scans the shared multi-tier columns plus the trailing
// final_score, reusing assembleMultiTierMemory for the column→struct mapping.
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
	_ = observedAt // scanned for column alignment; recency is computed in SQL

	m := assembleMultiTierMemory(workspaceID, &mem, metadataJSON, expiresAt,
		userID, agentID, sessionID, turnRange, accessedAt, accessCount,
		title, summary, bodySizeBytes)
	m.Score = finalScore
	return m, nil
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
