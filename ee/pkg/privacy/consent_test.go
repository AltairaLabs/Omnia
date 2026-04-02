/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"sort"
	"testing"
)

func TestCategoryInfo_NonPII(t *testing.T) {
	nonPII := []ConsentCategory{
		ConsentMemoryPreferences,
		ConsentMemoryContext,
		ConsentMemoryHistory,
	}
	for _, cat := range nonPII {
		requiresGrant, valid := CategoryInfo(cat)
		if !valid {
			t.Errorf("CategoryInfo(%q): expected valid=true, got false", cat)
		}
		if requiresGrant {
			t.Errorf("CategoryInfo(%q): expected requiresGrant=false, got true", cat)
		}
	}
}

func TestCategoryInfo_PII(t *testing.T) {
	pii := []ConsentCategory{
		ConsentMemoryIdentity,
		ConsentMemoryLocation,
		ConsentMemoryHealth,
		ConsentAnalyticsAggregate,
	}
	for _, cat := range pii {
		requiresGrant, valid := CategoryInfo(cat)
		if !valid {
			t.Errorf("CategoryInfo(%q): expected valid=true, got false", cat)
		}
		if !requiresGrant {
			t.Errorf("CategoryInfo(%q): expected requiresGrant=true, got false", cat)
		}
	}
}

func TestCategoryInfo_Unknown(t *testing.T) {
	requiresGrant, valid := CategoryInfo("memory:unknown")
	if valid {
		t.Error("CategoryInfo(\"memory:unknown\"): expected valid=false, got true")
	}
	if requiresGrant {
		t.Error("CategoryInfo(\"memory:unknown\"): expected requiresGrant=false, got true")
	}
}

func TestValidCategories_Count(t *testing.T) {
	cats := ValidCategories()
	if len(cats) != 7 {
		t.Errorf("ValidCategories(): expected 7 categories, got %d", len(cats))
	}
}

func TestValidCategories_Sorted(t *testing.T) {
	cats := ValidCategories()
	if !sort.SliceIsSorted(cats, func(i, j int) bool { return cats[i] < cats[j] }) {
		t.Errorf("ValidCategories(): result is not sorted: %v", cats)
	}
}

func TestValidCategories_Deterministic(t *testing.T) {
	first := ValidCategories()
	second := ValidCategories()
	if len(first) != len(second) {
		t.Fatalf("ValidCategories(): inconsistent length: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("ValidCategories(): non-deterministic at index %d: %q vs %q", i, first[i], second[i])
		}
	}
}

func TestValidCategories_ContainsAll(t *testing.T) {
	expected := []ConsentCategory{
		ConsentAnalyticsAggregate,
		ConsentMemoryContext,
		ConsentMemoryHealth,
		ConsentMemoryHistory,
		ConsentMemoryIdentity,
		ConsentMemoryLocation,
		ConsentMemoryPreferences,
	}
	cats := ValidCategories()
	if len(cats) != len(expected) {
		t.Fatalf("ValidCategories(): expected %d categories, got %d", len(expected), len(cats))
	}
	for i, cat := range cats {
		if cat != expected[i] {
			t.Errorf("ValidCategories()[%d]: expected %q, got %q", i, expected[i], cat)
		}
	}
}
