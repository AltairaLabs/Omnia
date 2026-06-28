/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import "context"

// permissivePreferencesStore is a no-op PreferencesStore used when no
// privacy-api URL is configured. It returns ErrPreferencesNotFound on every
// GetPreferences call, which ShouldRecord/ShouldRemember interpret as
// "opt-in by default" — recording and memory proceed.
type permissivePreferencesStore struct{}

// NewPermissivePreferencesStore returns a PreferencesStore and ConsentSource
// that always treats users as opted-in. Use this when no privacy-api endpoint
// is reachable so opt-out enforcement is disabled rather than fail-closed.
func NewPermissivePreferencesStore() *permissivePreferencesStore {
	return &permissivePreferencesStore{}
}

// Compile-time interface checks.
var _ PreferencesStore = (*permissivePreferencesStore)(nil)
var _ ConsentSource = (*permissivePreferencesStore)(nil)

// GetPreferences always returns ErrPreferencesNotFound, which ShouldRecord
// interprets as "no opt-out → proceed with recording".
func (*permissivePreferencesStore) GetPreferences(_ context.Context, _ string) (*Preferences, error) {
	return nil, ErrPreferencesNotFound
}

// SetOptOut is a no-op: when no privacy-api is configured opt-out preferences
// cannot be persisted. Returns nil so callers do not treat this as an error.
func (*permissivePreferencesStore) SetOptOut(_ context.Context, _, _, _ string) error {
	return nil
}

// RemoveOptOut is a no-op for the same reason as SetOptOut.
func (*permissivePreferencesStore) RemoveOptOut(_ context.Context, _, _, _ string) error {
	return nil
}

// GetConsentGrants returns an empty slice, treating the user as having no
// explicit grants. Non-sensitive categories default to allowed; sensitive
// categories that require an explicit grant will be denied.
func (*permissivePreferencesStore) GetConsentGrants(_ context.Context, _ string) ([]ConsentCategory, error) {
	return []ConsentCategory{}, nil
}
