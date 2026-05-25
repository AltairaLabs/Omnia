/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"

	eeprivacyv1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
)

// consolidationPIIRedactor adapts the EE redactor to the
// consolidation.PIIRedactor interface using a default-permissive
// PIIConfig covering the built-in identifier patterns (ssn,
// credit_card, phone_number, email, ip_address).
//
// The consolidation PII gate runs on outbound action content (summary
// text, rescope reason text) — a single static pattern set is the
// right shape; per-policy config would be a future extension.
type consolidationPIIRedactor struct {
	inner redaction.Redactor
}

func newConsolidationPIIRedactor() *consolidationPIIRedactor {
	return &consolidationPIIRedactor{inner: redaction.NewRedactor()}
}

// defaultConsolidationPIIConfig is the pattern set the consolidation
// PII gate screens against. Includes all built-in patterns; structural
// identifiers (ssn, credit_card, ip_address) and personal-detail
// patterns (phone_number, email) are both included since pack-emitted
// content has no provenance signal that would let us downgrade to
// structural-only.
var defaultConsolidationPIIConfig = &eeprivacyv1.PIIConfig{
	Redact:   true,
	Patterns: []string{"ssn", "credit_card", "phone_number", "email", "ip_address"},
}

// HasPII returns true when redaction would alter the content — used by
// the consolidation validator's PII gate to reject actions that carry
// detectable PII into the memory store.
func (r *consolidationPIIRedactor) HasPII(content string) bool {
	if content == "" {
		return false
	}
	redacted, events, err := r.inner.Redact(context.Background(), content, defaultConsolidationPIIConfig)
	if err != nil {
		// Fail closed: if redaction errored, treat as containing PII so
		// the validator rejects the action rather than persisting unsafe
		// content. The audit row captures the rejection reason.
		return true
	}
	return redacted != content || len(events) > 0
}
