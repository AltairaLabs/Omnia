/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package redaction

import (
	"context"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// TextRedactor strips PII from text.
// Implemented by PatternRedactor (configured with PIIConfig) and NoOpRedactor (passthrough).
// Satisfies the internal/memory.Redactor interface structurally.
type TextRedactor interface {
	RedactText(ctx context.Context, text string) (string, error)
}

// TrustAwareRedactor is a TextRedactor whose active pattern set varies by
// trust level — user_requested / operator_curated memories only get
// structural patterns applied; everything else gets the full set.
type TrustAwareRedactor interface {
	TextRedactor
	// RedactTextWithTrust applies PII redaction using the given trust level.
	// TrustExplicit drops non-structural patterns; TrustInferred applies all.
	RedactTextWithTrust(ctx context.Context, text string, trust TrustLevel) (string, error)
}

// PatternRedactor is a TextRedactor that applies a fixed PIIConfig on every call.
// It validates and pre-compiles patterns at construction time.
type PatternRedactor struct {
	inner Redactor
	pii   *omniav1alpha1.PIIConfig
}

// NoOpRedactor is a passthrough TextRedactor that performs no redaction.
type NoOpRedactor struct{}

// RedactText returns text unchanged.
func (NoOpRedactor) RedactText(_ context.Context, text string) (string, error) {
	return text, nil
}

// NewPatternRedactor constructs a TextRedactor from a PIIConfig.
// If config is nil, has Redact=false, or has no patterns, a NoOpRedactor is returned.
// Pattern names are validated at construction; an error is returned for unknown or
// invalid custom patterns.
func NewPatternRedactor(config *omniav1alpha1.PIIConfig) (TextRedactor, error) {
	if config == nil || !config.Redact || len(config.Patterns) == 0 {
		return NoOpRedactor{}, nil
	}

	// Eagerly validate patterns so callers surface errors at construction, not at first call.
	if _, err := resolvePatterns(config.Patterns); err != nil {
		return nil, err
	}

	return &PatternRedactor{
		inner: NewRedactor(),
		pii:   config,
	}, nil
}

// RedactText applies the configured PIIConfig to text and returns the redacted result.
func (r *PatternRedactor) RedactText(ctx context.Context, text string) (string, error) {
	redacted, _, err := r.inner.Redact(ctx, text, r.pii)
	return redacted, err
}

// RedactTextWithTrust applies the configured PIIConfig with the given trust
// level — TrustExplicit content (user_requested / operator_curated) only
// gets structural patterns redacted, TrustInferred gets the full set.
func (r *PatternRedactor) RedactTextWithTrust(
	ctx context.Context, text string, trust TrustLevel,
) (string, error) {
	redacted, _, err := r.inner.RedactWithTrust(ctx, text, r.pii, trust)
	return redacted, err
}

// RedactTextWithTrust on NoOpRedactor is a no-op like RedactText.
func (NoOpRedactor) RedactTextWithTrust(_ context.Context, text string, _ TrustLevel) (string, error) {
	return text, nil
}
