/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import (
	"context"
	"testing"
	"time"
)

func TestApplier_AppliesAcceptedActions(t *testing.T) {
	store := &MockStore{}
	applier := NewApplier(store)

	accepted := []Result{
		{Action: CreateSummaryAction{
			FromIDs: []string{"a"}, Scope: Scope{WorkspaceID: testWorkspaceID}, Content: "x",
		}, Accepted: true},
		{Action: InvalidateAction{
			TargetIDs: []string{"b"}, ValidUntil: time.Now(),
		}, Accepted: true},
	}
	if err := applier.Apply(context.Background(), ApplyContext{
		WorkspaceID: testWorkspaceID,
		RunID:       "run-1",
		PackRef:     "safe-default-summarizer",
	}, accepted); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(store.Summaries) != 1 || len(store.Invalidates) != 1 {
		t.Errorf("apply did not persist: %+v", store)
	}
	if store.Summaries[0].PromotedByPack != "safe-default-summarizer" {
		t.Errorf("lineage not populated: %+v", store.Summaries[0])
	}
}

func TestApplier_SkipsRejectedActions(t *testing.T) {
	store := &MockStore{}
	applier := NewApplier(store)
	results := []Result{
		{Action: DiscardAction{TargetIDs: []string{"a"}}, Accepted: false, Reason: ReasonMutabilityBlocked},
	}
	if err := applier.Apply(context.Background(), ApplyContext{
		WorkspaceID: testWorkspaceID, RunID: "run-1", PackRef: "p",
	}, results); err != nil {
		t.Fatal(err)
	}
	if store.DiscardCount != 0 {
		t.Errorf("rejected action was applied: discards=%d", store.DiscardCount)
	}
}

func TestApplier_AuditEmitsOneEntryPerAction(t *testing.T) {
	audit := &recordingAuditor{}
	a := NewApplierWithAudit(&MockStore{}, audit)
	results := []Result{
		{Action: CreateSummaryAction{
			FromIDs: []string{"a"}, Scope: Scope{WorkspaceID: testWorkspaceID}, Content: "x",
		}, Accepted: true},
		{Action: DiscardAction{TargetIDs: []string{"b"}}, Accepted: false, Reason: ReasonMutabilityBlocked},
	}
	if err := a.Apply(context.Background(), ApplyContext{
		WorkspaceID: testWorkspaceID, RunID: "run-1", PackRef: "p", Now: time.Now(),
	}, results); err != nil {
		t.Fatal(err)
	}
	if len(audit.entries) != 2 {
		t.Fatalf("expected 2 audit entries, got %d", len(audit.entries))
	}
	if audit.entries[0].Outcome != OutcomeApplied {
		t.Errorf("entries[0].Outcome = %q, want %q", audit.entries[0].Outcome, OutcomeApplied)
	}
	if audit.entries[1].Outcome != OutcomeRejectedValidation {
		t.Errorf("entries[1].Outcome = %q, want %q", audit.entries[1].Outcome, OutcomeRejectedValidation)
	}
}

// MockStore captures applier calls for assertion. Exported so the
// worker tests can reuse it.
type MockStore struct {
	Summaries    []SummaryWrite
	Invalidates  []InvalidateWrite
	Rescopes     []RescopeWrite
	Merges       []MergeWrite
	Rescores     []RescoreWrite
	Supersedes   []SupersedeWrite
	DiscardCount int
}

// SaveSummary records a SummaryWrite and returns a synthetic ID.
func (m *MockStore) SaveSummary(_ context.Context, w SummaryWrite) (string, error) {
	m.Summaries = append(m.Summaries, w)
	return "new-id", nil
}

// Invalidate records an InvalidateWrite.
func (m *MockStore) Invalidate(_ context.Context, w InvalidateWrite) error {
	m.Invalidates = append(m.Invalidates, w)
	return nil
}

// Rescope records a RescopeWrite.
func (m *MockStore) Rescope(_ context.Context, w RescopeWrite) error {
	m.Rescopes = append(m.Rescopes, w)
	return nil
}

// MergeEntities records a MergeWrite.
func (m *MockStore) MergeEntities(_ context.Context, w MergeWrite) error {
	m.Merges = append(m.Merges, w)
	return nil
}

