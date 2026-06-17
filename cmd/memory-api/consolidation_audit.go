/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"strings"

	"github.com/altairalabs/omnia/ee/pkg/memory/consolidation"
	sessionapi "github.com/altairalabs/omnia/internal/session/api"
)

// auditInner is the subset of eeaudit.Logger that consolidationAuditAdapter
// depends on. Lifted to an interface so the unit test can substitute a
// recording fake without dragging in the EE Logger's full surface.
type auditInner interface {
	LogEvent(ctx context.Context, e *sessionapi.AuditEntry)
}

// consolidationAuditAdapter satisfies consolidation.Auditor by
// translating AuditEntry → sessionapi.AuditEntry for the eeaudit
// Logger. Consolidation-specific fields (RunID, PackRef, Outcome,
// TargetIDs, Reason) land in Metadata under stable keys so the
// dashboard / Prometheus / forensic-lineage tooling can index on
// `consolidation_run_id`.
type consolidationAuditAdapter struct {
	inner auditInner
}

// LogConsolidation translates and enqueues one consolidation audit row.
func (a *consolidationAuditAdapter) LogConsolidation(ctx context.Context, e consolidation.AuditEntry) error {
	meta := map[string]string{
		"consolidation_run_id": e.RunID,
		"pack_ref":             e.PackRef,
		"outcome":              e.Outcome,
		"action_kind":          string(e.ActionKind),
	}
	if e.Reason != "" {
		meta["reason"] = e.Reason
	}
	if len(e.TargetIDs) > 0 {
		meta["target_ids"] = strings.Join(e.TargetIDs, ",")
	}
	a.inner.LogEvent(ctx, &sessionapi.AuditEntry{
		EventType: "memory.consolidation." + string(e.ActionKind),
		Workspace: e.WorkspaceID,
		Metadata:  meta,
	})
	return nil
}
