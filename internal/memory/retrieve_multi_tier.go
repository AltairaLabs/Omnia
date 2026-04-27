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
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Tier identifies which scope layer a memory belongs to in a multi-tier
// retrieval. The value is always one of the four named constants.
type Tier string

// Tier variants covered by RetrieveMultiTier.
const (
	TierInstitutional Tier = "institutional"
	TierAgent         Tier = "agent"
	TierUser          Tier = "user"
	TierUserForAgent  Tier = "user_for_agent"
)

// MultiTierRequest describes a single multi-tier retrieval query.
// WorkspaceID is required; all other fields are optional filters.
//
// SeedEntityIDs + MaxGraphHops + RelationTypes enable graph traversal as a
// supplemental source: entities reachable from the seeds are merged into the
// result and ranked alongside the tier-filtered rows.
//
// StructuredLookups allows the caller to add exact-filter queries (by kind,
// name-prefix, or purpose). Each entry produces its own result slice which
// is merged with the same dedupe-and-rank pipeline.
type MultiTierRequest struct {
	WorkspaceID   string
	UserID        string
	AgentID       string
	Query         string
	Types         []string
	MinConfidence float64
	Limit         int
	Now           time.Time

	// Purposes narrows the result set to memories tagged with one of the
	// listed purpose values (e.g. "support_continuity", "personalisation").
	// Empty means "no purpose filter" — all purposes are returned, preserving
	// the pre-filter default for callers that don't yet supply the field.
	// A nil-purpose memory (entity.purpose IS NULL) only matches when the
	// caller doesn't pass a filter; an explicit non-empty Purposes list
	// requires a match.
	Purposes []string

	// Multi-mode retrieval additions (Phase 3).
	SeedEntityIDs     []string
	MaxGraphHops      int
	RelationTypes     []string
	StructuredLookups []StructuredLookup

	// Ranker biases per-tier scores after the base score is computed.
	// Nil means identity (no change), preserving the pre-tier-precedence
	// behaviour for callers without a MemoryPolicy in scope.
	Ranker TierRanker
}

// MultiTierMemory augments a Memory with the tier it was retrieved from,
// the observation access count (used for frequency scoring), and the final
// ranking score computed by rankResults.
type MultiTierMemory struct {
	*Memory
	Tier        Tier
	AccessCount int
	Score       float64
}

// MultiTierResult is the top-level response returned by RetrieveMultiTier.
type MultiTierResult struct {
	Memories []*MultiTierMemory
	Total    int
}

// multiTierCandidatePool is the SQL-level candidate cap applied before
// Go-side ranking. It is intentionally larger than any client Limit so the
// ranker sees enough rows to sort meaningfully.
const multiTierCandidatePool = 200

// defaultMultiTierLimit is the caller-visible default Limit when the
// request does not specify one.
const defaultMultiTierLimit = 15

// ranking weight/normalisation constants.
const (
	weightConfidence = 0.5
	weightFrequency  = 0.3
	weightRecency    = 0.2
	freqLogCeiling   = 100.0
	recencyTauHours  = 168.0
)

// RetrieveMultiTier runs a single multi-tier SQL query covering institutional,
// agent, user, and user-for-agent scope variants, then ranks and truncates
// the results Go-side.
func (s *PostgresMemoryStore) RetrieveMultiTier(ctx context.Context, req MultiTierRequest) (*MultiTierResult, error) {
	sql, args, err := buildMultiTierQuery(req)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: multi-tier query: %w", err)
	}
	defer rows.Close()

	memories, err := scanMultiTierRows(rows, req.WorkspaceID)
	if err != nil {
		return nil, err
	}

	memories, err = s.mergeMultiMode(ctx, req, memories)
	if err != nil {
		return nil, err
	}

	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	rankResults(memories, now, req.Ranker)

	limit := req.Limit
	if limit <= 0 {
		limit = defaultMultiTierLimit
	}
	if len(memories) > limit {
		memories = memories[:limit]
	}

	// Bump accessed_at / access_count on the final (post-ranking, post-
	// truncation) result. Doing it after truncation means rows that didn't
	// make it into the caller's response don't get their LRU signal
	// refreshed — which is correct, they weren't actually used.
	s.touchAccessedOnRead(entityIDsFromMultiTier(memories))

	return &MultiTierResult{Memories: memories, Total: len(memories)}, nil
}

