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
)

// ShouldRecord checks whether session recording should proceed for a given user.
// It returns false if the user has opted out globally, for the specific workspace,
// or for the specific agent. Returns true if no opt-out preferences exist.
func ShouldRecord(ctx context.Context, store PreferencesStore, userID, workspace, agent string) bool {
	prefs, err := store.GetPreferences(ctx, userID)
	if err != nil {
		// If the user has no preferences, recording is allowed.
		if errors.Is(err, ErrPreferencesNotFound) {
			return true
		}
		// On unexpected errors, default to allowing recording to avoid data loss.
		return true
	}

	if prefs.OptOutAll {
		return false
	}

	if workspace != "" && containsStr(prefs.OptOutWorkspaces, workspace) {
		return false
	}

	if agent != "" && containsStr(prefs.OptOutAgents, agent) {
		return false
	}

	return true
}

// containsStr reports whether s is present in the slice.
func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
