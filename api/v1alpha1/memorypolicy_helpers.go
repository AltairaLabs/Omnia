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
	"strconv"
	"time"
)

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
// 0 when unset OR when the value parsed out of range / unparseable;
// callers fall back to the default. Use DedupAutoSupersedeAboveRaw
// to distinguish "unset" from "set but invalid" for logging.
func (p *MemoryPolicy) DedupAutoSupersedeAbove() float64 {
	return parsePolicyFloat(p, func(d *MemoryEmbeddingDedupConfig) string {
		return d.AutoSupersedeAbove
	})
}

// DedupAutoSupersedeAboveRaw returns the raw string the operator
// set on the CRD, before parsing. Empty when unset. Lets callers
// warn when a non-empty value parsed to 0 (set but invalid).
func (p *MemoryPolicy) DedupAutoSupersedeAboveRaw() string {
	return rawPolicyFloat(p, func(d *MemoryEmbeddingDedupConfig) string {
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

// DedupSurfaceDuplicatesAboveRaw — see DedupAutoSupersedeAboveRaw.
func (p *MemoryPolicy) DedupSurfaceDuplicatesAboveRaw() string {
	return rawPolicyFloat(p, func(d *MemoryEmbeddingDedupConfig) string {
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

// rawPolicyFloat returns the string the operator set on the CRD
// without parsing. "" means unset; any non-empty value that
// parsePolicyFloat returns 0 for is "set but invalid" — useful for
// the caller to log a warning.
func rawPolicyFloat(p *MemoryPolicy, get func(*MemoryEmbeddingDedupConfig) string) string {
	if p == nil || p.Spec.Dedup == nil || p.Spec.Dedup.EmbeddingSimilarity == nil {
		return ""
	}
	return get(p.Spec.Dedup.EmbeddingSimilarity)
}

// ResolvedSafetyGates returns the consolidation safety gates with
// defaults applied for unset fields. Safe to call when the policy is
// nil or the Consolidation / SafetyGates blocks are unset.
//
// Defaults:
//   - MinDistinctUserCount[agentScoped] = 5
//   - MinDistinctUserCount[userScoped]  = 1
//   - MaxScopeWidening                  = "workspace"
//   - RequirePIIRedaction               = true
//
// Operator-set values win over defaults; absent map keys keep
// their default. Per spec
// docs/local-backlog/2026-05-22-memory-consolidation-design.md.
func (p *MemoryPolicy) ResolvedSafetyGates() MemoryConsolidationSafetyGates {
	trueVal := true
	resolved := MemoryConsolidationSafetyGates{
		MinDistinctUserCount: map[string]int32{
			"agentScoped": 5,
			"userScoped":  1,
		},
		MaxScopeWidening:    "workspace",
		RequirePIIRedaction: &trueVal,
	}
	if p == nil || p.Spec.Consolidation == nil || p.Spec.Consolidation.SafetyGates == nil {
		return resolved
	}
	g := p.Spec.Consolidation.SafetyGates
	for k, v := range g.MinDistinctUserCount {
		resolved.MinDistinctUserCount[k] = v
	}
	if g.MaxScopeWidening != "" {
		resolved.MaxScopeWidening = g.MaxScopeWidening
	}
	// Pointer type distinguishes "operator left unset" (apply default) from
	// "operator set to false" (opt out).
	if g.RequirePIIRedaction != nil {
		resolved.RequirePIIRedaction = g.RequirePIIRedaction
	}
	return resolved
}

// PIIRedactionEnabled is a nil-safe deref of RequirePIIRedaction.
// Callers that consume the resolved gates (worker, validator, tests)
// use this rather than touching the pointer directly. Defaults to
// true to match the design's safe-default posture.
func (g MemoryConsolidationSafetyGates) PIIRedactionEnabled() bool {
	if g.RequirePIIRedaction == nil {
		return true
	}
	return *g.RequirePIIRedaction
}

// ResolvedTimeouts returns the consolidation FunctionCall +
// PassWallClock timeouts with design defaults applied (5m and 30m
// respectively). Safe to call when the policy is nil or the
// Consolidation / Timeouts blocks are unset.
func (p *MemoryPolicy) ResolvedTimeouts() (functionCall, passWallClock time.Duration) {
	functionCall = 5 * time.Minute
	passWallClock = 30 * time.Minute
	if p == nil || p.Spec.Consolidation == nil || p.Spec.Consolidation.Timeouts == nil {
		return
	}
	t := p.Spec.Consolidation.Timeouts
	if t.FunctionCall != nil && t.FunctionCall.Duration > 0 {
		functionCall = t.FunctionCall.Duration
	}
	if t.PassWallClock != nil && t.PassWallClock.Duration > 0 {
		passWallClock = t.PassWallClock.Duration
	}
	return
}

// ConsolidationDefaultSchedule is the cron used when a policy specifies no
// schedule. Kept in sync with the +kubebuilder:default on
// MemoryConsolidationConfig.Schedule.
const ConsolidationDefaultSchedule = "0 2 * * *"

// ResolvedSchedule returns the cron schedule for the given consolidation
// axis: the per-axis override in Spec.Consolidation.Schedules if set, else
// the policy-level Schedule, else ConsolidationDefaultSchedule. Safe to call
// when the policy or the Consolidation block is nil. axis is one of the
// consolidation.PreFilterAxis string values ("staleObservations",
// "crossScopeCandidates", "entityDuplicateCandidates").
func (p *MemoryPolicy) ResolvedSchedule(axis string) string {
	if p == nil || p.Spec.Consolidation == nil {
		return ConsolidationDefaultSchedule
	}
	if override := p.Spec.Consolidation.scheduleForAxis(axis); override != "" {
		return override
	}
	if p.Spec.Consolidation.Schedule != "" {
		return p.Spec.Consolidation.Schedule
	}
	return ConsolidationDefaultSchedule
}

// scheduleForAxis returns the per-axis schedule override, or "" when unset.
func (c *MemoryConsolidationConfig) scheduleForAxis(axis string) string {
	if c.Schedules == nil {
		return ""
	}
	switch axis {
	case "staleObservations":
		return c.Schedules.StaleObservations
	case "crossScopeCandidates":
		return c.Schedules.CrossScopeCandidates
	case "entityDuplicateCandidates":
		return c.Schedules.EntityDuplicateCandidates
	}
	return ""
}
