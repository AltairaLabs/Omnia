/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"testing"

	coreproj "github.com/altairalabs/omnia/internal/memory/projection"
	"github.com/stretchr/testify/assert"
)

// catContext and catPreferences are non-sensitive consent categories used
// across the projection tests (extracted to satisfy goconst).
const (
	catContext     = "memory:context"
	catPreferences = "memory:preferences"
)

func TestIsSensitiveCategory(t *testing.T) {
	for _, c := range []string{catIdentity, catLocation, catHealth} {
		assert.True(t, isSensitiveCategory(c), "want sensitive: %s", c)
	}
	for _, c := range []string{catPreferences, catContext, "memory:history", "", "other"} {
		assert.False(t, isSensitiveCategory(c), "want not-sensitive: %s", c)
	}
}

func TestMaskPoint_ZeroesIdentifyingFields(t *testing.T) {
	p := coreproj.Point{
		ID: "e1", X: 0.4, Y: -0.1, Tier: projTierUser, Type: "profile",
		User: "u1", UserRef: "u1", Category: catHealth,
		Confidence: 0.9, Title: "secret", Preview: "diagnosis: …",
	}
	maskPoint(&p)
	assert.True(t, p.Masked)
	assert.Empty(t, p.ID)
	assert.Empty(t, p.Title)
	assert.Empty(t, p.Preview)
	assert.Empty(t, p.User)
	assert.Empty(t, p.UserRef)
	assert.Empty(t, p.Category)
	assert.Empty(t, p.Type)
	// Safe metadata retained.
	assert.Equal(t, projTierUser, p.Tier)
	assert.Equal(t, 0.4, p.X)
	assert.Equal(t, 0.9, p.Confidence)
}

func TestPointMustBeMasked(t *testing.T) {
	assert.True(t, pointMustBeMasked(coreproj.Point{Category: catIdentity}))
	assert.False(t, pointMustBeMasked(coreproj.Point{Category: catContext}))
}
