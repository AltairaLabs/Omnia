/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package redaction

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/session"
)

func piiConfig(patterns ...string) *omniav1alpha1.PIIConfig {
	return &omniav1alpha1.PIIConfig{
		Redact:   true,
		Patterns: patterns,
	}
}

func piiConfigWithStrategy(strategy omniav1alpha1.RedactionStrategy, patterns ...string) *omniav1alpha1.PIIConfig {
	return &omniav1alpha1.PIIConfig{
		Redact:   true,
		Strategy: strategy,
		Patterns: patterns,
	}
}

func TestRedact_BuiltinPatterns(t *testing.T) {
	ctx := context.Background()
	r := NewRedactor()

	tests := []struct {
		name     string
		input    string
		patterns []string
		want     string
		events   int
	}{
		{
			name:     "ssn basic match",
			input:    "My SSN is 123-45-6789",
			patterns: []string{"ssn"},
			want:     "My SSN is [REDACTED_SSN]",
			events:   1,
		},
		{
			name:     "ssn multiple matches",
			input:    "SSNs: 123-45-6789 and 987-65-4321",
			patterns: []string{"ssn"},
			want:     "SSNs: [REDACTED_SSN] and [REDACTED_SSN]",
			events:   2,
		},
		{
			name:     "ssn no false positive on phone",
			input:    "Call 555-1234",
			patterns: []string{"ssn"},
			want:     "Call 555-1234",
			events:   0,
		},
		{
			name:     "credit_card basic match",
			input:    "Card: 4111-1111-1111-1111",
			patterns: []string{"credit_card"},
			want:     "Card: [REDACTED_CC]",
			events:   1,
		},
		{
			name:     "credit_card no separators",
			input:    "Card: 4111111111111111",
			patterns: []string{"credit_card"},
			want:     "Card: [REDACTED_CC]",
			events:   1,
		},
		{
			name:     "credit_card with spaces",
			input:    "Card: 4111 1111 1111 1111",
			patterns: []string{"credit_card"},
			want:     "Card: [REDACTED_CC]",
			events:   1,
		},
		{
			name:     "phone_number basic match",
			input:    "Call me at 555-123-4567",
			patterns: []string{"phone_number"},
			want:     "Call me at [REDACTED_PHONE]",
			events:   1,
		},
		{
			name:     "phone_number with dots",
			input:    "Phone: 555.123.4567",
			patterns: []string{"phone_number"},
			want:     "Phone: [REDACTED_PHONE]",
			events:   1,
		},
		{
			name:     "email basic match",
			input:    "Email user@example.com for info",
			patterns: []string{"email"},
			want:     "Email [REDACTED_EMAIL] for info",
			events:   1,
		},
		{
			name:     "email case insensitive",
			input:    "Contact User@Example.COM",
			patterns: []string{"email"},
			want:     "Contact [REDACTED_EMAIL]",
			events:   1,
		},
		{
			name:     "email no false positive",
			input:    "Not an email: @user",
			patterns: []string{"email"},
			want:     "Not an email: @user",
			events:   0,
		},
		{
			name:     "ip_address basic match",
			input:    "Server at 192.168.1.1 is down",
			patterns: []string{"ip_address"},
			want:     "Server at [REDACTED_IP] is down",
			events:   1,
		},
		{
			name:     "ip_address multiple matches",
			input:    "From 10.0.0.1 to 172.16.0.1",
			patterns: []string{"ip_address"},
			want:     "From [REDACTED_IP] to [REDACTED_IP]",
			events:   2,
		},
		{
			name:     "multiple patterns",
			input:    "SSN 123-45-6789, email user@test.com, IP 10.0.0.1",
			patterns: []string{"ssn", "email", "ip_address"},
			want:     "SSN [REDACTED_SSN], email [REDACTED_EMAIL], IP [REDACTED_IP]",
			events:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pii := piiConfig(tt.patterns...)
			got, events, err := r.Redact(ctx, tt.input, pii)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Len(t, events, tt.events)
		})
	}
}

func TestRedact_CustomPatterns(t *testing.T) {
	ctx := context.Background()
	r := NewRedactor()

	t.Run("valid custom regex", func(t *testing.T) {
		pii := piiConfig("custom:[A-Z]{2}\\d{6}")
		got, events, err := r.Redact(ctx, "ID: AB123456 is valid", pii)
		require.NoError(t, err)
		assert.Equal(t, "ID: [REDACTED_CUSTOM] is valid", got)
		assert.Len(t, events, 1)
		assert.Equal(t, "custom:[A-Z]{2}\\d{6}", events[0].Pattern)
	})

	t.Run("invalid custom regex returns error", func(t *testing.T) {
		pii := piiConfig("custom:[invalid")
		_, _, err := r.Redact(ctx, "test", pii)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid custom pattern")
	})
}

