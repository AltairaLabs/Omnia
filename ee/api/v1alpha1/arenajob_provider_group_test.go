/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package v1alpha1

import (
	"encoding/json"
	"testing"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestArenaProviderGroupUnmarshalJSON_Array(t *testing.T) {
	input := `[{"providerRef":{"name":"claude"}},{"agentRef":{"name":"my-agent"}}]`

	var g ArenaProviderGroup
	if err := json.Unmarshal([]byte(input), &g); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if g.IsMapMode() {
		t.Fatal("expected array mode, got map mode")
	}
	if len(g.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(g.Entries))
	}
	if g.Entries[0].ProviderRef == nil || g.Entries[0].ProviderRef.Name != "claude" {
		t.Errorf("first entry should be providerRef claude, got %+v", g.Entries[0])
	}
	if g.Entries[1].AgentRef == nil || g.Entries[1].AgentRef.Name != "my-agent" {
		t.Errorf("second entry should be agentRef my-agent, got %+v", g.Entries[1])
	}
}

func TestArenaProviderGroupUnmarshalJSON_Map(t *testing.T) {
	input := `{"judge-quality":{"providerRef":{"name":"haiku"}},"sim-provider":{"agentRef":{"name":"sim-agent"}}}`

	var g ArenaProviderGroup
	if err := json.Unmarshal([]byte(input), &g); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !g.IsMapMode() {
		t.Fatal("expected map mode, got array mode")
	}
	if len(g.Mapping) != 2 {
		t.Fatalf("expected 2 mapping entries, got %d", len(g.Mapping))
	}

	judge, ok := g.Mapping["judge-quality"]
	if !ok {
		t.Fatal("expected judge-quality mapping entry")
	}
	if judge.ProviderRef == nil || judge.ProviderRef.Name != "haiku" {
		t.Errorf("judge-quality should reference haiku, got %+v", judge)
	}

	sim, ok := g.Mapping["sim-provider"]
	if !ok {
		t.Fatal("expected sim-provider mapping entry")
	}
	if sim.AgentRef == nil || sim.AgentRef.Name != "sim-agent" {
		t.Errorf("sim-provider should reference sim-agent, got %+v", sim)
	}
}

func TestArenaProviderGroupMarshalJSON_Array(t *testing.T) {
	g := ArenaProviderGroup{
		Entries: []ArenaProviderEntry{
			{ProviderRef: &corev1alpha1.ProviderRef{Name: "claude"}},
		},
	}

	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should produce a JSON array
	if data[0] != '[' {
		t.Errorf("expected JSON array, got: %s", string(data))
	}

	// Round-trip
	var g2 ArenaProviderGroup
	if err := json.Unmarshal(data, &g2); err != nil {
		t.Fatalf("round-trip unmarshal error: %v", err)
	}
	if g2.IsMapMode() {
		t.Error("round-trip should produce array mode")
	}
	if len(g2.Entries) != 1 {
		t.Errorf("expected 1 entry after round-trip, got %d", len(g2.Entries))
	}
}

func TestArenaProviderGroupMarshalJSON_Map(t *testing.T) {
	g := ArenaProviderGroup{
		Mapping: map[string]ArenaProviderEntry{
			"judge": {ProviderRef: &corev1alpha1.ProviderRef{Name: "haiku"}},
		},
	}

	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should produce a JSON object
	if data[0] != '{' {
		t.Errorf("expected JSON object, got: %s", string(data))
	}

	// Round-trip
	var g2 ArenaProviderGroup
	if err := json.Unmarshal(data, &g2); err != nil {
		t.Fatalf("round-trip unmarshal error: %v", err)
	}
	if !g2.IsMapMode() {
		t.Error("round-trip should produce map mode")
	}
	if len(g2.Mapping) != 1 {
		t.Errorf("expected 1 mapping after round-trip, got %d", len(g2.Mapping))
	}
}

func TestArenaProviderGroupIsMapMode(t *testing.T) {
	arrayGroup := ArenaProviderGroup{
		Entries: []ArenaProviderEntry{
			{ProviderRef: &corev1alpha1.ProviderRef{Name: "claude"}},
		},
	}
	if arrayGroup.IsMapMode() {
		t.Error("array group should not be map mode")
	}

	mapGroup := ArenaProviderGroup{
		Mapping: map[string]ArenaProviderEntry{
			"judge": {ProviderRef: &corev1alpha1.ProviderRef{Name: "haiku"}},
		},
	}
	if !mapGroup.IsMapMode() {
		t.Error("map group should be map mode")
	}

	emptyGroup := ArenaProviderGroup{}
	if emptyGroup.IsMapMode() {
		t.Error("empty group should not be map mode")
	}
}

