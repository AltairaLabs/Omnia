/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"regexp"
)

// healthKeywords are checked with case-insensitive word boundary matching.
var healthKeywords = []string{
	"allergy", "allergic", "diagnosis", "diagnosed",
	"medication", "prescription", "disability",
	"blood type", "medical", "symptom",
}

// classifierPatterns holds compiled regexes for content classification.
type classifierPatterns struct {
	identity []*regexp.Regexp // SSN, CC, email, phone
	location []*regexp.Regexp // IP address, location phrases
	health   []*regexp.Regexp // health keyword word-boundary patterns
}

func compileClassifierPatterns() *classifierPatterns {
	identity := []*regexp.Regexp{
		regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),                                    // SSN
		regexp.MustCompile(`\b\d{4}[- ]?\d{4}[- ]?\d{4}[- ]?\d{4}\b`),                  // credit card
		regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`), // email
		regexp.MustCompile(`\b\d{3}[-.)\\s]?\d{3}[-.)\\s]?\d{4}\b`),                    // phone
	}

	location := []*regexp.Regexp{
		regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`), // IP address
		regexp.MustCompile(`(?i)\b(?:lives?\s+in|located\s+in|based\s+in|address\s+is)\b`),
	}

	health := make([]*regexp.Regexp, len(healthKeywords))
	for i, kw := range healthKeywords {
		health[i] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(kw) + `\b`)
	}

	return &classifierPatterns{identity: identity, location: location, health: health}
}

// NewContentClassifier returns a function that classifies memory content
// into consent categories based on PII pattern detection.
// Returns empty string when no sensitive content is detected.
func NewContentClassifier() func(content string) string {
	patterns := compileClassifierPatterns()

	return func(content string) string {
		if content == "" {
			return ""
		}

		// Health first (GDPR special category, highest sensitivity)
		for _, re := range patterns.health {
			if re.MatchString(content) {
				return string(ConsentMemoryHealth)
			}
		}

		// Location patterns
		for _, re := range patterns.location {
			if re.MatchString(content) {
				return string(ConsentMemoryLocation)
			}
		}

		// Identity PII patterns
		for _, re := range patterns.identity {
			if re.MatchString(content) {
				return string(ConsentMemoryIdentity)
			}
		}

		return ""
	}
}
