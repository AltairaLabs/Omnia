/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"testing"
)

func TestNewContentClassifier(t *testing.T) {
	classify := NewContentClassifier()

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		// Health keywords
		{name: "allergy keyword", content: "user has an allergy to peanuts", expected: string(ConsentMemoryHealth)},
		{name: "diagnosed keyword", content: "was diagnosed with diabetes", expected: string(ConsentMemoryHealth)},
		{name: "medication keyword", content: "takes medication daily", expected: string(ConsentMemoryHealth)},
		{name: "blood type keyword", content: "blood type is O positive", expected: string(ConsentMemoryHealth)},
		{name: "allergic keyword", content: "allergic to penicillin", expected: string(ConsentMemoryHealth)},
		{name: "diagnosis keyword", content: "awaiting diagnosis from doctor", expected: string(ConsentMemoryHealth)},
		{name: "prescription keyword", content: "has a prescription for metformin", expected: string(ConsentMemoryHealth)},
		{name: "disability keyword", content: "has a disability accommodation", expected: string(ConsentMemoryHealth)},
		{name: "medical keyword", content: "medical history reviewed", expected: string(ConsentMemoryHealth)},
		{name: "symptom keyword", content: "reporting symptom of fatigue", expected: string(ConsentMemoryHealth)},

		// Health case-insensitive
		{name: "allergy uppercase", content: "ALLERGY to shellfish", expected: string(ConsentMemoryHealth)},
		{name: "medication mixed case", content: "Medication: ibuprofen", expected: string(ConsentMemoryHealth)},

		// Health word boundary — should NOT match partial words
		{name: "premedical not matched", content: "premedical studies completed", expected: ""},
		// "allergically" contains "allergic" but \ballergic\b requires a word boundary after "c";
		// since "a" follows "c" in "allergically", the boundary does not match.
		{name: "allergically not matched", content: "allergically induced response", expected: ""},
		// "preallergy" — "allergy" appears as a suffix: \ballergy\b won't match inside "preallergy"
		{name: "preallergy not matched", content: "user preallergy profile loaded", expected: ""},

		// Location phrases
		{name: "lives in location", content: "lives in Edinburgh", expected: string(ConsentMemoryLocation)},
		{name: "located in", content: "located in London", expected: string(ConsentMemoryLocation)},
		{name: "based in", content: "based in Berlin", expected: string(ConsentMemoryLocation)},
		{name: "address is", content: "address is 42 Main Street", expected: string(ConsentMemoryLocation)},

		// IP address
		{name: "IP address", content: "server at 192.168.1.1", expected: string(ConsentMemoryLocation)},
		{name: "loopback IP", content: "connects to 127.0.0.1", expected: string(ConsentMemoryLocation)},

		// Identity PII — SSN
		{name: "SSN", content: "SSN is 123-45-6789", expected: string(ConsentMemoryIdentity)},

		// Identity PII — credit card
		{name: "credit card with dashes", content: "card 4111-1111-1111-1111", expected: string(ConsentMemoryIdentity)},
		{name: "credit card with spaces", content: "card 4111 1111 1111 1111", expected: string(ConsentMemoryIdentity)},
		{name: "credit card no separator", content: "card 4111111111111111", expected: string(ConsentMemoryIdentity)},

		// Identity PII — email
		{name: "email address", content: "email is test@example.com", expected: string(ConsentMemoryIdentity)},
		{name: "email uppercase domain", content: "contact at USER@DOMAIN.ORG", expected: string(ConsentMemoryIdentity)},

		// Identity PII — phone
		{name: "phone with dashes", content: "call 555-123-4567", expected: string(ConsentMemoryIdentity)},

		// No PII
		{name: "no PII dark mode", content: "user prefers dark mode", expected: ""},
		{name: "no PII generic", content: "user likes coffee in the morning", expected: ""},
		{name: "no PII preferences", content: "language preference is English", expected: ""},

		// Empty content
		{name: "empty string", content: "", expected: ""},

		// Priority: health > location > identity
		{
			name:     "health beats identity",
			content:  "allergic to penicillin, email test@example.com",
			expected: string(ConsentMemoryHealth),
		},
		{
			name:     "health beats location",
			content:  "lives in Paris, diagnosed with asthma",
			expected: string(ConsentMemoryHealth),
		},
		{
			name:     "location beats identity",
			content:  "lives in NYC, SSN is 123-45-6789",
			expected: string(ConsentMemoryLocation),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classify(tt.content)
			if got != tt.expected {
				t.Errorf("classify(%q) = %q, want %q", tt.content, got, tt.expected)
			}
		})
	}
}

func TestNewContentClassifier_ReturnsNewFunctionEachCall(t *testing.T) {
	c1 := NewContentClassifier()
	c2 := NewContentClassifier()

	// Both should produce consistent results independently
	result1 := c1("SSN is 123-45-6789")
	result2 := c2("SSN is 123-45-6789")

	if result1 != string(ConsentMemoryIdentity) {
		t.Errorf("c1 result = %q, want %q", result1, ConsentMemoryIdentity)
	}
	if result2 != string(ConsentMemoryIdentity) {
		t.Errorf("c2 result = %q, want %q", result2, ConsentMemoryIdentity)
	}
}
