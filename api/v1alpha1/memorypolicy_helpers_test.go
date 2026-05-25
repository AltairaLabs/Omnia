/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func TestMemoryPolicy_RecallHelpers_Defaults(t *testing.T) {
	var p *MemoryPolicy // nil-safe path
	if got := p.RecallInlineThresholdBytes(); got != 0 {
		t.Errorf("nil policy threshold: want 0, got %d", got)
	}
	if got := p.RecallMaxRelatedPerMemory(); got != 0 {
		t.Errorf("nil policy related cap: want 0, got %d", got)
	}

	empty := &MemoryPolicy{}
	if got := empty.RecallInlineThresholdBytes(); got != 0 {
		t.Errorf("empty policy threshold: want 0, got %d", got)
	}
}

func TestMemoryPolicy_RecallHelpers_Set(t *testing.T) {
	p := &MemoryPolicy{Spec: MemoryPolicySpec{Recall: &MemoryRecallConfig{
		InlineThresholdBytes: ptr(int32(4096)),
		MaxRelatedPerMemory:  ptr(int32(7)),
	}}}
	if got := p.RecallInlineThresholdBytes(); got != 4096 {
		t.Errorf("threshold: want 4096, got %d", got)
	}
	if got := p.RecallMaxRelatedPerMemory(); got != 7 {
		t.Errorf("related cap: want 7, got %d", got)
	}
}

func TestMemoryPolicy_DedupHelpers_Defaults(t *testing.T) {
	var p *MemoryPolicy
	if p.DedupRequireAboutForKinds() != nil {
		t.Error("nil policy must return nil kinds")
	}
	if !p.DedupEmbeddingEnabled() {
		t.Error("nil policy must default to enabled embedding dedup")
	}

	disabled := &MemoryPolicy{Spec: MemoryPolicySpec{Dedup: &MemoryDedupConfig{
		EmbeddingSimilarity: &MemoryEmbeddingDedupConfig{Enabled: ptr(false)},
	}}}
	if disabled.DedupEmbeddingEnabled() {
		t.Error("explicit enabled=false must disable embedding dedup")
	}
}

func TestMemoryPolicy_DedupHelpers_Thresholds(t *testing.T) {
	p := &MemoryPolicy{Spec: MemoryPolicySpec{Dedup: &MemoryDedupConfig{
		RequireAboutForKinds: []string{"fact", "preference"},
		EmbeddingSimilarity: &MemoryEmbeddingDedupConfig{
			AutoSupersedeAbove:     "0.92",
			SurfaceDuplicatesAbove: "0.78",
			CandidateLimit:         ptr(int32(8)),
		},
	}}}
	if got := p.DedupAutoSupersedeAbove(); got != 0.92 {
		t.Errorf("auto supersede: want 0.92, got %v", got)
	}
	if got := p.DedupSurfaceDuplicatesAbove(); got != 0.78 {
		t.Errorf("surface duplicates: want 0.78, got %v", got)
	}
	if got := p.DedupCandidateLimit(); got != 8 {
		t.Errorf("candidate limit: want 8, got %d", got)
	}
	kinds := p.DedupRequireAboutForKinds()
	if len(kinds) != 2 || kinds[0] != "fact" {
		t.Errorf("kinds: want [fact preference], got %v", kinds)
	}
}

func TestMemoryPolicy_DedupHelpers_RejectInvalidThresholds(t *testing.T) {
	cases := []string{"not-a-number", "-0.1", "1.5", ""}
	for _, raw := range cases {
		p := &MemoryPolicy{Spec: MemoryPolicySpec{Dedup: &MemoryDedupConfig{
			EmbeddingSimilarity: &MemoryEmbeddingDedupConfig{AutoSupersedeAbove: raw},
		}}}
		if got := p.DedupAutoSupersedeAbove(); got != 0 {
			t.Errorf("raw=%q: invalid value must return 0, got %v", raw, got)
		}
	}
}

// TestMemoryPolicy_DedupRawAccessors proves the *Raw() accessors
// surface the operator-set string verbatim — used by the runtime
// to distinguish "field unset" (raw == "") from "set but invalid"
// (raw != "" && parsed == 0) so misconfig logs fire.
func TestMemoryPolicy_DedupRawAccessors(t *testing.T) {
	var nilP *MemoryPolicy
	if got := nilP.DedupAutoSupersedeAboveRaw(); got != "" {
		t.Errorf("nil policy raw: want \"\", got %q", got)
	}
	if got := nilP.DedupSurfaceDuplicatesAboveRaw(); got != "" {
		t.Errorf("nil policy raw: want \"\", got %q", got)
	}

	empty := &MemoryPolicy{}
	if got := empty.DedupAutoSupersedeAboveRaw(); got != "" {
		t.Errorf("empty policy raw: want \"\", got %q", got)
	}

	bad := &MemoryPolicy{Spec: MemoryPolicySpec{Dedup: &MemoryDedupConfig{
		EmbeddingSimilarity: &MemoryEmbeddingDedupConfig{
			AutoSupersedeAbove:     "1.5",
			SurfaceDuplicatesAbove: "not-a-number",
		},
	}}}
	if got := bad.DedupAutoSupersedeAboveRaw(); got != "1.5" {
		t.Errorf("raw must surface invalid value verbatim, got %q", got)
	}
	if got := bad.DedupAutoSupersedeAbove(); got != 0 {
		t.Errorf("parsed must reject invalid value, got %v", got)
	}
	if got := bad.DedupSurfaceDuplicatesAboveRaw(); got != "not-a-number" {
		t.Errorf("raw must surface unparseable value verbatim, got %q", got)
	}
}

