/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestIdentityTierRanker_AdjustReturnsBaseUnchanged(t *testing.T) {
	r := IdentityTierRanker{}
	assert.Equal(t, 0.42, r.Adjust(0.42, TierInstitutional))
	assert.Equal(t, 0.42, r.Adjust(0.42, TierUserForAgent))
}

func TestMultiplicativeTierRanker_AdjustAppliesWeight(t *testing.T) {
	r := MultiplicativeTierRanker{Weights: map[Tier]float64{
		TierInstitutional: 1.5,
		TierAgent:         1.0,
		TierUser:          0.5,
	}}
	assert.InDelta(t, 0.6, r.Adjust(0.4, TierInstitutional), 1e-9)
	assert.InDelta(t, 0.4, r.Adjust(0.4, TierAgent), 1e-9)
	assert.InDelta(t, 0.2, r.Adjust(0.4, TierUser), 1e-9)
}

func TestMultiplicativeTierRanker_UserForAgentInheritsUserWeight(t *testing.T) {
	r := MultiplicativeTierRanker{Weights: map[Tier]float64{
		TierUser: 0.25,
	}}
	assert.InDelta(t, 0.1, r.Adjust(0.4, TierUserForAgent), 1e-9)
}

func TestMultiplicativeTierRanker_MissingTierFallsBackToOne(t *testing.T) {
	r := MultiplicativeTierRanker{Weights: map[Tier]float64{}}
	assert.Equal(t, 0.4, r.Adjust(0.4, TierAgent))
}

func TestNewTierRanker_NilPolicyReturnsIdentity(t *testing.T) {
	got := NewTierRanker(nil)
	_, ok := got.(IdentityTierRanker)
	assert.True(t, ok)
}

func TestNewTierRanker_NoTierPrecedenceReturnsIdentity(t *testing.T) {
	got := NewTierRanker(&omniav1alpha1.MemoryPolicy{})
	_, ok := got.(IdentityTierRanker)
	assert.True(t, ok)
}

func TestNewTierRanker_NilSiblingReturnsIdentity(t *testing.T) {
	policy := &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			TierPrecedence: &omniav1alpha1.TierPrecedenceConfig{},
		},
	}
	got := NewTierRanker(policy)
	_, ok := got.(IdentityTierRanker)
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
	assert.InDelta(t, 1.5, mr.Weights[TierInstitutional], 1e-9)
	assert.InDelta(t, 1.0, mr.Weights[TierAgent], 1e-9)
	assert.InDelta(t, 0.8, mr.Weights[TierUser], 1e-9)
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
	_, ok := got.(IdentityTierRanker)
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
	assert.Equal(t, 1.0, mr.Weights[TierInstitutional])
	assert.Equal(t, 1.0, mr.Weights[TierAgent])
	assert.Equal(t, 1.0, mr.Weights[TierUser])
}
