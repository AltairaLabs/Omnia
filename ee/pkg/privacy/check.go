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
	"slices"
)

// ShouldRecord checks whether session recording should proceed for a given user.
// It returns false if the user has opted out globally, for the specific workspace,
// or for the specific agent. Returns true if no opt-out preferences exist.
func ShouldRecord(ctx context.Context, store PreferencesStore, userID, workspace, agent string) bool {
	return shouldProceed(ctx, store, userID, workspace, agent)
}

// ShouldRemember checks whether memory storage should proceed for a given user.
// Uses the same three-level hierarchy as ShouldRecord: global opt-out,
// workspace opt-out, agent opt-out. Returns true if no opt-out applies.
func ShouldRemember(ctx context.Context, store PreferencesStore, userID, workspace, agent string) bool {
	return shouldProceed(ctx, store, userID, workspace, agent)
}

// shouldProceed implements the three-level opt-out check used by both
// ShouldRecord and ShouldRemember. Returns false if the user has opted
// out globally, for the workspace, or for the agent.
func shouldProceed(ctx context.Context, store PreferencesStore, userID, workspace, agent string) bool {
	prefs, err := store.GetPreferences(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrPreferencesNotFound) {
			return true
		}
		// On unexpected errors, default to allowing to avoid data loss.
		return true
	}

	if prefs.OptOutAll {
		return false
	}

	if workspace != "" && slices.Contains(prefs.OptOutWorkspaces, workspace) {
		return false
	}

	if agent != "" && slices.Contains(prefs.OptOutAgents, agent) {
		return false
	}

	return true
}
