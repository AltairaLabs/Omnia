/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestNewSampler_NilConfig(t *testing.T) {
	s := NewSampler(nil)
	require.NotNil(t, s)
	assert.Equal(t, int32(DefaultSamplingRate), s.DefaultRate())
	assert.Equal(t, int32(DefaultLLMJudgeRate), s.LLMJudgeRate())
}

func TestNewSampler_WithConfig(t *testing.T) {
	dr := int32(50)
	jr := int32(20)
	s := NewSampler(&v1alpha1.EvalSampling{
		DefaultRate:  &dr,
		LLMJudgeRate: &jr,
	})
	assert.Equal(t, int32(50), s.DefaultRate())
	assert.Equal(t, int32(20), s.LLMJudgeRate())
}

func TestNewSampler_PartialConfig(t *testing.T) {
	dr := int32(75)
	s := NewSampler(&v1alpha1.EvalSampling{
		DefaultRate: &dr,
	})
	assert.Equal(t, int32(75), s.DefaultRate())
	assert.Equal(t, int32(DefaultLLMJudgeRate), s.LLMJudgeRate())
}

func TestNewSampler_EmptyConfig(t *testing.T) {
	s := NewSampler(&v1alpha1.EvalSampling{})
	assert.Equal(t, int32(DefaultSamplingRate), s.DefaultRate())
	assert.Equal(t, int32(DefaultLLMJudgeRate), s.LLMJudgeRate())
}

func TestShouldSample_Rate100_AlwaysTrue(t *testing.T) {
	s := NewSampler(nil) // defaultRate = 100
	for i := 0; i < 100; i++ {
		assert.True(t, s.ShouldSample("session-abc", i, false))
	}
}

func TestShouldSample_Rate0_AlwaysFalse(t *testing.T) {
	dr := int32(0)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr})
	for i := 0; i < 100; i++ {
		assert.False(t, s.ShouldSample("session-xyz", i, false))
	}
}

func TestShouldSample_LLMJudgeRate0_AlwaysFalse(t *testing.T) {
	jr := int32(0)
	s := NewSampler(&v1alpha1.EvalSampling{LLMJudgeRate: &jr})
	for i := 0; i < 100; i++ {
		assert.False(t, s.ShouldSample("session-xyz", i, true))
	}
}

func TestShouldSample_Deterministic(t *testing.T) {
	dr := int32(50)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr})

	// Same inputs should always produce the same result.
	first := s.ShouldSample("session-123", 5, false)
	for i := 0; i < 50; i++ {
		assert.Equal(t, first, s.ShouldSample("session-123", 5, false),
			"sampling must be deterministic for the same inputs")
	}
}

func TestShouldSample_UsesLLMJudgeRate(t *testing.T) {
	dr := int32(100)
	jr := int32(0)
	s := NewSampler(&v1alpha1.EvalSampling{
		DefaultRate:  &dr,
		LLMJudgeRate: &jr,
	})

	// Non-judge evals should pass (rate 100).
	assert.True(t, s.ShouldSample("s1", 0, false))
	// Judge evals should fail (rate 0).
	assert.False(t, s.ShouldSample("s1", 0, true))
}

func TestShouldSample_Distribution(t *testing.T) {
	// With a 50% rate over many samples, we should see roughly 50% sampled.
	dr := int32(50)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr})

	sampled := 0
	total := 10000
	for i := 0; i < total; i++ {
		if s.ShouldSample("distribution-test", i, false) {
			sampled++
		}
	}

	ratio := float64(sampled) / float64(total)
	assert.InDelta(t, 0.50, ratio, 0.05,
		"expected ~50%% sampling, got %.2f%%", ratio*100)
}

func TestShouldSample_NegativeRate(t *testing.T) {
	dr := int32(-1)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr})
	assert.False(t, s.ShouldSample("s1", 0, false))
}

func TestShouldSample_DifferentSessionsDifferentResults(t *testing.T) {
	// With a rate that is not 0 or 100, different session IDs should
	// produce a mix of true and false.
	dr := int32(50)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr})

	results := make(map[bool]int)
	for i := 0; i < 100; i++ {
		result := s.ShouldSample("unique-session-"+string(rune(i+'A')), 0, false)
		results[result]++
	}

	// With 50% rate, we should have both true and false results.
	assert.Greater(t, results[true], 0, "expected some true results")
	assert.Greater(t, results[false], 0, "expected some false results")
}

func TestHashShouldSample_BoundaryRates(t *testing.T) {
	tests := []struct {
		name     string
		rate     int32
		allTrue  bool
		allFalse bool
	}{
		{name: "rate 0", rate: 0, allFalse: true},
		{name: "rate 100", rate: 100, allTrue: true},
		{name: "rate -5", rate: -5, allFalse: true},
		{name: "rate 150", rate: 150, allTrue: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 20; i++ {
				result := hashShouldSample("test", i, tc.rate)
				if tc.allTrue {
					assert.True(t, result)
				}
				if tc.allFalse {
					assert.False(t, result)
				}
			}
		})
	}
}
