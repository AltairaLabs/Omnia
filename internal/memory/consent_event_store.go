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
	"fmt"
)

// ConsentEventPruner can prune memory rows for a specific user and
// consent category in response to an inbound consent-revocation event.
// Implemented by PostgresMemoryStore; the interface allows the service
// layer to depend on the narrow contract rather than the concrete type.
type ConsentEventPruner interface {
	// SoftDeleteUserConsentCategory marks matching user-tier rows as
	// forgotten so the grace-window pass can hard-delete them later.
	// Returns the number of rows affected.
	SoftDeleteUserConsentCategory(ctx context.Context, workspaceID, userID, category string) (int64, error)

	// HardDeleteUserConsentCategory immediately removes matching user-
	// tier rows. Returns the number of rows deleted.
	HardDeleteUserConsentCategory(ctx context.Context, workspaceID, userID, category string) (int64, error)
}

// SoftDeleteUserConsentCategory marks user-tier memory rows forgotten
// for a specific (workspace, user, consent category) triple. Only rows
// that are not already forgotten are affected; this is safe to call
// multiple times (idempotent aside from forgotten_at timestamp).
//
// Non-user-tier rows (virtual_user_id IS NULL) and rows belonging to a
// different user or category are never touched — the WHERE predicates
// are explicit and bound, not derived from a JOIN.
func (s *PostgresMemoryStore) SoftDeleteUserConsentCategory(
	ctx context.Context, workspaceID, userID, category string,
) (int64, error) {
	if workspaceID == "" || userID == "" || category == "" {
		return 0, fmt.Errorf("memory: consent-event soft-delete: workspaceID, userID, and category are required")
	}
	q := `
UPDATE memory_entities
   SET forgotten = true, forgotten_at = now(), updated_at = now()
 WHERE workspace_id = $1
   AND virtual_user_id = $2
   AND consent_category = $3
   AND forgotten = false`

	tag, err := s.pool.Exec(ctx, q, workspaceID, userID, category)
	if err != nil {
		return 0, fmt.Errorf("memory: consent-event soft-delete: %w", err)
	}
	return tag.RowsAffected(), nil
}

// HardDeleteUserConsentCategory immediately removes all user-tier
// memory rows for a specific (workspace, user, consent category)
// triple. Unlike SoftDeleteUserConsentCategory this is irreversible.
//
// Non-user-tier rows (virtual_user_id IS NULL) and rows belonging to a
// different user or category are never touched.
func (s *PostgresMemoryStore) HardDeleteUserConsentCategory(
	ctx context.Context, workspaceID, userID, category string,
) (int64, error) {
	if workspaceID == "" || userID == "" || category == "" {
		return 0, fmt.Errorf("memory: consent-event hard-delete: workspaceID, userID, and category are required")
	}
	q := `
DELETE FROM memory_entities
 WHERE workspace_id = $1
   AND virtual_user_id = $2
   AND consent_category = $3`

	tag, err := s.pool.Exec(ctx, q, workspaceID, userID, category)
	if err != nil {
		return 0, fmt.Errorf("memory: consent-event hard-delete: %w", err)
	}
	return tag.RowsAffected(), nil
}
