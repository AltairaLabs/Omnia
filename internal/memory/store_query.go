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
	"fmt"
	"strings"

	"github.com/altairalabs/omnia/internal/pgutil"
)

// buildRetrieveQuery constructs the SQL and arguments for a Retrieve call.
// When query is non-empty it builds a FTS-scored variant; otherwise it
// returns the standard recency-ordered query.
func buildRetrieveQuery(scope map[string]string, query string, opts RetrieveOptions) (string, *pgutil.QueryBuilder) {
	qb := buildBaseMemoryQuery(scope, opts.Types, "")
	addConfidenceFilter(qb, opts.MinConfidence)

	queryArgIdx := addFTSPredicate(qb, query)
	if queryArgIdx == 0 {
		return formatMemorySQL(qb, opts.Limit, 0), qb
	}
	return formatMemoryFTSSQL(qb, queryArgIdx, opts.Limit, 0), qb
}

// formatMemoryFTSSQL renders a Retrieve query that ranks matching
// observations by a fused score combining lexical relevance with
// per-row signal multipliers:
//
//	score = ts_rank_cd(search_vector, query)
//	      × source_type_weight(entity.source_type)
//	      × confidence
//	      × recency_decay(observed_at, half_life=30d)
//
// At equal lexical relevance, a fact the user explicitly asked us to
// remember (source_type=user_requested, weight 1.0) outranks one we
// inferred from a conversation (conversation_extraction, weight 0.7).
// Newer beats older via the exponential decay. queryArgIdx is the
// 1-based placeholder of the user query already added to qb.
func formatMemoryFTSSQL(qb *pgutil.QueryBuilder, queryArgIdx, limit, offset int) string {
	if limit <= 0 {
		limit = defaultMemoryLimit
	}
	tsqueryExpr := fmt.Sprintf("websearch_to_tsquery('english', $%d)", queryArgIdx)
	scoreExpr := fmt.Sprintf(
		"(ts_rank_cd(o.search_vector, %s)) * (%s) * coalesce(o.confidence, 0.7) * (%s)",
		tsqueryExpr, sourceTypeWeightSQL, recencyDecaySQL,
	)

	// Inner query: per entity pick the observation with the highest
	// fused score (DISTINCT ON requires the ORDER BY to start with
	// the distinct key). Outer query: re-sort entities by that score.
	inner := fmt.Sprintf(`
		SELECT DISTINCT ON (e.id) %s, %s, %s AS rank
		FROM memory_entities %s%s
		WHERE %s%s
		ORDER BY e.id, rank DESC, o.observed_at DESC`,
		selectEntityCols, selectObserveCols, scoreExpr,
		entityTableAlias, observationJoin,
		colEntityForgot, qb.Where())

	outerCols := strings.ReplaceAll(selectEntityCols, "e.", "") + ", " +
		strings.ReplaceAll(selectObserveCols, "o.", "")
	sql := fmt.Sprintf(
		"SELECT %s FROM (%s) AS scored ORDER BY rank DESC, observed_at DESC",
		outerCols, inner,
	)
	return qb.AppendPagination(sql, limit, offset)
}

// buildListQuery constructs the SQL and arguments for a List call.
func buildListQuery(scope map[string]string, opts ListOptions) (string, *pgutil.QueryBuilder) {
	if scope[ScopeIncludeShared] == scopeFlagTrue {
		qb := buildVisibleToMeQuery(scope, opts.Types)
		return formatVisibleToMeSQL(qb, opts.Limit, opts.Offset), qb
	}
	qb := buildBaseMemoryQuery(scope, opts.Types, "")
	return formatMemorySQL(qb, opts.Limit, opts.Offset), qb
}

// buildVisibleToMeQuery builds the "everything visible to the user" list:
// workspace rows where virtual_user_id is NULL (institutional + agent
// tiers) OR equals the requesting user — which includes the user's own
// memories while excluding every other user's private memories. There is
// deliberately no agent_id filter: agent-tier rows have virtual_user_id
// NULL, so the user-tier clause already admits them. See #1254.
func buildVisibleToMeQuery(scope map[string]string, types []string) *pgutil.QueryBuilder {
	var qb pgutil.QueryBuilder
	qb.Add(colWorkspaceID, scope[ScopeWorkspaceID])
	addUserTierClause(&qb, scope[ScopeUserID])
	addTypeFilters(&qb, types)
	return &qb
}

// buildBaseMemoryQuery creates the common query builder for memory entity queries.
// It applies workspace, scope, type, and purpose filters.
func buildBaseMemoryQuery(scope map[string]string, types []string, purpose string) *pgutil.QueryBuilder {
	var qb pgutil.QueryBuilder
	qb.Add(colWorkspaceID, scope[ScopeWorkspaceID])
	addScopeFilters(&qb, scope)
	addTypeFilters(&qb, types)
	if purpose != "" {
		qb.Add("e.purpose=$?", purpose)
	}
	return &qb
}