func TestArenaProviderGroupAllEntries(t *testing.T) {
	arrayGroup := ArenaProviderGroup{
		Entries: []ArenaProviderEntry{
			{ProviderRef: &corev1alpha1.ProviderRef{Name: "a"}},
			{ProviderRef: &corev1alpha1.ProviderRef{Name: "b"}},
		},
	}
	if len(arrayGroup.AllEntries()) != 2 {
		t.Errorf("expected 2 entries, got %d", len(arrayGroup.AllEntries()))
	}

	mapGroup := ArenaProviderGroup{
		Mapping: map[string]ArenaProviderEntry{
			"x": {ProviderRef: &corev1alpha1.ProviderRef{Name: "a"}},
			"y": {AgentRef: &corev1alpha1.LocalObjectReference{Name: "b"}},
		},
	}
	if len(mapGroup.AllEntries()) != 2 {
		t.Errorf("expected 2 entries from map, got %d", len(mapGroup.AllEntries()))
	}

	emptyGroup := ArenaProviderGroup{}
	if len(emptyGroup.AllEntries()) != 0 {
		t.Errorf("expected 0 entries from empty group, got %d", len(emptyGroup.AllEntries()))
	}
}

func TestArenaProviderGroupLen(t *testing.T) {
	tests := []struct {
		name string
		g    ArenaProviderGroup
		want int
	}{
		{
			name: "array mode",
			g: ArenaProviderGroup{
				Entries: []ArenaProviderEntry{
					{ProviderRef: &corev1alpha1.ProviderRef{Name: "a"}},
					{ProviderRef: &corev1alpha1.ProviderRef{Name: "b"}},
				},
			},
			want: 2,
		},
		{
			name: "map mode",
			g: ArenaProviderGroup{
				Mapping: map[string]ArenaProviderEntry{
					"x": {ProviderRef: &corev1alpha1.ProviderRef{Name: "a"}},
				},
			},
			want: 1,
		},
		{
			name: "empty",
			g:    ArenaProviderGroup{},
			want: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.g.Len(); got != tc.want {
				t.Errorf("Len() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestArenaProviderGroupDeepCopy(t *testing.T) {
	original := ArenaProviderGroup{
		Entries: []ArenaProviderEntry{
			{ProviderRef: &corev1alpha1.ProviderRef{Name: "claude"}},
		},
	}

	copied := original.DeepCopy()

	// Modify original
	original.Entries[0].ProviderRef.Name = "modified"

	// Copy should be independent
	if copied.Entries[0].ProviderRef.Name != "claude" {
		t.Error("DeepCopy should create independent copy")
	}

	// Test map mode deep copy
	originalMap := ArenaProviderGroup{
		Mapping: map[string]ArenaProviderEntry{
			"judge": {ProviderRef: &corev1alpha1.ProviderRef{Name: "haiku"}},
		},
	}

	copiedMap := originalMap.DeepCopy()
	originalMap.Mapping["judge"] = ArenaProviderEntry{
		ProviderRef: &corev1alpha1.ProviderRef{Name: "modified"},
	}

	if copiedMap.Mapping["judge"].ProviderRef.Name != "haiku" {
		t.Error("DeepCopy of map mode should create independent copy")
	}

	// Test nil deep copy
	var nilGroup *ArenaProviderGroup
	if nilGroup.DeepCopy() != nil {
		t.Error("DeepCopy of nil should return nil")
	}
}

func TestArenaProviderGroupJSONRoundTrip_Mixed(t *testing.T) {
	// Test a full ArenaJobSpec with mixed provider group modes
	spec := ArenaJobSpec{
		SourceRef: corev1alpha1.LocalObjectReference{Name: "my-source"},
		Providers: map[string]ArenaProviderGroup{
			"default": {
				Entries: []ArenaProviderEntry{
					{ProviderRef: &corev1alpha1.ProviderRef{Name: "claude"}},
					{AgentRef: &corev1alpha1.LocalObjectReference{Name: "my-agent"}},
				},
			},
			"judges": {
				Mapping: map[string]ArenaProviderEntry{
					"judge-quality": {ProviderRef: &corev1alpha1.ProviderRef{Name: "haiku"}},
					"judge-safety":  {AgentRef: &corev1alpha1.LocalObjectReference{Name: "judge-agent"}},
				},
			},
		},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var spec2 ArenaJobSpec
	if err := json.Unmarshal(data, &spec2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Verify array mode
	defaultGroup := spec2.Providers["default"]
	if defaultGroup.IsMapMode() {
		t.Error("default group should be array mode")
	}
	if len(defaultGroup.Entries) != 2 {
		t.Errorf("default group should have 2 entries, got %d", len(defaultGroup.Entries))
	}

	// Verify map mode
	judgesGroup := spec2.Providers["judges"]
	if !judgesGroup.IsMapMode() {
		t.Error("judges group should be map mode")
	}
	if len(judgesGroup.Mapping) != 2 {
		t.Errorf("judges group should have 2 mappings, got %d", len(judgesGroup.Mapping))
	}
	if judgesGroup.Mapping["judge-quality"].ProviderRef.Name != "haiku" {
		t.Error("judge-quality mapping should reference haiku")
	}
}
