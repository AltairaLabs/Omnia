/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestNewTierHalfLife_DefaultsWhenUnset(t *testing.T) {
	for _, p := range []*omniav1alpha1.MemoryPolicy{
		nil,
		{Spec: omniav1alpha1.MemoryPolicySpec{}},
		{Spec: omniav1alpha1.MemoryPolicySpec{Recall: &omniav1alpha1.MemoryRecallConfig{}}},
	} {
		hl := NewTierHalfLife(p)
		assert.Equal(t, defaultRecallHalfLife, hl.User)
		assert.Equal(t, defaultRecallHalfLife, hl.Agent)
		assert.Equal(t, defaultRecallHalfLife, hl.Institutional)
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
	assert.Equal(t, defaultRecallHalfLife, hl.User, "unparseable falls back to default")
	assert.Equal(t, defaultRecallHalfLife, hl.Agent, "empty falls back to default")
	assert.Equal(t, 90*24*time.Hour, hl.Institutional)
}

func TestRecencyDecay(t *testing.T) {
	// age == halfLife → exactly 0.5 (the documented semantics).
	assert.InDelta(t, 0.5, recencyDecay(100, 100), 1e-9)
	// Fresh (age 0) → no decay.
	assert.InDelta(t, 1.0, recencyDecay(0, 100), 1e-9)
	// Non-positive half-life disables decay rather than dividing by zero.
	assert.Equal(t, 1.0, recencyDecay(9999, 0))
	assert.Equal(t, 1.0, recencyDecay(9999, -5))
}

func TestTierHalfLife_SecondsFor_UserForAgentInheritsUser(t *testing.T) {
	hl := TierHalfLife{
		User:          10 * time.Hour,
		Agent:         20 * time.Hour,
		Institutional: 30 * time.Hour,
	}
	assert.Equal(t, (10 * time.Hour).Seconds(), hl.secondsFor(TierUser))
	assert.Equal(t, (10 * time.Hour).Seconds(), hl.secondsFor(TierUserForAgent))
	assert.Equal(t, (20 * time.Hour).Seconds(), hl.secondsFor(TierAgent))
	assert.Equal(t, (30 * time.Hour).Seconds(), hl.secondsFor(TierInstitutional))
}
