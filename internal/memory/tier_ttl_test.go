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

func ttlTierPolicy() *omniav1alpha1.MemoryPolicy {
	return &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			Tiers: omniav1alpha1.MemoryRetentionTierSet{
				Institutional: &omniav1alpha1.MemoryTierConfig{
					TTL: &omniav1alpha1.MemoryTTLConfig{Default: "365d"},
				},
				Agent: &omniav1alpha1.MemoryTierConfig{
					TTL: &omniav1alpha1.MemoryTTLConfig{Default: "90d", MaxAge: "180d"},
				},
				User: &omniav1alpha1.MemoryTierConfig{
					TTL: &omniav1alpha1.MemoryTTLConfig{Default: "48h", MaxAge: "30d"},
				},
			},
		},
	}
}

func TestResolveTierTTL(t *testing.T) {
	policy := ttlTierPolicy()
	day := 24 * time.Hour

	cases := []struct {
		name        string
		scope       map[string]string
		wantDefault time.Duration
		wantMaxAge  time.Duration
	}{
		{"institutional tier", map[string]string{ScopeWorkspaceID: "w"}, 365 * day, 0},
		{"agent tier", map[string]string{ScopeWorkspaceID: "w", ScopeAgentID: "a"}, 90 * day, 180 * day},
		{"user tier", map[string]string{ScopeWorkspaceID: "w", ScopeUserID: "u"}, 48 * time.Hour, 30 * day},
		{
			"user-for-agent collapses into user",
			map[string]string{ScopeWorkspaceID: "w", ScopeUserID: "u", ScopeAgentID: "a"},
			48 * time.Hour, 30 * day,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveTierTTL(policy, tc.scope)
			assert.Equal(t, tc.wantDefault, got.Default)
			assert.Equal(t, tc.wantMaxAge, got.MaxAge)
		})
	}
}

func TestResolveTierTTL_UnsetAndInvalid(t *testing.T) {
	userScope := map[string]string{ScopeWorkspaceID: "w", ScopeUserID: "u"}

	// Nil policy → fully unset.
	assert.Equal(t, TierTTL{}, ResolveTierTTL(nil, userScope))

	// Tier present but no TTL block → unset.
	noTTL := &omniav1alpha1.MemoryPolicy{Spec: omniav1alpha1.MemoryPolicySpec{
		Tiers: omniav1alpha1.MemoryRetentionTierSet{User: &omniav1alpha1.MemoryTierConfig{}},
	}}
	assert.Equal(t, TierTTL{}, ResolveTierTTL(noTTL, userScope))

	// Unparseable / empty duration strings are treated as unset.
	bad := &omniav1alpha1.MemoryPolicy{Spec: omniav1alpha1.MemoryPolicySpec{
		Tiers: omniav1alpha1.MemoryRetentionTierSet{User: &omniav1alpha1.MemoryTierConfig{
			TTL: &omniav1alpha1.MemoryTTLConfig{Default: "nonsense", MaxAge: ""},
		}},
	}}
	assert.Equal(t, TierTTL{}, ResolveTierTTL(bad, userScope))
}
