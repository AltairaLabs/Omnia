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

// mockPreferencesStore implements PreferencesStore for testing ShouldRecord.
type mockPreferencesStore struct {
	prefs *Preferences
	err   error
}

func (m *mockPreferencesStore) GetPreferences(_ context.Context, _ string) (*Preferences, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.prefs, nil
}

func (m *mockPreferencesStore) SetOptOut(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *mockPreferencesStore) RemoveOptOut(_ context.Context, _, _, _ string) error {
	return nil
}

func TestShouldRecord_NoPreferences(t *testing.T) {
	store := &mockPreferencesStore{err: ErrPreferencesNotFound}
	result := ShouldRecord(context.Background(), store, "user1", "ws1", "agent1")
	assert.True(t, result)
}

func TestShouldRecord_UnexpectedError(t *testing.T) {
	store := &mockPreferencesStore{err: errors.New("db error")}
	result := ShouldRecord(context.Background(), store, "user1", "ws1", "agent1")
	assert.True(t, result, "should default to allowing recording on unexpected errors")
}

func TestShouldRecord_OptOutAll(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        true,
		OptOutWorkspaces: []string{},
		OptOutAgents:     []string{},
	}}
	result := ShouldRecord(context.Background(), store, "user1", "ws1", "agent1")
	assert.False(t, result)
}

func TestShouldRecord_OptOutWorkspace(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        false,
		OptOutWorkspaces: []string{"ws1", "ws2"},
		OptOutAgents:     []string{},
	}}

	assert.False(t, ShouldRecord(context.Background(), store, "user1", "ws1", "agent1"))
	assert.True(t, ShouldRecord(context.Background(), store, "user1", "ws3", "agent1"))
}

func TestShouldRecord_OptOutAgent(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        false,
		OptOutWorkspaces: []string{},
		OptOutAgents:     []string{"agent1"},
	}}

	assert.False(t, ShouldRecord(context.Background(), store, "user1", "ws1", "agent1"))
	assert.True(t, ShouldRecord(context.Background(), store, "user1", "ws1", "agent2"))
}

func TestShouldRecord_NoOptOut(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        false,
		OptOutWorkspaces: []string{},
		OptOutAgents:     []string{},
	}}
	result := ShouldRecord(context.Background(), store, "user1", "ws1", "agent1")
	assert.True(t, result)
}

func TestShouldRecord_EmptyWorkspaceAndAgent(t *testing.T) {
	store := &mockPreferencesStore{prefs: &Preferences{
		OptOutAll:        false,
		OptOutWorkspaces: []string{"ws1"},
		OptOutAgents:     []string{"agent1"},
	}}
	result := ShouldRecord(context.Background(), store, "user1", "", "")
	assert.True(t, result)
}

func TestContainsStr(t *testing.T) {
	assert.True(t, containsStr([]string{"a", "b", "c"}, "b"))
	assert.False(t, containsStr([]string{"a", "b", "c"}, "d"))
	assert.False(t, containsStr(nil, "a"))
	assert.False(t, containsStr([]string{}, "a"))
}
