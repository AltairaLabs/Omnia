/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStaticConsentSource_ValidGrants(t *testing.T) {
	src := NewStaticConsentSource([]string{
		"memory:preferences",
		"memory:context",
	})
	grants, err := src.GetConsentGrants(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Equal(t, []ConsentCategory{
		ConsentMemoryPreferences,
		ConsentMemoryContext,
	}, grants)
}

func TestNewStaticConsentSource_InvalidCategoriesFiltered(t *testing.T) {
	src := NewStaticConsentSource([]string{
		"memory:bogus",
		"memory:preferences",
		"analytics:fake",
	})
	grants, err := src.GetConsentGrants(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Equal(t, []ConsentCategory{ConsentMemoryPreferences}, grants)
}

func TestNewStaticConsentSource_EmptyInput(t *testing.T) {
	src := NewStaticConsentSource([]string{})
	grants, err := src.GetConsentGrants(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Empty(t, grants)
}

func TestNewStaticConsentSource_MixedValidAndInvalid(t *testing.T) {
	src := NewStaticConsentSource([]string{
		"memory:identity",
		"memory:bogus",
		"analytics:aggregate",
		"not:valid",
	})
	grants, err := src.GetConsentGrants(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Equal(t, []ConsentCategory{
		ConsentMemoryIdentity,
		ConsentAnalyticsAggregate,
	}, grants)
}

func TestNewStaticConsentSource_AllInvalid(t *testing.T) {
	src := NewStaticConsentSource([]string{"bogus", "memory:bogus", "analytics:fake"})
	grants, err := src.GetConsentGrants(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Empty(t, grants)
}

func TestStaticConsentSource_UserIDIgnored(t *testing.T) {
	src := NewStaticConsentSource([]string{"memory:history"})
	grants1, err := src.GetConsentGrants(context.Background(), "user-A")
	require.NoError(t, err)
	grants2, err := src.GetConsentGrants(context.Background(), "user-B")
	require.NoError(t, err)
	assert.Equal(t, grants1, grants2)
}
