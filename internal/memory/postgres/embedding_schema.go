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

package postgres

// embedding_schema.go owns the pgvector embedding columns and their indexes.
//
// These are deliberately NOT created by the SQL migrations (see #1309 and the
// note in 000001_initial_schema.up.sql). The column dimension must match the
// configured embedding provider, which isn't known until memory-api boots, so
// the migration can't hardcode it. EnsureEmbeddingSchema runs once at startup,
// after migrations and after the provider is built, and brings both embedding
// columns to the provider's Dimensions(). It is the single source of truth for
// the embedding-column shape.
//
// Changing the dimension on a store that already holds embeddings is
// destructive (every vector is discarded and must be re-embedded). That path
// is gated by a single-use consent marker (see consent.go) so it can never
// happen by accident.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	embeddingColumn   = "embedding"
	tableObservations = "memory_observations"
	tableEntities     = "memory_entities"
	consentTable      = "memory_embedding_dim_change_consent"

	// embeddingSchemaAdvisoryLock serialises EnsureEmbeddingSchema across
	// memory-api replicas so they can't race the DDL. Keyed on the issue.
	embeddingSchemaAdvisoryLock int64 = 1309

	// MaxIndexableEmbeddingDim is pgvector's HNSW/IVFFlat dimension cap. A
	// larger vector can be stored but not indexed, so the reconciler (and the
	// admin consent endpoint) reject it rather than let CREATE INDEX fail at
	// startup and crash-loop the pod. >2000 would need halfvec — out of scope.
	MaxIndexableEmbeddingDim = 2000
)

// vectorDimRe extracts N from a pgvector type rendering, tolerating an optional
// schema qualifier (e.g. "vector(768)" or "extensions.vector(768)").
var vectorDimRe = regexp.MustCompile(`^(?:[^.()]+\.)?vector\((\d+)\)$`)

// pgExecutor is the subset of the pgx API shared by *pgxpool.Pool and pgx.Tx,
// so the helpers below work against either a pool or a transaction.
type pgExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// embeddingTableSpec captures everything needed to manage one embedding
// column. All SQL is a compile-time constant except the ADD COLUMN, which
// formats only an int dimension — there is no string-built SQL from input.
type embeddingTableSpec struct {
	name       string
	hasDataSQL string
	dropSQL    string
	addFmt     string // single %d for the dimension
	indexSQL   string // empty when the table has no embedding index
}

// embeddingTables lists the columns EnsureEmbeddingSchema manages. Index
// policy matches the pre-#1309 schema: observations carry an HNSW index;
// entities are unindexed (the entity vector is read-only by consolidation
// dup-detection and was never indexed after migration 000007).
var embeddingTables = []embeddingTableSpec{
	{
		name:       tableObservations,
		hasDataSQL: `SELECT EXISTS(SELECT 1 FROM memory_observations WHERE embedding IS NOT NULL)`,
		dropSQL:    `ALTER TABLE memory_observations DROP COLUMN embedding`,
		addFmt:     `ALTER TABLE memory_observations ADD COLUMN embedding vector(%d)`,
		indexSQL:   `CREATE INDEX idx_memory_observations_embedding ON memory_observations USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64)`,
	},
	{
		name:       tableEntities,
		hasDataSQL: `SELECT EXISTS(SELECT 1 FROM memory_entities WHERE embedding IS NOT NULL)`,
		dropSQL:    `ALTER TABLE memory_entities DROP COLUMN embedding`,
		addFmt:     `ALTER TABLE memory_entities ADD COLUMN embedding vector(%d)`,
	},
}

// EnsureEmbeddingSchema brings both embedding columns to dim, creating them if
// absent and reshaping them if the dimension changed. A reshape that would
// discard existing embeddings requires a matching one-shot consent marker; the
// marker is consumed atomically with the reshape. The whole operation runs in
// one transaction under an advisory lock so concurrent replicas don't race.
func EnsureEmbeddingSchema(ctx context.Context, pool *pgxpool.Pool, dim int, log logr.Logger) error {
	if dim <= 0 {
		return fmt.Errorf("memory: invalid embedding dimension %d", dim)
	}
	if dim > MaxIndexableEmbeddingDim {
		return fmt.Errorf("memory: embedding dimension %d exceeds the maximum indexable dimension %d (pgvector HNSW cap); configure an embedding model with <= %d dimensions",
			dim, MaxIndexableEmbeddingDim, MaxIndexableEmbeddingDim)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("memory: begin embedding schema tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := reconcileEmbeddingTx(ctx, tx, dim, log); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("memory: commit embedding schema tx: %w", err)
	}
	return nil
}

// reconcileEmbeddingTx performs the locked reconcile inside an open
// transaction: take the advisory lock, gate destructive changes on consent,
// bring every embedding column to dim, then settle the consent marker.
func reconcileEmbeddingTx(ctx context.Context, tx pgExecutor, dim int, log logr.Logger) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, embeddingSchemaAdvisoryLock); err != nil {
		return fmt.Errorf("memory: embedding schema advisory lock: %w", err)
	}
	if err := ensureConsentTable(ctx, tx); err != nil {
		return err
	}

	destructive, err := needsDestructiveReshape(ctx, tx, dim)
	if err != nil {
		return err
	}
	if destructive {
		if err := requireConsent(ctx, tx, dim); err != nil {
			return err
		}
	}

	for i := range embeddingTables {
		if err := reconcileColumn(ctx, tx, embeddingTables[i], dim, log); err != nil {
			return err
		}
	}

	return settleConsent(ctx, tx, dim, destructive, log)
}

