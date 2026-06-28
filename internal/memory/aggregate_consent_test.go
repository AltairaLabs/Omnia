/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package memory

import "testing"

func TestAggregateConsentFilter_DefaultAlias(t *testing.T) {
	got := AggregateConsentFilter("e", "$5")
	want := "(e.virtual_user_id IS NULL OR e.virtual_user_id = ANY($5::text[]))"
	if got != want {
		t.Errorf("AggregateConsentFilter = %q, want %q", got, want)
	}
}

func TestAggregateConsentFilter_CustomAlias(t *testing.T) {
	got := AggregateConsentFilter("entity", "$3")
	want := "(entity.virtual_user_id IS NULL OR entity.virtual_user_id = ANY($3::text[]))"
	if got != want {
		t.Errorf("AggregateConsentFilter = %q, want %q", got, want)
	}
}

func TestAnalyticsAggregateCategory_Constant(t *testing.T) {
	// Guard against accidental renaming — the memory-api wire contract
	// depends on this literal.
	if AnalyticsAggregateCategory != "analytics:aggregate" {
		t.Errorf("AnalyticsAggregateCategory = %q, want \"analytics:aggregate\"", AnalyticsAggregateCategory)
	}
}
