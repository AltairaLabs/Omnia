/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import coreproj "github.com/altairalabs/omnia/internal/memory/projection"

// Sensitive (PII-tier) consent categories. Mirrored as string literals from the
// canonical definitions in ee/pkg/privacy/consent.go to keep this package free of
// an ee/pkg/privacy import (ee packages import this one — see the audit-event
// constants in privacy_middleware.go for the same pattern). Keep in sync.
const (
	catIdentity = "memory:identity"
	catLocation = "memory:location"
	catHealth   = "memory:health"
)

// isSensitiveCategory reports whether a memory's consent category is PII-tier and
// must not have its content surfaced in the cross-user Memory Galaxy view.
func isSensitiveCategory(cat string) bool {
	switch cat {
	case catIdentity, catLocation, catHealth:
		return true
	default:
		return false
	}
}

// pointMustBeMasked is the single masking decision for a projection point.
// Today: sensitive category. (#1642 will add a retroactive-opt-out term here once
// the consent store keys on the same pseudonym as virtual_user_id.)
func pointMustBeMasked(p coreproj.Point) bool {
	return isSensitiveCategory(p.Category)
}

// maskPoint strips a point's identifying and content fields in place, leaving an
// anonymous, non-interactive dot (no id → no click-through to the full memory).
// Done server-side before serialization, so the stripped data never reaches the wire.
func maskPoint(p *coreproj.Point) {
	p.ID = ""
	p.Title = ""
	p.Preview = ""
	p.User = ""
	p.UserRef = ""
	p.Category = ""
	p.Type = ""
	p.Masked = true
}
