/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import (
	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ReasonPIIBlocked is the reject reason emitted when the PII gate
// finds redactable content in a proposed action.
const ReasonPIIBlocked = "pii_blocked"

// PIIRedactor abstracts the redaction call so the OSS consolidation
// package doesn't depend on ee/pkg/redaction directly. The cmd/memory-api
// binary supplies an adapter wrapping the EE redactor.
//
// Implementations return true when content contains anything that would
// be redacted by the configured PII patterns.
type PIIRedactor interface {
	HasPII(content string) bool
}

// PIIGate runs the redactor over an action's content fields and
// returns ReasonPIIBlocked when anything would be redacted.
// A nil redactor is a no-op gate (used in tests without EE deps and
// when the validator has no redactor wired).
type PIIGate struct {
	redactor PIIRedactor
}

// NewPIIGate constructs a PIIGate. nil redactor disables the gate.
func NewPIIGate(r PIIRedactor) *PIIGate {
	return &PIIGate{redactor: r}
}

// Check returns "" to accept, ReasonPIIBlocked to reject. Skipped
// when requirePIIRedaction is false on the policy.
func (g *PIIGate) Check(a Action, gates memoryv1.MemoryConsolidationSafetyGates) string {
	if g == nil || g.redactor == nil || !gates.PIIRedactionEnabled() {
		return ""
	}
	for _, s := range contentFields(a) {
		if s == "" {
			continue
		}
		if g.redactor.HasPII(s) {
			return ReasonPIIBlocked
		}
	}
	return ""
}

// contentFields returns the human-readable string fields on an action
// that should be screened for PII before persistence.
func contentFields(a Action) []string {
	switch act := a.(type) {
	case CreateSummaryAction:
		return []string{act.Content}
	case RescopeAction:
		return []string{act.Reason}
	case InvalidateAction:
		return []string{act.Reason}
	case DiscardAction:
		return []string{act.Reason}
	default:
		return nil
	}
}
