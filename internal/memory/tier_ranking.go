/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"strconv"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// TierRanker adjusts a memory's base retrieval score given its tier.
// Implementations must return base unchanged when no policy applies so
// absent configuration behaves as the no-op identity ranker.
type TierRanker interface {
	Adjust(base float64, tier Tier) float64
}

// IdentityTierRanker is the zero-value ranker — base unchanged.
type IdentityTierRanker struct{}

// Adjust returns base unchanged regardless of tier.
func (IdentityTierRanker) Adjust(base float64, _ Tier) float64 { return base }

// MultiplicativeTierRanker scales the base score by the per-tier
// weight. Missing tier weights default to 1.0. TierUserForAgent reuses
// the TierUser weight when not set explicitly.
type MultiplicativeTierRanker struct {
	Weights map[Tier]float64
}

// Adjust multiplies base by the per-tier weight, falling back to the
// user weight for user_for_agent and to base unchanged for any other
// missing tier.
func (m MultiplicativeTierRanker) Adjust(base float64, tier Tier) float64 {
	w, ok := m.Weights[tier]
	if !ok && tier == TierUserForAgent {
		w, ok = m.Weights[TierUser]
	}
	if !ok {
		return base
	}
	return base * w
}

// NewTierRanker dispatches on which sibling field of TierPrecedence is
// populated. Nil policy / nil TierPrecedence / no sibling set returns
// the identity ranker. A forward-dated policy (manifest references a
// ranker sibling this binary doesn't know) naturally falls through.
// Parse errors on individual weights also fall through to identity —
// the controller should have rejected them at admission.
func NewTierRanker(policy *omniav1alpha1.MemoryPolicy) TierRanker {
	if policy == nil || policy.Spec.TierPrecedence == nil {
		return IdentityTierRanker{}
	}
	if m := policy.Spec.TierPrecedence.Multiplicative; m != nil {
		ranker, err := newMultiplicativeRanker(m)
		if err != nil {
			return IdentityTierRanker{}
		}
		return ranker
	}
	return IdentityTierRanker{}
}

// newMultiplicativeRanker parses the per-tier weight strings, defaulting
// missing values to 1.0. Returns an error if any weight fails to parse.
func newMultiplicativeRanker(cfg *omniav1alpha1.MultiplicativeTierPrecedence) (MultiplicativeTierRanker, error) {
	weights := map[Tier]float64{
		TierInstitutional: 1.0,
		TierAgent:         1.0,
		TierUser:          1.0,
	}
	for tier, raw := range map[Tier]string{
		TierInstitutional: cfg.Institutional,
		TierAgent:         cfg.Agent,
		TierUser:          cfg.User,
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
