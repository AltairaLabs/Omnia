/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"testing"

	"github.com/altairalabs/omnia/ee/pkg/memory/projection"
	"github.com/stretchr/testify/assert"
)

func TestIsSensitiveCategory(t *testing.T) {
	for _, c := range []string{"memory:identity", "memory:location", "memory:health"} {
		assert.True(t, isSensitiveCategory(c), "want sensitive: %s", c)
	}
	for _, c := range []string{"memory:preferences", "memory:context", "memory:history", "", "other"} {
		assert.False(t, isSensitiveCategory(c), "want not-sensitive: %s", c)
	}
}

func TestMaskPoint_ZeroesIdentifyingFields(t *testing.T) {
	p := projection.Point{
		ID: "e1", X: 0.4, Y: -0.1, Tier: "user", Type: "profile",
		User: "u1", UserRef: "u1", Category: "memory:health",
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
	assert.Equal(t, "user", p.Tier)
	assert.Equal(t, 0.4, p.X)
	assert.Equal(t, 0.9, p.Confidence)
}

func TestPointMustBeMasked(t *testing.T) {
	assert.True(t, pointMustBeMasked(projection.Point{Category: "memory:identity"}))
	assert.False(t, pointMustBeMasked(projection.Point{Category: "memory:context"}))
}
