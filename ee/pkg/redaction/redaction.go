/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package redaction

import (
	"context"
	"sort"
	"strings"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/session"
)

// RedactionEvent records a single redaction that occurred.
type RedactionEvent struct {
	// Pattern is the name of the pattern that matched.
	Pattern string
	// Original is the original matched text (only populated in audit mode).
	Original string
	// StartIndex is the byte offset in the original string where the match began.
	StartIndex int
	// EndIndex is the byte offset in the original string where the match ended.
	EndIndex int
}

// TrustLevel controls which patterns a redactor applies. Structural-only mode
// limits redaction to compliance-relevant identifiers (SSN, credit-card, IP)
// so user-curated and operator-curated memories can preserve the personal
// details the caller explicitly asked to remember (phone, email) while still
// scrubbing identifiers that always require protection.
type TrustLevel int

const (
	// TrustInferred applies the full configured pattern set. Default for
	// agent-extracted memories where the content is not something the user
	// explicitly asked to store.
	TrustInferred TrustLevel = iota
	// TrustExplicit skips non-structural patterns and only redacts
	// structural identifiers. Used for provenance=user_requested or
	// provenance=operator_curated memories.
	TrustExplicit
)

// Redactor performs PII redaction on text and session messages.
type Redactor interface {
	// Redact applies PII redaction to the given text using the provided PIIConfig.
	Redact(
		ctx context.Context, text string, pii *omniav1alpha1.PIIConfig,
	) (string, []RedactionEvent, error)
	// RedactWithTrust is like Redact but filters the active pattern set by
	// trust level — TrustExplicit content skips personal-detail patterns so
	// callers who intentionally asked to persist (e.g.) their work email
	// keep that detail, while structural identifiers are still redacted.
	RedactWithTrust(
		ctx context.Context, text string, pii *omniav1alpha1.PIIConfig, trust TrustLevel,
	) (string, []RedactionEvent, error)
	// RedactMessage applies PII redaction to a session Message, returning a copy.
	RedactMessage(
		ctx context.Context, msg *session.Message, pii *omniav1alpha1.PIIConfig,
	) (*session.Message, error)
}

// Option configures a redactor.
type Option func(*redactor)

// WithAuditMode enables audit mode, which populates RedactionEvent.Original.
func WithAuditMode(enabled bool) Option {
	return func(r *redactor) {
		r.auditMode = enabled
	}
}

type redactor struct {
	auditMode bool
}

// NewRedactor creates a new Redactor with the given options.
func NewRedactor(opts ...Option) Redactor {
	r := &redactor{}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// match represents a single regex match with its position and associated pattern.
type match struct {
	start   int
	end     int
	pattern patternDef
}

// Redact applies PII redaction to text based on the PIIConfig.
func (r *redactor) Redact(
	ctx context.Context, text string, pii *omniav1alpha1.PIIConfig,
) (string, []RedactionEvent, error) {
	return r.RedactWithTrust(ctx, text, pii, TrustInferred)
}

// RedactWithTrust is Redact with trust-level-aware pattern filtering.
// TrustExplicit drops non-structural patterns from the active set before
// matching — the structural identifiers in the config (SSN, credit-card,
// IP, custom patterns) are still enforced.
func (r *redactor) RedactWithTrust(
	ctx context.Context, text string, pii *omniav1alpha1.PIIConfig, trust TrustLevel,
) (string, []RedactionEvent, error) {
	if pii == nil || !pii.Redact || len(pii.Patterns) == 0 || text == "" {
		return text, nil, nil
	}

	patterns, err := resolvePatterns(pii.Patterns)
	if err != nil {
		return "", nil, err
	}

	patterns = filterPatternsByTrust(patterns, trust)
	if len(patterns) == 0 {
		return text, nil, nil
	}

	strategy := pii.Strategy
	if strategy == "" {
		strategy = omniav1alpha1.RedactionStrategyReplace
	}

	// Phase 1: Find all matches across all patterns.
	var matches []match
	for _, p := range patterns {
		locs := p.Regex.FindAllStringIndex(text, -1)
		for _, loc := range locs {
			matches = append(matches, match{
				start:   loc[0],
				end:     loc[1],
				pattern: p,
			})
		}
	}

	if len(matches) == 0 {
		return text, nil, nil
	}

	// Sort matches by start position, then by length (longer first to prefer more specific).
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start != matches[j].start {
			return matches[i].start < matches[j].start
		}
		return (matches[i].end - matches[i].start) > (matches[j].end - matches[j].start)
	})

	// Phase 2: Build result string, skipping overlapping matches.
	var result strings.Builder
	result.Grow(len(text))
	events := make([]RedactionEvent, 0, len(matches))
	lastEnd := 0

	for _, m := range matches {
		if m.start < lastEnd {
			// Overlapping match — skip.
			continue
		}

		// Append text before this match.
		result.WriteString(text[lastEnd:m.start])

		matched := text[m.start:m.end]
		replacement := applyStrategy(strategy, m.pattern.Token, matched)
		result.WriteString(replacement)

		event := RedactionEvent{
			Pattern:    m.pattern.Name,
			StartIndex: m.start,
			EndIndex:   m.end,
		}
		if r.auditMode {
			event.Original = matched
		}
		events = append(events, event)

		lastEnd = m.end
	}

	// Append remaining text after the last match.
	result.WriteString(text[lastEnd:])

	return result.String(), events, nil
}

// RedactMessage applies redaction to a session Message, returning a shallow copy
// with the Content field redacted and Metadata values redacted. The original
// message is not mutated.
func (r *redactor) RedactMessage(
	ctx context.Context, msg *session.Message, pii *omniav1alpha1.PIIConfig,
) (*session.Message, error) {
	if msg == nil {
		return nil, nil
	}

	// Shallow copy the message.
	redacted := *msg

	// Deep copy metadata to avoid mutating the original.
	if msg.Metadata != nil {
		redacted.Metadata = make(map[string]string, len(msg.Metadata))
		for k, v := range msg.Metadata {
			redacted.Metadata[k] = v
		}
	}

	// Redact content.
	content, _, err := r.Redact(ctx, redacted.Content, pii)
	if err != nil {
		return nil, err
	}
	redacted.Content = content

	// Redact metadata values.
	for k, v := range redacted.Metadata {
		val, _, err := r.Redact(ctx, v, pii)
		if err != nil {
			return nil, err
		}
		redacted.Metadata[k] = val
	}

	return &redacted, nil
}
