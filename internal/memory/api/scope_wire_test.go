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

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altairalabs/omnia/internal/memory"
)

func TestResolveVirtualUserID(t *testing.T) {
	tests := []struct {
		name       string
		params     map[string]string
		wantValue  string
		wantLegacy bool
	}{
		{"canonical", map[string]string{"virtual_user_id": "u1"}, "u1", false},
		{"legacy", map[string]string{"user_id": "u1"}, "u1", true},
		{"canonical wins over legacy", map[string]string{"virtual_user_id": "new", "user_id": "old"}, "new", false},
		{"neither", map[string]string{}, "", false},
		{"empty legacy ignored", map[string]string{"user_id": ""}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := func(k string) string { return tt.params[k] }
			value, legacy := resolveVirtualUserID(get)
			assert.Equal(t, tt.wantValue, value)
			assert.Equal(t, tt.wantLegacy, legacy)
		})
	}
}

func TestNormalizeScopeUserID(t *testing.T) {
	t.Run("legacy key upgraded to canonical", func(t *testing.T) {
		scope := map[string]string{memory.ScopeLegacyUserID: "u1"}
		usedLegacy := normalizeScopeUserID(scope)
		assert.True(t, usedLegacy)
		assert.Equal(t, "u1", scope[memory.ScopeVirtualUserID])
		_, hasLegacy := scope[memory.ScopeLegacyUserID]
		assert.False(t, hasLegacy, "legacy key must be removed")
	})

	t.Run("canonical wins when both present", func(t *testing.T) {
		scope := map[string]string{
			memory.ScopeVirtualUserID: "canonical",
			memory.ScopeLegacyUserID:  "legacy",
		}
		usedLegacy := normalizeScopeUserID(scope)
		assert.True(t, usedLegacy)
		assert.Equal(t, "canonical", scope[memory.ScopeVirtualUserID])
		_, hasLegacy := scope[memory.ScopeLegacyUserID]
		assert.False(t, hasLegacy)
	})

	t.Run("no legacy key is a no-op", func(t *testing.T) {
		scope := map[string]string{memory.ScopeVirtualUserID: "u1"}
		usedLegacy := normalizeScopeUserID(scope)
		assert.False(t, usedLegacy)
		assert.Equal(t, "u1", scope[memory.ScopeVirtualUserID])
	})

	t.Run("idempotent", func(t *testing.T) {
		scope := map[string]string{memory.ScopeLegacyUserID: "u1"}
		assert.True(t, normalizeScopeUserID(scope))
		assert.False(t, normalizeScopeUserID(scope), "second pass sees no legacy key")
		assert.Equal(t, "u1", scope[memory.ScopeVirtualUserID])
	})

	t.Run("empty legacy value drops key without setting canonical", func(t *testing.T) {
		scope := map[string]string{memory.ScopeLegacyUserID: ""}
		usedLegacy := normalizeScopeUserID(scope)
		assert.True(t, usedLegacy)
		_, hasCanonical := scope[memory.ScopeVirtualUserID]
		assert.False(t, hasCanonical)
	})

	t.Run("nil scope is safe", func(t *testing.T) {
		assert.False(t, normalizeScopeUserID(nil))
	})
}
