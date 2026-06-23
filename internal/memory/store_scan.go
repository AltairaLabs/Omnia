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
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/jackc/pgx/v5"
)

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

// scanMemory scans a single row into a Memory, stamping it with the request
// scope. Used by every list/retrieve path except visible-to-me.
func scanMemory(row pgx.Rows, scope map[string]string) (*Memory, error) {
	return scanMemoryRow(row, scope, "", false)
}

// scanMemoryRow scans one memory row. When withRowScope is false it stamps
// the request scope (the standard SELECT has no scope columns). When true it
// scans two trailing columns — virtual_user_id, agent_id — and builds the
// per-row scope, so callers like the visible-to-me list (which returns mixed
// tiers) derive each row's real tier instead of the request's. See #1254.
func scanMemoryRow(row pgx.Rows, scope map[string]string, workspaceID string, withRowScope bool) (*Memory, error) {
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
		userID         *string
		agentID        *string
	)

	dest := []any{
		&mem.ID, &mem.Type, &metadataJSON, &mem.CreatedAt, &expiresAt, &title,
		&mem.Content, &mem.Confidence, &sessionID, &turnRange, &observedAt, &accessedAt,
		&summary, &bodySizeBytes,
	}
	if withRowScope {
		dest = append(dest, &userID, &agentID)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, fmt.Errorf("memory: scan row: %w", err)
	}

	if withRowScope {
		mem.Scope = buildScope(workspaceID, userID, agentID)
	} else {
		mem.Scope = copyScope(scope)
	}
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

// scanVisibleToMeMemories scans rows from the visible-to-me list query, which
// appends virtual_user_id and agent_id so each Memory carries its real
// per-row scope (and therefore its real tier).
func scanVisibleToMeMemories(rows pgx.Rows, workspaceID string) ([]*Memory, error) {
	var results []*Memory
	for rows.Next() {
		mem, err := scanMemoryRow(rows, nil, workspaceID, true)
		if err != nil {
			return nil, err
		}
		results = append(results, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: visible-to-me rows iteration: %w", err)
	}
	if results == nil {
		results = []*Memory{}
	}
	return results, nil
}
