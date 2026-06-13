/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import (
	"testing"
	"time"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	testWorkspaceID = "ws-1"
	testObsID       = "obs-1"
	testUserID      = "u-1"
)

func TestValidator_RejectsInstitutionalRescope(t *testing.T) {
	v := NewValidator(ValidatorOptions{WorkspaceID: testWorkspaceID})
	actions := []Action{
		RescopeAction{
			TargetIDs: []string{testObsID},
			NewScope:  Scope{WorkspaceID: testWorkspaceID}, // (ws, null, null) = institutional
			Reason:    "trying to promote",
		},
	}
	ctx := ValidationContext{
		RowMutability: map[string]string{testObsID: MutabilityMutable},
	}
	results := v.Validate(actions, ctx)
	if len(results) != 1 || results[0].Accepted {
		t.Fatalf("expected reject, got %+v", results)
	}
	if results[0].Reason != ReasonInstitutionalWriteBlocked {
		t.Errorf("reason = %q, want %q", results[0].Reason, ReasonInstitutionalWriteBlocked)
	}
}

func TestValidator_RejectsActionOnImmutableTarget(t *testing.T) {
	v := NewValidator(ValidatorOptions{WorkspaceID: testWorkspaceID})
	actions := []Action{
		InvalidateAction{TargetIDs: []string{"obs-r"}, Reason: "stale", ValidUntil: time.Now().Add(time.Hour)},
	}
	ctx := ValidationContext{
		RowMutability: map[string]string{"obs-r": "immutable"},
	}
	results := v.Validate(actions, ctx)
	if len(results) != 1 || results[0].Accepted {
		t.Fatalf("expected reject, got %+v", results)
	}
	if results[0].Reason != ReasonMutabilityBlocked {
		t.Errorf("reason = %q, want %q", results[0].Reason, ReasonMutabilityBlocked)
	}
}

func TestValidator_RejectsRescopeBelowKAnonymity(t *testing.T) {
	gates := memoryv1.MemoryConsolidationSafetyGates{
		MinDistinctUserCount: map[string]int32{
			SafetyGateAgentScoped: 5,
			SafetyGateUserScoped:  1,
		},
	}
	v := NewValidator(ValidatorOptions{WorkspaceID: testWorkspaceID, Gates: gates})
	actions := []Action{
		RescopeAction{
			TargetIDs: []string{testObsID},
			NewScope:  Scope{WorkspaceID: testWorkspaceID, AgentID: "ag-1"}, // agent-scoped (needs k=5)
		},
	}
	ctx := ValidationContext{
		RowMutability:           map[string]string{testObsID: MutabilityMutable},
		BucketDistinctUserCount: 3, // below the agentScoped threshold
	}
	results := v.Validate(actions, ctx)
	if len(results) != 1 || results[0].Accepted {
		t.Fatalf("expected reject, got %+v", results)
	}
	if results[0].Reason != ReasonAnonymityBelowThreshold {
		t.Errorf("reason = %q, want %q", results[0].Reason, ReasonAnonymityBelowThreshold)
	}
}

func TestValidator_AcceptsRescopeAtThreshold(t *testing.T) {
	gates := memoryv1.MemoryConsolidationSafetyGates{
		MinDistinctUserCount: map[string]int32{SafetyGateAgentScoped: 5},
	}
	v := NewValidator(ValidatorOptions{WorkspaceID: testWorkspaceID, Gates: gates})
	actions := []Action{
		RescopeAction{
			TargetIDs: []string{testObsID},
			NewScope:  Scope{WorkspaceID: testWorkspaceID, AgentID: "ag-1"},
		},
	}
	ctx := ValidationContext{
		RowMutability:           map[string]string{testObsID: MutabilityMutable},
		BucketDistinctUserCount: 5,
	}
	results := v.Validate(actions, ctx)
	if !results[0].Accepted {
		t.Errorf("expected accept at threshold, got: %+v", results[0])
	}
}

