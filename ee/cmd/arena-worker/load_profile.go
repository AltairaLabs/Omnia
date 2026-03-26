/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"math"
	"time"
)

// LoadProfile controls how allowed concurrency changes over time.
// During ramp-up, concurrency linearly increases from 0 to targetConcurrency.
// During steady state, it returns targetConcurrency.
// During ramp-down (triggered by low pending count), it linearly decreases.
type LoadProfile struct {
	targetConcurrency int
	rampUpDuration    time.Duration
	rampDownDuration  time.Duration
	startTime         time.Time
}

// NewLoadProfile creates a load profile from config.
func NewLoadProfile(concurrency int, rampUp, rampDown time.Duration) *LoadProfile {
	return &LoadProfile{
		targetConcurrency: concurrency,
		rampUpDuration:    rampUp,
		rampDownDuration:  rampDown,
	}
}

// Start records the start time.
func (lp *LoadProfile) Start() {
	lp.startTime = time.Now()
}

// AllowedConcurrency returns the current allowed concurrency based on elapsed
// time and remaining work items.
//
// Ramp-up: linearly increases from 0 to targetConcurrency over rampUpDuration.
// Steady state: returns targetConcurrency.
// Ramp-down: triggered when pending < targetConcurrency * 2, linearly decreases.
//
// If no ramp durations are configured, always returns targetConcurrency.
func (lp *LoadProfile) AllowedConcurrency(elapsed time.Duration, pending int) int {
	if lp.targetConcurrency <= 0 {
		return 0
	}

	// No ramp configured — return static target.
	if lp.rampUpDuration <= 0 && lp.rampDownDuration <= 0 {
		return lp.targetConcurrency
	}

	allowed := lp.targetConcurrency

	// Check ramp-up phase.
	if lp.rampUpDuration > 0 && elapsed < lp.rampUpDuration {
		allowed = rampUpAllowed(elapsed, lp.rampUpDuration, lp.targetConcurrency)
	}

	// Check ramp-down phase (triggered by remaining item count).
	if lp.rampDownDuration > 0 {
		rampDownAllowed := rampDownByPending(pending, lp.targetConcurrency)
		if rampDownAllowed < allowed {
			allowed = rampDownAllowed
		}
	}

	return allowed
}

// rampUpAllowed calculates allowed concurrency during the ramp-up phase.
func rampUpAllowed(elapsed, rampUpDuration time.Duration, target int) int {
	ratio := float64(elapsed) / float64(rampUpDuration)
	if ratio > 1 {
		ratio = 1
	}
	return int(math.Ceil(ratio * float64(target)))
}

// rampDownByPending calculates allowed concurrency based on remaining pending items.
// Ramp-down is triggered when pending < targetConcurrency * 2.
func rampDownByPending(pending, target int) int {
	threshold := target * 2
	if pending >= threshold {
		return target
	}
	ratio := float64(pending) / float64(threshold)
	allowed := int(math.Ceil(ratio * float64(target)))
	if allowed < 1 {
		allowed = 1
	}
	return allowed
}
