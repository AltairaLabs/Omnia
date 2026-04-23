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
	"time"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
)

// CompactionCandidate is one (workspace, user, agent) bucket of memories that
// are due for temporal summarization. Each candidate carries the observation
// IDs that would be marked superseded and the textual content the summarizer
// should read.
type CompactionCandidate struct {
	WorkspaceID    string
	UserID         string
	AgentID        string
	ObservationIDs []string
	Entries        []CompactionEntry
}

// CompactionEntry is a single observation line fed to the summarizer.
type CompactionEntry struct {
	EntityID      string
	ObservationID string
	Kind          string
	Content       string
	ObservedAt    time.Time
}

// FindCompactionCandidatesOptions configures the candidate scan.
type FindCompactionCandidatesOptions struct {
	WorkspaceID string
	OlderThan   time.Time
	// MinGroupSize — don't bother summarizing unless at least this many
	// observations are ready in a single (workspace, user, agent) bucket.
	MinGroupSize int
	// MaxCandidates caps the number of buckets returned in one scan.
	MaxCandidates int
	// MaxPerCandidate caps the number of observations attached to each bucket.
	// Prevents unbounded summarizer input sizes.
	MaxPerCandidate int
}

// Default values for FindCompactionCandidatesOptions.
const (
	defaultCompactionMinGroupSize    = 10
	defaultCompactionMaxCandidates   = 20
	defaultCompactionMaxPerCandidate = 50
)