func TestMemoryPolicy_ResolvedSafetyGates_Defaults(t *testing.T) {
	// Nil policy returns defaults: agentScoped=5, userScoped=1,
	// PII redaction required, scope-widening capped at workspace.
	var nilP *MemoryPolicy
	got := nilP.ResolvedSafetyGates()
	if got.MinDistinctUserCount["agentScoped"] != 5 {
		t.Errorf("nil policy agentScoped: want 5, got %d",
			got.MinDistinctUserCount["agentScoped"])
	}
	if got.MinDistinctUserCount["userScoped"] != 1 {
		t.Errorf("nil policy userScoped: want 1, got %d",
			got.MinDistinctUserCount["userScoped"])
	}
	if !got.PIIRedactionEnabled() {
		t.Error("nil policy: PIIRedactionEnabled default must be true")
	}
	if got.MaxScopeWidening != "workspace" {
		t.Errorf("nil policy MaxScopeWidening: want \"workspace\", got %q",
			got.MaxScopeWidening)
	}

	// Empty policy (no Consolidation block) returns defaults too.
	empty := &MemoryPolicy{}
	emptyGates := empty.ResolvedSafetyGates()
	if emptyGates.MinDistinctUserCount["agentScoped"] != 5 {
		t.Errorf("empty policy agentScoped: want 5, got %d",
			emptyGates.MinDistinctUserCount["agentScoped"])
	}
}

func TestMemoryPolicy_ResolvedSafetyGates_Override(t *testing.T) {
	// Operator-set values win over defaults; unset keys keep defaults.
	p := &MemoryPolicy{Spec: MemoryPolicySpec{
		Consolidation: &MemoryConsolidationConfig{
			SafetyGates: &MemoryConsolidationSafetyGates{
				MinDistinctUserCount: map[string]int32{
					"agentScoped": 50, // override
					// userScoped not set → keep default of 1
				},
				MaxScopeWidening:    "workspace",
				RequirePIIRedaction: ptr(true),
			},
		},
	}}
	got := p.ResolvedSafetyGates()
	if got.MinDistinctUserCount["agentScoped"] != 50 {
		t.Errorf("override agentScoped: want 50, got %d",
			got.MinDistinctUserCount["agentScoped"])
	}
	if got.MinDistinctUserCount["userScoped"] != 1 {
		t.Errorf("unset userScoped should keep default 1, got %d",
			got.MinDistinctUserCount["userScoped"])
	}
}

func TestMemoryPolicy_ResolvedTimeouts_Defaults(t *testing.T) {
	// nil policy returns the design defaults (5m / 30m).
	var p *MemoryPolicy
	fc, wc := p.ResolvedTimeouts()
	if fc != 5*time.Minute {
		t.Errorf("nil policy FunctionCall: want 5m, got %s", fc)
	}
	if wc != 30*time.Minute {
		t.Errorf("nil policy PassWallClock: want 30m, got %s", wc)
	}

	// Empty policy (no Consolidation block) returns defaults too.
	empty := &MemoryPolicy{}
	fc, wc = empty.ResolvedTimeouts()
	if fc != 5*time.Minute || wc != 30*time.Minute {
		t.Errorf("empty policy: got (%s, %s), want (5m, 30m)", fc, wc)
	}
}

func TestMemoryPolicy_ResolvedTimeouts_Override(t *testing.T) {
	p := &MemoryPolicy{Spec: MemoryPolicySpec{
		Consolidation: &MemoryConsolidationConfig{
			Timeouts: &MemoryConsolidationTimeouts{
				FunctionCall:  &metav1.Duration{Duration: 90 * time.Second},
				PassWallClock: &metav1.Duration{Duration: 10 * time.Minute},
			},
		},
	}}
	fc, wc := p.ResolvedTimeouts()
	if fc != 90*time.Second {
		t.Errorf("override FunctionCall: want 90s, got %s", fc)
	}
	if wc != 10*time.Minute {
		t.Errorf("override PassWallClock: want 10m, got %s", wc)
	}
}

func TestMemoryPolicy_ResolvedTimeouts_PartialOverride(t *testing.T) {
	// FunctionCall set, PassWallClock unset → PassWallClock keeps default.
	p := &MemoryPolicy{Spec: MemoryPolicySpec{
		Consolidation: &MemoryConsolidationConfig{
			Timeouts: &MemoryConsolidationTimeouts{
				FunctionCall: &metav1.Duration{Duration: 2 * time.Minute},
			},
		},
	}}
	fc, wc := p.ResolvedTimeouts()
	if fc != 2*time.Minute {
		t.Errorf("FunctionCall: %s", fc)
	}
	if wc != 30*time.Minute {
		t.Errorf("PassWallClock default: %s", wc)
	}
}
