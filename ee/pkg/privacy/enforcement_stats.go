/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"fmt"

	"github.com/altairalabs/omnia/ee/pkg/audit"
)

// EnforcementStats summarises workspace-scoped privacy enforcement activity
// for the dashboard. PIIBlocked counts opt-out write blocks; Redactions
// counts PII redaction events. Both are derived from audit_log rows.
type EnforcementStats struct {
	PIIBlocked int64 `json:"piiBlocked"`
	Redactions int64 `json:"redactions"`
}

// EnforcementStats returns workspace-scoped privacy enforcement counts from
// audit_log. One round-trip; uses conditional aggregation so a single row is
// returned even when neither event type is present.
func (s *PreferencesPostgresStore) EnforcementStats(ctx context.Context, workspace string) (EnforcementStats, error) {
	const query = `
		SELECT
		    COUNT(*) FILTER (WHERE event_type = $2)::bigint AS pii_blocked,
		    COUNT(*) FILTER (WHERE event_type = $3)::bigint AS redactions
		FROM audit_log
		WHERE workspace = $1 AND event_type = ANY($4)`

	eventTypes := []string{audit.EventMemoryWriteBlocked, audit.EventPIIRedacted}
	var stats EnforcementStats
	if err := s.pool.QueryRow(ctx, query,
		workspace,
		audit.EventMemoryWriteBlocked,
		audit.EventPIIRedacted,
		eventTypes,
	).Scan(&stats.PIIBlocked, &stats.Redactions); err != nil {
		return EnforcementStats{}, fmt.Errorf("privacy: enforcement stats query: %w", err)
	}
	return stats, nil
}
