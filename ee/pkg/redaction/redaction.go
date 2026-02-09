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

// Redactor performs PII redaction on text and session messages.
type Redactor interface {
	// Redact applies PII redaction to the given text using the provided PIIConfig.
	Redact(ctx context.Context, text string, pii *omniav1alpha1.PIIConfig) (string, []RedactionEvent, error)
	// RedactMessage applies PII redaction to a session Message, returning a copy.
	RedactMessage(ctx context.Context, msg *session.Message, pii *omniav1alpha1.PIIConfig) (*session.Message, error)
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
	if pii == nil || !pii.Redact || len(pii.Patterns) == 0 || text == "" {
		return text, nil, nil
	}

	patterns, err := resolvePatterns(pii.Patterns)
	if err != nil {
		return "", nil, err
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