func TestRedact_Strategies(t *testing.T) {
	ctx := context.Background()
	r := NewRedactor()

	t.Run("replace strategy", func(t *testing.T) {
		pii := piiConfigWithStrategy(omniav1alpha1.RedactionStrategyReplace, "ssn")
		got, _, err := r.Redact(ctx, "SSN: 123-45-6789", pii)
		require.NoError(t, err)
		assert.Equal(t, "SSN: [REDACTED_SSN]", got)
	})

	t.Run("hash strategy deterministic", func(t *testing.T) {
		pii := piiConfigWithStrategy(omniav1alpha1.RedactionStrategyHash, "ssn")
		got1, _, err := r.Redact(ctx, "SSN: 123-45-6789", pii)
		require.NoError(t, err)
		got2, _, err := r.Redact(ctx, "SSN: 123-45-6789", pii)
		require.NoError(t, err)
		assert.Equal(t, got1, got2, "hash should be deterministic")
		assert.Contains(t, got1, "[HASH_SSN:")
		assert.NotContains(t, got1, "123-45-6789")
	})

	t.Run("hash strategy different values produce different hashes", func(t *testing.T) {
		pii := piiConfigWithStrategy(omniav1alpha1.RedactionStrategyHash, "ssn")
		got1, _, err := r.Redact(ctx, "123-45-6789", pii)
		require.NoError(t, err)
		got2, _, err := r.Redact(ctx, "987-65-4321", pii)
		require.NoError(t, err)
		assert.NotEqual(t, got1, got2)
	})

	t.Run("mask strategy", func(t *testing.T) {
		pii := piiConfigWithStrategy(omniav1alpha1.RedactionStrategyMask, "ssn")
		got, _, err := r.Redact(ctx, "SSN: 123-45-6789", pii)
		require.NoError(t, err)
		// "123-45-6789" is 11 chars, mask first 7, keep last 4
		assert.Equal(t, "SSN: *******6789", got)
	})

	t.Run("mask short value", func(t *testing.T) {
		// Use a custom pattern that matches a short value
		pii := piiConfigWithStrategy(omniav1alpha1.RedactionStrategyMask, "custom:\\d{3}")
		got, _, err := r.Redact(ctx, "code: 123", pii)
		require.NoError(t, err)
		assert.Equal(t, "code: ***", got)
	})

	t.Run("default strategy is replace", func(t *testing.T) {
		pii := &omniav1alpha1.PIIConfig{
			Redact:   true,
			Patterns: []string{"ssn"},
			// Strategy intentionally empty
		}
		got, _, err := r.Redact(ctx, "SSN: 123-45-6789", pii)
		require.NoError(t, err)
		assert.Equal(t, "SSN: [REDACTED_SSN]", got)
	})
}

func TestRedact_EdgeCases(t *testing.T) {
	ctx := context.Background()
	r := NewRedactor()

	t.Run("empty input", func(t *testing.T) {
		got, events, err := r.Redact(ctx, "", piiConfig("ssn"))
		require.NoError(t, err)
		assert.Equal(t, "", got)
		assert.Nil(t, events)
	})

	t.Run("redact disabled", func(t *testing.T) {
		pii := &omniav1alpha1.PIIConfig{
			Redact:   false,
			Patterns: []string{"ssn"},
		}
		got, events, err := r.Redact(ctx, "SSN: 123-45-6789", pii)
		require.NoError(t, err)
		assert.Equal(t, "SSN: 123-45-6789", got)
		assert.Nil(t, events)
	})

	t.Run("nil PIIConfig", func(t *testing.T) {
		got, events, err := r.Redact(ctx, "test", nil)
		require.NoError(t, err)
		assert.Equal(t, "test", got)
		assert.Nil(t, events)
	})

	t.Run("empty patterns", func(t *testing.T) {
		pii := &omniav1alpha1.PIIConfig{
			Redact:   true,
			Patterns: []string{},
		}
		got, events, err := r.Redact(ctx, "SSN: 123-45-6789", pii)
		require.NoError(t, err)
		assert.Equal(t, "SSN: 123-45-6789", got)
		assert.Nil(t, events)
	})

	t.Run("unknown pattern name", func(t *testing.T) {
		pii := piiConfig("nonexistent_pattern")
		_, _, err := r.Redact(ctx, "test", pii)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown PII pattern")
	})

	t.Run("overlapping matches prefer earlier and longer", func(t *testing.T) {
		// Two custom patterns where one overlaps the other in "123-45-6789":
		// Pattern 1 "\d{3}-\d{2}" matches "123-45" at [7,13]
		// Pattern 2 "\d{2}-\d{4}" matches "45-6789" at [10,17] — starts before 13, so it overlaps and is skipped
		pii := &omniav1alpha1.PIIConfig{
			Redact:   true,
			Patterns: []string{"custom:\\d{3}-\\d{2}", "custom:\\d{2}-\\d{4}"},
		}
		got, events, err := r.Redact(ctx, "value: 123-45-6789", pii)
		require.NoError(t, err)
		assert.NotContains(t, got, "123-45")
		// Only the first match is kept; the second overlaps and is skipped
		assert.Len(t, events, 1)
		assert.Equal(t, 7, events[0].StartIndex)
	})

	t.Run("no matches in text", func(t *testing.T) {
		got, events, err := r.Redact(ctx, "Hello world", piiConfig("ssn"))
		require.NoError(t, err)
		assert.Equal(t, "Hello world", got)
		assert.Nil(t, events)
	})
}

