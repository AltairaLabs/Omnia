/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import "github.com/altairalabs/omnia/internal/memory"

// deprecatedUserIDParam is the log message emitted when a caller still uses the
// pre-#1280 "user_id" wire name (query param or request-body scope key) instead
// of the canonical "virtual_user_id". memory-api accepts the legacy name for one
// transition release and drops it the release after. See #1280.
const deprecatedUserIDParam = "deprecated user_id scope param received; use virtual_user_id"

// resolveVirtualUserID reads the per-subject scope value from a query getter,
// accepting the canonical virtual_user_id param and, during the #1280 transition
// window, the legacy user_id param. The canonical param wins when both are
// present. usedLegacy reports that the value came from the legacy param so the
// caller can log a deprecation warning.
func resolveVirtualUserID(get func(string) string) (value string, usedLegacy bool) {
	if v := get(memory.ScopeVirtualUserID); v != "" {
		return v, false
	}
	if v := get(memory.ScopeLegacyUserID); v != "" {
		return v, true
	}
	return "", false
}

// normalizeScopeUserID upgrades a request-body scope map in place: a legacy
// user_id key is moved to virtual_user_id (the canonical key wins if both are
// set) and the legacy key is removed so downstream readers see only the
// canonical name. Reports whether a legacy key was present so the caller can log
// a deprecation warning. Idempotent and nil-safe.
func normalizeScopeUserID(scope map[string]string) (usedLegacy bool) {
	legacy, ok := scope[memory.ScopeLegacyUserID]
	if !ok {
		return false
	}
	delete(scope, memory.ScopeLegacyUserID)
	if _, hasCanonical := scope[memory.ScopeVirtualUserID]; !hasCanonical && legacy != "" {
		scope[memory.ScopeVirtualUserID] = legacy
	}
	return true
}
