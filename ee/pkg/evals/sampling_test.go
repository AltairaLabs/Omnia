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
	assert.Equal(t, int32(DefaultExtendedRate), s.ExtendedRate())
}

func TestNewSampler_WithConfig(t *testing.T) {
	dr := int32(50)
	jr := int32(20)
	s := NewSampler(&v1alpha1.EvalSampling{
		DefaultRate:  &dr,
		ExtendedRate: &jr,
	})
	assert.Equal(t, int32(50), s.DefaultRate())
	assert.Equal(t, int32(20), s.ExtendedRate())
}

func TestNewSampler_PartialConfig(t *testing.T) {
	dr := int32(75)
	s := NewSampler(&v1alpha1.EvalSampling{
		DefaultRate: &dr,
	})
	assert.Equal(t, int32(75), s.DefaultRate())
	assert.Equal(t, int32(DefaultExtendedRate), s.ExtendedRate())
}

func TestNewSampler_EmptyConfig(t *testing.T) {
	s := NewSampler(&v1alpha1.EvalSampling{})
	assert.Equal(t, int32(DefaultSamplingRate), s.DefaultRate())
	assert.Equal(t, int32(DefaultExtendedRate), s.ExtendedRate())
}

func TestEvalTiersForSession_DefaultConfig_IncludesLightweight(t *testing.T) {
	s := NewSampler(nil) // defaultRate=100, extendedRate=10
	tiers := s.EvalTiersForSession("session-abc")
	assert.Contains(t, tiers, TierLightweight, "defaultRate=100 should always include lightweight")
}

func TestEvalTiersForSession_Rate0_ExcludesTier(t *testing.T) {
	dr := int32(0)
	er := int32(0)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr, ExtendedRate: &er})
	tiers := s.EvalTiersForSession("session-xyz")
	assert.Empty(t, tiers)
}

func TestEvalTiersForSession_Rate100_IncludesBoth(t *testing.T) {
	dr := int32(100)
	er := int32(100)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr, ExtendedRate: &er})
	tiers := s.EvalTiersForSession("session-123")
	assert.Contains(t, tiers, TierLightweight)
	assert.Contains(t, tiers, TierExtended)
}

func TestEvalTiersForSession_Deterministic(t *testing.T) {
	dr := int32(50)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr})

	first := s.EvalTiersForSession("session-123")
	for i := 0; i < 50; i++ {
		assert.Equal(t, first, s.EvalTiersForSession("session-123"),
			"sampling must be deterministic for the same session")
	}
}

func TestEvalTiersForSession_Distribution(t *testing.T) {
	dr := int32(50)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr})

	sampled := 0
	total := 10000
	for i := 0; i < total; i++ {
		tiers := s.EvalTiersForSession("distribution-test-" + string(rune(i)))
		for _, tier := range tiers {
			if tier == TierLightweight {
				sampled++
			}
		}
	}

	ratio := float64(sampled) / float64(total)
	assert.InDelta(t, 0.50, ratio, 0.05,
		"expected ~50%% sampling, got %.2f%%", ratio*100)
}

func TestEvalTiersForSession_DifferentSessionsDifferentResults(t *testing.T) {
	dr := int32(50)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr})

	results := make(map[int]int) // count of tiers per session
	for i := 0; i < 100; i++ {
		tiers := s.EvalTiersForSession("unique-session-" + string(rune(i+'A')))
		results[len(tiers)]++
	}

	assert.Greater(t, results[0]+results[1]+results[2], 0, "expected varied results")
}

func TestEvalTiersForSession_NegativeRate(t *testing.T) {
	dr := int32(-1)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr})
	tiers := s.EvalTiersForSession("s1")
	assert.NotContains(t, tiers, TierLightweight)
}

func TestEvalTiersForSession_TiersAreIndependent(t *testing.T) {
	// With different rates, the sampling decisions should be independent.
	dr := int32(100)
	er := int32(0)
	s := NewSampler(&v1alpha1.EvalSampling{DefaultRate: &dr, ExtendedRate: &er})
	tiers := s.EvalTiersForSession("s1")
	assert.Contains(t, tiers, TierLightweight)
	assert.NotContains(t, tiers, TierExtended)
}

func TestSessionShouldSample_BoundaryRates(t *testing.T) {
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
				result := sessionShouldSample("test", TierLightweight, tc.rate)
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
