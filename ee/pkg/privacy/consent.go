/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"sort"
)

// ConsentCategory represents a granular memory consent scope.
type ConsentCategory string

const (
	ConsentMemoryPreferences  ConsentCategory = "memory:preferences"
	ConsentMemoryContext      ConsentCategory = "memory:context"
	ConsentMemoryHistory      ConsentCategory = "memory:history"
	ConsentMemoryIdentity     ConsentCategory = "memory:identity"
	ConsentMemoryLocation     ConsentCategory = "memory:location"
	ConsentMemoryHealth       ConsentCategory = "memory:health"
	ConsentAnalyticsAggregate ConsentCategory = "analytics:aggregate"
)

type categoryMeta struct {
	RequiresExplicitGrant bool
}

var categoryRegistry = map[ConsentCategory]categoryMeta{
	ConsentMemoryPreferences:  {RequiresExplicitGrant: false},
	ConsentMemoryContext:      {RequiresExplicitGrant: false},
	ConsentMemoryHistory:      {RequiresExplicitGrant: false},
	ConsentMemoryIdentity:     {RequiresExplicitGrant: true},
	ConsentMemoryLocation:     {RequiresExplicitGrant: true},
	ConsentMemoryHealth:       {RequiresExplicitGrant: true},
	ConsentAnalyticsAggregate: {RequiresExplicitGrant: true},
}

// CategoryInfo returns whether a category requires an explicit user grant
// and whether it is a valid platform-defined category.
func CategoryInfo(c ConsentCategory) (requiresGrant bool, valid bool) {
	meta, ok := categoryRegistry[c]
	return meta.RequiresExplicitGrant, ok
}

// ValidCategories returns all defined consent categories in deterministic order.
func ValidCategories() []ConsentCategory {
	cats := make([]ConsentCategory, 0, len(categoryRegistry))
	for c := range categoryRegistry {
		cats = append(cats, c)
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i] < cats[j] })
	return cats
}

// ConsentSource abstracts where consent grants come from.
// Implemented by PreferencesPostgresStore (reads from DB) and potentially
// by a future TokenConsentSource (reads from JWT claims in context).
type ConsentSource interface {
	GetConsentGrants(ctx context.Context, userID string) ([]ConsentCategory, error)
}
