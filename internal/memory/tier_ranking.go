/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import "time"

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