// #1334: an unsupported maxScopeWidening value must fail closed rather than be
// silently ignored.
func TestValidator_RejectsUnsupportedMaxScopeWidening(t *testing.T) {
	gates := memoryv1.MemoryConsolidationSafetyGates{MaxScopeWidening: "global"} // unsupported in v1
	v := NewValidator(ValidatorOptions{WorkspaceID: testWorkspaceID, Gates: gates})
	actions := []Action{
		RescopeAction{
			TargetIDs: []string{testObsID},
			NewScope:  Scope{WorkspaceID: testWorkspaceID, AgentID: "ag-1"},
		},
	}
	ctx := ValidationContext{
		RowMutability:           map[string]string{testObsID: MutabilityMutable},
		BucketDistinctUserCount: 100, // well above any threshold
	}
	results := v.Validate(actions, ctx)
	if len(results) != 1 || results[0].Accepted {
		t.Fatalf("expected reject, got %+v", results)
	}
	if results[0].Reason != ReasonScopeWideningUnsupported {
		t.Errorf("reason = %q, want %q", results[0].Reason, ReasonScopeWideningUnsupported)
	}
}

func TestValidator_RejectsScopeOutsideWorkspace(t *testing.T) {
	v := NewValidator(ValidatorOptions{WorkspaceID: testWorkspaceID})
	actions := []Action{
		RescopeAction{
			TargetIDs: []string{testObsID},
			NewScope:  Scope{WorkspaceID: "ws-other", AgentID: "ag-1"},
		},
	}
	ctx := ValidationContext{
		RowMutability: map[string]string{testObsID: MutabilityMutable},
	}
	results := v.Validate(actions, ctx)
	if results[0].Reason != ReasonScopeOutsideWorkspace {
		t.Errorf("reason = %q, want %q", results[0].Reason, ReasonScopeOutsideWorkspace)
	}
}

func TestValidator_RejectsUnknownTarget(t *testing.T) {
	v := NewValidator(ValidatorOptions{WorkspaceID: testWorkspaceID})
	actions := []Action{
		InvalidateAction{TargetIDs: []string{"obs-missing"}, ValidUntil: time.Now().Add(time.Hour)},
	}
	results := v.Validate(actions, ValidationContext{RowMutability: map[string]string{}})
	if results[0].Reason != ReasonTargetUnknown {
		t.Errorf("reason = %q, want %q", results[0].Reason, ReasonTargetUnknown)
	}
}

func TestValidator_AcceptsValidCreateSummary(t *testing.T) {
	v := NewValidator(ValidatorOptions{WorkspaceID: testWorkspaceID})
	actions := []Action{
		CreateSummaryAction{
			FromIDs: []string{testObsID, "obs-2"},
			Scope:   Scope{WorkspaceID: testWorkspaceID, UserID: "u-1"},
			Content: "Alice prefers Celsius.",
		},
	}
	// CreateSummary doesn't modify rows; FromIDs aren't targets.
	results := v.Validate(actions, ValidationContext{})
	if !results[0].Accepted {
		t.Fatalf("expected accept, got %+v", results[0])
	}
}

func TestValidator_RejectsShapeInvalid(t *testing.T) {
	cases := []struct {
		name   string
		action Action
	}{
		{"summary no fromIDs", CreateSummaryAction{Content: "x"}},
		{"summary no content", CreateSummaryAction{FromIDs: []string{"a"}}},
		{"supersede no targets", SupersedeAction{WithID: "s"}},
		{"supersede no withID", SupersedeAction{TargetIDs: []string{"a"}}},
		{"rescope no targets", RescopeAction{NewScope: Scope{WorkspaceID: testWorkspaceID, AgentID: "a"}}},
		{"invalidate no targets", InvalidateAction{ValidUntil: time.Now()}},
		{"merge no canonical", MergeEntitiesAction{MergeIDs: []string{"a"}}},
		{"merge no merge ids", MergeEntitiesAction{CanonicalID: "c"}},
		{"discard no targets", DiscardAction{}},
		{"rescore no target", RescoreAction{Importance: 0.5}},
	}
	v := NewValidator(ValidatorOptions{WorkspaceID: testWorkspaceID})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := v.Validate([]Action{tc.action}, ValidationContext{})
			if r[0].Reason != ReasonShapeInvalid {
				t.Errorf("expected shape_invalid, got %q", r[0].Reason)
			}
		})
	}
}