// formatMemorySQL formats the standard memory SELECT with the given WHERE conditions and pagination.
func formatMemorySQL(qb *pgutil.QueryBuilder, limit, offset int) string {
	if limit <= 0 {
		limit = defaultMemoryLimit
	}

	sql := fmt.Sprintf(`
		SELECT DISTINCT ON (e.id) %s, %s
		FROM memory_entities %s%s
		WHERE %s%s
		ORDER BY e.id, o.observed_at DESC`,
		selectEntityCols, selectObserveCols,
		entityTableAlias, observationJoin,
		colEntityForgot, qb.Where())

	return qb.AppendPagination(sql, limit, offset)
}

// formatVisibleToMeSQL is formatMemorySQL with the per-row scope columns
// (virtual_user_id, agent_id) appended, so the visible-to-me list — which
// spans institutional, agent and user tiers — can derive each row's real
// tier instead of inheriting the request scope. Scanned by
// scanVisibleToMeMemories. See #1254.
func formatVisibleToMeSQL(qb *pgutil.QueryBuilder, limit, offset int) string {
	if limit <= 0 {
		limit = defaultMemoryLimit
	}

	sql := fmt.Sprintf(`
		SELECT DISTINCT ON (e.id) %s, %s, %s
		FROM memory_entities %s%s
		WHERE %s%s
		ORDER BY e.id, o.observed_at DESC`,
		selectEntityCols, selectObserveCols, selectEntityScopeCols,
		entityTableAlias, observationJoin,
		colEntityForgot, qb.Where())

	return qb.AppendPagination(sql, limit, offset)
}

// addScopeFilters appends optional user_id and agent_id filters.
func addScopeFilters(qb *pgutil.QueryBuilder, scope map[string]string) {
	// SEC-3: an empty user_id must mean "institutional/agent rows only"
	// (virtual_user_id IS NULL), NOT "no constraint" — otherwise a
	// workspace-scoped read (e.g. the runtime's semantic strategy, which
	// passes no user) returns every user's private memories in the workspace.
	// A populated user_id stays strict (that user's rows only).
	if uid := scope[ScopeUserID]; uid != "" {
		qb.Add(colVirtualUserID, uid)
	} else {
		qb.AddRaw("virtual_user_id IS NULL")
	}
	if aid := scope[ScopeAgentID]; aid != "" {
		qb.Add("e.agent_id=$?", aid)
	}
}

// addTypeFilters appends a single kind = $N filter when one type is specified,
// or kind = ANY($N) when multiple types are specified.
func addTypeFilters(qb *pgutil.QueryBuilder, types []string) {
	switch len(types) {
	case 0:
		return
	case 1:
		qb.Add(entityKindFilter, types[0])
	default:
		qb.Add("e.kind = ANY($?)", types)
	}
}

// addPurposeFilters appends a purpose-equals or purpose=ANY filter. Empty
// list is a no-op so callers can pass req.Purposes without conditional
// scaffolding. Shared between Retrieve and RetrieveMultiTier so future
// purpose-related work (e.g. SUPPORT_CONTINUITY scoring) lands in both.
func addPurposeFilters(qb *pgutil.QueryBuilder, purposes []string) {
	switch len(purposes) {
	case 0:
		return
	case 1:
		qb.Add("e.purpose=$?", purposes[0])
	default:
		qb.Add("e.purpose = ANY($?)", purposes)
	}
}

// addConfidenceFilter appends an "o.confidence >= $?" predicate when min > 0.
// Both single-tier Retrieve and multi-tier RetrieveMultiTier filter on this
// — extracting the helper means a future tweak (e.g. switching to a fused
// confidence × source-type score) can't drift between paths.
func addConfidenceFilter(qb *pgutil.QueryBuilder, min float64) {
	if min > 0 {
		qb.Add(confidenceFilter, min)
	}
}

// joinAnd joins SQL WHERE fragments with " AND ". Used by the
// non-QueryBuilder structured-lookup path; QueryBuilder users don't
// need it because Where() handles the join.
func joinAnd(parts []string) string {
	return strings.Join(parts, " AND ")
}

// addFTSPredicate appends a websearch_to_tsquery match against the
// observation's stored search_vector and returns the 1-based positional
// index of the bound query string. The caller can re-use that index to
// reference the same parameter in a scoring expression (e.g. ts_rank_cd
// in the single-tier FTS path) without binding the query twice.
//
// This is the single-source-of-truth for "how do we match the user's
// query against the FTS index". The April ILIKE→FTS migration only
// touched the single-tier path, leaving the multi-tier path on
// literal-substring matching for two months until #1038 surfaced it
// (the agent answered "I don't recall Morocco" two sentences after
// storing three Morocco memories). Sharing the predicate keeps that
// kind of drift impossible by construction.
//
// Returns 0 when query is empty (no clause appended). Callers needing
// the index unconditionally should branch on that themselves.
func addFTSPredicate(qb *pgutil.QueryBuilder, query string) int {
	if query == "" {
		return 0
	}
	qb.Add("o.search_vector @@ websearch_to_tsquery('english', $?)", query)
	return len(qb.Args())
}
