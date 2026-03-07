/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"fmt"
	"hash/fnv"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Eval tier names. Tiers group evals by cost profile so that sampling
// rates can be applied independently per tier.
const (
	TierLightweight = "lightweight"
	TierExtended    = "extended"
)

// Default sampling rates (percentages 0-100).
const (
	DefaultSamplingRate = 100
	DefaultExtendedRate = 10
	samplingModulus     = 100
)

// Sampler provides deterministic hash-based session-level sampling for eval execution.
// It uses FNV hashing on sessionID to ensure consistent sampling decisions
// across restarts and replicas. Sampling is per-session: if a session is
// sampled in for a tier, all turns in that session run evals of that tier.
type Sampler struct {
	defaultRate  int32
	extendedRate int32
}

// NewSampler creates a Sampler from CRD config, applying defaults for nil values.
func NewSampler(config *v1alpha1.EvalSampling) *Sampler {
	s := &Sampler{
		defaultRate:  DefaultSamplingRate,
		extendedRate: DefaultExtendedRate,
	}
	if config == nil {
		return s
	}
	if config.DefaultRate != nil {
		s.defaultRate = *config.DefaultRate
	}
	if config.ExtendedRate != nil {
		s.extendedRate = *config.ExtendedRate
	}
	return s
}

// EvalTiersForSession returns the eval tiers that should execute for the
// given session based on deterministic hash sampling. Each tier is sampled
// independently using a different hash seed.
func (s *Sampler) EvalTiersForSession(sessionID string) []string {
	var tiers []string
	if sessionShouldSample(sessionID, TierLightweight, s.defaultRate) {
		tiers = append(tiers, TierLightweight)
	}
	if sessionShouldSample(sessionID, TierExtended, s.extendedRate) {
		tiers = append(tiers, TierExtended)
	}
	return tiers
}

// DefaultRate returns the configured default sampling rate.
func (s *Sampler) DefaultRate() int32 {
	return s.defaultRate
}

// ExtendedRate returns the configured extended eval sampling rate.
func (s *Sampler) ExtendedRate() int32 {
	return s.extendedRate
}

// sessionShouldSample performs a deterministic hash check for a session and tier.
// The tier name is included in the hash to ensure independence between tiers.
func sessionShouldSample(sessionID, tier string, rate int32) bool {
	if rate <= 0 {
		return false
	}
	if rate >= samplingModulus {
		return true
	}
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s:%s", sessionID, tier)
	return int32(h.Sum32()%samplingModulus) < rate
}

// IsExtendedEvalType returns true if the eval type requires an external
// service call (e.g. LLM judge). These are classified as "extended" tier.
func IsExtendedEvalType(evalType string) bool {
	switch evalType {
	case "llm_judge", "llm_judge_turn", "llm_judge_session":
		return true
	default:
		return false
	}
}

// EvalTierForType returns the tier name for a given eval type.
func EvalTierForType(evalType string) string {
	if IsExtendedEvalType(evalType) {
		return TierExtended
	}
	return TierLightweight
}

// FilterEvalsByTiers returns only evals whose tier is in the allowed set.
func FilterEvalsByTiers(evals []EvalDef, tiers []string) []EvalDef {
	if len(tiers) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(tiers))
	for _, t := range tiers {
		allowed[t] = true
	}
	filtered := make([]EvalDef, 0, len(evals))
	for _, e := range evals {
		if allowed[EvalTierForType(e.Type)] {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