// Discard increments DiscardCount.
func (m *MockStore) Discard(_ context.Context, _ DiscardWrite) error {
	m.DiscardCount++
	return nil
}

// Rescore records a RescoreWrite.
func (m *MockStore) Rescore(_ context.Context, w RescoreWrite) error {
	m.Rescores = append(m.Rescores, w)
	return nil
}

// Supersede records a SupersedeWrite.
func (m *MockStore) Supersede(_ context.Context, w SupersedeWrite) error {
	m.Supersedes = append(m.Supersedes, w)
	return nil
}

type recordingAuditor struct{ entries []AuditEntry }

func (r *recordingAuditor) LogConsolidation(_ context.Context, e AuditEntry) error {
	r.entries = append(r.entries, e)
	return nil
}

func TestApplier_DispatchesAllActionKinds(t *testing.T) {
	store := &MockStore{}
	a := NewApplier(store)
	now := time.Now()
	results := []Result{
		{Action: CreateSummaryAction{FromIDs: []string{"a"}, Scope: Scope{WorkspaceID: testWorkspaceID}, Content: "x"}, Accepted: true},
		{Action: SupersedeAction{TargetIDs: []string{"a"}, WithID: "s"}, Accepted: true},
		{Action: RescopeAction{TargetIDs: []string{"a"}, NewScope: Scope{WorkspaceID: testWorkspaceID, AgentID: "ag"}}, Accepted: true},
		{Action: InvalidateAction{TargetIDs: []string{"a"}, ValidUntil: now}, Accepted: true},
		{Action: MergeEntitiesAction{CanonicalID: "c", MergeIDs: []string{"m"}}, Accepted: true},
		{Action: DiscardAction{TargetIDs: []string{"a"}}, Accepted: true},
		{Action: RescoreAction{TargetID: "a", Confidence: 0.9}, Accepted: true},
	}
	if err := a.Apply(context.Background(), ApplyContext{
		WorkspaceID: testWorkspaceID, RunID: "r1", PackRef: "p", Now: now,
	}, results); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(store.Summaries) != 1 || len(store.Supersedes) != 1 ||
		len(store.Rescopes) != 1 || len(store.Invalidates) != 1 ||
		len(store.Merges) != 1 || store.DiscardCount != 1 ||
		len(store.Rescores) != 1 {
		t.Errorf("not all action kinds dispatched: %+v", store)
	}
}

type failingStore struct{ MockStore }

func (failingStore) Invalidate(context.Context, InvalidateWrite) error {
	return errFakeStoreFailure
}

var errFakeStoreFailure = errInvalidateFailed{}

type errInvalidateFailed struct{}

func (errInvalidateFailed) Error() string { return "fake store failure" }

func TestApplier_StoreErrorEmitsAuditAndReturns(t *testing.T) {
	audit := &recordingAuditor{}
	a := NewApplierWithAudit(&failingStore{}, audit)
	results := []Result{
		{Action: InvalidateAction{TargetIDs: []string{"a"}, ValidUntil: time.Now()}, Accepted: true},
	}
	err := a.Apply(context.Background(), ApplyContext{
		WorkspaceID: testWorkspaceID, RunID: "r1", PackRef: "p",
	}, results)
	if err == nil {
		t.Fatal("expected error from failing store")
	}
	if len(audit.entries) != 1 || audit.entries[0].Outcome != OutcomeApplyFailed {
		t.Errorf("expected apply_failed audit, got %+v", audit.entries)
	}
}

func TestApplier_NowDefaultsToTimeNow(t *testing.T) {
	// Confirm ApplyContext.Now zero value gets filled in.
	store := &MockStore{}
	a := NewApplier(store)
	results := []Result{
		{Action: CreateSummaryAction{FromIDs: []string{"a"}, Scope: Scope{WorkspaceID: testWorkspaceID}, Content: "x"}, Accepted: true},
	}
	before := time.Now()
	if err := a.Apply(context.Background(), ApplyContext{
		WorkspaceID: testWorkspaceID, RunID: "r1", PackRef: "p",
		// Now intentionally zero
	}, results); err != nil {
		t.Fatal(err)
	}
	if store.Summaries[0].PromotedAt.Before(before) {
		t.Errorf("PromotedAt %v predates test start %v", store.Summaries[0].PromotedAt, before)
	}
}