// FindCompactionCandidates scans for (workspace, user, agent) buckets whose
// non-superseded, non-forgotten observations are older than OlderThan. Returns
// up to MaxCandidates buckets, each containing up to MaxPerCandidate entries.
// Buckets with fewer than MinGroupSize observations are skipped — temporal
// summarization is only worthwhile at volume.
func (s *PostgresMemoryStore) FindCompactionCandidates(ctx context.Context, opts FindCompactionCandidatesOptions) ([]CompactionCandidate, error) {
	if opts.WorkspaceID == "" {
		return nil, errors.New(errWorkspaceRequired)
	}
	if opts.OlderThan.IsZero() {
		return nil, errors.New("memory: OlderThan is required")
	}
	if opts.MinGroupSize <= 0 {
		opts.MinGroupSize = defaultCompactionMinGroupSize
	}
	if opts.MaxCandidates <= 0 {
		opts.MaxCandidates = defaultCompactionMaxCandidates
	}
	if opts.MaxPerCandidate <= 0 {
		opts.MaxPerCandidate = defaultCompactionMaxPerCandidate
	}

	// Step 1: find buckets with enough eligible observations.
	buckets, err := s.findCompactionBuckets(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Step 2: for each bucket, fetch up to MaxPerCandidate entries.
	candidates := make([]CompactionCandidate, 0, len(buckets))
	for _, b := range buckets {
		entries, err := s.fetchCompactionEntries(ctx, opts, b)
		if err != nil {
			return nil, err
		}
		if len(entries) < opts.MinGroupSize {
			continue
		}
		obsIDs := make([]string, 0, len(entries))
		for _, e := range entries {
			obsIDs = append(obsIDs, e.ObservationID)
		}
		candidates = append(candidates, CompactionCandidate{
			WorkspaceID:    opts.WorkspaceID,
			UserID:         b.userID,
			AgentID:        b.agentID,
			ObservationIDs: obsIDs,
			Entries:        entries,
		})
	}
	return candidates, nil
}

// compactionBucket is the (user, agent) coordinates of a bucket found by the
// first-pass aggregate query.
type compactionBucket struct {
	userID  string
	agentID string
}

func (s *PostgresMemoryStore) findCompactionBuckets(ctx context.Context, opts FindCompactionCandidatesOptions) ([]compactionBucket, error) {
	const sql = `
		SELECT COALESCE(e.virtual_user_id, ''), COALESCE(e.agent_id::text, ''), COUNT(o.id) AS cnt
		FROM memory_entities e
		JOIN memory_observations o ON o.entity_id = e.id
		WHERE e.workspace_id = $1
		  AND e.forgotten = false
		  AND o.superseded_by IS NULL
		  AND o.observed_at < $2
		GROUP BY e.virtual_user_id, e.agent_id
		HAVING COUNT(o.id) >= $3
		ORDER BY cnt DESC
		LIMIT $4`
	rows, err := s.pool.Query(ctx, sql, opts.WorkspaceID, opts.OlderThan, opts.MinGroupSize, opts.MaxCandidates)
	if err != nil {
		return nil, fmt.Errorf("memory: find compaction buckets: %w", err)
	}
	defer rows.Close()

	var out []compactionBucket
	for rows.Next() {
		var b compactionBucket
		var cnt int64
		if err := rows.Scan(&b.userID, &b.agentID, &cnt); err != nil {
			return nil, fmt.Errorf("memory: scan bucket: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: iterate buckets: %w", err)
	}
	return out, nil
}

func (s *PostgresMemoryStore) fetchCompactionEntries(ctx context.Context, opts FindCompactionCandidatesOptions, b compactionBucket) ([]CompactionEntry, error) {
	args := []any{opts.WorkspaceID, opts.OlderThan, opts.MaxPerCandidate}
	userClause := "e.virtual_user_id IS NULL"
	if b.userID != "" {
		args = append(args, b.userID)
		userClause = fmt.Sprintf("e.virtual_user_id = $%d", len(args))
	}
	agentClause := "e.agent_id IS NULL"
	if b.agentID != "" {
		args = append(args, b.agentID)
		agentClause = fmt.Sprintf("e.agent_id = $%d::uuid", len(args))
	}
	sql := fmt.Sprintf(`
		SELECT e.id, o.id, e.kind, o.content, o.observed_at
		FROM memory_entities e
		JOIN memory_observations o ON o.entity_id = e.id
		WHERE e.workspace_id = $1
		  AND e.forgotten = false
		  AND o.superseded_by IS NULL
		  AND o.observed_at < $2
		  AND %s
		  AND %s
		ORDER BY o.observed_at ASC
		LIMIT $3`, userClause, agentClause)

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: fetch compaction entries: %w", err)
	}
	defer rows.Close()

	var out []CompactionEntry
	for rows.Next() {
		var e CompactionEntry
		if err := rows.Scan(&e.EntityID, &e.ObservationID, &e.Kind, &e.Content, &e.ObservedAt); err != nil {
			return nil, fmt.Errorf("memory: scan entry: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: iterate entries: %w", err)
	}
	return out, nil
}

// CompactionSummary is the summarizer's output: a single synthetic memory
// replacing N originals. The scope identifies the (workspace, user, agent)
// bucket the summary belongs to; SupersededObservations is the list of
// observation IDs whose superseded_by pointer must be set to the new
// summary's observation row.
type CompactionSummary struct {
	WorkspaceID            string
	UserID                 string // empty = institutional/agent-only
	AgentID                string // empty = institutional/user-only
	Content                string
	Kind                   string
	Confidence             float64
	SupersededObservations []string
}

// errSupersededAlready indicates that compaction raced with another writer
// and the observations the caller wanted to supersede are already superseded.
// Callers can treat this as "no-op" rather than a hard error.
var errSupersededAlready = errors.New("memory: compaction target already superseded")

// SaveCompactionSummary persists a new summary memory and updates the
// superseded_by pointer on the originals. All of this happens in a single
// transaction so a partial update can't leave the graph in a confused state.
// Returns errSupersededAlready if every target observation is already
// superseded (no row updates happened); that signals a race we can swallow.
func (s *PostgresMemoryStore) SaveCompactionSummary(ctx context.Context, summary CompactionSummary) (string, error) {
	if summary.WorkspaceID == "" {
		return "", errors.New(errWorkspaceRequired)
	}
	if summary.Content == "" {
		return "", errors.New("memory: compaction summary content is required")
	}
	if len(summary.SupersededObservations) == 0 {
		return "", errors.New("memory: compaction summary must supersede at least one observation")
	}
	kind := summary.Kind
	if kind == "" {
		kind = "temporal_summary"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("memory: begin compaction tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var summaryEntityID string
	row := tx.QueryRow(ctx, `
		INSERT INTO memory_entities
		  (workspace_id, virtual_user_id, agent_id, name, kind, metadata, trust_model, source_type)
		VALUES
		  ($1, $2, $3, $4, $5, '{"provenance":"system_generated"}'::jsonb, 'curated', 'temporal_summary')
		RETURNING id`,
		summary.WorkspaceID,
		nullIfEmpty(summary.UserID),
		nullIfEmpty(summary.AgentID),
		summary.Content,
		kind,
	)
	if err := row.Scan(&summaryEntityID); err != nil {
		return "", fmt.Errorf("memory: insert summary entity: %w", err)
	}

	confidence := summary.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = 1.0
	}
	var summaryObsID string
	row = tx.QueryRow(ctx, `
		INSERT INTO memory_observations (entity_id, content, confidence, source_type)
		VALUES ($1, $2, $3, 'temporal_summary')
		RETURNING id`,
		summaryEntityID, summary.Content, confidence,
	)
	if err := row.Scan(&summaryObsID); err != nil {
		return "", fmt.Errorf("memory: insert summary observation: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE memory_observations
		SET superseded_by = $1
		WHERE id = ANY($2) AND superseded_by IS NULL`,
		summaryObsID, summary.SupersededObservations,
	)
	if err != nil {
		return "", fmt.Errorf("memory: mark superseded: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", errSupersededAlready
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("memory: commit compaction tx: %w", err)
	}
	return summaryEntityID, nil
}

// ErrCompactionRaced is the exported alias so callers outside memory/ can
// test for the race condition without depending on the unexported sentinel.
var ErrCompactionRaced = errSupersededAlready

// Compile-time assertion so the alias and sentinel don't drift.
var _ = pkmemory.ProvenanceSystemGenerated

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