// mergeMultiMode augments the tier-filtered result with graph traversal and
// structured lookup rows when the request asks for them. Deduplicates by
// entity ID with tier-first precedence: if a row is already present (from
// the tier query), we keep its access_count and tier; otherwise we wrap the
// supplemental row with a zero access count and tier derived from scope.
func (s *PostgresMemoryStore) mergeMultiMode(ctx context.Context, req MultiTierRequest, existing []*MultiTierMemory) ([]*MultiTierMemory, error) {
	seen := make(map[string]bool, len(existing))
	for _, m := range existing {
		if m.Memory != nil {
			seen[m.ID] = true
		}
	}

	if len(req.SeedEntityIDs) > 0 {
		graphRows, err := s.TraverseRelations(ctx, GraphTraversal{
			WorkspaceID:   req.WorkspaceID,
			SeedIDs:       req.SeedEntityIDs,
			RelationTypes: req.RelationTypes,
			MaxHops:       req.MaxGraphHops,
			Limit:         multiTierCandidatePool,
		})
		if err != nil {
			return nil, fmt.Errorf("memory: merge graph: %w", err)
		}
		existing = appendDeduped(existing, graphRows, seen)
	}

	for _, q := range req.StructuredLookups {
		if q.WorkspaceID == "" {
			q.WorkspaceID = req.WorkspaceID
		}
		if q.Limit <= 0 {
			q.Limit = multiTierCandidatePool
		}
		rows, err := s.LookupStructured(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("memory: merge structured: %w", err)
		}
		existing = appendDeduped(existing, rows, seen)
	}

	return existing, nil
}

// appendDeduped wraps each new *Memory in a MultiTierMemory (tier inferred
// from scope, AccessCount=0) and appends to dst if not already seen.
func appendDeduped(dst []*MultiTierMemory, rows []*Memory, seen map[string]bool) []*MultiTierMemory {
	for _, m := range rows {
		if m == nil || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		dst = append(dst, &MultiTierMemory{
			Memory: m,
			Tier:   classifyTierFromScope(m.Scope),
		})
	}
	return dst
}

// classifyTierFromScope returns the tier a row belongs to based on which
// scope keys are populated. Matches scanMultiTierRow's logic but reads from
// the scope map rather than *string columns.
func classifyTierFromScope(scope map[string]string) Tier {
	hasUser := scope[ScopeUserID] != ""
	hasAgent := scope[ScopeAgentID] != ""
	switch {
	case hasUser && hasAgent:
		return TierUserForAgent
	case hasUser:
		return TierUser
	case hasAgent:
		return TierAgent
	default:
		return TierInstitutional
	}
}

// buildMultiTierQuery constructs the SQL and positional arguments for a
// multi-tier retrieval. It returns an error when WorkspaceID is empty; the
// candidate LIMIT is a constant (multiTierCandidatePool) independent of the
// caller's Limit.
func buildMultiTierQuery(req MultiTierRequest) (string, []any, error) {
	if req.WorkspaceID == "" {
		return "", nil, errors.New(errWorkspaceRequired)
	}

	args := make([]any, 0, 6)
	args = append(args, req.WorkspaceID)
	clauses := []string{
		"e.workspace_id=$" + strconv.Itoa(len(args)),
		colEntityForgot,
	}

	clauses = append(clauses, userTierClause(req.UserID, &args))
	clauses = append(clauses, agentTierClause(req.AgentID, &args))

	if len(req.Types) == 1 {
		args = append(args, req.Types[0])
		clauses = append(clauses, "e.kind=$"+strconv.Itoa(len(args)))
	} else if len(req.Types) > 1 {
		args = append(args, req.Types)
		clauses = append(clauses, "e.kind = ANY($"+strconv.Itoa(len(args))+")")
	}

	if req.MinConfidence > 0 {
		args = append(args, req.MinConfidence)
		clauses = append(clauses, "o.confidence >= $"+strconv.Itoa(len(args)))
	}

	if req.Query != "" {
		args = append(args, "%"+req.Query+"%")
		clauses = append(clauses, "o.content ILIKE $"+strconv.Itoa(len(args)))
	}

	if len(req.Purposes) == 1 {
		args = append(args, req.Purposes[0])
		clauses = append(clauses, "e.purpose=$"+strconv.Itoa(len(args)))
	} else if len(req.Purposes) > 1 {
		args = append(args, req.Purposes)
		clauses = append(clauses, "e.purpose = ANY($"+strconv.Itoa(len(args))+")")
	}

	sql := fmt.Sprintf(`SELECT DISTINCT ON (e.id) e.id, e.kind, e.metadata, e.created_at, e.expires_at, e.title, e.virtual_user_id, e.agent_id, o.content, o.confidence, o.session_id, o.turn_range, o.observed_at, o.accessed_at, o.access_count, o.summary, o.body_size_bytes FROM memory_entities e JOIN memory_observations o ON o.entity_id = e.id AND o.superseded_by IS NULL AND (o.valid_until IS NULL OR o.valid_until > now()) WHERE %s ORDER BY e.id, o.observed_at DESC LIMIT %d`,
		joinAnd(clauses), multiTierCandidatePool)

	return sql, args, nil
}

