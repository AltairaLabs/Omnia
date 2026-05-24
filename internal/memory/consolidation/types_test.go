/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import (
	"encoding/json"
	"testing"
)

func TestActionUnmarshal(t *testing.T) {
	raw := []byte(`[
      {"action":"create_summary","fromIDs":["a","b"],"scope":{"workspaceID":"ws-1"},"content":"x"},
      {"action":"rescope","targetIDs":["c"],"newScope":{"workspaceID":"ws-1","agentID":"ag-1"},"reason":"r"},
      {"action":"supersede","targetIDs":["a","b"],"withID":"s1"},
      {"action":"invalidate","targetIDs":["d"],"validUntil":"2026-05-23T00:00:00Z"},
      {"action":"merge_entities","canonicalID":"c1","mergeIDs":["m1","m2"]},
      {"action":"discard","targetIDs":["e"],"reason":"ephemeral"},
      {"action":"rescore","targetID":"f","importance":0.85}
    ]`)
	actions, err := UnmarshalActions(raw)
	if err != nil {
		t.Fatalf("UnmarshalActions: %v", err)
	}
	wantKinds := []ActionKind{
		ActionCreateSummary, ActionRescope, ActionSupersede,
		ActionInvalidate, ActionMergeEntities, ActionDiscard, ActionRescore,
	}
	if len(actions) != len(wantKinds) {
		t.Fatalf("len(actions) = %d, want %d", len(actions), len(wantKinds))
	}
	for i, want := range wantKinds {
		if actions[i].Kind() != want {
			t.Errorf("actions[%d].Kind() = %v, want %v", i, actions[i].Kind(), want)
		}
	}
}

func TestActionUnmarshal_RejectsUnknownAction(t *testing.T) {
	raw := []byte(`[{"action":"frobnicate","targetIDs":["a"]}]`)
	_, err := UnmarshalActions(raw)
	if err == nil {
		t.Fatal("expected error for unknown action kind")
	}
}

func TestActionUnmarshal_RejectsInvalidJSON(t *testing.T) {
	_, err := UnmarshalActions([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestScopeShape(t *testing.T) {
	cases := []struct {
		name  string
		scope Scope
		want  ScopeShape
	}{
		{"institutional", Scope{WorkspaceID: "w"}, ScopeShapeInstitutional},
		{"agent-scoped", Scope{WorkspaceID: "w", AgentID: "a"}, ScopeShapeAgentScoped},
		{"user-scoped", Scope{WorkspaceID: "w", UserID: "u"}, ScopeShapeUserScoped},
		{"user-for-agent", Scope{WorkspaceID: "w", AgentID: "a", UserID: "u"}, ScopeShapeUserForAgent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.scope.Shape(); got != tc.want {
				t.Errorf("Shape() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPreFilterAxisString(t *testing.T) {
	cases := map[PreFilterAxis]string{
		AxisStaleObservations:         "staleObservations",
		AxisCrossScopeCandidates:      "crossScopeCandidates",
		AxisEntityDuplicateCandidates: "entityDuplicateCandidates",
	}
	for axis, want := range cases {
		if got := axis.String(); got != want {
			t.Errorf("axis %v string = %q, want %q", axis, got, want)
		}
	}
}

func TestFunctionInputRoundTrip(t *testing.T) {
	in := FunctionInput{
		Axis:        AxisStaleObservations,
		WorkspaceID: testWorkspaceID,
		Buckets: []Bucket{{
			Key: "kind=preference;name=units",
			Entries: []BucketEntry{{
				ID: testObsID, Content: "...", Mutability: MutabilityMutable,
				Scope: Scope{WorkspaceID: testWorkspaceID, UserID: "u-1"},
			}},
		}},
		Gates: ResolvedGates{
			MinDistinctUserCount: map[string]int32{SafetyGateAgentScoped: 5},
			RequirePIIRedaction:  true,
		},
	}
	out, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var decoded FunctionInput
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Axis != in.Axis || decoded.WorkspaceID != in.WorkspaceID || len(decoded.Buckets) != 1 {
		t.Errorf("round-trip mismatch: %+v", decoded)
	}
	if decoded.Gates.MinDistinctUserCount[SafetyGateAgentScoped] != 5 {
		t.Errorf("gates round-trip mismatch: %+v", decoded.Gates)
	}
}
