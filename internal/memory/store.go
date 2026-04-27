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
	"maps"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"

	"github.com/altairalabs/omnia/internal/pgutil"
)

// Compile-time interface check.
var _ Store = (*PostgresMemoryStore)(nil)

// Scope key constants used in memory scope maps.
const (
	ScopeWorkspaceID = "workspace_id"
	ScopeUserID      = "user_id"
	ScopeAgentID     = "agent_id"
)

// Error message constants (SonarCloud S1192).
const (
	errWorkspaceRequired = "memory: workspace_id scope is required"
	errUserIDRequired    = "memory: user_id scope is required"
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
)

// PostgresMemoryStore implements Store against the memory_entities / memory_observations
// PostgreSQL tables created by the memory database initial schema migration.
type PostgresMemoryStore struct {
	pool *pgxpool.Pool
}

// NewPostgresMemoryStore creates a new store backed by the given connection pool.
func NewPostgresMemoryStore(pool *pgxpool.Pool) *PostgresMemoryStore {
	return &PostgresMemoryStore{pool: pool}
}

// Pool returns the underlying connection pool. Used by retrieval strategies that
// need direct pool access (e.g. OmniaRetriever delegating to RetrievalStrategy).
func (s *PostgresMemoryStore) Pool() *pgxpool.Pool {
	return s.pool
}

// Save persists a memory. When Memory.ID is empty a new entity and observation are
// inserted. When Memory.ID is set the entity metadata is updated and a new observation
// is appended (upsert pattern). The Memory is mutated in place: ID and CreatedAt are
// populated on return.
// Save implements pkmemory.Store. Backwards-compatible thin wrapper
// around SaveWithResult that discards the rich result. New Omnia
// callers prefer SaveWithResult so they can surface dedup info to
// the agent.
func (s *PostgresMemoryStore) Save(ctx context.Context, mem *Memory) error {
	_, err := s.SaveWithResult(ctx, mem)
	return err
}

