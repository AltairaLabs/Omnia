/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadProfile_NoRamp(t *testing.T) {
	lp := NewLoadProfile(10, 0, 0)
	lp.Start()

	// Should always return target concurrency regardless of elapsed or pending.
	assert.Equal(t, 10, lp.AllowedConcurrency(0, 100))
	assert.Equal(t, 10, lp.AllowedConcurrency(5*time.Minute, 100))
	assert.Equal(t, 10, lp.AllowedConcurrency(5*time.Minute, 1))
}

func TestLoadProfile_RampUp(t *testing.T) {
	lp := NewLoadProfile(10, 2*time.Minute, 0)
	lp.Start()

	// Large pending so ramp-down doesn't trigger.
	largePending := 1000

	// At 0% elapsed: ceil(0 * 10) = 0, but rampUpAllowed uses ceil
	// Actually at 0 elapsed, ratio=0, ceil(0)=0 — but the function should return
	// at least ceil(0)=0. For 0 elapsed exactly, we get 0.
	// Let's test a tiny elapsed.
	assert.Equal(t, 0, lp.AllowedConcurrency(0, largePending))

	// At 25%: 30s of 2m = 0.25 * 10 = 2.5 -> ceil = 3
	assert.Equal(t, 3, lp.AllowedConcurrency(30*time.Second, largePending))

	// At 50%: 60s of 2m = 0.5 * 10 = 5 -> ceil = 5
	assert.Equal(t, 5, lp.AllowedConcurrency(60*time.Second, largePending))

	// At 100%: 120s of 2m = 1.0 * 10 = 10 -> ceil = 10
	// But elapsed >= rampUpDuration, so we skip ramp-up and return target.
	assert.Equal(t, 10, lp.AllowedConcurrency(2*time.Minute, largePending))

	// At 75%: 90s of 2m = 0.75 * 10 = 7.5 -> ceil = 8
	assert.Equal(t, 8, lp.AllowedConcurrency(90*time.Second, largePending))

	// Past ramp-up: should return target.
	assert.Equal(t, 10, lp.AllowedConcurrency(5*time.Minute, largePending))
}

func TestLoadProfile_RampDown(t *testing.T) {
	lp := NewLoadProfile(10, 0, 30*time.Second)
	lp.Start()

	// threshold = 10 * 2 = 20
	// Above threshold: return target.
	assert.Equal(t, 10, lp.AllowedConcurrency(5*time.Minute, 20))
	assert.Equal(t, 10, lp.AllowedConcurrency(5*time.Minute, 100))

	// At pending = 15: ratio = 15/20 = 0.75, ceil(0.75 * 10) = ceil(7.5) = 8
	assert.Equal(t, 8, lp.AllowedConcurrency(5*time.Minute, 15))

	// At pending = 10: ratio = 10/20 = 0.5, ceil(0.5 * 10) = ceil(5) = 5
	assert.Equal(t, 5, lp.AllowedConcurrency(5*time.Minute, 10))

	// At pending = 5: ratio = 5/20 = 0.25, ceil(0.25 * 10) = ceil(2.5) = 3
	assert.Equal(t, 3, lp.AllowedConcurrency(5*time.Minute, 5))

	// At pending = 1: ratio = 1/20 = 0.05, ceil(0.05 * 10) = ceil(0.5) = 1
	assert.Equal(t, 1, lp.AllowedConcurrency(5*time.Minute, 1))

	// At pending = 0: ratio = 0, ceil(0) = 0 -> max(1, 0) = 1
	assert.Equal(t, 1, lp.AllowedConcurrency(5*time.Minute, 0))
}

func TestLoadProfile_RampUpAndDown(t *testing.T) {
	lp := NewLoadProfile(10, 2*time.Minute, 30*time.Second)
	lp.Start()

	// During ramp-up with plenty of pending items.
	assert.Equal(t, 3, lp.AllowedConcurrency(30*time.Second, 100))

	// Steady state: past ramp-up, plenty of pending items.
	assert.Equal(t, 10, lp.AllowedConcurrency(5*time.Minute, 100))

	// During ramp-up AND low pending: should take the minimum.
	// Ramp-up at 30s: ceil(0.25 * 10) = 3
	// Ramp-down at pending=5: ceil(0.25 * 10) = 3
	assert.Equal(t, 3, lp.AllowedConcurrency(30*time.Second, 5))

	// During ramp-up at 90s: ceil(0.75 * 10) = 8
	// Ramp-down at pending=10: ceil(0.5 * 10) = 5
	// min(8, 5) = 5
	assert.Equal(t, 5, lp.AllowedConcurrency(90*time.Second, 10))
}

