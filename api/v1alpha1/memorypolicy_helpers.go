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

import "strconv"

// RecallInlineThresholdBytes returns the configured large-memory
// inline cutoff or 0 if the policy doesn't override it. The recall
// handler treats 0 as "use the API default".
func (p *MemoryPolicy) RecallInlineThresholdBytes() int {
	if p == nil || p.Spec.Recall == nil || p.Spec.Recall.InlineThresholdBytes == nil {
		return 0
	}
	return int(*p.Spec.Recall.InlineThresholdBytes)
}

// RecallMaxRelatedPerMemory returns the configured per-memory
// `related[]` cap or 0 if unset. The recall handler treats 0 as
// "use the store default".
func (p *MemoryPolicy) RecallMaxRelatedPerMemory() int {
	if p == nil || p.Spec.Recall == nil || p.Spec.Recall.MaxRelatedPerMemory == nil {
		return 0
	}
	return int(*p.Spec.Recall.MaxRelatedPerMemory)
}

// DedupRequireAboutForKinds returns the configured kinds requiring
// an `about={kind, key}` hint, or nil if unset. nil disables the
// check (back-compat default).
func (p *MemoryPolicy) DedupRequireAboutForKinds() []string {
	if p == nil || p.Spec.Dedup == nil {
		return nil
	}
	return p.Spec.Dedup.RequireAboutForKinds
}

// DedupEmbeddingEnabled reports whether the embedding-similarity
// dedup path is enabled. Defaults to true (the existing behaviour)
// when the policy doesn't specify.
func (p *MemoryPolicy) DedupEmbeddingEnabled() bool {
	if p == nil || p.Spec.Dedup == nil || p.Spec.Dedup.EmbeddingSimilarity == nil {
		return true
	}
	if p.Spec.Dedup.EmbeddingSimilarity.Enabled == nil {
		return true
	}
	return *p.Spec.Dedup.EmbeddingSimilarity.Enabled
}

// DedupAutoSupersedeAbove returns the cosine-similarity threshold
// above which the server auto-supersedes a near-duplicate. Returns
// 0 when unset; callers fall back to the default.
func (p *MemoryPolicy) DedupAutoSupersedeAbove() float64 {
	return parsePolicyFloat(p, func(d *MemoryEmbeddingDedupConfig) string {
		return d.AutoSupersedeAbove
	})
}

// DedupSurfaceDuplicatesAbove returns the cosine-similarity threshold
// above which the server surfaces a near-duplicate as a
// `potential_duplicate` for the agent to consider. Returns 0 when
// unset.
func (p *MemoryPolicy) DedupSurfaceDuplicatesAbove() float64 {
	return parsePolicyFloat(p, func(d *MemoryEmbeddingDedupConfig) string {
		return d.SurfaceDuplicatesAbove
	})
}

// DedupCandidateLimit returns the maximum number of candidates the
// server returns in `potential_duplicates`. Zero means "use the
// API default".
func (p *MemoryPolicy) DedupCandidateLimit() int {
	if p == nil || p.Spec.Dedup == nil || p.Spec.Dedup.EmbeddingSimilarity == nil {
		return 0
	}
	if p.Spec.Dedup.EmbeddingSimilarity.CandidateLimit == nil {
		return 0
	}
	return int(*p.Spec.Dedup.EmbeddingSimilarity.CandidateLimit)
}

// parsePolicyFloat extracts a string-typed similarity threshold
// from the embedding-dedup block and parses it. Returns 0 on any
// missing field, parse error, or out-of-range value — callers treat
// 0 as "use the default".
func parsePolicyFloat(p *MemoryPolicy, get func(*MemoryEmbeddingDedupConfig) string) float64 {
	if p == nil || p.Spec.Dedup == nil || p.Spec.Dedup.EmbeddingSimilarity == nil {
		return 0
	}
	raw := get(p.Spec.Dedup.EmbeddingSimilarity)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 || v > 1 {
		return 0
	}
	return v
}
