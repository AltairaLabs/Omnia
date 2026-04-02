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

	"github.com/stretchr/testify/assert"
)

type mockConsentSource struct {
	grants []ConsentCategory
	err    error
}

func (m *mockConsentSource) GetConsentGrants(_ context.Context, _ string) ([]ConsentCategory, error) {
	return m.grants, m.err
}

func TestShouldRememberCategory_OptOutAll(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        true,
		OptOutWorkspaces: []string{},
		OptOutAgents:     []string{},
	}}
	source := &mockConsentSource{grants: []ConsentCategory{ConsentMemoryIdentity}}
	result := ShouldRememberCategory(context.Background(), store, source, "user1", "ws1", "agent1", ConsentMemoryIdentity)
	assert.False(t, result)
}

func TestShouldRememberCategory_OptOutWorkspace(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        false,
		OptOutWorkspaces: []string{"ws1"},
		OptOutAgents:     []string{},
	}}
	source := &mockConsentSource{grants: []ConsentCategory{ConsentMemoryIdentity}}
	result := ShouldRememberCategory(context.Background(), store, source, "user1", "ws1", "agent1", ConsentMemoryIdentity)
	assert.False(t, result)
}

func TestShouldRememberCategory_NonPII_NoGrants(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        false,
		OptOutWorkspaces: []string{},
		OptOutAgents:     []string{},
	}}
	source := &mockConsentSource{grants: []ConsentCategory{}}
	result := ShouldRememberCategory(context.Background(), store, source, "user1", "ws1", "agent1", ConsentMemoryContext)
	assert.True(t, result)
}

func TestShouldRememberCategory_NonPII_NewUser(t *testing.T) {
	store := &mockPreferencesStore{err: ErrPreferencesNotFound}
	source := &mockConsentSource{grants: []ConsentCategory{}}
	result := ShouldRememberCategory(context.Background(), store, source, "user1", "ws1", "agent1", ConsentMemoryContext)
	assert.True(t, result)
}

func TestShouldRememberCategory_PII_NoGrants(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        false,
		OptOutWorkspaces: []string{},
		OptOutAgents:     []string{},
	}}
	source := &mockConsentSource{grants: []ConsentCategory{}}
	result := ShouldRememberCategory(context.Background(), store, source, "user1", "ws1", "agent1", ConsentMemoryIdentity)
	assert.False(t, result)
}

func TestShouldRememberCategory_PII_MatchingGrant(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        false,
		OptOutWorkspaces: []string{},
		OptOutAgents:     []string{},
	}}
	source := &mockConsentSource{grants: []ConsentCategory{ConsentMemoryIdentity}}
	result := ShouldRememberCategory(context.Background(), store, source, "user1", "ws1", "agent1", ConsentMemoryIdentity)
	assert.True(t, result)
}

func TestShouldRememberCategory_PII_DifferentGrant(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        false,
		OptOutWorkspaces: []string{},
		OptOutAgents:     []string{},
	}}
	// User granted ConsentMemoryLocation but we're checking ConsentMemoryIdentity
	source := &mockConsentSource{grants: []ConsentCategory{ConsentMemoryLocation}}
	result := ShouldRememberCategory(context.Background(), store, source, "user1", "ws1", "agent1", ConsentMemoryIdentity)
	assert.False(t, result)
}

func TestShouldRememberCategory_UnknownCategory(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        false,
		OptOutWorkspaces: []string{},
		OptOutAgents:     []string{},
	}}
	source := &mockConsentSource{grants: []ConsentCategory{}}
	unknown := ConsentCategory("memory:unknown")
	result := ShouldRememberCategory(context.Background(), store, source, "user1", "ws1", "agent1", unknown)
	assert.False(t, result)
}

func TestShouldRememberCategory_ConsentSourceError(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        false,
		OptOutWorkspaces: []string{},
		OptOutAgents:     []string{},
	}}
	source := &mockConsentSource{err: errors.New("db error")}
	result := ShouldRememberCategory(context.Background(), store, source, "user1", "ws1", "agent1", ConsentMemoryIdentity)
	assert.False(t, result, "should fail closed on ConsentSource error for PII categories")
}

func TestShouldRememberCategory_NoPreferences_NonPII(t *testing.T) {
	store := &mockPreferencesStore{err: ErrPreferencesNotFound}
	source := &mockConsentSource{grants: []ConsentCategory{}}
	result := ShouldRememberCategory(context.Background(), store, source, "user1", "ws1", "agent1", ConsentMemoryContext)
	assert.True(t, result)
}

func TestShouldRememberCategory_NoPreferences_PII(t *testing.T) {
	store := &mockPreferencesStore{err: ErrPreferencesNotFound}
	// No preferences means no grants exist
	source := &mockConsentSource{grants: []ConsentCategory{}}
	result := ShouldRememberCategory(context.Background(), store, source, "user1", "ws1", "agent1", ConsentMemoryIdentity)
	assert.False(t, result)
}
