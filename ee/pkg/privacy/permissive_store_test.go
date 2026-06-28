/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"errors"
	"testing"
)

func TestPermissivePreferencesStore_GetPreferences(t *testing.T) {
	store := NewPermissivePreferencesStore()
	prefs, err := store.GetPreferences(context.Background(), "user1")
	if prefs != nil {
		t.Errorf("expected nil preferences, got %v", prefs)
	}
	if !errors.Is(err, ErrPreferencesNotFound) {
		t.Errorf("expected ErrPreferencesNotFound, got %v", err)
	}
}

func TestPermissivePreferencesStore_SetOptOut(t *testing.T) {
	store := NewPermissivePreferencesStore()
	if err := store.SetOptOut(context.Background(), "user1", ScopeAll, ""); err != nil {
		t.Errorf("SetOptOut: expected nil error, got %v", err)
	}
}

func TestPermissivePreferencesStore_RemoveOptOut(t *testing.T) {
	store := NewPermissivePreferencesStore()
	if err := store.RemoveOptOut(context.Background(), "user1", ScopeAgent, "my-agent"); err != nil {
		t.Errorf("RemoveOptOut: expected nil error, got %v", err)
	}
}

func TestPermissivePreferencesStore_GetConsentGrants(t *testing.T) {
	store := NewPermissivePreferencesStore()
	grants, err := store.GetConsentGrants(context.Background(), "user1")
	if err != nil {
		t.Errorf("GetConsentGrants: expected nil error, got %v", err)
	}
	if len(grants) != 0 {
		t.Errorf("GetConsentGrants: expected empty slice, got %v", grants)
	}
}
