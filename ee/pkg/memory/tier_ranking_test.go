/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	ossmemory "github.com/altairalabs/omnia/internal/memory"
)

func TestMultiplicativeTierRanker_AdjustAppliesWeight(t *testing.T) {
	r := MultiplicativeTierRanker{Weights: map[ossmemory.Tier]float64{
		ossmemory.TierInstitutional: 1.5,
		ossmemory.TierAgent:         1.0,
		ossmemory.TierUser:          0.5,
	}}
	assert.InDelta(t, 0.6, r.Adjust(0.4, ossmemory.TierInstitutional), 1e-9)
	assert.InDelta(t, 0.4, r.Adjust(0.4, ossmemory.TierAgent), 1e-9)
	assert.InDelta(t, 0.2, r.Adjust(0.4, ossmemory.TierUser), 1e-9)
}

func TestMultiplicativeTierRanker_UserForAgentInheritsUserWeight(t *testing.T) {
	r := MultiplicativeTierRanker{Weights: map[ossmemory.Tier]float64{
		ossmemory.TierUser: 0.25,
	}}
	assert.InDelta(t, 0.1, r.Adjust(0.4, ossmemory.TierUserForAgent), 1e-9)
}

func TestMultiplicativeTierRanker_MissingTierFallsBackToOne(t *testing.T) {
	r := MultiplicativeTierRanker{Weights: map[ossmemory.Tier]float64{}}
	assert.Equal(t, 0.4, r.Adjust(0.4, ossmemory.TierAgent))
}

func TestNewTierRanker_NilPolicyReturnsIdentity(t *testing.T) {
	got := NewTierRanker(nil)
	_, ok := got.(ossmemory.IdentityTierRanker)
	assert.True(t, ok)
}

func TestNewTierRanker_NoTierPrecedenceReturnsIdentity(t *testing.T) {
	got := NewTierRanker(&omniav1alpha1.MemoryPolicy{})
	_, ok := got.(ossmemory.IdentityTierRanker)
	assert.True(t, ok)
}

func TestNewTierRanker_NilSiblingReturnsIdentity(t *testing.T) {
	policy := &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			TierPrecedence: &omniav1alpha1.TierPrecedenceConfig{},
		},
	}
	got := NewTierRanker(policy)
	_, ok := got.(ossmemory.IdentityTierRanker)
	assert.True(t, ok)
}

func TestNewTierRanker_MultiplicativePopulatedReturnsMultiplicative(t *testing.T) {
	policy := &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			TierPrecedence: &omniav1alpha1.TierPrecedenceConfig{
				Multiplicative: &omniav1alpha1.MultiplicativeTierPrecedence{
					Institutional: "1.5",
					Agent:         "1.0",
					User:          "0.8",
				},
			},
		},
	}
	got := NewTierRanker(policy)
	mr, ok := got.(MultiplicativeTierRanker)
	assert.True(t, ok)
	assert.InDelta(t, 1.5, mr.Weights[ossmemory.TierInstitutional], 1e-9)
	assert.InDelta(t, 1.0, mr.Weights[ossmemory.TierAgent], 1e-9)
	assert.InDelta(t, 0.8, mr.Weights[ossmemory.TierUser], 1e-9)
}

func TestNewTierRanker_MultiplicativeWithBadParseFallsBackToIdentity(t *testing.T) {
	policy := &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			TierPrecedence: &omniav1alpha1.TierPrecedenceConfig{
				Multiplicative: &omniav1alpha1.MultiplicativeTierPrecedence{
					Institutional: "not a number",
				},
			},
		},
	}
	got := NewTierRanker(policy)
	_, ok := got.(ossmemory.IdentityTierRanker)
	assert.True(t, ok)
}

func TestNewTierRanker_MultiplicativeEmptyStringDefaultsToOne(t *testing.T) {
	policy := &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			TierPrecedence: &omniav1alpha1.TierPrecedenceConfig{
				Multiplicative: &omniav1alpha1.MultiplicativeTierPrecedence{},
			},
		},
	}
	got := NewTierRanker(policy)
	mr, ok := got.(MultiplicativeTierRanker)
	assert.True(t, ok)
	assert.Equal(t, 1.0, mr.Weights[ossmemory.TierInstitutional])
	assert.Equal(t, 1.0, mr.Weights[ossmemory.TierAgent])
	assert.Equal(t, 1.0, mr.Weights[ossmemory.TierUser])
}

// defaultRecallHalfLifeEE mirrors the OSS constant (30 days) for test assertions.
// The EE package delegates to ossmemory.TierHalfLife.OrDefaults() which uses the
// same value — this local constant keeps tests readable without importing the
// unexported OSS constant.
const defaultRecallHalfLifeEE = 30 * 24 * time.Hour

func TestNewTierHalfLife_DefaultsWhenUnset(t *testing.T) {
	for _, p := range []*omniav1alpha1.MemoryPolicy{
		nil,
		{Spec: omniav1alpha1.MemoryPolicySpec{}},
		{Spec: omniav1alpha1.MemoryPolicySpec{Recall: &omniav1alpha1.MemoryRecallConfig{}}},
	} {
		hl := NewTierHalfLife(p)
		assert.Equal(t, defaultRecallHalfLifeEE, hl.User)
		assert.Equal(t, defaultRecallHalfLifeEE, hl.Agent)
		assert.Equal(t, defaultRecallHalfLifeEE, hl.Institutional)
	}
}

func TestNewTierHalfLife_ParsesPerTierAndDaySuffix(t *testing.T) {
	hl := NewTierHalfLife(&omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			Recall: &omniav1alpha1.MemoryRecallConfig{
				HalfLife: &omniav1alpha1.MemoryRecallHalfLife{
					User:          "7d",
					Agent:         "720h",
					Institutional: "365d",
				},
			},
		},
	})
	assert.Equal(t, 7*24*time.Hour, hl.User)
	assert.Equal(t, 720*time.Hour, hl.Agent)
	assert.Equal(t, 365*24*time.Hour, hl.Institutional)
}

func TestNewTierHalfLife_InvalidOrEmptyFallsBackPerTier(t *testing.T) {
	hl := NewTierHalfLife(&omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			Recall: &omniav1alpha1.MemoryRecallConfig{
				HalfLife: &omniav1alpha1.MemoryRecallHalfLife{
					User:          "garbage",
					Agent:         "", // unset
					Institutional: "90d",
				},
			},
		},
	})
	assert.Equal(t, defaultRecallHalfLifeEE, hl.User, "unparseable falls back to default")
	assert.Equal(t, defaultRecallHalfLifeEE, hl.Agent, "empty falls back to default")
	assert.Equal(t, 90*24*time.Hour, hl.Institutional)
}
