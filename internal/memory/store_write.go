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

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

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
	if mem.Scope[ScopeVirtualUserID] == "" {
		return nil, errors.New(errUserIDRequired)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf(errBeginTxFormat, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	res := &SaveResult{Action: SaveActionAdded}
	if err := resolveSaveEntity(ctx, tx, mem, res); err != nil {
		return nil, err
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

// resolveSaveEntity creates, reuses, or updates the entity for mem within
// tx, populating mem.ID. On the structured-key dedup path it supersedes any
// prior active observation and records the supersession on res. Extracted
// from SaveWithResult to keep that method's cognitive complexity in budget.
func resolveSaveEntity(ctx context.Context, tx pgx.Tx, mem *Memory, res *SaveResult) error {
	// Structured-key dedup path: when the caller passed
	// about_kind+about_key, look up (or atomically create) the entity
	// keyed by (scope, about_kind, about_key) and supersede any prior
	// active observation under it. This is what fixes the "user
	// changes name and old name still shows up" failure.
	switch {
	case mem.ID == "" && hasAboutKey(mem):
		conflicted, err := upsertEntityByAboutKey(ctx, tx, mem)
		if err != nil {
			return err
		}
		if conflicted {
			supersededIDs, err := supersedePriorObservations(ctx, tx, mem.ID)
			if err != nil {
				return err
			}
			res.Action = SaveActionAutoSuperseded
			res.SupersededObservationIDs = supersededIDs
			res.SupersedeReason = ReasonStructuredKey
		}
	case mem.ID == "":
		return insertEntity(ctx, tx, mem)
	default:
		return updateEntity(ctx, tx, mem)
	}
	return nil
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
		scopeOrNil(mem.Scope, ScopeVirtualUserID),
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
		scopeOrNil(mem.Scope, ScopeVirtualUserID),
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

// updateEntity updates the entity metadata and updated_at timestamp.
func updateEntity(ctx context.Context, tx pgx.Tx, mem *Memory) error {
	metaJSON, err := marshalMetadata(mem.Metadata)
	if err != nil {
		return err
	}

	// Scope guard: a workspace-only check would let a caller in
	// workspace W rewrite any user's entity in W. Always require the
	// caller's user/agent partition match the row's; missing scope
	// keys mean "must also be NULL on the row" so an institutional
	// caller can't mutate user-scoped rows and vice versa.
	tag, err := tx.Exec(ctx, `
		UPDATE memory_entities
		SET metadata = $1, updated_at = now(), expires_at = $2
		WHERE id = $3
		  AND workspace_id = $4
		  AND virtual_user_id IS NOT DISTINCT FROM $5::text
		  AND agent_id IS NOT DISTINCT FROM $6::uuid`,
		metaJSON,
		mem.ExpiresAt,
		mem.ID,
		mem.Scope[ScopeWorkspaceID],
		scopeOrNil(mem.Scope, ScopeVirtualUserID),
		scopeOrNil(mem.Scope, ScopeAgentID),
	)
	if err != nil {
		return fmt.Errorf("memory: update entity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory: entity %s not found in scope: %w", mem.ID, ErrNotFound)
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
		return nil, fmt.Errorf(errBeginTxFormat, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Defence in depth: callers (embedding-similarity auto-supersede)
	// pass an entityID that the store-level FindSimilarObservations
	// returned. Re-check the entity is in the caller's scope before
	// mutating it — a future bug in similarity scoping must not
	// silently let one user's observation supersede another's.
	if err := assertEntitiesInScope(ctx, tx, []string{entityID}, mem.Scope); err != nil {
		return nil, err
	}

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
	// User scope is mandatory at the store layer too — the HTTP
	// handler already enforces this, but other callers (gRPC,
	// in-process) must not be able to supersede across all users
	// in a workspace by omitting user_id.
	if mem.Scope[ScopeVirtualUserID] == "" {
		return "", nil, errors.New(errUserIDRequired)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", nil, fmt.Errorf(errBeginTxFormat, err)
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
//
// Duplicate IDs in the input are deduped before the count check —
// `id = ANY($1)` returns one row per matching id regardless of how
// many times the id appears in the array, so without dedup a
// duplicate caller-side would always trip the missing-rows path
// with a confusing "1 of 2 source entities not found" message.
//
// Uses SELECT … FOR KEY SHARE: only blocks foreign-key-like
// changes to the row (the Delete cascade that sets forgotten=true)
// without blocking concurrent supersedes that target the entity's
// observations. FOR UPDATE would serialise every concurrent
// SaveWithResult / SupersedeMany hitting the same anchor entity —
// the structured-key supersede path can have many writers per
// second on the user-identity row, and serialising them would tank
// p99 under load.
func assertEntitiesInScope(ctx context.Context, tx pgx.Tx, entityIDs []string, scope map[string]string) error {
	uniqueIDs := dedupeStrings(entityIDs)
	rows, err := tx.Query(ctx, `
		SELECT id FROM memory_entities
		WHERE id = ANY($1)
		  AND workspace_id = $2
		  AND ($3::text IS NULL OR virtual_user_id = $3)
		  AND ($4::uuid IS NULL OR agent_id = $4)
		  AND NOT forgotten
		FOR KEY SHARE`,
		uniqueIDs,
		scope[ScopeWorkspaceID],
		scopeOrNil(scope, ScopeVirtualUserID),
		scopeOrNil(scope, ScopeAgentID),
	)
	if err != nil {
		return fmt.Errorf("memory: scope assertion: %w", err)
	}
	defer rows.Close()
	var n int
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("memory: scope assertion iter: %w", err)
	}
	if n != len(uniqueIDs) {
		return fmt.Errorf("memory: %d of %d source entities not found in scope: %w",
			len(uniqueIDs)-n, len(uniqueIDs), ErrNotFound)
	}
	return nil
}

// UpdateEmbedding sets the embedding vector and model name on the
// latest observation for an entity. modelName is the identifier of
// the embedding provider that produced the vector — written so the
// re-embed worker can detect when a new model has been configured
// and re-embed everything from the old generation. Empty modelName
// is allowed (tests and back-compat callers without a known
// provider) and clears the column.
func (s *PostgresMemoryStore) UpdateEmbedding(ctx context.Context, entityID string, embedding []float32, modelName string) error {
	var modelArg any
	if modelName != "" {
		modelArg = modelName
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE memory_observations
		SET embedding = $1, embedding_model = $2
		WHERE id = (
			SELECT id FROM memory_observations
			WHERE entity_id = $3
			ORDER BY observed_at DESC
			LIMIT 1
		)`, pgvector.NewVector(embedding), modelArg, entityID)
	if err != nil {
		return fmt.Errorf("memory: update embedding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory: no observation found for entity %s", entityID)
	}
	return nil
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
		return fmt.Errorf("memory: observation %s not found: %w", observationID, ErrNotFound)
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
