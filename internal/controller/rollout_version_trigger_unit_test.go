/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// TestVersionsNewer covers versionsNewer's comparison + parse-failure
// fallback paths (used by maybeTriggerVersionRollout to decide whether the
// channel-latest PromptPack version warrants a new rollout candidate).
func TestVersionsNewer(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"a strictly newer", "1.1.0", "1.0.0", true},
		{"a older", "1.0.0", "1.1.0", false},
		{"equal", "1.0.0", "1.0.0", false},
		{"v-prefix on both sides", "v1.1.0", "1.0.0", true},
		{"unparseable a -> false", "not-a-version", "1.0.0", false},
		{"unparseable b -> false", "1.1.0", "not-a-version", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, versionsNewer(tc.a, tc.b))
		})
	}
}

// TestRecordRolledBackVersion verifies the rolled-back version is stamped as an
// annotation for a version-pinned candidate, and is a no-op otherwise (so the
// version-trigger's re-canary guard only engages for real version rollbacks).
func TestRecordRolledBackVersion(t *testing.T) {
	v := "2.0.0"
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Rollout: &omniav1alpha1.RolloutConfig{
				Candidate: &omniav1alpha1.CandidateOverrides{
					PromptPackRef: &omniav1alpha1.PromptPackRef{Name: "p", Version: &v},
				},
			},
		},
	}
	recordRolledBackVersion(ar)
	assert.Equal(t, "2.0.0", ar.Annotations[lastRolledBackVersionAnnotation])

	// No candidate -> no stamp.
	bare := &omniav1alpha1.AgentRuntime{}
	recordRolledBackVersion(bare)
	_, ok := bare.Annotations[lastRolledBackVersionAnnotation]
	assert.False(t, ok)

	// Candidate without a pinned version -> no stamp.
	noVer := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Rollout: &omniav1alpha1.RolloutConfig{
				Candidate: &omniav1alpha1.CandidateOverrides{
					PromptPackRef: &omniav1alpha1.PromptPackRef{Name: "p"},
				},
			},
		},
	}
	recordRolledBackVersion(noVer)
	_, ok = noVer.Annotations[lastRolledBackVersionAnnotation]
	assert.False(t, ok)
}