func TestLoadProfile_SteadyState(t *testing.T) {
	lp := NewLoadProfile(10, 1*time.Minute, 30*time.Second)
	lp.Start()

	// Between ramp-up end and ramp-down trigger: target concurrency.
	// elapsed > rampUpDuration, pending >= threshold (20)
	assert.Equal(t, 10, lp.AllowedConcurrency(2*time.Minute, 50))
	assert.Equal(t, 10, lp.AllowedConcurrency(10*time.Minute, 20))
}

func TestLoadProfile_ZeroConcurrency(t *testing.T) {
	lp := NewLoadProfile(0, 1*time.Minute, 30*time.Second)
	lp.Start()

	assert.Equal(t, 0, lp.AllowedConcurrency(30*time.Second, 100))
	assert.Equal(t, 0, lp.AllowedConcurrency(5*time.Minute, 0))
}

func TestLoadProfile_ZeroRampDurations(t *testing.T) {
	// Zero ramp durations should behave like no ramp.
	lp := NewLoadProfile(5, 0, 0)
	lp.Start()

	assert.Equal(t, 5, lp.AllowedConcurrency(0, 0))
	assert.Equal(t, 5, lp.AllowedConcurrency(time.Hour, 1000))
}

func TestLoadProfile_LargeValues(t *testing.T) {
	lp := NewLoadProfile(1000, 10*time.Minute, 1*time.Minute)
	lp.Start()

	// 50% ramp-up: ceil(0.5 * 1000) = 500
	assert.Equal(t, 500, lp.AllowedConcurrency(5*time.Minute, 10000))

	// Steady state.
	assert.Equal(t, 1000, lp.AllowedConcurrency(15*time.Minute, 10000))

	// Ramp-down at pending = 1000: threshold = 2000, ratio = 0.5, ceil(500) = 500
	assert.Equal(t, 500, lp.AllowedConcurrency(15*time.Minute, 1000))
}

func TestLoadProfile_ConcurrencyOfOne(t *testing.T) {
	lp := NewLoadProfile(1, 1*time.Minute, 30*time.Second)
	lp.Start()

	// Ramp-up at 50%: ceil(0.5 * 1) = ceil(0.5) = 1
	assert.Equal(t, 1, lp.AllowedConcurrency(30*time.Second, 100))

	// Ramp-down at pending 1: threshold=2, ratio=0.5, ceil(0.5*1)=1
	assert.Equal(t, 1, lp.AllowedConcurrency(5*time.Minute, 1))

	// Ramp-down at pending 0: max(1, 0) = 1
	assert.Equal(t, 1, lp.AllowedConcurrency(5*time.Minute, 0))
}

func TestLoadProfile_Start(t *testing.T) {
	lp := NewLoadProfile(10, 1*time.Minute, 0)
	assert.True(t, lp.startTime.IsZero())
	lp.Start()
	assert.False(t, lp.startTime.IsZero())
}

func TestRampUpAllowed(t *testing.T) {
	tests := []struct {
		name     string
		elapsed  time.Duration
		rampUp   time.Duration
		target   int
		expected int
	}{
		{"zero elapsed", 0, 2 * time.Minute, 10, 0},
		{"quarter", 30 * time.Second, 2 * time.Minute, 10, 3},
		{"half", 1 * time.Minute, 2 * time.Minute, 10, 5},
		{"full", 2 * time.Minute, 2 * time.Minute, 10, 10},
		{"over 100 percent", 5 * time.Minute, 2 * time.Minute, 10, 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, rampUpAllowed(tc.elapsed, tc.rampUp, tc.target))
		})
	}
}

func TestRampDownByPending(t *testing.T) {
	tests := []struct {
		name     string
		pending  int
		target   int
		expected int
	}{
		{"above threshold", 30, 10, 10},
		{"at threshold", 20, 10, 10},
		{"three quarters", 15, 10, 8},
		{"half", 10, 10, 5},
		{"quarter", 5, 10, 3},
		{"one item", 1, 10, 1},
		{"zero pending", 0, 10, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, rampDownByPending(tc.pending, tc.target))
		})
	}
}
