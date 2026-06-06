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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractiveSummarizer_LeadSentences(t *testing.T) {
	s := NewExtractiveSummarizer(2, 600)
	out, err := s.Summarize(context.Background(),
		"First sentence. Second sentence. Third sentence. Fourth.")
	require.NoError(t, err)
	assert.Equal(t, "First sentence. Second sentence.", out)
}

func TestExtractiveSummarizer_FewerSentencesThanMax(t *testing.T) {
	s := NewExtractiveSummarizer(5, 600)
	out, err := s.Summarize(context.Background(), "Only one sentence here.")
	require.NoError(t, err)
	assert.Equal(t, "Only one sentence here.", out)
}

func TestExtractiveSummarizer_CharCapTruncates(t *testing.T) {
	s := NewExtractiveSummarizer(10, 20)
	// One long sentence with no early terminator → capped by maxChars.
	out, err := s.Summarize(context.Background(), strings.Repeat("ab ", 50))
	require.NoError(t, err)
	assert.LessOrEqual(t, len(out), 20)
	assert.NotEmpty(t, out)
}

func TestExtractiveSummarizer_EmptyText(t *testing.T) {
	s := NewExtractiveSummarizer(3, 600)
	out, err := s.Summarize(context.Background(), "   ")
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestExtractiveSummarizer_DefaultsOnNonPositive(t *testing.T) {
	s := NewExtractiveSummarizer(0, 0)
	// Defaults (3 sentences) → returns the first three.
	out, err := s.Summarize(context.Background(), "A. B. C. D. E.")
	require.NoError(t, err)
	assert.Equal(t, "A. B. C.", out)
}

// TestExtractiveSummarizer_SatisfiesDocumentSummarizer is a compile-time check
// that ExtractiveSummarizer can back a SummaryStrategy.
func TestExtractiveSummarizer_SatisfiesDocumentSummarizer(t *testing.T) {
	var _ DocumentSummarizer = NewExtractiveSummarizer(3, 600)
	items, err := NewSummaryStrategy(NewExtractiveSummarizer(2, 600)).Ingest(
		context.Background(),
		SourceDoc{Title: "Runbook", URL: "u", Site: "allowed", Text: "Step one. Step two. Step three."},
	)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Step one. Step two.", items[0].Content)
	assert.Equal(t, "summary", items[0].Metadata["kind"])
}