// userTierClause returns the user-scope predicate for the multi-tier query.
// When userID is empty the predicate anchors to NULL so institutional-only
// retrievals do not bleed through other users' memories. The column is
// unqualified because memory_observations does not share it, avoiding an
// alias prefix keeps the emitted SQL aligned with the agreed contract.
func userTierClause(userID string, args *[]any) string {
	if userID == "" {
		return "virtual_user_id IS NULL"
	}
	*args = append(*args, userID)
	return "(virtual_user_id IS NULL OR virtual_user_id=$" + strconv.Itoa(len(*args)) + ")"
}

// agentTierClause returns the agent-scope predicate. Same NULL-anchoring
// behaviour as userTierClause.
func agentTierClause(agentID string, args *[]any) string {
	if agentID == "" {
		return "agent_id IS NULL"
	}
	*args = append(*args, agentID)
	return "(agent_id IS NULL OR agent_id=$" + strconv.Itoa(len(*args)) + ")"
}

// joinAnd joins SQL WHERE fragments with " AND ".
func joinAnd(parts []string) string {
	return strings.Join(parts, " AND ")
}

// scanMultiTierRows reads multi-tier query rows into MultiTierMemory values.
// workspaceID seeds the memory Scope so downstream consumers see the same
// scope keys Retrieve/List populate.
func scanMultiTierRows(rows pgx.Rows, workspaceID string) ([]*MultiTierMemory, error) {
	var results []*MultiTierMemory
	for rows.Next() {
		mem, err := scanMultiTierRow(rows, workspaceID)
		if err != nil {
			return nil, err
		}
		results = append(results, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: multi-tier rows iteration: %w", err)
	}
	if results == nil {
		results = []*MultiTierMemory{}
	}
	return results, nil
}

// scanMultiTierRow scans a single row from the multi-tier query.
func scanMultiTierRow(row pgx.Rows, workspaceID string) (*MultiTierMemory, error) {
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
	)

	err := row.Scan(
		&mem.ID, &mem.Type, &metadataJSON, &mem.CreatedAt, &expiresAt, &title,
		&userID, &agentID,
		&mem.Content, &mem.Confidence, &sessionID, &turnRange, &observedAt, &accessedAt, &accessCount,
		&summary, &bodySizeBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: scan multi-tier row: %w", err)
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
	}, nil
}

// buildScope assembles the Memory.Scope map from the non-nullable workspace
// plus any nullable tier identifiers present on the row.
func buildScope(workspaceID string, userID, agentID *string) map[string]string {
	scope := map[string]string{ScopeWorkspaceID: workspaceID}
	if userID != nil && *userID != "" {
		scope[ScopeUserID] = *userID
	}
	if agentID != nil && *agentID != "" {
		scope[ScopeAgentID] = *agentID
	}
	return scope
}

// classifyTier maps the nullable user/agent identifiers from a row to the
// corresponding Tier constant.
func classifyTier(userID, agentID *string) Tier {
	hasUser := userID != nil
	hasAgent := agentID != nil
	switch {
	case hasUser && hasAgent:
		return TierUserForAgent
	case hasUser:
		return TierUser
	case hasAgent:
		return TierAgent
	default:
		return TierInstitutional
	}
}

// rankResults assigns a Score to each MultiTierMemory and sorts the slice in
// descending score order. The base formula mixes confidence, access-count
// frequency (log-normalised), and recency (exponential decay); the supplied
// TierRanker then biases the score per tier (an identity ranker preserves
// the existing behaviour for callers without a policy).
func rankResults(results []*MultiTierMemory, now time.Time, ranker TierRanker) {
	if ranker == nil {
		ranker = IdentityTierRanker{}
	}
	for _, r := range results {
		r.Score = ranker.Adjust(computeScore(r, now), r.Tier)
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
}

// computeScore returns the ranking score for a single memory.
func computeScore(r *MultiTierMemory, now time.Time) float64 {
	confidence := r.Confidence
	freq := math.Log1p(float64(r.AccessCount)) / math.Log1p(freqLogCeiling)
	if freq < 0 {
		freq = 0
	}
	if freq > 1 {
		freq = 1
	}
	ref := r.AccessedAt
	if ref.IsZero() {
		ref = r.CreatedAt
	}
	ageHours := now.Sub(ref).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	recency := math.Exp(-ageHours / recencyTauHours)
	return weightConfidence*confidence + weightFrequency*freq + weightRecency*recency
}
