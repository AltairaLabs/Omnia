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

package auth

import "testing"

// In-package tests for claimToString. The end-to-end oidc_test.go
// covers the common paths via real JWT flow, but jwt.MapClaims always
// marshals arrays as []any — the []string and numericClaim fallback
// branches only surface when a non-JWT caller uses the coercer
// directly. Keep those paths green so a future refactor doesn't
// silently drop them.
func TestClaimToString_TypedArrays(t *testing.T) {
	t.Parallel()
	if got, ok := claimToString([]string{"a", "b"}); !ok || got != "a,b" {
		t.Errorf("[]string: got (%q,%v), want (a,b, true)", got, ok)
	}
	// Empty-after-filtering []string must drop.
	if _, ok := claimToString([]string{"", ""}); ok {
		t.Errorf("[]string of empties should drop")
	}
	// Mix of empty and non-empty: only non-empty survives.
	if got, ok := claimToString([]string{"", "x", ""}); !ok || got != "x" {
		t.Errorf("[]string with blanks: got (%q,%v), want (x, true)", got, ok)
	}
}

func TestClaimToString_EmptyInputs(t *testing.T) {
	t.Parallel()
	if _, ok := claimToString(""); ok {
		t.Error("empty string must drop")
	}
	if _, ok := claimToString([]any{}); ok {
		t.Error("empty []any must drop")
	}
	if _, ok := claimToString([]string{}); ok {
		t.Error("empty []string must drop")
	}
}

func TestClaimToString_NumericVariants(t *testing.T) {
	t.Parallel()
	if got, ok := claimToString(float64(3.14)); !ok || got != "3.14" {
		t.Errorf("float: got (%q,%v)", got, ok)
	}
	if got, ok := claimToString(float64(42)); !ok || got != "42" {
		t.Errorf("whole-float: got (%q,%v)", got, ok)
	}
}

func TestClaimToString_Bools(t *testing.T) {
	t.Parallel()
	if got, ok := claimToString(true); !ok || got != "true" {
		t.Errorf("true: got (%q,%v)", got, ok)
	}
	if got, ok := claimToString(false); !ok || got != "false" {
		t.Errorf("false: got (%q,%v)", got, ok)
	}
}

func TestClaimToString_Unsupported(t *testing.T) {
	t.Parallel()
	// Nested object / pointer / int — all dropped.
	if _, ok := claimToString(map[string]string{"a": "b"}); ok {
		t.Error("map must drop")
	}
	if _, ok := claimToString(int(5)); ok {
		t.Error("plain int must drop (jwt.MapClaims never gives us int)")
	}
	if _, ok := claimToString(nil); ok {
		t.Error("nil must drop")
	}
}
