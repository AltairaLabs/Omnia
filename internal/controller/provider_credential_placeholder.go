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

package controller

import (
	"strings"
)

// IsPlaceholderCredential reports whether the given secret value is one
// of the well-known placeholder strings that ship with the dev samples
// (config/samples/dev/samples.yaml). Operators are expected to replace
// these with real keys; surfacing the placeholder via a Provider
// condition catches the "I forgot to set my key" failure mode at
// reconcile time instead of at chat time (issue #1037).
//
// The matcher is conservative — it only flags strings that are
// clearly placeholders (the "replace-with-real-key" suffix is the
// canonical marker, plus a few well-known dev-sample variants).
// A real Anthropic key happens to start with "sk-ant-" but won't
// contain "replace-with-real-key", so false positives are minimal.
//
// Empty values return false — that's a "missing key" condition and
// the existing key-presence check handles it.
func IsPlaceholderCredential(value string) bool {
	if value == "" {
		return false
	}
	v := strings.ToLower(value)
	for _, marker := range placeholderMarkers {
		if strings.Contains(v, marker) {
			return true
		}
	}
	return false
}

// placeholderMarkers are substrings that, if present, confirm the
// value is a dev-sample placeholder. The strings live in
// config/samples/dev/samples.yaml — keep this list in sync if more
// placeholder shapes appear there.
var placeholderMarkers = []string{
	"replace-with-real-key",
	"replace-me",
	"your-api-key-here",
	"changeme",
}
