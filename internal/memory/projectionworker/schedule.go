/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package projectionworker

import (
	"fmt"
	"time"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// shouldRender decides whether scope needs a (re)render given its stored layout,
// the live fingerprint, and the policy's projection config.
func shouldRender(stored *memory.StoredProjection, live string, cfg memoryv1.MemoryProjectionConfig, now time.Time) (bool, error) {
	if stored == nil {
		return true, nil // never rendered
	}
	if stored.Fingerprint == live {
		return false, nil // unchanged; layout still valid
	}
	// Changed. Apply gates.
	if cfg.ChangeThreshold != nil && *cfg.ChangeThreshold > 0 {
		delta := abs(fpCount(live) - fpCount(stored.Fingerprint))
		if int32(delta) < *cfg.ChangeThreshold {
			return false, nil // not enough change yet
		}
	}
	if cfg.Schedule != "" {
		sched, err := cronParser.Parse(cfg.Schedule)
		if err != nil {
			return false, fmt.Errorf("projection: invalid cron %q: %w", cfg.Schedule, err)
		}
		// Next render is due once the schedule's next tick after the last
		// render has been reached. A tick landing exactly on now counts as
		// "too recent" — render only once now is strictly past it.
		if !sched.Next(stored.ComputedAt).Before(now) {
			return false, nil // rendered too recently
		}
	}
	return true, nil
}

func fpCount(fingerprint string) int {
	if fingerprint == "" {
		return 0
	}
	var c int
	_, _ = fmt.Sscanf(fingerprint, "%d:", &c)
	return c
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}
