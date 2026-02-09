/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package redaction

import (
	"crypto/sha256"
	"fmt"
	"strings"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// applyStrategy applies the configured redaction strategy to a matched value.
func applyStrategy(strategy omniav1alpha1.RedactionStrategy, token, matched string) string {
	switch strategy {
	case omniav1alpha1.RedactionStrategyHash:
		return hashValue(token, matched)
	case omniav1alpha1.RedactionStrategyMask:
		return maskValue(matched)
	default:
		// RedactionStrategyReplace or empty (default)
		return token
	}
}

// hashValue returns a deterministic SHA-256 truncated hash representation.
// Format: [HASH_<LABEL>:<first 12 hex chars>]
func hashValue(token, value string) string {
	// Extract label from token, e.g. "[REDACTED_SSN]" -> "SSN"
	label := token
	label = strings.TrimPrefix(label, "[REDACTED_")
	label = strings.TrimSuffix(label, "]")

	h := sha256.Sum256([]byte(value))
	return fmt.Sprintf("[HASH_%s:%x]", label, h[:6])
}

// maskValue preserves the last 4 characters of value, masking the rest with *.
// Values with 4 or fewer characters are fully masked.
func maskValue(value string) string {
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:]
}
