/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"testing"
	"time"

	"github.com/altairalabs/omnia/ee/pkg/memory/consolidation"
	sessionapi "github.com/altairalabs/omnia/internal/session/api"
)

const testAuditWorkspaceID = "ws-1"

type recordingInner struct{ got []sessionapi.AuditEntry }

func (r *recordingInner) LogEvent(_ context.Context, e *sessionapi.AuditEntry) {
	r.got = append(r.got, *e)
}

func TestConsolidationAuditAdapter_TranslatesAppliedEntry(t *testing.T) {
	inner := &recordingInner{}
	a := &consolidationAuditAdapter{inner: inner}
	err := a.LogConsolidation(context.Background(), consolidation.AuditEntry{
		RunID:       "ws-1-1700000000",
		WorkspaceID: testAuditWorkspaceID,
		PackRef:     "safe-default-summarizer",
		ActionKind:  consolidation.ActionCreateSummary,
		Outcome:     consolidation.OutcomeApplied,
		TargetIDs:   []string{"o1", "o2"},
		Now:         time.Unix(1700000000, 0),
	})
	if err != nil {
		t.Fatalf("LogConsolidation: %v", err)
	}
	if len(inner.got) != 1 {
		t.Fatalf("want 1 emit, got %d", len(inner.got))
	}
	e := inner.got[0]
	if e.EventType != "memory.consolidation.create_summary" {
		t.Errorf("event type: %q", e.EventType)
	}
	if e.Workspace != testAuditWorkspaceID {
		t.Errorf("workspace: %q", e.Workspace)
	}
	if e.Metadata["consolidation_run_id"] != "ws-1-1700000000" {
		t.Errorf("missing run_id: %+v", e.Metadata)
	}
	if e.Metadata["pack_ref"] != "safe-default-summarizer" {
		t.Errorf("missing pack_ref: %+v", e.Metadata)
	}
	if e.Metadata["outcome"] != "applied" {
		t.Errorf("missing outcome: %+v", e.Metadata)
	}
	if e.Metadata["target_ids"] != "o1,o2" {
		t.Errorf("target_ids: %q", e.Metadata["target_ids"])
	}
}

func TestConsolidationAuditAdapter_PreservesRejectionReason(t *testing.T) {
	inner := &recordingInner{}
	a := &consolidationAuditAdapter{inner: inner}
	err := a.LogConsolidation(context.Background(), consolidation.AuditEntry{
		RunID:       "ws-1-1700000000",
		WorkspaceID: testAuditWorkspaceID,
		PackRef:     "demo-rescope",
		ActionKind:  consolidation.ActionRescope,
		Outcome:     consolidation.OutcomeRejectedValidation,
		Reason:      consolidation.ReasonInstitutionalWriteBlocked,
		TargetIDs:   []string{"o1"},
	})
	if err != nil {
		t.Fatalf("LogConsolidation: %v", err)
	}
	if len(inner.got) != 1 {
		t.Fatalf("want 1 emit, got %d", len(inner.got))
	}
	e := inner.got[0]
	if e.Metadata["outcome"] != "rejected_validation" {
		t.Errorf("outcome: %q", e.Metadata["outcome"])
	}
	if e.Metadata["reason"] != consolidation.ReasonInstitutionalWriteBlocked {
		t.Errorf("reason: %q", e.Metadata["reason"])
	}
}

func TestConsolidationAuditAdapter_HandlesEmptyTargets(t *testing.T) {
	inner := &recordingInner{}
	a := &consolidationAuditAdapter{inner: inner}
	_ = a.LogConsolidation(context.Background(), consolidation.AuditEntry{
		RunID:       "r",
		WorkspaceID: "ws",
		PackRef:     "p",
		ActionKind:  consolidation.ActionCreateSummary,
		Outcome:     consolidation.OutcomeApplied,
		// TargetIDs intentionally nil
	})
	e := inner.got[0]
	if _, ok := e.Metadata["target_ids"]; ok {
		t.Errorf("target_ids should be omitted when nil, got %q", e.Metadata["target_ids"])
	}
}
