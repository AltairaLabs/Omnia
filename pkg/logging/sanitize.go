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

package logging

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// hashPrefix is the number of hex characters to include from the SHA-256 hash.
const hashPrefix = 12

// truncatedSuffix is appended to strings that are truncated.
const truncatedSuffix = "[truncated]"

// Truncate returns the first maxLen characters of s, appending "[truncated]"
// if the string was truncated. Returns the original string if it fits within
// maxLen. Returns an empty string if maxLen is negative.
func Truncate(s string, maxLen int) string {
	if maxLen < 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + truncatedSuffix
}

// HashID returns a stable, non-reversible hash prefix of an identifier.
// The format is "[hash:<12 hex chars>]" derived from SHA-256. This allows
// correlating log entries without exposing the original value.
func HashID(id string) string {
	h := sha256.Sum256([]byte(id))
	return "[hash:" + hex.EncodeToString(h[:])[:hashPrefix] + "]"
}

// SafeMapKeys returns only the sorted keys of a map, omitting all values.
// This is useful for logging structured data shapes without exposing content.
func SafeMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ContentLength returns the length of s in bytes, for use as a safe log value
// instead of logging the content itself.
func ContentLength(s string) int {
	return len(s)
}
