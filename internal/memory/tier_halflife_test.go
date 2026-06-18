/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
