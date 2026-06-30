/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/altairalabs/omnia/ee/pkg/audit"
)

// auditColumnsPerRow is the number of columns bound per audit_log row in
// InsertEvents. Keep in sync with auditInsertColumns.
const auditColumnsPerRow = 15

// auditInsertColumns lists the audit_log columns InsertEvents writes, in the
// same order it binds parameters.
const auditInsertColumns = `source_service, source_id, "timestamp", event_type, ` +
	`session_id, user_id, workspace, agent_name, namespace, query, result_count, ` +
	`ip_address, user_agent, reason, metadata`

// AuditStore writes audit events forwarded by memory-api / session-api into
// privacy-api's central audit_log (the privacy/compliance audit hub, #1673).
type AuditStore struct {
	pool dbPool
}

// NewAuditStore creates an AuditStore backed by the given pool.
func NewAuditStore(pool dbPool) *AuditStore {
	return &AuditStore{pool: pool}
}

// InsertEvents stores a batch of audit events forwarded from sourceService.
// Delivery is at-least-once, so the write is idempotent: rows already present
// (same source_service + source_id) are skipped via ON CONFLICT DO NOTHING.
// Returns the number of newly-inserted rows (duplicates are silently ignored).
func (s *AuditStore) InsertEvents(ctx context.Context, sourceService string, events []*audit.Entry) (int, error) {
	if sourceService == "" {
		return 0, fmt.Errorf("privacy: audit ingest: sourceService is required")
	}

	args := make([]any, 0, len(events)*auditColumnsPerRow)
	rows := make([]string, 0, len(events))
	for _, e := range events {
		if e == nil {
			continue
		}
		meta, err := marshalMetadata(e.Metadata)
		if err != nil {
			return 0, fmt.Errorf("privacy: audit ingest: marshal metadata: %w", err)
		}
		base := len(args)
		ph := make([]string, auditColumnsPerRow)
		for j := range ph {
			ph[j] = fmt.Sprintf("$%d", base+j+1)
		}
		rows = append(rows, "("+strings.Join(ph, ",")+")")
		args = append(args,
			sourceService, e.ID, e.Timestamp, e.EventType,
			nullIfEmpty(e.SessionID), nullIfEmpty(e.UserID), nullIfEmpty(e.Workspace),
			nullIfEmpty(e.AgentName), nullIfEmpty(e.Namespace), nullIfEmpty(e.Query),
			e.ResultCount, nullIfEmpty(e.IPAddress), nullIfEmpty(e.UserAgent),
			nullIfEmpty(e.Reason), meta,
		)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	query := fmt.Sprintf(
		"INSERT INTO audit_log (%s) VALUES %s ON CONFLICT (source_service, source_id) DO NOTHING",
		auditInsertColumns, strings.Join(rows, ","),
	)
	tag, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("privacy: audit ingest: insert: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// nullIfEmpty returns nil for an empty string so the column is stored as SQL
// NULL rather than an empty string.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// marshalMetadata renders an audit entry's metadata map as JSONB bytes,
// defaulting to an empty object so the NOT NULL column is always satisfied.
func marshalMetadata(m map[string]string) ([]byte, error) {
	if len(m) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}
