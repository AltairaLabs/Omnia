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

import "testing"

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
