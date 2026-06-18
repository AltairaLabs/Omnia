/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import (
	"strings"
	"testing"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// piiTrigger is the substring fakeRedactor flags as PII. Extracted to
// satisfy goconst (5+ uses of the same string literal).
const piiTrigger = "PII"

// ptrTrue returns *bool pointing at true (for the RequirePIIRedaction
// pointer-typed field on MemoryConsolidationSafetyGates).
func ptrTrue() *bool { v := true; return &v }

// ptrFalse returns *bool pointing at false.
func ptrFalse() *bool { v := false; return &v }

// fakeRedactor flags any content containing the substring "PII".
// Tests don't need real PII pattern matching — they verify the gate's
// dispatch logic.
type fakeRedactor struct{ trigger string }

func (f fakeRedactor) HasPII(s string) bool {
	return strings.Contains(s, f.trigger)
}

func TestPIIGate_RejectsCreateSummaryWithPII(t *testing.T) {
	g := NewPIIGate(fakeRedactor{trigger: piiTrigger})
	r := g.Check(CreateSummaryAction{
		FromIDs: []string{"o1"},
		Scope:   Scope{WorkspaceID: "ws"},
		Content: "Contains PII tokens that should not persist",
	}, memoryv1.MemoryConsolidationSafetyGates{RequirePIIRedaction: ptrTrue()})
	if r != ReasonPIIBlocked {
		t.Fatalf("want %q, got %q", ReasonPIIBlocked, r)
	}
}

func TestPIIGate_AllowsCleanContent(t *testing.T) {
	g := NewPIIGate(fakeRedactor{trigger: piiTrigger})
	r := g.Check(CreateSummaryAction{
		FromIDs: []string{"o1"},
		Scope:   Scope{WorkspaceID: "ws"},
		Content: "user prefers metric units",
	}, memoryv1.MemoryConsolidationSafetyGates{RequirePIIRedaction: ptrTrue()})
	if r != "" {
		t.Fatalf("want allow, got reason %q", r)
	}
}

func TestPIIGate_DisabledByFlag(t *testing.T) {
	g := NewPIIGate(fakeRedactor{trigger: piiTrigger})
	r := g.Check(CreateSummaryAction{
		FromIDs: []string{"o1"},
		Scope:   Scope{WorkspaceID: "ws"},
		Content: "Has PII but flag disables gate",
	}, memoryv1.MemoryConsolidationSafetyGates{RequirePIIRedaction: ptrFalse()})
	if r != "" {
		t.Fatalf("disabled flag should allow, got %q", r)
	}
}

func TestPIIGate_NilRedactorIsNoop(t *testing.T) {
	g := NewPIIGate(nil)
	r := g.Check(CreateSummaryAction{
		FromIDs: []string{"o1"},
		Scope:   Scope{WorkspaceID: "ws"},
		Content: "Has PII — but no redactor wired",
	}, memoryv1.MemoryConsolidationSafetyGates{RequirePIIRedaction: ptrTrue()})
	if r != "" {
		t.Errorf("nil redactor should no-op, got %q", r)
	}
}

func TestPIIGate_ChecksRescopeReason(t *testing.T) {
	g := NewPIIGate(fakeRedactor{trigger: piiTrigger})
	r := g.Check(RescopeAction{
		TargetIDs: []string{"o1"},
		NewScope:  Scope{WorkspaceID: "ws", AgentID: "a"},
		Reason:    "Promote to agent-scoped — contains PII justification",
	}, memoryv1.MemoryConsolidationSafetyGates{RequirePIIRedaction: ptrTrue()})
	if r != ReasonPIIBlocked {
		t.Errorf("want PII block on rescope.Reason, got %q", r)
	}
}

func TestPIIGate_SkipsNonContentActions(t *testing.T) {
	g := NewPIIGate(fakeRedactor{trigger: piiTrigger})
	// Supersede has no content field — gate must allow.
	r := g.Check(SupersedeAction{TargetIDs: []string{"o1"}, WithID: "o2"},
		memoryv1.MemoryConsolidationSafetyGates{RequirePIIRedaction: ptrTrue()})
	if r != "" {
		t.Errorf("supersede has no content; want allow, got %q", r)
	}
}
