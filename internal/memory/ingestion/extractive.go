/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ingestion

import (
	"context"
	"strings"
)

// ExtractiveSummarizer is a dependency-free DocumentSummarizer that returns a
// document's lead sentences (a "lead-N" extractive summary), capped by a
// character budget. It is the default summarizer for SummaryStrategy: it needs
// no LLM/completion provider, so the summary-index strategy is usable out of the
// box. A higher-quality LLM-backed summarizer can swap in behind the
// DocumentSummarizer interface without touching SummaryStrategy.
type ExtractiveSummarizer struct {
	maxSentences int
	maxChars     int
}

// NewExtractiveSummarizer builds an ExtractiveSummarizer. Non-positive values
// fall back to sensible defaults (3 sentences, 600 characters).
func NewExtractiveSummarizer(maxSentences, maxChars int) *ExtractiveSummarizer {
	if maxSentences <= 0 {
		maxSentences = 3
	}
	if maxChars <= 0 {
		maxChars = 600
	}
	return &ExtractiveSummarizer{maxSentences: maxSentences, maxChars: maxChars}
}

// Summarize returns the leading sentences of text, stopping at whichever limit
// is hit first (maxSentences sentence terminators or maxChars). Never errors.
func (e *ExtractiveSummarizer) Summarize(_ context.Context, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	var b strings.Builder
	sentences := 0
	for _, r := range text {
		b.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			sentences++
			if sentences >= e.maxSentences {
				break
			}
		}
		if b.Len() >= e.maxChars {
			break
		}
	}
	return strings.TrimSpace(b.String()), nil
}