// settleConsent consumes the one-shot marker after a destructive reshape. On
// any non-destructive reconcile it clears ALL markers: a marker that wasn't
// consumed this run authorises a change that isn't happening (a different
// target, or one made moot by an empty-column reshape), so it must not survive
// to silently permit a later swap.
func settleConsent(ctx context.Context, tx pgExecutor, dim int, destructive bool, log logr.Logger) error {
	if destructive {
		if err := consumeConsent(ctx, tx); err != nil {
			return err
		}
		log.Info("embedding dimension change consent consumed", "dimensions", dim)
		return nil
	}
	return clearStaleConsent(ctx, tx)
}

// needsDestructiveReshape reports whether any embedding column holds real data
// at a dimension other than the target — the only situation that loses data
// and therefore needs consent. In practice only memory_observations is ever
// populated; memory_entities.embedding is never written.
func needsDestructiveReshape(ctx context.Context, db pgExecutor, dim int) (bool, error) {
	for i := range embeddingTables {
		spec := embeddingTables[i]
		cur, present, err := currentEmbeddingDim(ctx, db, spec.name)
		if err != nil {
			return false, err
		}
		if !present || cur == dim {
			continue
		}
		hasData, err := hasEmbeddings(ctx, db, spec)
		if err != nil {
			return false, err
		}
		if hasData {
			return true, nil
		}
	}
	return false, nil
}

// requireConsent fails unless a one-shot consent marker authorises a change to
// exactly dim. The error tells the operator how to record consent.
func requireConsent(ctx context.Context, db pgExecutor, dim int) error {
	want, ok, err := readConsent(ctx, db)
	if err != nil {
		return err
	}
	if !ok || want != dim {
		return fmt.Errorf("memory: changing the embedding dimension to %d would discard existing embeddings and requires one-shot consent (marker present=%t target=%d); record it via POST /admin/embedding-dimension-change {\"target_dim\":%d} or INSERT INTO %s (target_dim) VALUES (%d)",
			dim, ok, want, dim, consentTable, dim)
	}
	return nil
}

// reconcileColumn creates the embedding column if absent, no-ops if it already
// matches dim, and otherwise drops + recreates it at dim (dropping the column
// drops any dependent index, which is then recreated). Callers must have
// already cleared the consent gate for any destructive case.
func reconcileColumn(ctx context.Context, db pgExecutor, spec embeddingTableSpec, dim int, log logr.Logger) error {
	cur, present, err := currentEmbeddingDim(ctx, db, spec.name)
	if err != nil {
		return err
	}
	if present && cur == dim {
		return nil
	}
	if present {
		if _, err := db.Exec(ctx, spec.dropSQL); err != nil {
			return fmt.Errorf("memory: drop %s.%s: %w", spec.name, embeddingColumn, err)
		}
		log.Info("embedding column reshaped", "table", spec.name, "fromDim", cur, "toDim", dim)
	} else {
		log.Info("embedding column created", "table", spec.name, "dimensions", dim)
	}
	if _, err := db.Exec(ctx, fmt.Sprintf(spec.addFmt, dim)); err != nil {
		return fmt.Errorf("memory: add %s.%s: %w", spec.name, embeddingColumn, err)
	}
	if spec.indexSQL != "" {
		if _, err := db.Exec(ctx, spec.indexSQL); err != nil {
			return fmt.Errorf("memory: index %s.%s: %w", spec.name, embeddingColumn, err)
		}
	}
	return nil
}

// currentEmbeddingDim returns the declared dimension of the embedding column
// and whether the column exists. An unconstrained "vector" column reports
// present with dim 0.
func currentEmbeddingDim(ctx context.Context, db pgExecutor, table string) (dim int, present bool, err error) {
	var ft string
	err = db.QueryRow(ctx, `
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = $1 AND a.attname = $2
		  AND a.attnum > 0 AND NOT a.attisdropped`, table, embeddingColumn).Scan(&ft)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("memory: introspect %s.%s: %w", table, embeddingColumn, err)
	}
	m := vectorDimRe.FindStringSubmatch(ft)
	if m == nil {
		return 0, true, nil
	}
	n, _ := strconv.Atoi(m[1])
	return n, true, nil
}

// hasEmbeddings reports whether the column holds at least one non-NULL vector.
func hasEmbeddings(ctx context.Context, db pgExecutor, spec embeddingTableSpec) (bool, error) {
	var exists bool
	if err := db.QueryRow(ctx, spec.hasDataSQL).Scan(&exists); err != nil {
		return false, fmt.Errorf("memory: has embeddings %s: %w", spec.name, err)
	}
	return exists, nil
}
