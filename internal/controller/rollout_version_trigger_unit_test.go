/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
