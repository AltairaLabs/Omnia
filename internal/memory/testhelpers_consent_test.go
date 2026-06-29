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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// insertWorkspace returns a fresh workspace_id UUID suitable for
// isolating a single test. Because memory_entities has no foreign-key
// to a workspaces table, a UUID is sufficient.
func insertWorkspace(t *testing.T, _ *pgxpool.Pool) string {
	t.Helper()
	return uuid.New().String()
}

// insertMemoryEntity inserts a minimal memory_entities row and returns its id.
// userID, agentID, and category are optional (pass nil to omit).
func insertMemoryEntity(
	t *testing.T, pool *pgxpool.Pool,
	workspaceID string, userID, agentID, category *string,
) string {
	t.Helper()
	var catArg, userArg, agentArg any
	if category != nil {
		catArg = *category
	}
	if userID != nil {
		userArg = *userID
	}
	if agentID != nil {
		agentArg = *agentID
	}

	var id string
	err := pool.QueryRow(context.Background(), `
INSERT INTO memory_entities (workspace_id, virtual_user_id, agent_id, name, kind, metadata, consent_category)
VALUES ($1, $2, $3, 'test', 'fact', '{}', $4)
RETURNING id`,
		workspaceID, userArg, agentArg, catArg,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// isForgotten returns true if the row with the given id has forgotten=true.
func isForgotten(t *testing.T, pool *pgxpool.Pool, id string) bool {
	t.Helper()
	var forgotten bool
	err := pool.QueryRow(context.Background(),
		`SELECT forgotten FROM memory_entities WHERE id = $1`, id,
	).Scan(&forgotten)
	require.NoError(t, err)
	return forgotten
}

// rowExists returns true if a row with the given id exists in memory_entities.
func rowExists(t *testing.T, pool *pgxpool.Pool, id string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM memory_entities WHERE id = $1)`, id,
	).Scan(&exists)
	require.NoError(t, err)
	return exists
}

// ptr returns a pointer to s. Convenience for optional string parameters in
// test helpers.
func ptr(s string) *string { return &s }
