/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package redaction_test

import (
	"context"
	"strings"
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
)

func piiConfig(patterns []string, strategy omniav1alpha1.RedactionStrategy) *omniav1alpha1.PIIConfig {
	return &omniav1alpha1.PIIConfig{
		Redact:   true,
		Patterns: patterns,
		Strategy: strategy,
	}
}

// TestNewPatternRedactor_NilConfig verifies that a nil config returns a NoOpRedactor.
func TestNewPatternRedactor_NilConfig(t *testing.T) {
	r, err := redaction.NewPatternRedactor(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := "My SSN is 123-45-6789"
	got, err := r.RedactText(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != text {
		t.Errorf("expected passthrough, got %q", got)
	}
}

// TestNewPatternRedactor_RedactFalse verifies that Redact=false returns a NoOpRedactor.
func TestNewPatternRedactor_RedactFalse(t *testing.T) {
	cfg := &omniav1alpha1.PIIConfig{
		Redact:   false,
		Patterns: []string{"ssn"},
	}
	r, err := redaction.NewPatternRedactor(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := "My SSN is 123-45-6789"
	got, err := r.RedactText(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != text {
		t.Errorf("expected passthrough, got %q", got)
	}
}

// TestNewPatternRedactor_EmptyPatterns verifies that empty patterns returns a NoOpRedactor.
func TestNewPatternRedactor_EmptyPatterns(t *testing.T) {
	cfg := &omniav1alpha1.PIIConfig{
		Redact:   true,
		Patterns: []string{},
	}
	r, err := redaction.NewPatternRedactor(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := "My SSN is 123-45-6789"
	got, err := r.RedactText(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != text {
		t.Errorf("expected passthrough, got %q", got)
	}
}

// TestNewPatternRedactor_UnknownPattern verifies that an unknown built-in name returns an error.
func TestNewPatternRedactor_UnknownPattern(t *testing.T) {
	cfg := piiConfig([]string{"nonexistent_pattern"}, omniav1alpha1.RedactionStrategyReplace)
	_, err := redaction.NewPatternRedactor(cfg)
	if err == nil {
		t.Fatal("expected error for unknown pattern, got nil")
	}
}

// TestNewPatternRedactor_InvalidCustomPattern verifies that an invalid custom regex returns an error.
func TestNewPatternRedactor_InvalidCustomPattern(t *testing.T) {
	cfg := piiConfig([]string{"custom:[invalid("}, omniav1alpha1.RedactionStrategyReplace)
	_, err := redaction.NewPatternRedactor(cfg)
	if err == nil {
		t.Fatal("expected error for invalid custom regex, got nil")
	}
}

// TestPatternRedactor_SSN verifies SSN redaction with replace strategy.
func TestPatternRedactor_SSN(t *testing.T) {
	r, err := redaction.NewPatternRedactor(piiConfig([]string{"ssn"}, omniav1alpha1.RedactionStrategyReplace))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := r.RedactText(context.Background(), "My SSN is 123-45-6789 and that's sensitive.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "123-45-6789") {
		t.Errorf("SSN not redacted: %q", got)
	}
	if !strings.Contains(got, "[REDACTED_SSN]") {
		t.Errorf("expected [REDACTED_SSN] token, got: %q", got)
	}
}

// TestPatternRedactor_CreditCard verifies credit card redaction.
func TestPatternRedactor_CreditCard(t *testing.T) {
	r, err := redaction.NewPatternRedactor(piiConfig([]string{"credit_card"}, omniav1alpha1.RedactionStrategyReplace))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := r.RedactText(context.Background(), "Card: 4111-1111-1111-1111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "4111") {
		t.Errorf("credit card not redacted: %q", got)
	}
	if !strings.Contains(got, "[REDACTED_CC]") {
		t.Errorf("expected [REDACTED_CC] token, got: %q", got)
	}
}

// TestPatternRedactor_Email verifies email redaction.
func TestPatternRedactor_Email(t *testing.T) {
	r, err := redaction.NewPatternRedactor(piiConfig([]string{"email"}, omniav1alpha1.RedactionStrategyReplace))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := r.RedactText(context.Background(), "Contact user@example.com for details.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "user@example.com") {
		t.Errorf("email not redacted: %q", got)
	}
	if !strings.Contains(got, "[REDACTED_EMAIL]") {
		t.Errorf("expected [REDACTED_EMAIL] token, got: %q", got)
	}
}

// TestPatternRedactor_Phone verifies phone number redaction.
func TestPatternRedactor_Phone(t *testing.T) {
	r, err := redaction.NewPatternRedactor(piiConfig([]string{"phone_number"}, omniav1alpha1.RedactionStrategyReplace))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := r.RedactText(context.Background(), "Call 555-867-5309 now.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "555-867-5309") {
		t.Errorf("phone not redacted: %q", got)
	}
	if !strings.Contains(got, "[REDACTED_PHONE]") {
		t.Errorf("expected [REDACTED_PHONE] token, got: %q", got)
	}
}

// TestPatternRedactor_IPAddress verifies IP address redaction.
func TestPatternRedactor_IPAddress(t *testing.T) {
	r, err := redaction.NewPatternRedactor(piiConfig([]string{"ip_address"}, omniav1alpha1.RedactionStrategyReplace))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := r.RedactText(context.Background(), "Request from 192.168.1.1.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "192.168.1.1") {
		t.Errorf("IP not redacted: %q", got)
	}
	if !strings.Contains(got, "[REDACTED_IP]") {
		t.Errorf("expected [REDACTED_IP] token, got: %q", got)
	}
}

// TestPatternRedactor_HashStrategy verifies that hash strategy produces deterministic output.
func TestPatternRedactor_HashStrategy(t *testing.T) {
	r, err := redaction.NewPatternRedactor(piiConfig([]string{"email"}, omniav1alpha1.RedactionStrategyHash))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := "Contact user@example.com for details."
	got1, err := r.RedactText(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	got2, err := r.RedactText(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if got1 != got2 {
		t.Errorf("hash strategy is not deterministic: %q != %q", got1, got2)
	}
	if strings.Contains(got1, "user@example.com") {
		t.Errorf("email not redacted: %q", got1)
	}
	if !strings.Contains(got1, "[HASH_EMAIL:") {
		t.Errorf("expected [HASH_EMAIL:...] token, got: %q", got1)
	}
}

// TestPatternRedactor_MaskStrategy verifies that mask strategy preserves last 4 chars.
func TestPatternRedactor_MaskStrategy(t *testing.T) {
	r, err := redaction.NewPatternRedactor(piiConfig([]string{"ssn"}, omniav1alpha1.RedactionStrategyMask))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// SSN "123-45-6789" — last 4 chars are "6789"
	got, err := r.RedactText(context.Background(), "SSN: 123-45-6789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "123-45") {
		t.Errorf("SSN prefix not masked: %q", got)
	}
	if !strings.Contains(got, "6789") {
		t.Errorf("expected last 4 chars preserved, got: %q", got)
	}
}

// TestPatternRedactor_CustomPattern verifies custom regex via "custom:" prefix.
func TestPatternRedactor_CustomPattern(t *testing.T) {
	// Custom pattern matching employee IDs like EMP-12345
	cfg := piiConfig([]string{"custom:EMP-\\d{5}"}, omniav1alpha1.RedactionStrategyReplace)
	r, err := redaction.NewPatternRedactor(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := r.RedactText(context.Background(), "Employee EMP-99999 filed a report.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "EMP-99999") {
		t.Errorf("custom pattern not redacted: %q", got)
	}
	if !strings.Contains(got, "[REDACTED_CUSTOM]") {
		t.Errorf("expected [REDACTED_CUSTOM] token, got: %q", got)
	}
}

// TestPatternRedactor_MultiplePatterns verifies multiple patterns in a single text.
func TestPatternRedactor_MultiplePatterns(t *testing.T) {
	r, err := redaction.NewPatternRedactor(piiConfig(
		[]string{"ssn", "email"},
		omniav1alpha1.RedactionStrategyReplace,
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := "SSN: 123-45-6789. Email: user@example.com."
	got, err := r.RedactText(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "123-45-6789") {
		t.Errorf("SSN not redacted: %q", got)
	}
	if strings.Contains(got, "user@example.com") {
		t.Errorf("email not redacted: %q", got)
	}
	if !strings.Contains(got, "[REDACTED_SSN]") {
		t.Errorf("expected [REDACTED_SSN] token, got: %q", got)
	}
	if !strings.Contains(got, "[REDACTED_EMAIL]") {
		t.Errorf("expected [REDACTED_EMAIL] token, got: %q", got)
	}
}

// TestPatternRedactor_NoPIIInText verifies that text without PII is returned unchanged.
func TestPatternRedactor_NoPIIInText(t *testing.T) {
	r, err := redaction.NewPatternRedactor(piiConfig([]string{"ssn", "email"}, omniav1alpha1.RedactionStrategyReplace))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := "This text contains no personally identifiable information."
	got, err := r.RedactText(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != text {
		t.Errorf("expected unchanged text, got %q", got)
	}
}

// TestPatternRedactor_EmptyText verifies that empty text is returned as-is.
func TestPatternRedactor_EmptyText(t *testing.T) {
	r, err := redaction.NewPatternRedactor(piiConfig([]string{"ssn"}, omniav1alpha1.RedactionStrategyReplace))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := r.RedactText(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// TestNoOpRedactor_PassthroughDirectly exercises NoOpRedactor directly.
func TestNoOpRedactor_PassthroughDirectly(t *testing.T) {
	var r redaction.NoOpRedactor
	text := "some text with 123-45-6789"
	got, err := r.RedactText(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != text {
		t.Errorf("expected passthrough, got %q", got)
	}
}

// TestNoOpRedactor_RedactTextWithTrustPassthrough ensures the trust-aware
// entrypoint stays a no-op on NoOpRedactor regardless of trust level.
func TestNoOpRedactor_RedactTextWithTrustPassthrough(t *testing.T) {
	var r redaction.NoOpRedactor
	text := "alice@example.com at 555-867-5309"

	for _, trust := range []redaction.TrustLevel{redaction.TrustInferred, redaction.TrustExplicit} {
		got, err := r.RedactTextWithTrust(context.Background(), text, trust)
		if err != nil {
			t.Fatalf("unexpected error at trust=%d: %v", trust, err)
		}
		if got != text {
			t.Errorf("trust=%d: expected passthrough, got %q", trust, got)
		}
	}
}

// TestPatternRedactor_RedactTextWithTrust_ExplicitKeepsPersonalDetails
// exercises the trust-aware entrypoint on the PatternRedactor wrapper so the
// coverage gate on redactor.go is satisfied.
func TestPatternRedactor_RedactTextWithTrust_ExplicitKeepsPersonalDetails(t *testing.T) {
	cfg := piiConfig(
		[]string{"ssn", "credit_card", "ip_address", "email", "phone_number"},
		omniav1alpha1.RedactionStrategyReplace,
	)
	r, err := redaction.NewPatternRedactor(cfg)
	if err != nil {
		t.Fatalf("new pattern redactor: %v", err)
	}

	text := "alice@example.com, 555-867-5309, 4111 1111 1111 1111, 123-45-6789, 10.0.0.1"

	// Inferred: all five patterns apply.
	inferred, err := r.(redaction.TrustAwareRedactor).
		RedactTextWithTrust(context.Background(), text, redaction.TrustInferred)
	if err != nil {
		t.Fatalf("RedactTextWithTrust inferred: %v", err)
	}
	for _, want := range []string{
		"[REDACTED_EMAIL]", "[REDACTED_PHONE]", "[REDACTED_CC]", "[REDACTED_SSN]", "[REDACTED_IP]",
	} {
		if !strings.Contains(inferred, want) {
			t.Errorf("inferred: expected %s, got %q", want, inferred)
		}
	}

	// Explicit: personal patterns drop, structural ones stay.
	explicit, err := r.(redaction.TrustAwareRedactor).
		RedactTextWithTrust(context.Background(), text, redaction.TrustExplicit)
	if err != nil {
		t.Fatalf("RedactTextWithTrust explicit: %v", err)
	}
	if !strings.Contains(explicit, "alice@example.com") {
		t.Errorf("explicit should KEEP email, got %q", explicit)
	}
	if !strings.Contains(explicit, "555-867-5309") {
		t.Errorf("explicit should KEEP phone, got %q", explicit)
	}
	if !strings.Contains(explicit, "[REDACTED_SSN]") {
		t.Errorf("explicit should still redact SSN, got %q", explicit)
	}
	if !strings.Contains(explicit, "[REDACTED_CC]") {
		t.Errorf("explicit should still redact credit-card, got %q", explicit)
	}
	if !strings.Contains(explicit, "[REDACTED_IP]") {
		t.Errorf("explicit should still redact IP, got %q", explicit)
	}
}
