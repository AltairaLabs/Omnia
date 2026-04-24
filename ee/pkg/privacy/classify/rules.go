/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

// Package classify implements the EE memory consent classifier:
// PII regex (RuleClassifier), embedding similarity (EmbeddingClassifier),
// and a Validator that merges caller-supplied categories with both
// signals using upgrade-only semantics.
package classify

import (
	"regexp"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// RuleClassifier inspects content and returns a ConsentCategory or ""
// when no PII pattern matches. Pure function; sub-millisecond.
type RuleClassifier interface {
	Classify(content string) privacy.ConsentCategory
}

// healthKeywords are checked with case-insensitive word-boundary matching.
var healthKeywords = []string{
	"allergy", "allergic", "diagnosis", "diagnosed",
	"medication", "prescription", "disability",
	"blood type", "medical", "symptom",
}

type rulePatterns struct {
	identity []*regexp.Regexp
	location []*regexp.Regexp
	health   []*regexp.Regexp
}

func compileRulePatterns() *rulePatterns {
	identity := []*regexp.Regexp{
		regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),                                    // SSN
		regexp.MustCompile(`\b\d{4}[- ]?\d{4}[- ]?\d{4}[- ]?\d{4}\b`),                  // credit card
		regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`), // email
		regexp.MustCompile(`\b\d{3}[-.)\\s]?\d{3}[-.)\\s]?\d{4}\b`),                    // phone
	}
	location := []*regexp.Regexp{
		regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
		regexp.MustCompile(`(?i)\b(?:lives?\s+in|located\s+in|based\s+in|address\s+is)\b`),
	}
	health := make([]*regexp.Regexp, len(healthKeywords))
	for i, kw := range healthKeywords {
		health[i] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(kw) + `\b`)
	}
	return &rulePatterns{identity: identity, location: location, health: health}
}

type ruleClassifier struct {
	patterns *rulePatterns
}

// NewRuleClassifier builds a stateless RuleClassifier with the canonical
// PII pattern set.
func NewRuleClassifier() RuleClassifier {
	return &ruleClassifier{patterns: compileRulePatterns()}
}

// Classify returns a category when any pattern matches. Order is:
// health > location > identity. Returns "" when nothing matches.
func (r *ruleClassifier) Classify(content string) privacy.ConsentCategory {
	if content == "" {
		return ""
	}
	for _, re := range r.patterns.health {
		if re.MatchString(content) {
			return privacy.ConsentMemoryHealth
		}
	}
	for _, re := range r.patterns.location {
		if re.MatchString(content) {
			return privacy.ConsentMemoryLocation
		}
	}
	for _, re := range r.patterns.identity {
		if re.MatchString(content) {
			return privacy.ConsentMemoryIdentity
		}
	}
	return ""
}
