/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package memory

import "testing"

func TestAggregateConsentJoin_DefaultAlias(t *testing.T) {
	join, where := AggregateConsentJoin("e")
	wantJoin := "LEFT JOIN user_privacy_preferences p ON p.user_id = e.virtual_user_id"
	wantWhere := "(e.virtual_user_id IS NULL OR 'analytics:aggregate' = ANY(p.consent_grants))"
	if join != wantJoin {
		t.Errorf("join = %q, want %q", join, wantJoin)
	}
	if where != wantWhere {
		t.Errorf("where = %q, want %q", where, wantWhere)
	}
}

func TestAggregateConsentJoin_CustomAlias(t *testing.T) {
	join, where := AggregateConsentJoin("entity")
	if join != "LEFT JOIN user_privacy_preferences p ON p.user_id = entity.virtual_user_id" {
		t.Errorf("unexpected join for custom alias: %q", join)
	}
	if where != "(entity.virtual_user_id IS NULL OR 'analytics:aggregate' = ANY(p.consent_grants))" {
		t.Errorf("unexpected where for custom alias: %q", where)
	}
}

func TestAnalyticsAggregateCategory_Constant(t *testing.T) {
	// Guard against accidental renaming — the memory-api wire contract
	// depends on this literal.
	if AnalyticsAggregateCategory != "analytics:aggregate" {
		t.Errorf("AnalyticsAggregateCategory = %q, want \"analytics:aggregate\"", AnalyticsAggregateCategory)
	}
}
