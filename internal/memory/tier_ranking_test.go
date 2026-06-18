/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentityTierRanker_AdjustReturnsBaseUnchanged(t *testing.T) {
	r := IdentityTierRanker{}
	assert.Equal(t, 0.42, r.Adjust(0.42, TierInstitutional))
	assert.Equal(t, 0.42, r.Adjust(0.42, TierUserForAgent))
}
