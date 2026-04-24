/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package classify

import (
	"context"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// Source identifies which classifier produced a category. Used in
// metric labels so we can see whether regex or embedding catches more.
const (
	SourceRegex     = "regex"
	SourceEmbedding = "embedding"
)

// Result reports the outcome of a Validator.Apply call.
type Result struct {
	// Category is the final consent_category to persist; "" means leave NULL.
	Category privacy.ConsentCategory
	// Overridden is true when the validator upgraded a caller-supplied category.
	Overridden bool
	// From is the caller's claim when Overridden, otherwise empty.
	From privacy.ConsentCategory
	// Source identifies which pass produced the upgrade or fill ("" when
	// the caller's claim stood unchanged).
	Source string
}

// embeddingPass is the minimum interface Validator needs from the
// embedding classifier. Allows tests to substitute a stub without
// constructing a real EmbeddingClassifier with an embedder.
type embeddingPass interface {
	Classify(ctx context.Context, content string) (privacy.ConsentCategory, error)
}

// Validator combines a RuleClassifier and (optionally) an embeddingPass
// into an upgrade-only consent-category decision. Embedding may be nil —
// the validator then degrades to regex-only.
type Validator struct {
	rules     RuleClassifier
	embedding embeddingPass
}

// NewValidator constructs a Validator. embedding may be nil.
func NewValidator(rules RuleClassifier, embedding embeddingPass) *Validator {
	return &Validator{rules: rules, embedding: embedding}
}

// restrictivenessRank assigns each consent category a rank used by the
// upgrade-only logic. Higher rank = more restrictive. Categories not in
// the map (including analytics:aggregate) get rank 0 and are never
// considered for upgrade.
var restrictivenessRank = map[privacy.ConsentCategory]int{
	privacy.ConsentMemoryHealth:      30,
	privacy.ConsentMemoryIdentity:    20,
	privacy.ConsentMemoryLocation:    20,
	privacy.ConsentMemoryHistory:     10,
	privacy.ConsentMemoryContext:     10,
	privacy.ConsentMemoryPreferences: 10,
}

// Apply produces the final category for a memory write. Decision logic:
//   - analytics:aggregate caller passes through unchanged
//   - run regex; run embedding (treat errors as empty)
//   - regex beats embedding on conflict (regex is high-precision)
//   - if detected category is more restrictive than caller's claim, upgrade
//   - otherwise keep caller's claim (or detected when caller is empty)
func (v *Validator) Apply(ctx context.Context, caller privacy.ConsentCategory, content string) Result {
	if caller == privacy.ConsentAnalyticsAggregate {
		return Result{Category: caller}
	}

	regexCat := v.rules.Classify(content)
	var embedCat privacy.ConsentCategory
	if v.embedding != nil {
		c, err := v.embedding.Classify(ctx, content)
		if err == nil {
			embedCat = c
		}
		// Embedding errors fall through silently; the middleware records
		// the error metric at the call site to keep this package free of
		// metric deps.
	}

	detected, source := pickDetected(regexCat, embedCat)

	if detected == "" {
		return Result{Category: caller}
	}
	if caller == "" {
		return Result{Category: detected, Source: source}
	}
	if restrictivenessRank[detected] > restrictivenessRank[caller] {
		return Result{Category: detected, Overridden: true, From: caller, Source: source}
	}
	return Result{Category: caller}
}

// pickDetected returns the category to consider plus its source. Regex
// always wins when both fire — it's high-precision for structured PII.
func pickDetected(regexCat, embedCat privacy.ConsentCategory) (privacy.ConsentCategory, string) {
	if regexCat != "" {
		return regexCat, SourceRegex
	}
	if embedCat != "" {
		return embedCat, SourceEmbedding
	}
	return "", ""
}
