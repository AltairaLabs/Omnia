/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package memory

import (
	"strconv"
	"time"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	ossmemory "github.com/altairalabs/omnia/internal/memory"
)

// MultiplicativeTierRanker scales the base score by the per-tier weight.
// Implements ossmemory.TierRanker. Missing tier weights default to 1.0;
// TierUserForAgent reuses the TierUser weight when not set explicitly.
type MultiplicativeTierRanker struct {
	Weights map[ossmemory.Tier]float64
}

// Adjust multiplies base by the per-tier weight, falling back to the user
// weight for user_for_agent and to base unchanged for any other missing tier.
func (m MultiplicativeTierRanker) Adjust(base float64, tier ossmemory.Tier) float64 {
	w, ok := m.Weights[tier]
	if !ok && tier == ossmemory.TierUserForAgent {
		w, ok = m.Weights[ossmemory.TierUser]
	}
	if !ok {
		return base
	}
	return base * w
}

// NewTierRanker dispatches on MemoryPolicy.spec.tierPrecedence. Nil policy /
// nil TierPrecedence / no sibling set returns the identity ranker. Parse errors
// on weights fall through to identity (the controller rejects them at admission).
func NewTierRanker(policy *omniav1alpha1.MemoryPolicy) ossmemory.TierRanker {
	if policy == nil || policy.Spec.TierPrecedence == nil {
		return ossmemory.IdentityTierRanker{}
	}
	if m := policy.Spec.TierPrecedence.Multiplicative; m != nil {
		ranker, err := newMultiplicativeRanker(m)
		if err != nil {
			return ossmemory.IdentityTierRanker{}
		}
		return ranker
	}
	return ossmemory.IdentityTierRanker{}
}

// newMultiplicativeRanker parses the per-tier weight strings, defaulting missing
// values to 1.0. Returns an error if any weight fails to parse.
func newMultiplicativeRanker(cfg *omniav1alpha1.MultiplicativeTierPrecedence) (MultiplicativeTierRanker, error) {
	weights := map[ossmemory.Tier]float64{
		ossmemory.TierInstitutional: 1.0,
		ossmemory.TierAgent:         1.0,
		ossmemory.TierUser:          1.0,
	}
	for tier, raw := range map[ossmemory.Tier]string{
		ossmemory.TierInstitutional: cfg.Institutional,
		ossmemory.TierAgent:         cfg.Agent,
		ossmemory.TierUser:          cfg.User,
	} {
		if raw == "" {
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return MultiplicativeTierRanker{}, err
		}
		weights[tier] = v
	}
	return MultiplicativeTierRanker{Weights: weights}, nil
}

// NewTierHalfLife resolves per-tier recency half-lives from a MemoryPolicy,
// defaulting unset/invalid tiers via the OSS uniform default (OrDefaults).
func NewTierHalfLife(policy *omniav1alpha1.MemoryPolicy) ossmemory.TierHalfLife {
	var hl ossmemory.TierHalfLife
	if policy != nil && policy.Spec.Recall != nil && policy.Spec.Recall.HalfLife != nil {
		h := policy.Spec.Recall.HalfLife
		hl.User = parseHalfLifeOrZero(h.User)
		hl.Agent = parseHalfLifeOrZero(h.Agent)
		hl.Institutional = parseHalfLifeOrZero(h.Institutional)
	}
	return hl.OrDefaults()
}

// parseHalfLifeOrZero parses a CRD half-life string, returning 0 for empty /
// invalid / non-positive so OrDefaults substitutes the baseline.
func parseHalfLifeOrZero(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := ossmemory.ParseRetentionDuration(s)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}