func TestRedact_AuditMode(t *testing.T) {
	ctx := context.Background()

	t.Run("audit mode populates Original", func(t *testing.T) {
		r := NewRedactor(WithAuditMode(true))
		_, events, err := r.Redact(ctx, "SSN: 123-45-6789", piiConfig("ssn"))
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "123-45-6789", events[0].Original)
		assert.Equal(t, "ssn", events[0].Pattern)
	})

	t.Run("non-audit mode does not populate Original", func(t *testing.T) {
		r := NewRedactor(WithAuditMode(false))
		_, events, err := r.Redact(ctx, "SSN: 123-45-6789", piiConfig("ssn"))
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Empty(t, events[0].Original)
	})

	t.Run("default is non-audit mode", func(t *testing.T) {
		r := NewRedactor()
		_, events, err := r.Redact(ctx, "SSN: 123-45-6789", piiConfig("ssn"))
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Empty(t, events[0].Original)
	})
}

func TestRedact_EventPositions(t *testing.T) {
	ctx := context.Background()
	r := NewRedactor()

	got, events, err := r.Redact(ctx, "SSN: 123-45-6789 done", piiConfig("ssn"))
	require.NoError(t, err)
	assert.Equal(t, "SSN: [REDACTED_SSN] done", got)
	require.Len(t, events, 1)
	assert.Equal(t, 5, events[0].StartIndex)
	assert.Equal(t, 16, events[0].EndIndex)
}

func TestRedactMessage(t *testing.T) {
	ctx := context.Background()
	r := NewRedactor()
	pii := piiConfig("ssn", "email")

	t.Run("content redacted original not mutated", func(t *testing.T) {
		original := &session.Message{
			ID:        "msg-1",
			Role:      "user",
			Content:   "My SSN is 123-45-6789 and email is user@test.com",
			Timestamp: time.Now(),
			Metadata:  map[string]string{"note": "from user@test.com"},
		}
		originalContent := original.Content
		originalMeta := original.Metadata["note"]

		result, err := r.RedactMessage(ctx, original, pii)
		require.NoError(t, err)

		// Result should be redacted
		assert.Contains(t, result.Content, "[REDACTED_SSN]")
		assert.Contains(t, result.Content, "[REDACTED_EMAIL]")
		assert.NotContains(t, result.Content, "123-45-6789")
		assert.NotContains(t, result.Content, "user@test.com")

		// Metadata should be redacted
		assert.Contains(t, result.Metadata["note"], "[REDACTED_EMAIL]")

		// Original should NOT be mutated
		assert.Equal(t, originalContent, original.Content)
		assert.Equal(t, originalMeta, original.Metadata["note"])

		// Other fields preserved
		assert.Equal(t, "msg-1", result.ID)
		assert.Equal(t, session.MessageRole("user"), result.Role)
	})

	t.Run("nil message returns nil", func(t *testing.T) {
		result, err := r.RedactMessage(ctx, nil, pii)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("nil metadata handled", func(t *testing.T) {
		msg := &session.Message{
			ID:      "msg-2",
			Content: "SSN: 123-45-6789",
		}
		result, err := r.RedactMessage(ctx, msg, pii)
		require.NoError(t, err)
		assert.Contains(t, result.Content, "[REDACTED_SSN]")
		assert.Nil(t, result.Metadata)
	})

	t.Run("nil pii returns copy unchanged", func(t *testing.T) {
		msg := &session.Message{
			ID:      "msg-3",
			Content: "SSN: 123-45-6789",
		}
		result, err := r.RedactMessage(ctx, msg, nil)
		require.NoError(t, err)
		assert.Equal(t, msg.Content, result.Content)
	})
}

func BenchmarkRedact_AllPatterns(b *testing.B) {
	ctx := context.Background()
	r := NewRedactor()
	pii := piiConfig("ssn", "credit_card", "phone_number", "email", "ip_address")
	input := "Contact user@example.com, SSN 123-45-6789, card 4111-1111-1111-1111, phone 555-123-4567, server 192.168.1.1"

	b.ResetTimer()
	for b.Loop() {
		_, _, _ = r.Redact(ctx, input, pii)
	}
}

func BenchmarkRedact_LongText(b *testing.B) {
	ctx := context.Background()
	r := NewRedactor()
	pii := piiConfig("ssn", "credit_card", "phone_number", "email", "ip_address")

	// Build a longer text with scattered PII
	var sb strings.Builder
	for range 100 {
		sb.WriteString("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ")
	}
	sb.WriteString("Contact user@example.com, SSN 123-45-6789. ")
	for range 100 {
		sb.WriteString("Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. ")
	}
	input := sb.String()

	b.ResetTimer()
	for b.Loop() {
		_, _, _ = r.Redact(ctx, input, pii)
	}
}
