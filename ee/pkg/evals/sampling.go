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

// Default sampling rates (percentages 0-100).
const (
	DefaultSamplingRate = 100
	DefaultLLMJudgeRate = 10
	samplingModulus     = 100
)

// Sampler provides deterministic hash-based sampling for eval execution.
// It uses FNV hashing on sessionID + turnIndex to ensure consistent
// sampling decisions across restarts and replicas.
type Sampler struct {
	defaultRate  int32
	llmJudgeRate int32
}

// NewSampler creates a Sampler from CRD config, applying defaults for nil values.
func NewSampler(config *v1alpha1.EvalSampling) *Sampler {
	s := &Sampler{
		defaultRate:  DefaultSamplingRate,
		llmJudgeRate: DefaultLLMJudgeRate,
	}
	if config == nil {
		return s
	}
	if config.DefaultRate != nil {
		s.defaultRate = *config.DefaultRate
	}
	if config.LLMJudgeRate != nil {
		s.llmJudgeRate = *config.LLMJudgeRate
	}
	return s
}

// ShouldSample returns true if the given session turn should be sampled.
// The decision is deterministic: the same (sessionID, turnIndex, isLLMJudge)
// tuple always produces the same result.
func (s *Sampler) ShouldSample(sessionID string, turnIndex int, isLLMJudge bool) bool {
	rate := s.defaultRate
	if isLLMJudge {
		rate = s.llmJudgeRate
	}
	return hashShouldSample(sessionID, turnIndex, rate)
}

// hashShouldSample performs the deterministic hash check against the rate.
func hashShouldSample(sessionID string, turnIndex int, rate int32) bool {
	if rate <= 0 {
		return false
	}
	if rate >= samplingModulus {
		return true
	}
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s:%d", sessionID, turnIndex)
	return int32(h.Sum32()%samplingModulus) < rate
}

// DefaultRate returns the configured default sampling rate.
func (s *Sampler) DefaultRate() int32 {
	return s.defaultRate
}

// LLMJudgeRate returns the configured LLM judge sampling rate.
func (s *Sampler) LLMJudgeRate() int32 {
	return s.llmJudgeRate
}