// SaveWithResult is Omnia's enriched write API. Returns SaveResult
// describing whether the write was a fresh INSERT or an
// auto-supersede via the structured-key dedup path. Embedding-
// similarity dedup is layered on top by the api/service.go caller
// (which has the embedding provider) — this method covers the
// structured-key path only.
func (s *PostgresMemoryStore) SaveWithResult(ctx context.Context, mem *Memory) (*SaveResult, error) {
	if mem.Scope[ScopeWorkspaceID] == "" {
		return nil, errors.New(errWorkspaceRequired)
	}
	if mem.Scope[ScopeUserID] == "" {
		return nil, errors.New(errUserIDRequired)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("memory: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	res := &SaveResult{Action: SaveActionAdded}

	// Structured-key dedup path: when the caller passed
	// about_kind+about_key, look up (or atomically create) the entity
	// keyed by (scope, about_kind, about_key) and supersede any prior
	// active observation under it. This is what fixes the "user
	// changes name and old name still shows up" failure.
	switch {
	case mem.ID == "" && hasAboutKey(mem):
		conflicted, err := upsertEntityByAboutKey(ctx, tx, mem)
		if err != nil {
			return nil, err
		}
		if conflicted {
			supersededIDs, err := supersedePriorObservations(ctx, tx, mem.ID)
			if err != nil {
				return nil, err
			}
			res.Action = SaveActionAutoSuperseded
			res.SupersededObservationIDs = supersededIDs
			res.SupersedeReason = ReasonStructuredKey
		}
	case mem.ID == "":
		if err := insertEntity(ctx, tx, mem); err != nil {
			return nil, err
		}
	default:
		if err := updateEntity(ctx, tx, mem); err != nil {
			return nil, err
		}
	}

	if err := insertObservation(ctx, tx, mem); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	res.ID = mem.ID
	return res, nil
}

// hasAboutKey reports whether the caller asked for structured-key
// dedup by setting both metadata keys.
func hasAboutKey(mem *Memory) bool {
	return stringFromMeta(mem.Metadata, MetaKeyAboutKind) != "" &&
		stringFromMeta(mem.Metadata, MetaKeyAboutKey) != ""
}

// upsertEntityByAboutKey atomically returns the existing entity for
// (scope, about_kind, about_key) — creating it if absent. Sets mem.ID
// and mem.CreatedAt either way. Implements the structured-key
// dedup path via ON CONFLICT against the partial unique index.
// Returns conflicted=true when an existing entity was reused (the
// caller will then supersede the entity's prior active observations).
func upsertEntityByAboutKey(ctx context.Context, tx pgx.Tx, mem *Memory) (bool, error) {
	metaJSON, err := marshalMetadata(mem.Metadata)
	if err != nil {
		return false, err
	}
	trustModel, sourceType := trustFromProvenance(mem.Metadata)
	purpose := purposeFromMetadata(mem.Metadata)
	consentCategory := consentCategoryFromMetadata(mem.Metadata)
	aboutKind := stringFromMeta(mem.Metadata, MetaKeyAboutKind)
	aboutKey := stringFromMeta(mem.Metadata, MetaKeyAboutKey)
	title := stringFromMeta(mem.Metadata, MetaKeyTitle)

	// ON CONFLICT against the partial unique index. The DO UPDATE
	// SET clause is what unblocks RETURNING on conflict — without an
	// update Postgres skips the row entirely. Bumping updated_at
	// also signals "this entity got new content" for downstream
	// consumers (dashboard "last activity" timestamps, retention
	// freshness checks).
	//
	// xmax = 0 marks freshly inserted rows; on the ON CONFLICT path
	// xmax holds the conflicting xact's id and is non-zero. We use
	// that to tell the caller whether dedup fired.
	row := tx.QueryRow(ctx, `
		INSERT INTO memory_entities
		  (workspace_id, virtual_user_id, agent_id, name, kind, metadata, expires_at,
		   trust_model, source_type, purpose, consent_category,
		   about_kind, about_key, title)
		VALUES
		  ($1, $2, $3, $4, $5, $6, $7,
		    COALESCE($8, 'inferred'),
		    COALESCE($9, 'conversation_extraction'),
		    COALESCE($10, 'support_continuity'),
		    $11, $12, $13, NULLIF($14, ''))
		ON CONFLICT (workspace_id, virtual_user_id, agent_id,
		             about_kind, about_key)
		WHERE about_kind IS NOT NULL AND NOT forgotten
		DO UPDATE SET updated_at = now(),
		              metadata = EXCLUDED.metadata,
		              title = COALESCE(EXCLUDED.title, memory_entities.title)
		RETURNING id, created_at, (xmax <> 0) AS conflicted`,
		mem.Scope[ScopeWorkspaceID],
		scopeOrNil(mem.Scope, ScopeUserID),
		scopeOrNil(mem.Scope, ScopeAgentID),
		mem.Content,
		mem.Type,
		metaJSON,
		mem.ExpiresAt,
		trustModel,
		sourceType,
		purpose,
		consentCategory,
		aboutKind,
		aboutKey,
		title,
	)
	var conflicted bool
	if err := row.Scan(&mem.ID, &mem.CreatedAt, &conflicted); err != nil {
		return false, err
	}
	return conflicted, nil
}

// supersedePriorObservations marks any active observation under the
// entity as superseded. Uses valid_until = now() so recall's
// active-only filter excludes them immediately; superseded_by stays
// NULL because the new observation hasn't been inserted yet (and the
// observation-level explicit-update path sets superseded_by when it
// has the new id available). Idempotent. Returns the IDs that were
// marked so SaveResult can surface them to the agent.
func supersedePriorObservations(ctx context.Context, tx pgx.Tx, entityID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		UPDATE memory_observations
		SET valid_until = now()
		WHERE entity_id = $1
		  AND superseded_by IS NULL
		  AND valid_until IS NULL
		RETURNING id`,
		entityID,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: supersede prior observations: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("memory: supersede scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MetaKeyPurpose is the metadata key carrying the Omnia purpose tag
// (e.g. "support_continuity", "personalisation"). The value is read at
// insert time and written to memory_entities.purpose so retrieval can
// filter on it without re-parsing JSON metadata. Empty / missing values
// fall through to the schema default ('support_continuity').
const MetaKeyPurpose = "purpose"

// MetaKeyConsentCategory is the metadata key carrying the consent
// category tag (e.g. "memory:health", "memory:location"). Read at
// insert time and written to memory_entities.consent_category so
// the retention worker's consent revocation cascade can match rows
// against the user's current grants without scanning JSON metadata.
// Empty / missing values leave the column NULL — those rows fall
// under the default (non-per-category) retention policy.
const MetaKeyConsentCategory = "consent_category"

// MetaKeyAboutKind / MetaKeyAboutKey carry the structured-dedup hint
// from PromptKit's `about` parameter. When both are set, Save treats
// (workspace, user, agent, about_kind, about_key) as a soft-unique
// key: a second write atomically supersedes the first under the same
// entity. Used for identity-class facts where the agent knows what
// attribute it is writing (name, location, single-valued
// preference). Free-form writes leave both empty and fall through
// to similarity-based dedup.
const (
	MetaKeyAboutKind = "about_kind"
	MetaKeyAboutKey  = "about_key"
)

// MetaKeyTitle / MetaKeySummary carry display fields for large
// memories (workspace docs, session summaries, skill manifests).
// Written to memory_entities.title and memory_observations.summary
// respectively so the recall path can return a synopsis instead of
// the full body.
//
// MetaKeyBodySize is the server-stamped octet length of the active
// observation's content. Surfaced via Memory.Metadata so the API DTO
// can decide whether to inline the full body or return a preview +
// has_full_body=true and let the agent fetch via memory__open.
const (
	MetaKeyTitle    = "title"
	MetaKeySummary  = "summary"
	MetaKeyBodySize = "body_size_bytes"
)

// stringFromMeta returns the trimmed lowercased value of meta[key]
// for the about_* keys (where consistent normalisation prevents
// silent dedup misses across casing or whitespace), and the raw
// trimmed value otherwise. Empty / missing → "".
func stringFromMeta(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, _ := meta[key].(string)
	v = strings.TrimSpace(v)
	if key == MetaKeyAboutKind || key == MetaKeyAboutKey {
		return strings.ToLower(v)
	}
	return v
}

// insertEntity inserts a new memory_entities row and populates mem.ID / mem.CreatedAt.
//
// trust_model and source_type are derived from the provenance metadata key
// (pkmemory.MetaKeyProvenance) so the redactor and retention pipelines can
// tell operator-curated / user-requested rows from agent-extracted ones.
// purpose is derived from the MetaKeyPurpose metadata key. In both cases a
// missing value falls through to the schema default, preserving behaviour
// for callers that haven't started tagging.
func insertEntity(ctx context.Context, tx pgx.Tx, mem *Memory) error {
	metaJSON, err := marshalMetadata(mem.Metadata)
	if err != nil {
		return err
	}

	trustModel, sourceType := trustFromProvenance(mem.Metadata)
	purpose := purposeFromMetadata(mem.Metadata)
	consentCategory := consentCategoryFromMetadata(mem.Metadata)

	row := tx.QueryRow(ctx, `
		INSERT INTO memory_entities
		  (workspace_id, virtual_user_id, agent_id, name, kind, metadata, expires_at,
		   trust_model, source_type, purpose, consent_category)
		VALUES
		  ($1, $2, $3, $4, $5, $6, $7,
		    COALESCE($8, 'inferred'),
		    COALESCE($9, 'conversation_extraction'),
		    COALESCE($10, 'support_continuity'),
		    $11)
		RETURNING id, created_at`,
		mem.Scope[ScopeWorkspaceID],
		scopeOrNil(mem.Scope, ScopeUserID),
		scopeOrNil(mem.Scope, ScopeAgentID),
		mem.Content, // entity name = content (short identifier)
		mem.Type,
		metaJSON,
		mem.ExpiresAt,
		trustModel,
		sourceType,
		purpose,
		consentCategory,
	)

	return row.Scan(&mem.ID, &mem.CreatedAt)
}

// trustFromProvenance maps a PromptKit provenance value to the
// (trust_model, source_type) pair persisted on memory_entities.
// Returns (nil, nil) when the caller didn't set a provenance so the
// schema-level defaults apply.
func trustFromProvenance(meta map[string]any) (trustModel, sourceType *string) {
	if meta == nil {
		return nil, nil
	}
	prov, _ := meta[pkmemory.MetaKeyProvenance].(string)
	switch prov {
	case string(pkmemory.ProvenanceUserRequested):
		tm, st := "explicit", "user_requested"
		return &tm, &st
	case string(pkmemory.ProvenanceOperatorCurated):
		tm, st := "curated", "operator_curated"
		return &tm, &st
	case string(pkmemory.ProvenanceAgentExtracted):
		tm, st := "inferred", "conversation_extraction"
		return &tm, &st
	case string(pkmemory.ProvenanceSystemGenerated):
		tm, st := "inferred", "system_generated"
		return &tm, &st
	default:
		return nil, nil
	}
}

// purposeFromMetadata returns a pointer to the Metadata[MetaKeyPurpose] value
// when set, or nil so the INSERT falls through to the schema default.
func purposeFromMetadata(meta map[string]any) *string {
	if meta == nil {
		return nil
	}
	v, ok := meta[MetaKeyPurpose].(string)
	if !ok || v == "" {
		return nil
	}
	return &v
}

// consentCategoryFromMetadata reads MetaKeyConsentCategory from the
// memory metadata, returning nil when absent so the column stays NULL
// and the row falls under the default retention policy rather than a
// per-category override.
func consentCategoryFromMetadata(meta map[string]any) *string {
	if meta == nil {
		return nil
	}
	v, ok := meta[MetaKeyConsentCategory].(string)
	if !ok || v == "" {
		return nil
	}
	return &v
}

// updateEntity updates the entity metadata and updated_at timestamp.
func updateEntity(ctx context.Context, tx pgx.Tx, mem *Memory) error {
	metaJSON, err := marshalMetadata(mem.Metadata)
	if err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `
		UPDATE memory_entities
		SET metadata = $1, updated_at = now(), expires_at = $2
		WHERE id = $3 AND workspace_id = $4`,
		metaJSON,
		mem.ExpiresAt,
		mem.ID,
		mem.Scope[ScopeWorkspaceID],
	)
	if err != nil {
		return fmt.Errorf("memory: update entity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory: entity %s not found in workspace", mem.ID)
	}
	return nil
}

// insertObservation appends an observation row linked to the entity.
// Carries the optional summary from MetaKeySummary so large memories
// (workspace docs, session compactions) surface a short blurb on
// recall without the agent paying the full body in context every
// time.
func insertObservation(ctx context.Context, tx pgx.Tx, mem *Memory) error {
	var turnRange []int
	if mem.TurnRange != [2]int{} {
		turnRange = mem.TurnRange[:]
	}

	var sessionID *string
	if mem.SessionID != "" {
		sessionID = &mem.SessionID
	}

	var summary *string
	if s := stringFromMeta(mem.Metadata, MetaKeySummary); s != "" {
		summary = &s
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO memory_observations (entity_id, content, summary, confidence, session_id, turn_range)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		mem.ID,
		mem.Content,
		summary,
		mem.Confidence,
		sessionID,
		turnRange,
	)
	if err != nil {
		return fmt.Errorf("memory: insert observation: %w", err)
	}
	return nil
}

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

// buildRetrieveQuery constructs the SQL and arguments for a Retrieve call.
// When query is non-empty it builds a FTS-scored variant; otherwise it
// returns the standard recency-ordered query.
func buildRetrieveQuery(scope map[string]string, query string, opts RetrieveOptions) (string, *pgutil.QueryBuilder) {
	qb := buildBaseMemoryQuery(scope, opts.Types, "")

	if opts.MinConfidence > 0 {
		qb.Add(confidenceFilter, opts.MinConfidence)
	}

	if query == "" {
		return formatMemorySQL(qb, opts.Limit, 0), qb
	}

	// FTS path: filter observations by tsquery match, pick the highest-
	// ranked observation per entity, then sort entities by that rank.
	// The query argument is referenced twice (WHERE + ORDER BY rank); we
	// capture its placeholder index so both spots see the same parameter.
	queryArgIdx := len(qb.Args()) + 1
	qb.Add("o.search_vector @@ websearch_to_tsquery('english', $?)", query)
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

	return scanMemories(rows, scope)
}

// buildListQuery constructs the SQL and arguments for a List call.
func buildListQuery(scope map[string]string, opts ListOptions) (string, *pgutil.QueryBuilder) {
	qb := buildBaseMemoryQuery(scope, opts.Types, "")
	return formatMemorySQL(qb, opts.Limit, opts.Offset), qb
}

// Delete performs a soft delete by setting forgotten = true on the entity.
func (s *PostgresMemoryStore) Delete(ctx context.Context, scope map[string]string, memoryID string) error {
	if scope[ScopeWorkspaceID] == "" {
		return errors.New(errWorkspaceRequired)
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE memory_entities SET forgotten = true, updated_at = now()
		WHERE id = $1 AND workspace_id = $2`,
		memoryID, scope[ScopeWorkspaceID])
	if err != nil {
		return fmt.Errorf("memory: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory: entity %s not found", memoryID)
	}
	return nil
}

// DeleteAll hard-deletes all entities (and cascading observations/relations) for the scope.
func (s *PostgresMemoryStore) DeleteAll(ctx context.Context, scope map[string]string) error {
	if scope[ScopeWorkspaceID] == "" {
		return errors.New(errWorkspaceRequired)
	}

	sql, qb := buildDeleteAllQuery(scope)

	_, err := s.pool.Exec(ctx, sql, qb.Args()...)
	if err != nil {
		return fmt.Errorf("memory: delete all: %w", err)
	}
	return nil
}

// buildDeleteAllQuery constructs the SQL and arguments for a DeleteAll call.
func buildDeleteAllQuery(scope map[string]string) (string, *pgutil.QueryBuilder) {
	var qb pgutil.QueryBuilder
	qb.Add(colWorkspaceID, scope[ScopeWorkspaceID])

	if uid := scope[ScopeUserID]; uid != "" {
		qb.Add(colVirtualUserID, uid)
	}

	sql := "DELETE FROM memory_entities WHERE 1=1" + qb.Where()

	return sql, &qb
}

// BatchDelete hard-deletes up to limit entities (and cascading observations/relations) for the scope.
// It returns the count of deleted rows. Use limit=500 in a loop until count=0 for DSAR cascades.
func (s *PostgresMemoryStore) BatchDelete(ctx context.Context, scope map[string]string, limit int) (int, error) {
	if scope[ScopeWorkspaceID] == "" {
		return 0, errors.New(errWorkspaceRequired)
	}

	sql, qb := buildBatchDeleteQuery(scope, limit)

	tag, err := s.pool.Exec(ctx, sql, qb.Args()...)
	if err != nil {
		return 0, fmt.Errorf("memory: batch delete: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// buildBatchDeleteQuery constructs the SQL and arguments for a BatchDelete call.
func buildBatchDeleteQuery(scope map[string]string, limit int) (string, *pgutil.QueryBuilder) {
	var qb pgutil.QueryBuilder
	qb.Add(colWorkspaceID, scope[ScopeWorkspaceID])

	if uid := scope[ScopeUserID]; uid != "" {
		qb.Add(colVirtualUserID, uid)
	}

	subquery := "SELECT id FROM memory_entities WHERE 1=1" + qb.Where()
	subquery = qb.AppendPagination(subquery, limit, 0)

	sql := "DELETE FROM memory_entities WHERE id IN (" + subquery + ")"

	return sql, &qb
}

// exportAllLimit is the maximum number of memories returned by ExportAll (DSAR cap).
const exportAllLimit = 10000

// ExportAll returns all memories for a scope without pagination (DSAR export).
// It uses a high limit cap to avoid unbounded result sets while still returning
// all practical user data.
func (s *PostgresMemoryStore) ExportAll(ctx context.Context, scope map[string]string) ([]*Memory, error) {
	if scope[ScopeWorkspaceID] == "" {
		return nil, errors.New(errWorkspaceRequired)
	}

	qb := buildBaseMemoryQuery(scope, nil, "")
	sql := formatMemorySQL(qb, exportAllLimit, 0)

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
		  AND ($3::text IS NULL OR e.virtual_user_id = $3)
		  AND ($4::uuid IS NULL OR e.agent_id = $4)
		  AND NOT e.forgotten
		ORDER BY o.observed_at DESC
		LIMIT 1`,
		entityID,
		scope[ScopeWorkspaceID],
		scopeOrNil(scope, ScopeUserID),
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

// LinkEntities inserts a row into memory_relations connecting source
// to target with the given relation_type. weight defaults to 1.0
// when zero. Returns the relation ID. Validates that both entities
// belong to the requested workspace before linking.
func (s *PostgresMemoryStore) LinkEntities(ctx context.Context, scope map[string]string,
	sourceEntityID, targetEntityID, relationType string, weight float64,
) (string, error) {
	if scope[ScopeWorkspaceID] == "" {
		return "", errors.New(errWorkspaceRequired)
	}
	if sourceEntityID == "" || targetEntityID == "" {
		return "", errors.New("memory: source and target entity IDs are required")
	}
	if relationType == "" {
		return "", errors.New("memory: relation_type is required")
	}
	if weight == 0 {
		weight = 1.0
	}

	var relationID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO memory_relations
		  (workspace_id, source_entity_id, target_entity_id, relation_type, weight)
		SELECT $1, $2, $3, $4, $5
		WHERE EXISTS (
		    SELECT 1 FROM memory_entities
		    WHERE id = $2 AND workspace_id = $1 AND NOT forgotten
		) AND EXISTS (
		    SELECT 1 FROM memory_entities
		    WHERE id = $3 AND workspace_id = $1 AND NOT forgotten
		)
		RETURNING id`,
		scope[ScopeWorkspaceID],
		sourceEntityID,
		targetEntityID,
		relationType,
		weight,
	).Scan(&relationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("memory: link entities: %w", err)
	}
	return relationID, nil
}

// scanSingleMemory decodes one row in the same shape as scanMemories
// returns. Inlined here so the hot path of GetMemory doesn't allocate
// for the multi-row scanner. Carries the title (entity), summary +
// body_size_bytes (observation) extracted in selectEntityCols /
// selectObserveCols — they get stamped onto Metadata so the recall
// DTO can decide between inline content and a preview.
func scanSingleMemory(row pgx.Row, scope map[string]string) (*Memory, error) {
	var (
		id, kind, content     string
		metaJSON              []byte
		createdAt, observedAt time.Time
		expiresAt, accessedAt *time.Time
		confidence            float64
		sessionID             *string
		turnRange             []int
		title, summary        *string
		bodySizeBytes         *int32
	)
	if err := row.Scan(&id, &kind, &metaJSON, &createdAt, &expiresAt, &title,
		&content, &confidence, &sessionID, &turnRange, &observedAt, &accessedAt,
		&summary, &bodySizeBytes); err != nil {
		return nil, err
	}

	var meta map[string]any
	if len(metaJSON) > 0 {
		if err := json.Unmarshal(metaJSON, &meta); err != nil {
			return nil, fmt.Errorf("memory: decode metadata: %w", err)
		}
	}

	mem := &Memory{
		ID:         id,
		Type:       kind,
		Content:    content,
		Confidence: confidence,
		Metadata:   meta,
		Scope:      maps.Clone(scope),
		CreatedAt:  createdAt,
		ExpiresAt:  expiresAt,
	}
	if sessionID != nil {
		mem.SessionID = *sessionID
	}
	if len(turnRange) >= 2 {
		mem.TurnRange = [2]int{turnRange[0], turnRange[1]}
	}
	if accessedAt != nil {
		mem.AccessedAt = *accessedAt
	}
	stampLargeMemoryFields(mem, title, summary, bodySizeBytes)
	// observedAt isn't on the PromptKit Memory struct; if we ever
	// surface "when was this observed" separately from CreatedAt
	// it'll need to ride in metadata.
	_ = observedAt
	return mem, nil
}

// stampLargeMemoryFields populates Metadata with the title / summary
// / body-size fields read from dedicated columns. They round-trip
// the same keys callers used at write time so the API DTO can
// extract them without having to know about column-vs-JSON
// duality. Existing JSON metadata values are overwritten — the
// dedicated columns are the source of truth post-migration.
func stampLargeMemoryFields(mem *Memory, title, summary *string, bodySizeBytes *int32) {
	if title == nil && summary == nil && bodySizeBytes == nil {
		return
	}
	if mem.Metadata == nil {
		mem.Metadata = map[string]any{}
	}
	if title != nil && *title != "" {
		mem.Metadata[MetaKeyTitle] = *title
	}
	if summary != nil && *summary != "" {
		mem.Metadata[MetaKeySummary] = *summary
	}
	if bodySizeBytes != nil {
		mem.Metadata[MetaKeyBodySize] = int(*bodySizeBytes)
	}
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

// AppendObservationToEntity attaches a new observation to an existing
// entity, marking all prior active observations as superseded in the
// same transaction. Used by the embedding-similarity dedup path: when
// SaveMemoryWithResult finds a match above the auto-supersede
// threshold, it routes the write through this helper instead of
// creating a new entity. The structured-key path doesn't need this —
// upsertEntityByAboutKey + supersedePriorObservations already do the
// equivalent in Save.
//
// Mutates mem.ID to entityID. Returns the observation IDs that were
// marked superseded so SaveResult can surface them to the agent.
func (s *PostgresMemoryStore) AppendObservationToEntity(
	ctx context.Context,
	entityID string,
	mem *Memory,
) ([]string, error) {
	if entityID == "" {
		return nil, errors.New("memory: entityID required")
	}
	if mem.Scope[ScopeWorkspaceID] == "" {
		return nil, errors.New(errWorkspaceRequired)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("memory: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	mem.ID = entityID

	supersededIDs, err := supersedePriorObservations(ctx, tx, entityID)
	if err != nil {
		return nil, err
	}

	if err := insertObservation(ctx, tx, mem); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("memory: commit append: %w", err)
	}
	return supersededIDs, nil
}

// SupersedeMany atomically marks every source entity's active
// observations inactive and writes a new active observation under
// the first source entity. See Store.SupersedeMany for the agent-
// facing semantics. The two-step pattern (supersede then insert)
// runs inside one transaction so a failure between steps doesn't
// strand the caller with half-applied state.
func (s *PostgresMemoryStore) SupersedeMany(
	ctx context.Context,
	sourceMemoryIDs []string,
	mem *Memory,
) (string, []string, error) {
	if len(sourceMemoryIDs) == 0 {
		return "", nil, errors.New("memory: at least one source memory ID is required")
	}
	if mem.Scope[ScopeWorkspaceID] == "" {
		return "", nil, errors.New(errWorkspaceRequired)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("memory: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Verify every source entity belongs to the requested workspace
	// (and user / agent when those scope keys are set). Cross-tenant
	// supersede must fail loudly, not silently miss rows.
	if err := assertEntitiesInScope(ctx, tx, sourceMemoryIDs, mem.Scope); err != nil {
		return "", nil, err
	}

	var allSuperseded []string
	for _, id := range sourceMemoryIDs {
		ids, err := supersedePriorObservations(ctx, tx, id)
		if err != nil {
			return "", nil, err
		}
		allSuperseded = append(allSuperseded, ids...)
	}

	anchor := sourceMemoryIDs[0]
	mem.ID = anchor
	if err := insertObservation(ctx, tx, mem); err != nil {
		return "", nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", nil, fmt.Errorf("memory: commit supersede: %w", err)
	}
	return anchor, allSuperseded, nil
}

// assertEntitiesInScope rejects the request if any of entityIDs
// resolves to a row outside the requested workspace / user / agent
// scope. Cheap single-statement guard so the supersede transaction
// can't bleed across tenants.
func assertEntitiesInScope(ctx context.Context, tx pgx.Tx, entityIDs []string, scope map[string]string) error {
	row := tx.QueryRow(ctx, `
		SELECT count(*) FROM memory_entities
		WHERE id = ANY($1)
		  AND workspace_id = $2
		  AND ($3::text IS NULL OR virtual_user_id = $3)
		  AND ($4::uuid IS NULL OR agent_id = $4)
		  AND NOT forgotten`,
		entityIDs,
		scope[ScopeWorkspaceID],
		scopeOrNil(scope, ScopeUserID),
		scopeOrNil(scope, ScopeAgentID),
	)
	var n int
	if err := row.Scan(&n); err != nil {
		return fmt.Errorf("memory: scope assertion: %w", err)
	}
	if n != len(entityIDs) {
		return fmt.Errorf("memory: %d of %d source entities not found in scope",
			len(entityIDs)-n, len(entityIDs))
	}
	return nil
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

	rows, err := s.pool.Query(ctx, `
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
		scopeOrNil(scope, ScopeUserID),
		scopeOrNil(scope, ScopeAgentID),
		maxDistance,
		k,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: similar observations: %w", err)
	}
	defer rows.Close()

	var out []SimilarObservation
	for rows.Next() {
		var m SimilarObservation
		if err := rows.Scan(&m.ObservationID, &m.EntityID, &m.Content, &m.Similarity); err != nil {
			return nil, fmt.Errorf("memory: similar scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

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
      AND coalesce(o.confidence, 0.7) >= $8
      AND o.search_vector @@ websearch_to_tsquery('english', $4)
    ORDER BY e.id, fts_rank DESC
    LIMIT $6
), fts_ranked AS (
    SELECT entity_id, row_number() OVER (ORDER BY fts_rank DESC) AS fts_rn
    FROM fts
), cosine AS (
    SELECT DISTINCT ON (e.id)
        e.id AS entity_id,
        o.embedding <=> $5 AS cos_dist
    FROM memory_entities e
    JOIN memory_observations o ON o.entity_id = e.id
        AND o.superseded_by IS NULL
        AND (o.valid_until IS NULL OR o.valid_until > now())
    WHERE e.workspace_id = $1
      AND ($2::text IS NULL OR e.virtual_user_id = $2)
      AND ($3::uuid IS NULL OR e.agent_id = $3)
      AND e.forgotten = false
      AND coalesce(o.confidence, 0.7) >= $8
      AND o.embedding IS NOT NULL
    ORDER BY e.id, cos_dist
    LIMIT $6
), cosine_ranked AS (
    SELECT entity_id, row_number() OVER (ORDER BY cos_dist) AS cos_rn
    FROM cosine
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
        * coalesce(o.confidence, 0.7)
        * exp(-EXTRACT(EPOCH FROM (now() - o.observed_at)) / 2592000.0) AS final_score
FROM fused
JOIN memory_entities e ON e.id = fused.entity_id
JOIN memory_observations o ON o.entity_id = e.id
    AND o.superseded_by IS NULL
    AND (o.valid_until IS NULL OR o.valid_until > now())
WHERE e.forgotten = false
ORDER BY e.id, o.observed_at DESC, final_score DESC
LIMIT $7`

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

	rows, err := s.pool.Query(ctx, hybridRetrieveSQL,
		scope[ScopeWorkspaceID],
		scopeOrNil(scope, ScopeUserID),
		scopeOrNil(scope, ScopeAgentID),
		query,
		pgvector.NewVector(queryEmbedding),
		fanout,
		limit,
		minConfidence,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: hybrid retrieve: %w", err)
	}
	defer rows.Close()

	mems, err := scanHybridMemories(rows, scope)
	if err != nil {
		return nil, err
	}
	s.touchAccessedOnRead(entityIDsFromMemories(mems))
	return mems, nil
}

// scanHybridMemories scans the hybrid-retrieve row set, which carries
// one trailing column (final_score) beyond the standard scanMemory
// shape. The score is read and discarded — it influenced ordering at
// the SQL layer; callers don't need it on the returned Memory.
func scanHybridMemories(rows pgx.Rows, scope map[string]string) ([]*Memory, error) {
	var results []*Memory
	for rows.Next() {
		mem, err := scanHybridMemory(rows, scope)
		if err != nil {
			return nil, err
		}
		results = append(results, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: hybrid rows iteration: %w", err)
	}
	if results == nil {
		results = []*Memory{}
	}
	return results, nil
}

// scanHybridMemory scans a single row from the hybrid-retrieve query.
// Mirrors scanMemory but with one extra trailing float64 column for
// the final_score (consumed and discarded — present in SELECT only
// so the row order is deterministic).
func scanHybridMemory(row pgx.Rows, scope map[string]string) (*Memory, error) {
	var (
		mem            Memory
		metadataJSON   []byte
		expiresAt      *time.Time
		sessionID      *string
		turnRange      []int
		observedAt     *time.Time
		accessedAt     *time.Time
		title, summary *string
		bodySizeBytes  *int32
		finalScore     float64
	)

	if err := row.Scan(
		&mem.ID, &mem.Type, &metadataJSON, &mem.CreatedAt, &expiresAt, &title,
		&mem.Content, &mem.Confidence, &sessionID, &turnRange, &observedAt, &accessedAt,
		&summary, &bodySizeBytes,
		&finalScore,
	); err != nil {
		return nil, fmt.Errorf("memory: scan hybrid row: %w", err)
	}

	mem.Scope = copyScope(scope)
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
	_ = observedAt // observed_at influenced ordering; not surfaced on Memory
	return &mem, nil
}

// UpdateEmbedding sets the embedding vector on the latest observation for an entity.
func (s *PostgresMemoryStore) UpdateEmbedding(ctx context.Context, entityID string, embedding []float32) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE memory_observations
		SET embedding = $1
		WHERE id = (
			SELECT id FROM memory_observations
			WHERE entity_id = $2
			ORDER BY observed_at DESC
			LIMIT 1
		)`, pgvector.NewVector(embedding), entityID)
	if err != nil {
		return fmt.Errorf("memory: update embedding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory: no observation found for entity %s", entityID)
	}
	return nil
}

// MissingEmbedding describes one observation that needs re-embedding.
// Returned by FindObservationsMissingEmbedding so the worker has the
// content to feed the provider plus the row identity to write back.
type MissingEmbedding struct {
	ObservationID string
	EntityID      string
	Content       string
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

// UpdateObservationEmbedding writes the embedding + the model name
// for one specific observation. Distinct from UpdateEmbedding (which
// targets the latest observation per entity) — the re-embed worker
// needs to address rows by ID since one entity may have several
// observations all needing different embeddings.
func (s *PostgresMemoryStore) UpdateObservationEmbedding(
	ctx context.Context, observationID string, embedding []float32, modelName string,
) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE memory_observations
		SET embedding = $1, embedding_model = $2
		WHERE id = $3`,
		pgvector.NewVector(embedding), modelName, observationID)
	if err != nil {
		return fmt.Errorf("memory: update observation embedding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory: observation %s not found", observationID)
	}
	return nil
}

// ExpireMemories deletes entities past their expires_at timestamp.
// Returns the number of expired entities.
func (s *PostgresMemoryStore) ExpireMemories(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		"DELETE FROM memory_entities WHERE expires_at IS NOT NULL AND expires_at < now()")
	if err != nil {
		return 0, fmt.Errorf("memory: expire: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListWorkspaceIDs returns the distinct set of workspace IDs that currently
// hold at least one non-forgotten memory entity. Used by background workers
// that need to iterate workspaces without an external discovery mechanism
// (e.g. the compaction worker).
func (s *PostgresMemoryStore) ListWorkspaceIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		"SELECT DISTINCT workspace_id FROM memory_entities WHERE forgotten = false")
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

// --- helpers -----------------------------------------------------------------

// defaultMemoryLimit is applied when no explicit limit is provided.
const defaultMemoryLimit = 50

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

// addScopeFilters appends optional user_id and agent_id filters.
func addScopeFilters(qb *pgutil.QueryBuilder, scope map[string]string) {
	if uid := scope[ScopeUserID]; uid != "" {
		qb.Add(colVirtualUserID, uid)
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

// scanMemories collects Memory structs from query rows.
func scanMemories(rows pgx.Rows, scope map[string]string) ([]*Memory, error) {
	var results []*Memory
	for rows.Next() {
		mem, err := scanMemory(rows, scope)
		if err != nil {
			return nil, err
		}
		results = append(results, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: rows iteration: %w", err)
	}
	if results == nil {
		results = []*Memory{}
	}
	return results, nil
}

// scanMemory scans a single row into a Memory.
func scanMemory(row pgx.Rows, scope map[string]string) (*Memory, error) {
	var (
		mem            Memory
		metadataJSON   []byte
		expiresAt      *time.Time
		sessionID      *string
		turnRange      []int
		observedAt     *time.Time
		accessedAt     *time.Time
		title, summary *string
		bodySizeBytes  *int32
	)

	err := row.Scan(
		&mem.ID, &mem.Type, &metadataJSON, &mem.CreatedAt, &expiresAt, &title,
		&mem.Content, &mem.Confidence, &sessionID, &turnRange, &observedAt, &accessedAt,
		&summary, &bodySizeBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: scan row: %w", err)
	}

	mem.Scope = copyScope(scope)
	mem.ExpiresAt = expiresAt
	if sessionID != nil {
		mem.SessionID = *sessionID
	}
	if len(turnRange) == 2 {
		mem.TurnRange = [2]int{turnRange[0], turnRange[1]}
	}
	stampLargeMemoryFields(&mem, title, summary, bodySizeBytes)
	if accessedAt != nil {
		mem.AccessedAt = *accessedAt
	}
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &mem.Metadata)
	}

	return &mem, nil
}

// copyScope returns a shallow copy of the scope map.
func copyScope(scope map[string]string) map[string]string {
	out := make(map[string]string, len(scope))
	maps.Copy(out, scope)
	return out
}

// scopeOrNil returns a *string for the given scope key, or nil if absent.
func scopeOrNil(scope map[string]string, key string) *string {
	if v, ok := scope[key]; ok && v != "" {
		return &v
	}
	return nil
}

// marshalMetadata serializes metadata to JSON, defaulting to "{}".
func marshalMetadata(meta map[string]any) ([]byte, error) {
	if len(meta) == 0 {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("memory: marshal metadata: %w", err)
	}
	return b, nil
}
