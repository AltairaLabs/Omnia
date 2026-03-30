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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

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
const errWorkspaceRequired = "memory: workspace_id scope is required"

// SQL column/filter constants to avoid duplication (SonarCloud S1192).
const (
	colWorkspaceID    = "workspace_id=$?"
	colVirtualUserID  = "virtual_user_id=$?"
	colEntityForgot   = "e.forgotten = false"
	entityKindFilter  = "e.kind=$?"
	confidenceFilter  = "o.confidence >= $?"
	observationJoin   = " JOIN memory_observations o ON o.entity_id = e.id"
	entityTableAlias  = "e"
	selectEntityCols  = "e.id, e.kind, e.metadata, e.created_at, e.expires_at"
	selectObserveCols = "o.content, o.confidence, o.session_id, o.turn_range, o.observed_at, o.accessed_at"
)

// PostgresMemoryStore implements Store against the memory_entities / memory_observations
// PostgreSQL tables created by migration 000025.
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
func (s *PostgresMemoryStore) Save(ctx context.Context, mem *Memory) error {
	if mem.Scope[ScopeWorkspaceID] == "" {
		return errors.New(errWorkspaceRequired)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("memory: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if mem.ID == "" {
		if err := insertEntity(ctx, tx, mem); err != nil {
			return err
		}
	} else {
		if err := updateEntity(ctx, tx, mem); err != nil {
			return err
		}
	}

	if err := insertObservation(ctx, tx, mem); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// insertEntity inserts a new memory_entities row and populates mem.ID / mem.CreatedAt.
func insertEntity(ctx context.Context, tx pgx.Tx, mem *Memory) error {
	metaJSON, err := marshalMetadata(mem.Metadata)
	if err != nil {
		return err
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, virtual_user_id, agent_id, name, kind, metadata, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		mem.Scope[ScopeWorkspaceID],
		scopeOrNil(mem.Scope, ScopeUserID),
		scopeOrNil(mem.Scope, ScopeAgentID),
		mem.Content, // entity name = content (short identifier)
		mem.Type,
		metaJSON,
		mem.ExpiresAt,
	)

	return row.Scan(&mem.ID, &mem.CreatedAt)
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
func insertObservation(ctx context.Context, tx pgx.Tx, mem *Memory) error {
	var turnRange []int
	if mem.TurnRange != [2]int{} {
		turnRange = mem.TurnRange[:]
	}

	var sessionID *string
	if mem.SessionID != "" {
		sessionID = &mem.SessionID
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO memory_observations (entity_id, content, confidence, session_id, turn_range)
		VALUES ($1, $2, $3, $4, $5)`,
		mem.ID,
		mem.Content,
		mem.Confidence,
		sessionID,
		turnRange,
	)
	if err != nil {
		return fmt.Errorf("memory: insert observation: %w", err)
	}
	return nil
}

// Retrieve returns memories matching scope, a substring query, and options.
// Results are ordered by observed_at DESC and limited to opts.Limit (default 50).
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

	return scanMemories(rows, scope)
}

// buildRetrieveQuery constructs the SQL and arguments for a Retrieve call.
func buildRetrieveQuery(scope map[string]string, query string, opts RetrieveOptions) (string, *pgutil.QueryBuilder) {
	qb := buildBaseMemoryQuery(scope, opts.Types, "")

	if opts.MinConfidence > 0 {
		qb.Add(confidenceFilter, opts.MinConfidence)
	}
	if query != "" {
		qb.Add("o.content ILIKE $?", "%"+query+"%")
	}

	return formatMemorySQL(qb, opts.Limit, 0), qb
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
		mem          Memory
		metadataJSON []byte
		expiresAt    *time.Time
		sessionID    *string
		turnRange    []int
		observedAt   *time.Time
		accessedAt   *time.Time
	)

	err := row.Scan(
		&mem.ID, &mem.Type, &metadataJSON, &mem.CreatedAt, &expiresAt,
		&mem.Content, &mem.Confidence, &sessionID, &turnRange, &observedAt, &accessedAt,
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
