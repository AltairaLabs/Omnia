/*
Copyright 2026.

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

package policy

import "testing"

// These constants are a downstream contract — ToolPolicy rules compare
// against them and session-logging consumers persist them. Pin the
// on-the-wire values so typo-level changes show up as test failures.

func TestOriginConstants(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		OriginManagementPlane: "management-plane",
		OriginSharedToken:     "shared-token",
		OriginAPIKey:          "api-key",
		OriginOIDC:            "oidc",
		OriginEdgeTrust:       "edge-trust",
	}
	for got, expected := range want {
		if got != expected {
			t.Errorf("origin constant = %q, want %q", got, expected)
		}
	}
}

func TestRoleConstants(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		RoleAdmin:  "admin",
		RoleEditor: "editor",
		RoleViewer: "viewer",
	}
	for got, expected := range want {
		if got != expected {
			t.Errorf("role constant = %q, want %q", got, expected)
		}
	}
}
