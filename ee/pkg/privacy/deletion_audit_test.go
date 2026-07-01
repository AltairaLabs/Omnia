/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/ee/pkg/audit"
	"github.com/altairalabs/omnia/internal/session/api"
)

type fakeAuditInserter struct {
	sourceService string
	events        []*audit.Entry
	err           error
	calls         int
}

func (f *fakeAuditInserter) InsertEvents(_ context.Context, sourceService string, events []*audit.Entry) (int, error) {
	f.calls++
	f.sourceService = sourceService
	f.events = append(f.events, events...)
	if f.err != nil {
		return 0, f.err
	}
	return len(events), nil
}

func TestDeletionAuditLogger_LogEvent_MapsAndLifts(t *testing.T) {
	ins := &fakeAuditInserter{}
	l := NewDeletionAuditLogger(ins, "ws-uid", logr.Discard())

	l.LogEvent(context.Background(), &api.AuditEntry{
		EventType: "deletion_completed",
		Metadata: map[string]string{
			"virtual_user_id":  "vu-1",
			"reason":           testReasonGDPR,
			"sessions_deleted": "3",
		},
	})

	if ins.calls != 1 || len(ins.events) != 1 {
		t.Fatalf("calls=%d events=%d, want 1/1", ins.calls, len(ins.events))
	}
	if ins.sourceService != auditSourceServiceDSAR {
		t.Errorf("sourceService = %q, want %q", ins.sourceService, auditSourceServiceDSAR)
	}
	e := ins.events[0]
	if e.EventType != "deletion_completed" {
		t.Errorf("eventType = %q", e.EventType)
	}
	if e.Workspace != "ws-uid" {
		t.Errorf("workspace = %q, want ws-uid", e.Workspace)
	}
	if e.UserID != "vu-1" {
		t.Errorf("userID = %q, want vu-1 (lifted from metadata)", e.UserID)
	}
	if e.Reason != testReasonGDPR {
		t.Errorf("reason = %q, want gdpr_erasure (lifted from metadata)", e.Reason)
	}
	if e.ID == 0 || e.Timestamp.IsZero() {
		t.Errorf("id/timestamp must be set: id=%d ts=%v", e.ID, e.Timestamp)
	}
}

func TestDeletionAuditLogger_LogEvent_SourceIDsAreUnique(t *testing.T) {
	ins := &fakeAuditInserter{}
	l := NewDeletionAuditLogger(ins, "ws-uid", logr.Discard())

	for i := 0; i < 5; i++ {
		l.LogEvent(context.Background(), &api.AuditEntry{EventType: "deletion_requested"})
	}
	seen := map[int64]bool{}
	for _, e := range ins.events {
		if seen[e.ID] {
			t.Fatalf("duplicate source_id %d — ON CONFLICT would drop a real event", e.ID)
		}
		seen[e.ID] = true
	}
}

func TestDeletionAuditLogger_LogEvent_NilEntryNoop(t *testing.T) {
	ins := &fakeAuditInserter{}
	l := NewDeletionAuditLogger(ins, "ws-uid", logr.Discard())
	l.LogEvent(context.Background(), nil)
	if ins.calls != 0 {
		t.Fatalf("nil entry should be a no-op, got %d calls", ins.calls)
	}
}

func TestDeletionAuditLogger_LogEvent_WriteErrorSwallowed(t *testing.T) {
	ins := &fakeAuditInserter{err: errors.New("db down")}
	l := NewDeletionAuditLogger(ins, "ws-uid", logr.Discard())
	// Must not panic or propagate — audit never blocks erasure.
	l.LogEvent(context.Background(), &api.AuditEntry{EventType: "deletion_failed"})
	if ins.calls != 1 {
		t.Fatalf("expected the insert to be attempted once, got %d", ins.calls)
	}
}

// compile-time check: DeletionAuditLogger satisfies the DeletionService audit sink.
var _ AuditLogger = (*DeletionAuditLogger)(nil)
