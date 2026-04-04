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

// Package identity provides pseudonymization functions for user identity.
// All user identifiers are hashed before storage to ensure no PII persists
// in the platform's databases or logs.
package identity

import (
	"crypto/sha256"
	"encoding/hex"
)

// pseudonymLength is the number of hex characters from the SHA-256 hash.
// 16 hex chars = 64 bits of entropy, sufficient for uniqueness within a workspace.
const pseudonymLength = 16

// PseudonymizeID returns a deterministic, non-reversible pseudonym for a user ID.
// The result is a 16-character hex string derived from SHA-256 of the input.
// Both the facade (at ingestion) and the dashboard (at query time) must use
// this function to ensure consistent pseudonyms.
//
// Empty input returns an empty string (no pseudonym for absent identity).
func PseudonymizeID(raw string) string {
	if raw == "" {
		return ""
	}
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])[:pseudonymLength]
}
