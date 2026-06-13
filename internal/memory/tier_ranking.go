/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"strconv"
	"time"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ln2 converts a half-life into the exponential-decay rate: a recency
// multiplier of exp(-ln2 * age / halfLife) equals 0.5 exactly when age ==
// halfLife, matching the MemoryPolicy.recall.halfLife documented semantics.
const ln2 = 0.6931471805599453

// defaultRecallHalfLife is the per-tier recency half-life applied when the
// MemoryPolicy doesn't set one. 30 days matches the historical baked-in
// recall-SQL default.
const defaultRecallHalfLife = 30 * 24 * time.Hour

// TierHalfLife carries the per-tier recency half-life durations used by the
// recall recency-decay multiplier. The zero value is "unset" — call
// orDefaults (or go through a store method that does) before use.
type TierHalfLife struct {
	User          time.Duration
	Agent         time.Duration
	Institutional time.Duration
}

// NewTierHalfLife resolves the per-tier half-life from a MemoryPolicy,
// substituting defaultRecallHalfLife for any tier the policy leaves unset or
// that fails to parse. Nil policy / nil Recall / nil HalfLife yields all
// defaults. Mirrors NewTierRanker so the service can derive both from one
// policy load.
func NewTierHalfLife(policy *omniav1alpha1.MemoryPolicy) TierHalfLife {
	var hl TierHalfLife
	if policy != nil && policy.Spec.Recall != nil && policy.Spec.Recall.HalfLife != nil {
		h := policy.Spec.Recall.HalfLife
		hl.User = parseHalfLifeOrZero(h.User)
		hl.Agent = parseHalfLifeOrZero(h.Agent)
		hl.Institutional = parseHalfLifeOrZero(h.Institutional)
	}
	return hl.orDefaults()
}

// parseHalfLifeOrZero parses a CRD half-life string ("30d", "720h", ...),
// returning 0 for empty / invalid / non-positive values so orDefaults can
// substitute the baseline. Reuses the same "d"-suffix parser as retention.
func parseHalfLifeOrZero(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := parseRetentionDuration(s)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// orDefaults substitutes defaultRecallHalfLife for any tier left at zero, so
// direct store callers (no policy) still get a sane decay curve.
func (h TierHalfLife) orDefaults() TierHalfLife {
	if h.User <= 0 {
		h.User = defaultRecallHalfLife
	}
	if h.Agent <= 0 {
		h.Agent = defaultRecallHalfLife
	}
	if h.Institutional <= 0 {
		h.Institutional = defaultRecallHalfLife
	}
	return h
}

// secondsFor returns the half-life in seconds for a tier. TierUserForAgent
// inherits the user half-life, matching MultiplicativeTierRanker's weight
// inheritance.
func (h TierHalfLife) secondsFor(t Tier) float64 {
	switch t {
	case TierUser, TierUserForAgent:
		return h.User.Seconds()
	case TierAgent:
		return h.Agent.Seconds()
	default:
		return h.Institutional.Seconds()
	}
}

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
