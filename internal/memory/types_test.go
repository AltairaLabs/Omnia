/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import "testing"

func TestMutabilityAllowsModification(t *testing.T) {
	cases := []struct {
		m    Mutability
		want bool
	}{
		{MutabilityMutable, true},
		{MutabilitySummarisableOnly, false},
		{MutabilityImmutable, false},
		{Mutability(""), false},
		{Mutability("unknown"), false},
	}
	for _, tc := range cases {
		t.Run(string(tc.m), func(t *testing.T) {
			if got := tc.m.AllowsModification(); got != tc.want {
				t.Errorf("Mutability(%q).AllowsModification() = %v, want %v",
					tc.m, got, tc.want)
			}
		})
	}
}

func TestMutabilityStringValues(t *testing.T) {
	// String values must match the Postgres `mutability` column values.
	cases := map[Mutability]string{
		MutabilityMutable:          "mutable",
		MutabilitySummarisableOnly: "summarisable_only",
		MutabilityImmutable:        "immutable",
	}
	for m, want := range cases {
		if string(m) != want {
			t.Errorf("Mutability(%s) = %q, want %q", m, string(m), want)
		}
	}
}
