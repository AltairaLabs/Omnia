/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package classify

import (
	"testing"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

func TestRuleClassifier_PII(t *testing.T) {
	rc := NewRuleClassifier()

	tests := []struct {
		name    string
		content string
		want    privacy.ConsentCategory
	}{
		// Health
		{"allergy keyword", "user has an allergy to peanuts", privacy.ConsentMemoryHealth},
		{"diagnosed", "diagnosed with diabetes", privacy.ConsentMemoryHealth},
		{"medication", "takes medication daily", privacy.ConsentMemoryHealth},
		{"blood type", "blood type is O positive", privacy.ConsentMemoryHealth},
		{"symptom", "reporting symptom of fatigue", privacy.ConsentMemoryHealth},

		// Health case-insensitive + word boundary
		{"allergy uppercase", "ALLERGY to shellfish", privacy.ConsentMemoryHealth},
		{"premedical not matched", "premedical studies completed", ""},
		{"allergically not matched", "allergically induced response", ""},

		// Location
		{"lives in", "lives in Edinburgh", privacy.ConsentMemoryLocation},
		{"located in", "located in London", privacy.ConsentMemoryLocation},
		{"based in", "based in Berlin", privacy.ConsentMemoryLocation},
		{"address is", "address is 42 Main Street", privacy.ConsentMemoryLocation},
		{"IP address", "server at 192.168.1.1", privacy.ConsentMemoryLocation},

		// Identity
		{"SSN", "SSN is 123-45-6789", privacy.ConsentMemoryIdentity},
		{"credit card dashes", "card 4111-1111-1111-1111", privacy.ConsentMemoryIdentity},
		{"email", "email is test@example.com", privacy.ConsentMemoryIdentity},
		{"phone dashes", "call 555-123-4567", privacy.ConsentMemoryIdentity},

		// No match
		{"no PII dark mode", "user prefers dark mode", ""},
		{"empty", "", ""},

		// Priority: health > location > identity
		{"health beats identity", "allergic to penicillin, email test@example.com", privacy.ConsentMemoryHealth},
		{"health beats location", "lives in Paris, diagnosed with asthma", privacy.ConsentMemoryHealth},
		{"location beats identity", "lives in NYC, SSN is 123-45-6789", privacy.ConsentMemoryLocation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rc.Classify(tt.content)
			if got != tt.want {
				t.Errorf("Classify(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

func TestRuleClassifier_Stateless(t *testing.T) {
	r1 := NewRuleClassifier()
	r2 := NewRuleClassifier()
	if r1.Classify("SSN is 123-45-6789") != r2.Classify("SSN is 123-45-6789") {
		t.Fatal("classifiers should be stateless and deterministic")
	}
}
