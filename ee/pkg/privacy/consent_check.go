/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"slices"
)

// ShouldRememberCategory checks whether memory storage should proceed for a
// given user and consent category. It first checks the binary opt-out (global,
// workspace, agent). If not opted out, it checks category consent:
//   - Categories that don't require explicit grant → allowed
//   - Categories that require explicit grant → check user's consent grants
//   - Unknown categories → rejected (fail closed)
//   - ConsentSource errors → rejected for PII categories (fail closed)
func ShouldRememberCategory(
	ctx context.Context,
	store PreferencesStore,
	source ConsentSource,
	userID, workspace, agent string,
	category ConsentCategory,
) bool {
	// 1. Binary opt-out check (reuse existing shouldProceed)
	if !shouldProceed(ctx, store, userID, workspace, agent) {
		return false
	}

	// 2. Category validation
	requiresGrant, valid := CategoryInfo(category)
	if !valid {
		return false
	}

	// 3. Non-PII categories don't need explicit grant
	if !requiresGrant {
		return true
	}

	// 4. PII categories need explicit grant
	grants, err := source.GetConsentGrants(ctx, userID)
	if err != nil {
		// Fail closed for PII categories
		return false
	}
	return slices.Contains(grants, category)
}
