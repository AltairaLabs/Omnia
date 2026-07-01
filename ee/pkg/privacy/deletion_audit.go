/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/ee/pkg/audit"
	"github.com/altairalabs/omnia/internal/session/api"
)

// auditSourceServiceDSAR is the source_service value for DSAR lifecycle events
// that privacy-api originates itself (as opposed to events forwarded from
// memory-api / session-api).
const auditSourceServiceDSAR = "privacy-api-dsar"

// auditEntryInserter is the subset of AuditStore that DeletionAuditLogger needs.
type auditEntryInserter interface {
	InsertEvents(ctx context.Context, sourceService string, events []*audit.Entry) (int, error)
}

// DeletionAuditLogger adapts DSAR lifecycle events (deletion_requested /
// deletion_completed / deletion_failed, delivered as *api.AuditEntry by
// DeletionService) into privacy-api's central audit_log via AuditStore. It
// implements the privacy AuditLogger interface. Source ids are process-monotonic
// (seeded from the wall clock at construction) so each event is a distinct row
// under source_service — a fresh pod seeds later than any prior pod, and the
// atomic counter guarantees uniqueness within a pod, so ON CONFLICT never drops a
// real event.
type DeletionAuditLogger struct {
	store     auditEntryInserter
	workspace string
	seq       *atomic.Int64
	now       func() time.Time
	log       logr.Logger
}

// NewDeletionAuditLogger builds a DeletionAuditLogger writing to store, tagging
// every row with the workspace (UID) so it is scoped like the forwarded events.
func NewDeletionAuditLogger(store auditEntryInserter, workspace string, log logr.Logger) *DeletionAuditLogger {
	seq := &atomic.Int64{}
	seq.Store(time.Now().UnixNano())
	return &DeletionAuditLogger{
		store:     store,
		workspace: workspace,
		seq:       seq,
		now:       time.Now,
		log:       log.WithName("dsar-audit"),
	}
}

// LogEvent converts entry to an audit.Entry and writes it to the hub. DSAR events
// carry virtual_user_id + reason in Metadata (see DeletionService.logAuditEvent);
// those are lifted to the dedicated columns so audit queries can filter on them.
// Write failures are logged, not returned — audit must never block erasure.
func (l *DeletionAuditLogger) LogEvent(ctx context.Context, entry *api.AuditEntry) {
	if entry == nil {
		return
	}
	e := &audit.Entry{
		ID:          l.seq.Add(1),
		Timestamp:   l.now().UTC(),
		EventType:   entry.EventType,
		SessionID:   entry.SessionID,
		Workspace:   l.workspace,
		AgentName:   entry.AgentName,
		Namespace:   entry.Namespace,
		Query:       entry.Query,
		ResultCount: entry.ResultCount,
		IPAddress:   entry.IPAddress,
		UserAgent:   entry.UserAgent,
		Metadata:    entry.Metadata,
	}
	if entry.Metadata != nil {
		e.UserID = entry.Metadata["virtual_user_id"]
		e.Reason = entry.Metadata["reason"]
	}
	if _, err := l.store.InsertEvents(ctx, auditSourceServiceDSAR, []*audit.Entry{e}); err != nil {
		l.log.Error(err, "DSAR audit write failed", "eventType", entry.EventType)
	}
}
