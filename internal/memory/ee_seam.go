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
	"time"

	"github.com/jackc/pgx/v5"
)

// This file is the narrow exported seam that the enterprise memory package
// (ee/pkg/memory) consumes. It re-exports the shared SQL + metadata helpers
// the relocated InstitutionalStore needs, plus the policy-resolution helpers
// the relocated tier ranker needs, WITHOUT exposing the concrete store. The
// helpers themselves stay OSS (they are shared with the Save/agent-scoped
// paths); only the institutional/ranking *callers* moved to EE.

// InstitutionalStore is the enterprise admin surface for workspace-scoped
// memories. Defined here (OSS core) so the enterprise implementation in
// ee/pkg/memory satisfies it without inverting the core→EE dependency direction.
type InstitutionalStore interface {
	SaveInstitutional(ctx context.Context, mem *Memory) error
	ListInstitutional(ctx context.Context, workspaceID string, opts ListOptions) ([]*Memory, error)
	DeleteInstitutional(ctx context.Context, workspaceID, memoryID string) error
}

// ErrNotInstitutional is returned when DeleteInstitutional is called with a
// memory ID that belongs to a user- or agent-scoped row. Callers MUST use
// errors.Is against this sentinel so the HTTP handler can map it to a 400
// response rather than a 500.
var ErrNotInstitutional = errors.New("memory: target is not an institutional memory")

// HasAboutKey reports whether the memory carries both about_kind and about_key
// metadata (the structured-dedup signal). Exported for the EE institutional store.
func HasAboutKey(mem *Memory) bool { return hasAboutKey(mem) }

// InsertObservation inserts a new observation row for mem within tx. Exported
// for the EE institutional store, which begins its own transaction.
func InsertObservation(ctx context.Context, tx pgx.Tx, mem *Memory) error {
	return insertObservation(ctx, tx, mem)
}

// SupersedePriorObservations marks the active observations of entityID inactive
// within tx, returning the superseded observation IDs. Exported for EE.
func SupersedePriorObservations(ctx context.Context, tx pgx.Tx, entityID string) ([]string, error) {
	return supersedePriorObservations(ctx, tx, entityID)
}

// MarshalMetadata serialises a memory metadata map to JSON. Exported for EE.
func MarshalMetadata(meta map[string]any) ([]byte, error) { return marshalMetadata(meta) }

// StringFromMeta returns the string value at key, or "" when absent / non-string.
// Exported for EE.
func StringFromMeta(meta map[string]any, key string) string { return stringFromMeta(meta, key) }

// ScanMemories materialises memory rows from a pgx.Rows into *Memory values
// scoped to scope. Exported for the EE institutional list path.
func ScanMemories(rows pgx.Rows, scope map[string]string) ([]*Memory, error) {
	return scanMemories(rows, scope)
}

// ParseRetentionDuration parses a CRD duration string ("30d", "720h", ...).
// Exported so the EE half-life resolver reuses the same parser as retention.
func ParseRetentionDuration(s string) (time.Duration, error) { return parseRetentionDuration(s) }

// OrDefaults substitutes the OSS uniform default half-life for any tier left at
// zero. Exported so the EE policy resolver (NewTierHalfLife) applies the same
// defaulting as the OSS recall path.
func (h TierHalfLife) OrDefaults() TierHalfLife { return h.orDefaults() }

// WorkspaceInvalidator is implemented by the cache-fronted store (*CachedStore)
// so the service layer can invalidate a workspace's cached list/search results
// after an out-of-band write (e.g. an institutional save handled by the EE
// store rather than flowing through the cache wrapper). The bare
// *PostgresMemoryStore does NOT implement it — when caching is disabled the
// type assertion fails and no bump occurs, matching the pre-refactor behaviour.
type WorkspaceInvalidator interface {
	InvalidateWorkspace(ctx context.Context, workspaceID string)
}
