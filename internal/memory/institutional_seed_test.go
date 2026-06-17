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
	"testing"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
)

// seedInstitutional inserts a workspace-scoped (no user/agent) institutional
// memory for OSS tests that exercise the institutional READ/retention/recall
// path. The production institutional WRITE path lives in ee/pkg/memory; OSS
// cannot import it (import cycle), so this test-only helper reproduces the same
// insert. It is in a _test.go file (excluded from SonarCloud CPD) and is not
// shipped product code.
//
// Behavior mirrors SaveInstitutional exactly:
//   - Scope is replaced to workspace-only (no user/agent leak).
//   - Provenance is forced to operator_curated.
//   - Entity is inserted with virtual_user_id=NULL, agent_id=NULL,
//     trust_model='curated', source_type='operator_curated'.
//   - Observation is inserted via insertObservation.
//
// None of the 34 OSS call sites pass about_kind+about_key metadata, so the
// plain-insert path is sufficient; the upsert path is not needed here.
func seedInstitutional(t *testing.T, store *PostgresMemoryStore, mem *Memory) {
	t.Helper()
	ctx := context.Background()

	workspaceID := mem.Scope[ScopeWorkspaceID]
	if workspaceID == "" {
		t.Fatalf("seedInstitutional: workspace_id scope is required")
	}

	// Replace the scope map entirely so user/agent keys cannot leak.
	mem.Scope = map[string]string{ScopeWorkspaceID: workspaceID}

	if mem.Metadata == nil {
		mem.Metadata = map[string]any{}
	}
	mem.Metadata[pkmemory.MetaKeyProvenance] = string(pkmemory.ProvenanceOperatorCurated)

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("seedInstitutional: begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	metaJSON, err := marshalMetadata(mem.Metadata)
	if err != nil {
		t.Fatalf("seedInstitutional: marshal metadata: %v", err)
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO memory_entities
		  (workspace_id, virtual_user_id, agent_id, name, kind, metadata, trust_model, source_type, expires_at)
		VALUES
		  ($1, NULL, NULL, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		workspaceID,
		mem.Content,
		mem.Type,
		metaJSON,
		"curated",
		"operator_curated",
		mem.ExpiresAt,
	)
	if err := row.Scan(&mem.ID, &mem.CreatedAt); err != nil {
		t.Fatalf("seedInstitutional: insert entity: %v", err)
	}

	if err := insertObservation(ctx, tx, mem); err != nil {
		t.Fatalf("seedInstitutional: insert observation: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("seedInstitutional: commit: %v", err)
	}
}

// deleteInstitutional soft-deletes an institutional memory by setting
// forgotten=true. Used only by TestListWorkspaceIDs_ExcludesForgotten in
// compaction_worker_test.go to seed a forgotten-only workspace. Mirrors the
// core of DeleteInstitutional without the cross-scope guard (not needed for
// test seeding where we control the IDs).
func deleteInstitutional(t *testing.T, store *PostgresMemoryStore, workspaceID, memoryID string) {
	t.Helper()
	ctx := context.Background()
	tag, err := store.pool.Exec(ctx,
		`UPDATE memory_entities SET forgotten = true, updated_at = now()
		 WHERE id = $1 AND workspace_id = $2`,
		memoryID, workspaceID,
	)
	if err != nil {
		t.Fatalf("deleteInstitutional: %v", err)
	}
	if tag.RowsAffected() == 0 {
		t.Fatalf("deleteInstitutional: entity %s not found in workspace %s", memoryID, workspaceID)
	}
}
