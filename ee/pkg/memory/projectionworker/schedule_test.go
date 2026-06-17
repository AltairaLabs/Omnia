/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package projectionworker

import (
	"testing"
	"time"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
)

// baseFP is the baseline "<count>:<nanos>" fingerprint reused across cases.
const baseFP = "100:1"

func ptr(i int32) *int32 { return &i }

func TestShouldRender(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		stored *memory.StoredProjection
		live   string
		cfg    memoryv1.MemoryProjectionConfig
		want   bool
	}{
		{"cold-never-rendered", nil, baseFP, memoryv1.MemoryProjectionConfig{}, true},
		{"unchanged-fingerprint", &memory.StoredProjection{Fingerprint: baseFP, ComputedAt: now.Add(-time.Hour)}, baseFP, memoryv1.MemoryProjectionConfig{}, false},
		{"changed-no-gates", &memory.StoredProjection{Fingerprint: baseFP, ComputedAt: now.Add(-time.Hour)}, "101:2", memoryv1.MemoryProjectionConfig{}, true},
		{"changed-below-threshold", &memory.StoredProjection{Fingerprint: baseFP, ComputedAt: now.Add(-time.Hour)}, "150:2", memoryv1.MemoryProjectionConfig{ChangeThreshold: ptr(100)}, false},
		{"changed-above-threshold", &memory.StoredProjection{Fingerprint: baseFP, ComputedAt: now.Add(-time.Hour)}, "250:2", memoryv1.MemoryProjectionConfig{ChangeThreshold: ptr(100)}, true},
		// Eligibility flip (lexical→dense) with an unchanged entity count must
		// bypass the count threshold — a backfill crossing the dense threshold
		// changes nothing but the third bit, yet must re-render.
		{"eligibility-flip-bypasses-threshold", &memory.StoredProjection{Fingerprint: "100:1:0", ComputedAt: now.Add(-time.Hour)}, "100:2:1", memoryv1.MemoryProjectionConfig{ChangeThreshold: ptr(100)}, true},
		// Same eligibility, count change below threshold → still suppressed.
		{"eligibility-same-below-threshold", &memory.StoredProjection{Fingerprint: "100:1:1", ComputedAt: now.Add(-time.Hour)}, "150:2:1", memoryv1.MemoryProjectionConfig{ChangeThreshold: ptr(100)}, false},
		{"changed-cron-not-due", &memory.StoredProjection{Fingerprint: baseFP, ComputedAt: now.Add(-time.Minute)}, "101:2", memoryv1.MemoryProjectionConfig{Schedule: "0 * * * *"}, false},
		{"changed-cron-due", &memory.StoredProjection{Fingerprint: baseFP, ComputedAt: now.Add(-2 * time.Hour)}, "101:2", memoryv1.MemoryProjectionConfig{Schedule: "0 * * * *"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := shouldRender(c.stored, c.live, c.cfg, now)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("shouldRender = %v, want %v", got, c.want)
			}
		})
	}
}
